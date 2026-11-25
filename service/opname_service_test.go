package service_test

import (
	"backend-cashier/db"
	"backend-cashier/domain"
	"backend-cashier/service"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestGenerateOpnameNoAll5Locations(t *testing.T) {
	setupFeatureTestDB(t)
	database := db.Postgres.DB
	opnameSvc := service.NewOpnameService(database, service.NewStorageService())

	cases := []struct {
		loc    string
		prefix string
	}{
		{"Toko Utama", "OPN-UTM-"},
		{"Toko Sudirman", "OPN-SDR-"},
		{"Toko Paye", "OPN-PAY-"},
		{"Gudang Karangsokong", "OPN-KRS-"},
		{"Gudang BTN", "OPN-BTN-"},
	}

	for _, tc := range cases {
		no := opnameSvc.GenerateOpnameNo(tc.loc)
		if !strings.HasPrefix(no, tc.prefix) {
			t.Errorf("location %q expected prefix %q, got %q", tc.loc, tc.prefix, no)
		}
	}
}

func TestStockOpnameMultiLocationLifecycle(t *testing.T) {
	setupFeatureTestDB(t)
	database := db.Postgres.DB

	storageSvc := service.NewStorageService()
	opnameSvc := service.NewOpnameService(database, storageSvc)

	testSuffix := fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
	prodCode := "PROD-OPN-" + testSuffix

	// Buat Kategori, Brand, Supplier dummy untuk isolasi testing
	var category domain.Category
	database.FirstOrCreate(&category, domain.Category{Name: "Kategori Opname " + testSuffix})

	var brand domain.Brand
	database.FirstOrCreate(&brand, domain.Brand{Name: "Brand Opname " + testSuffix})

	var supplier domain.Supplier
	database.FirstOrCreate(&supplier, domain.Supplier{ID: "SUPP-OPN-" + testSuffix, Name: "Supplier Opname"})

	// Buat Master Produk Dummy
	dummyProduct := domain.Product{
		Model:      gorm.Model{CreatedAt: time.Now()},
		Nama:       "Produk Opname Test " + testSuffix,
		Kode:       prodCode,
		Saldo:      10, // Saldo global master yang harus dijaga selama masa transisi SO
		HBeli:      100000,
		HJual:      150000,
		Satuan:     "UNIT",
		CategoryID: category.ID,
		BrandID:    brand.ID,
		SupplierID: supplier.ID,
	}
	if err := database.Create(&dummyProduct).Error; err != nil {
		t.Fatalf("failed to create dummy product: %v", err)
	}

	var sessionIDs []uint
	defer func() {
		database.Unscoped().Where("product_id = ?", dummyProduct.ID).Delete(&domain.StockMovement{})
		database.Unscoped().Where("product_id = ?", dummyProduct.ID).Delete(&domain.StockOpnameItem{})
		database.Unscoped().Where("product_id = ?", dummyProduct.ID).Delete(&domain.ProductStock{})
		if len(sessionIDs) > 0 {
			database.Unscoped().Where("id IN ?", sessionIDs).Delete(&domain.StockOpname{})
		}
		database.Unscoped().Where("kode = ?", prodCode).Delete(&domain.Product{})
	}()

	// =========================================================================
	// 1. UJI COBA SESI OPNAME CABANG 1: TOKO SUDIRMAN
	// =========================================================================
	locSudirman := "Toko Sudirman"

	session1, err := opnameSvc.CreateSession(locSudirman, &category.ID, "USER-AUDIT-1", "Opname Perdana Sudirman")
	if err != nil {
		t.Fatalf("failed to create opname session: %v", err)
	}
	if session1.Status != "Draft" {
		t.Errorf("expected session status 'Draft', got %s", session1.Status)
	}
	if session1.TotalItems < 1 {
		t.Errorf("expected at least 1 item in session, got %d", session1.TotalItems)
	}

	sessionIDs = append(sessionIDs, session1.ID)

	// Verifikasi saldo awal cabang Toko Sudirman terinisialisasi 0
	var checkPS domain.ProductStock
	if err := database.Where("product_id = ? AND location = ?", dummyProduct.ID, locSudirman).First(&checkPS).Error; err != nil {
		t.Fatalf("expected ProductStock created for Sudirman: %v", err)
	}
	if checkPS.Saldo != 0 {
		t.Errorf("expected initial branch saldo 0, got %f", checkPS.Saldo)
	}

	// 2. Input hasil hitung fisik di rak: Ditemukan 3 unit fisik di Toko Sudirman
	updatedItem, err := opnameSvc.UpdateCountItem(session1.ID, dummyProduct.ID, domain.UpdateOpnameItemRequest{
		PhysicalQty: 3,
		Reason:      "Stok Awal Fisik",
		Notes:       "Rak Elektronik A1",
		PhotoURL:    "/uploads/opname/test_sudirman.jpg",
	})
	if err != nil {
		t.Fatalf("failed to update count item: %v", err)
	}

	if updatedItem.PhysicalQty != 3 || updatedItem.DiscrepancyQty != 3 || updatedItem.DiscrepancyValue != 300000 {
		t.Errorf("unexpected updatedItem: %+v", updatedItem)
	}

	// Pastikan dalam status DRAFT, saldo riil di database belum berubah
	var psStill0 domain.ProductStock
	database.Where("product_id = ? AND location = ?", dummyProduct.ID, locSudirman).First(&psStill0)
	if psStill0.Saldo != 0 {
		t.Errorf("expected branch saldo remains 0 during Draft, got %f", psStill0.Saldo)
	}

	// 3. Ajukan untuk approval supervisor
	sessionSubmitted, err := opnameSvc.SubmitForApproval(session1.ID, "Mohon disetujui")
	if err != nil {
		t.Fatalf("failed to submit for approval: %v", err)
	}
	if sessionSubmitted.Status != "Waiting Approval" {
		t.Errorf("expected status 'Waiting Approval', got %s", sessionSubmitted.Status)
	}

	// 4. Supervisor menyetujui dokumen & penyesuaian stok dieksekusi secara atomik
	sessionApproved, err := opnameSvc.ApproveSession(session1.ID, "SUPERVISOR-SDR")
	if err != nil {
		t.Fatalf("failed to approve session: %v", err)
	}
	if sessionApproved.Status != "Completed" {
		t.Errorf("expected status 'Completed', got %s", sessionApproved.Status)
	}

	// Verifikasi saldo di Toko Sudirman sekarang = 3
	var psSudirmanAfter domain.ProductStock
	database.Where("product_id = ? AND location = ?", dummyProduct.ID, locSudirman).First(&psSudirmanAfter)
	if psSudirmanAfter.Saldo != 3 {
		t.Errorf("expected Sudirman saldo 3 after approval, got %f", psSudirmanAfter.Saldo)
	}

	// Verifikasi Kartu Stok / Audit Trail (StockMovement)
	var movementSdr domain.StockMovement
	if err := database.Where("product_id = ? AND location = ? AND movement_type = ?", dummyProduct.ID, locSudirman, "OPNAME_ADJUST").First(&movementSdr).Error; err != nil {
		t.Fatalf("expected StockMovement recorded for Sudirman: %v", err)
	}
	if movementSdr.QtyIn != 3 || movementSdr.SaldoBefore != 0 || movementSdr.SaldoAfter != 3 {
		t.Errorf("unexpected StockMovement data: QtyIn=%f, Before=%f, After=%f", movementSdr.QtyIn, movementSdr.SaldoBefore, movementSdr.SaldoAfter)
	}

	// =========================================================================
	// 2. UJI COBA SESI OPNAME CABANG 2: TOKO PAYE (Independensi & Proteksi products.saldo)
	// =========================================================================
	locPaye := "Toko Paye"
	session2, err := opnameSvc.CreateSession(locPaye, &category.ID, "USER-AUDIT-2", "Opname Toko Paye")
	if err != nil {
		t.Fatalf("failed to create session for Paye: %v", err)
	}
	sessionIDs = append(sessionIDs, session2.ID)

	_, err = opnameSvc.UpdateCountItem(session2.ID, dummyProduct.ID, domain.UpdateOpnameItemRequest{
		PhysicalQty: 2,
		Reason:      "Stok Fisik Display",
		Notes:       "Etalase Depan",
		PhotoURL:    "/uploads/opname/test_paye.jpg",
	})
	if err != nil {
		t.Fatalf("failed to update count for Paye: %v", err)
	}

	_, err = opnameSvc.ApproveSession(session2.ID, "SUPERVISOR-PAY")
	if err != nil {
		t.Fatalf("failed to approve session Paye: %v", err)
	}

	// Verifikasi Saldo Toko Paye = 2, dan Toko Sudirman tetap 3
	var psPayeAfter domain.ProductStock
	database.Where("product_id = ? AND location = ?", dummyProduct.ID, locPaye).First(&psPayeAfter)
	if psPayeAfter.Saldo != 2 {
		t.Errorf("expected Paye saldo 2, got %f", psPayeAfter.Saldo)
	}

	database.Where("product_id = ? AND location = ?", dummyProduct.ID, locSudirman).First(&psSudirmanAfter)
	if psSudirmanAfter.Saldo != 3 {
		t.Errorf("expected Sudirman saldo remains 3, got %f", psSudirmanAfter.Saldo)
	}

	// Verifikasi Requirement Transisi SO: products.saldo global master TIDAK ditimpa SUM cabang (tetap 10)
	var prodAfterAll domain.Product
	database.First(&prodAfterAll, dummyProduct.ID)
	if prodAfterAll.Saldo != 10 {
		t.Errorf("expected master product global saldo preserved as 10 during SO transition, got %f", prodAfterAll.Saldo)
	}

	// Verifikasi Rincian Multi-Lokasi via Service
	breakdown, err := opnameSvc.GetProductStockBreakdown(dummyProduct.ID)
	if err != nil {
		t.Fatalf("failed to get product stock breakdown: %v", err)
	}
	if len(breakdown) < 2 {
		t.Fatalf("expected at least 2 branches in breakdown, got %d", len(breakdown))
	}

	// =========================================================================
	// 3. UJI COBA TRANSAKSI KASIR (POTONG STOK CABANG SPESIFIK & AUDIT TRAIL)
	// =========================================================================
	tx := database.Begin()
	errAdjust := service.AdjustProductStock(tx, dummyProduct.ID, locSudirman, -1, "SALE", "INV-TEST-OPN-1", "Penjualan Toko Sudirman", "KASIR-1")
	if errAdjust != nil {
		tx.Rollback()
		t.Fatalf("failed to adjust product stock on sale: %v", errAdjust)
	}
	tx.Commit()

	database.Where("product_id = ? AND location = ?", dummyProduct.ID, locSudirman).First(&psSudirmanAfter)
	if psSudirmanAfter.Saldo != 2 {
		t.Errorf("expected Sudirman saldo 2 after sale, got %f", psSudirmanAfter.Saldo)
	}

	database.Where("product_id = ? AND location = ?", dummyProduct.ID, locPaye).First(&psPayeAfter)
	if psPayeAfter.Saldo != 2 {
		t.Errorf("expected Paye saldo remains 2, got %f", psPayeAfter.Saldo)
	}

	stockCardSdr, countSdr, err := opnameSvc.GetStockCard(dummyProduct.ID, locSudirman, 10, 0)
	if err != nil {
		t.Fatalf("failed to get stock card: %v", err)
	}
	if countSdr < 2 {
		t.Errorf("expected at least 2 stock movements for Sudirman, got %d", countSdr)
	}
	t.Logf("Sukses! Kartu stok Toko Sudirman tercatat %d mutasi, saldo akhir %.0f", countSdr, stockCardSdr[0].SaldoAfter)
}

func TestStockOpnameSalesDeltaAfterCount(t *testing.T) {
	setupFeatureTestDB(t)
	database := db.Postgres.DB

	accSvc := service.NewAccountingService(database)
	trxSvc := service.NewTransactionService(database, accSvc)
	opnameSvc := service.NewOpnameService(database, service.NewStorageService())

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
	prodCode := "PROD-DELTA-" + suffix

	var category domain.Category
	database.FirstOrCreate(&category, domain.Category{Name: "Kategori Delta " + suffix})
	var brand domain.Brand
	database.FirstOrCreate(&brand, domain.Brand{Name: "Brand Delta " + suffix})
	var supplier domain.Supplier
	database.FirstOrCreate(&supplier, domain.Supplier{ID: "SUPP-DLT-" + suffix, Name: "Supplier Delta"})

	var user domain.User
	database.Where("id = ?", "USER-DEL-1").FirstOrCreate(&user, domain.User{ID: "USER-DEL-1", Name: "Admin Del", Username: "admin_del", Role: "admin", Location: "Toko Utama"})

	prod := domain.Product{
		Nama:       "TV Delta Test " + suffix,
		Kode:       prodCode,
		Saldo:      20,
		HBeli:      1000000,
		HJual:      1250000,
		Satuan:     "UNIT",
		CategoryID: category.ID,
		BrandID:    brand.ID,
		SupplierID: supplier.ID,
	}
	if err := database.Create(&prod).Error; err != nil {
		t.Fatalf("failed to create product: %v", err)
	}

	var sessionID uint
	invSdr := "PJL-DLT-SDR-" + suffix
	invPaye := "PJL-DLT-PAY-" + suffix

	defer func() {
		database.Unscoped().Where("journal_entry_id IN (SELECT id FROM journal_entries WHERE entry_number IN ?)", []string{"JV-SALES-" + invSdr, "JV-SALES-" + invPaye}).Delete(&domain.JournalItem{})
		database.Unscoped().Where("entry_number IN ?", []string{"JV-SALES-" + invSdr, "JV-SALES-" + invPaye}).Delete(&domain.JournalEntry{})
		database.Unscoped().Where("sales_id IN (SELECT id FROM sales WHERE invoice IN ?)", []string{invSdr, invPaye}).Delete(&domain.PaymentMethod{})
		database.Unscoped().Where("invoice IN ?", []string{invSdr, invPaye}).Delete(&domain.Sales{})
		database.Unscoped().Where("product_id = ?", prod.ID).Delete(&domain.StockMovement{})
		database.Unscoped().Where("product_id = ?", prod.ID).Delete(&domain.StockOpnameItem{})
		database.Unscoped().Where("product_id = ?", prod.ID).Delete(&domain.ProductStock{})
		if sessionID > 0 {
			database.Unscoped().Where("id = ?", sessionID).Delete(&domain.StockOpname{})
		}
		database.Unscoped().Where("id = ?", prod.ID).Delete(&domain.Product{})
	}()

	// 1. Buka sesi SO untuk Toko Sudirman
	sess, err := opnameSvc.CreateSession("Toko Sudirman", &category.ID, "SUPERADMIN-1", "Test Sales Delta")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	sessionID = sess.ID

	// 2. Hitung fisik di rak Toko Sudirman: 8 unit (pada waktu T1)
	_, err = opnameSvc.UpdateCountItem(sess.ID, prod.ID, domain.UpdateOpnameItemRequest{
		PhysicalQty: 8,
		Reason:      "Hitung Fisik Awal",
	})
	if err != nil {
		t.Fatalf("UpdateCountItem failed: %v", err)
	}

	// Jeda kecil agar timestamp penjualan jelas > item.UpdatedAt
	time.Sleep(50 * time.Millisecond)

	// 3a. Kasir Toko Sudirman menjual 3 unit SETELAH barang dihitung fisik
	_, err = trxSvc.CreateSales(domain.CreateTransactionRequest{
		Invoice:       invSdr,
		UserID:        user.ID,
		Location:      "Toko Sudirman",
		PaymentMethod: "Tunai",
		AmountPaid:    3750000,
		Items: []domain.SalesItemRequest{
			{ProductSearch: prodCode, Qty: 3, Price: 1250000},
		},
	})
	if err != nil {
		t.Fatalf("CreateSales Sudirman failed: %v", err)
	}

	// 3b. Kasir Toko Paye menjual 1 unit (tidak boleh memotong hitungan Toko Sudirman!)
	_, err = trxSvc.CreateSales(domain.CreateTransactionRequest{
		Invoice:       invPaye,
		UserID:        user.ID,
		Location:      "Toko Paye",
		PaymentMethod: "Tunai",
		AmountPaid:    1250000,
		Items: []domain.SalesItemRequest{
			{ProductSearch: prodCode, Qty: 1, Price: 1250000},
		},
	})
	if err != nil {
		t.Fatalf("CreateSales Paye failed: %v", err)
	}

	// 4. Submit & Approve SO Toko Sudirman
	if _, err := opnameSvc.SubmitForApproval(sess.ID, "Siap Approve"); err != nil {
		t.Fatalf("SubmitForApproval failed: %v", err)
	}
	if _, err := opnameSvc.ApproveSession(sess.ID, "SUPERADMIN-1"); err != nil {
		t.Fatalf("ApproveSession failed: %v", err)
	}

	// 5. Verifikasi Formula Delta: PhysicalQty (8) - SalesAfterCount Sudirman (3) = 5
	var psSdr domain.ProductStock
	if err := database.Where("product_id = ? AND location = ?", prod.ID, "Toko Sudirman").First(&psSdr).Error; err != nil {
		t.Fatalf("failed to fetch Sudirman ProductStock: %v", err)
	}
	if psSdr.Saldo != 5 {
		t.Errorf("Sales Delta formula failed: expected Toko Sudirman saldo 5 (8 - 3), got %.2f", psSdr.Saldo)
	}

	// 6. Verifikasi Saldo Global Master tetap 16 (20 - 3 - 1) & tidak tertimpa 5
	var prodCheck domain.Product
	database.First(&prodCheck, prod.ID)
	if prodCheck.Saldo != 16 {
		t.Errorf("expected global products.saldo 16 (20 - 3 - 1), got %.2f", prodCheck.Saldo)
	}
}

func TestPurchaseTransitAndReleaseWithOpname(t *testing.T) {
	setupFeatureTestDB(t)
	database := db.Postgres.DB

	accSvc := service.NewAccountingService(database)
	purchaseSvc := service.NewPurchaseService(database, accSvc)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
	prodCode := "PROD-TRN-" + suffix
	notaTransit := "PBL-TRANSIT-" + suffix
	notaCancel := "PBL-CANCEL-TRN-" + suffix

	var category domain.Category
	database.FirstOrCreate(&category, domain.Category{Name: "Kategori Transit " + suffix})
	var brand domain.Brand
	database.FirstOrCreate(&brand, domain.Brand{Name: "Brand Transit " + suffix})
	var supplier domain.Supplier
	database.FirstOrCreate(&supplier, domain.Supplier{ID: "SUPP-TRN-" + suffix, Name: "Supplier Transit"})

	prod := domain.Product{
		Nama:       "AC Transit Test " + suffix,
		Kode:       prodCode,
		Saldo:      10,
		HBeli:      2000000,
		HJual:      2500000,
		Satuan:     "UNIT",
		CategoryID: category.ID,
		BrandID:    brand.ID,
		SupplierID: supplier.ID,
	}
	if err := database.Create(&prod).Error; err != nil {
		t.Fatalf("failed to create product: %v", err)
	}

	// Set initial branch stock di Gudang Karangsokong = 5
	database.Create(&domain.ProductStock{
		ProductID: prod.ID,
		Location:  "Gudang Karangsokong",
		Saldo:     5,
	})

	defer func() {
		database.Unscoped().Where("nota IN ?", []string{notaTransit, notaCancel}).Delete(&domain.Purchase{})
		database.Unscoped().Where("product_id = ?", prod.ID).Delete(&domain.StockMovement{})
		database.Unscoped().Where("product_id = ?", prod.ID).Delete(&domain.ProductStock{})
		database.Unscoped().Where("id = ?", prod.ID).Delete(&domain.Product{})
	}()

	// 1. Buat pembelian status Transit ke Gudang Karangsokong
	err := purchaseSvc.CreateBatch(domain.CreatePurchaseBatchRequest{
		Nota:       notaTransit,
		Tanggal:    time.Now().Format("2006-01-02"),
		SupplierID: domain.FlexibleID(supplier.ID),
		Location:   "Gudang Karangsokong",
		DocType:    "Surat Jalan",
		Status:     "Transit",
		Items: []domain.PurchaseItemRequest{
			{ProductID: prod.ID, Qty: 7, HBeli: 2000000},
		},
	})
	if err != nil {
		t.Fatalf("CreateBatch Transit failed: %v", err)
	}

	// Pastikan stok global & gudang TIDAK bertambah selama status masih Transit
	var prodCheck domain.Product
	database.First(&prodCheck, prod.ID)
	if prodCheck.Saldo != 10 {
		t.Errorf("expected global saldo still 10 while Transit, got %.2f", prodCheck.Saldo)
	}
	var psGdg domain.ProductStock
	database.Where("product_id = ? AND location = ?", prod.ID, "Gudang Karangsokong").First(&psGdg)
	if psGdg.Saldo != 5 {
		t.Errorf("expected Gudang Karangsokong saldo still 5 while Transit, got %.2f", psGdg.Saldo)
	}

	// Pastikan GetAll mengembalikan Status == "Transit" pada grup
	groups, _, err := purchaseSvc.GetAll(1, 10, "nota", notaTransit, "")
	if err != nil || len(groups) != 1 {
		t.Fatalf("expected 1 group for %s, got %d (err=%v)", notaTransit, len(groups), err)
	}
	if groups[0].Status != "Transit" {
		t.Errorf("expected group Status 'Transit', got %q", groups[0].Status)
	}
	if groups[0].Location != "Gudang Karangsokong" {
		t.Errorf("expected group Location 'Gudang Karangsokong', got %q", groups[0].Location)
	}

	// 2. Buat & hapus nota Transit lain -> stok tidak boleh berkurang
	err = purchaseSvc.CreateBatch(domain.CreatePurchaseBatchRequest{
		Nota:     notaCancel,
		Location: "Gudang Karangsokong",
		Status:   "Transit",
		Items:    []domain.PurchaseItemRequest{{ProductID: prod.ID, Qty: 4, HBeli: 2000000}},
	})
	if err != nil {
		t.Fatalf("CreateBatch notaCancel failed: %v", err)
	}
	if err := purchaseSvc.DeleteByNota(notaCancel); err != nil {
		t.Fatalf("DeleteByNota transit failed: %v", err)
	}
	database.First(&prodCheck, prod.ID)
	if prodCheck.Saldo != 10 {
		t.Errorf("DeleteByNota on Transit must not decrement global saldo: expected 10, got %.2f", prodCheck.Saldo)
	}

	// 3. Rilis nota Transit pasca Stock Opname selesai
	if err := purchaseSvc.ReleaseTransitNota(notaTransit); err != nil {
		t.Fatalf("ReleaseTransitNota failed: %v", err)
	}

	// Pastikan saldo global & Gudang Karangsokong resmi bertambah +7
	database.First(&prodCheck, prod.ID)
	if prodCheck.Saldo != 17 {
		t.Errorf("expected global saldo 17 after release (10 + 7), got %.2f", prodCheck.Saldo)
	}
	database.Where("product_id = ? AND location = ?", prod.ID, "Gudang Karangsokong").First(&psGdg)
	if psGdg.Saldo != 12 {
		t.Errorf("expected Gudang Karangsokong saldo 12 after release (5 + 7), got %.2f", psGdg.Saldo)
	}

	// Pastikan StockMovement PURCHASE_RELEASE tercatat
	var mv domain.StockMovement
	if err := database.Where("product_id = ? AND reference_no = ? AND movement_type = ?", prod.ID, notaTransit, "PURCHASE_RELEASE").First(&mv).Error; err != nil {
		t.Fatalf("expected PURCHASE_RELEASE StockMovement: %v", err)
	}
	if mv.QtyIn != 7 || mv.SaldoBefore != 5 || mv.SaldoAfter != 12 {
		t.Errorf("unexpected PURCHASE_RELEASE movement: %+v", mv)
	}

	// Pastikan rilis kedua kali ditolak
	if err := purchaseSvc.ReleaseTransitNota(notaTransit); err == nil {
		t.Errorf("expected error when releasing an already-Received nota")
	}
}

func TestProductImagesAndMinIOStorage(t *testing.T) {
	setupFeatureTestDB(t)
	database := db.Postgres.DB

	storageSvc := service.NewStorageService()
	prodSvc := service.NewProductService(database)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
	var category domain.Category
	database.FirstOrCreate(&category, domain.Category{Name: "Kategori Img " + suffix})
	var brand domain.Brand
	database.FirstOrCreate(&brand, domain.Brand{Name: "Brand Img " + suffix})
	var supplier domain.Supplier
	database.FirstOrCreate(&supplier, domain.Supplier{ID: "SUPP-IMG-" + suffix, Name: "Supplier Img"})

	prod := domain.Product{
		Nama:       "Kulkas MinIO Test " + suffix,
		Kode:       "PROD-IMG-" + suffix,
		Saldo:      5,
		CategoryID: category.ID,
		BrandID:    brand.ID,
		SupplierID: supplier.ID,
	}
	if err := database.Create(&prod).Error; err != nil {
		t.Fatalf("failed to create product: %v", err)
	}
	defer func() {
		database.Unscoped().Where("product_id = ?", prod.ID).Delete(&domain.ProductImage{})
		database.Unscoped().Where("id = ?", prod.ID).Delete(&domain.Product{})
	}()

	// Minimal 1x1 PNG payload
	pngBytes := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
		0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	}

	uploadedURL, err := storageSvc.UploadFile(context.Background(), "products", "test_btn.png", bytes.NewReader(pngBytes), int64(len(pngBytes)), "image/png")
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}
	if !strings.HasPrefix(uploadedURL, "http://127.0.0.1:9000/stock-opname/") {
		t.Errorf("expected MinIO public URL, got %q", uploadedURL)
	}

	// Verifikasi URL di MinIO bisa diakses HTTP GET 200 OK
	resp, err := http.Get(uploadedURL)
	if err != nil {
		t.Fatalf("failed HTTP GET to MinIO URL %s: %v", uploadedURL, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected HTTP 200 from MinIO for %s, got %d", uploadedURL, resp.StatusCode)
	}

	// Simpan ke ProductImage untuk Gudang BTN
	img, err := prodSvc.AddProductImage(prod.ID, "Gudang BTN", uploadedURL, "Rak Gudang BTN")
	if err != nil {
		t.Fatalf("AddProductImage failed: %v", err)
	}

	// Uji GetProductImages dengan filter lokasi (sebelumnya bug SQL syntax)
	imgs, err := prodSvc.GetProductImages(prod.ID, "Gudang BTN")
	if err != nil {
		t.Fatalf("GetProductImages failed: %v", err)
	}
	if len(imgs) != 1 || imgs[0].ID != img.ID {
		t.Errorf("expected 1 image for Gudang BTN, got %+v", imgs)
	}

	if err := prodSvc.DeleteProductImage(img.ID); err != nil {
		t.Fatalf("DeleteProductImage failed: %v", err)
	}
}

func TestHybridGlobalStockSyncAndNormalization(t *testing.T) {
	setupFeatureTestDB(t)
	database := db.Postgres.DB

	accSvc := service.NewAccountingService(database)
	trxSvc := service.NewTransactionService(database, accSvc)
	opnameSvc := service.NewOpnameService(database, service.NewStorageService())

	// 0. Uji langsung NormalizeStockLocation untuk semua varian nama lokasi
	normCases := map[string]string{
		"":                    "Toko Utama",
		"Pusat":               "Toko Utama",
		"pusat":               "Toko Utama",
		"Utama":               "Toko Utama",
		"Toko Utama":          "Toko Utama",
		"sudirman":            "Toko Sudirman",
		"Toko Sudirman":       "Toko Sudirman",
		"paye":                "Toko Paye",
		"Toko Paye":           "Toko Paye",
		"karangsokong":        "Gudang Karangsokong",
		"Gudang Karangsokong": "Gudang Karangsokong",
		"btn":                 "Gudang BTN",
		"Gudang BTN":          "Gudang BTN",
	}
	for input, expected := range normCases {
		if got := service.NormalizeStockLocation(input); got != expected {
			t.Errorf("NormalizeStockLocation(%q) = %q, want %q", input, got, expected)
		}
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1000000)

	var catAll5 domain.Category
	database.FirstOrCreate(&catAll5, domain.Category{Name: "Kategori Hybrid5 " + suffix})
	var catPartial domain.Category
	database.FirstOrCreate(&catPartial, domain.Category{Name: "Kategori HybridPartial " + suffix})
	var brand domain.Brand
	database.FirstOrCreate(&brand, domain.Brand{Name: "Brand Hybrid " + suffix})
	var supplier domain.Supplier
	database.FirstOrCreate(&supplier, domain.Supplier{ID: "SUPP-HYB-" + suffix, Name: "Supplier Hybrid"})

	var user domain.User
	database.Where("id = ?", "USER-HYB-1").FirstOrCreate(&user, domain.User{
		ID: "USER-HYB-1", Name: "Kasir Hybrid", Username: "kasir_hyb_" + suffix, Role: "kasir", Location: "Toko Sudirman",
	})

	// Produk A: Skenario user — Global awal 5, fisik total 10, tapi ada penjualan 1 unit di tengah SO (akhir 9)
	prodA := domain.Product{
		Nama: "TV Hybrid A " + suffix, Kode: "HYB-A-" + suffix,
		Saldo: 5, HBeli: 1000000, HJual: 1200000, Satuan: "UNIT",
		CategoryID: catAll5.ID, BrandID: brand.ID, SupplierID: supplier.ID,
	}
	// Produk B: Global awal kebesaran 15, fisik total 5 lokasi = 10 -> baru turun saat lokasi ke-5 selesai
	prodB := domain.Product{
		Nama: "TV Hybrid B " + suffix, Kode: "HYB-B-" + suffix,
		Saldo: 15, HBeli: 1000000, HJual: 1200000, Satuan: "UNIT",
		CategoryID: catAll5.ID, BrandID: brand.ID, SupplierID: supplier.ID,
	}
	// Produk C: Global awal 15, hanya di-SO di 2 cabang (total 6) -> diuji manual SyncGlobalStockFromOpname
	prodC := domain.Product{
		Nama: "TV Hybrid C " + suffix, Kode: "HYB-C-" + suffix,
		Saldo: 15, HBeli: 1000000, HJual: 1200000, Satuan: "UNIT",
		CategoryID: catPartial.ID, BrandID: brand.ID, SupplierID: supplier.ID,
	}

	for _, p := range []*domain.Product{&prodA, &prodB, &prodC} {
		if err := database.Create(p).Error; err != nil {
			t.Fatalf("failed creating product %s: %v", p.Kode, err)
		}
	}

	// Snapshot saldo produk real (non-test) yang sudah punya SO Completed agar SyncGlobalStockFromOpname tidak menyisakan efek samping
	type prodSnapshot struct {
		ID    uint
		Saldo float64
	}
	var existingCompletedProds []prodSnapshot
	database.Table("products AS p").
		Select("DISTINCT p.id, p.saldo").
		Joins("JOIN stock_opname_items AS soi ON soi.product_id = p.id").
		Joins("JOIN stock_opnames AS so ON so.id = soi.stock_opname_id").
		Where("so.status = ? AND so.deleted_at IS NULL AND soi.deleted_at IS NULL AND p.id NOT IN ?", "Completed", []uint{prodA.ID, prodB.ID, prodC.ID}).
		Scan(&existingCompletedProds)

	var sessionIDs []uint
	invMidSale := "PJL-HYB-MID-" + suffix

	defer func() {
		// Pulihkan saldo produk eksisting bila tersentuh manual sync
		for _, snap := range existingCompletedProds {
			database.Model(&domain.Product{}).Where("id = ?", snap.ID).Update("saldo", snap.Saldo)
		}
		database.Unscoped().Where("journal_entry_id IN (SELECT id FROM journal_entries WHERE entry_number = ?)", "JV-SALES-"+invMidSale).Delete(&domain.JournalItem{})
		database.Unscoped().Where("entry_number = ?", "JV-SALES-"+invMidSale).Delete(&domain.JournalEntry{})
		database.Unscoped().Where("sales_id IN (SELECT id FROM sales WHERE invoice = ?)", invMidSale).Delete(&domain.PaymentMethod{})
		database.Unscoped().Where("invoice = ?", invMidSale).Delete(&domain.Sales{})

		prodIDs := []uint{prodA.ID, prodB.ID, prodC.ID}
		database.Unscoped().Where("product_id IN ?", prodIDs).Delete(&domain.StockMovement{})
		database.Unscoped().Where("product_id IN ?", prodIDs).Delete(&domain.StockOpnameItem{})
		database.Unscoped().Where("product_id IN ?", prodIDs).Delete(&domain.ProductStock{})
		if len(sessionIDs) > 0 {
			database.Unscoped().Where("id IN ?", sessionIDs).Delete(&domain.StockOpname{})
		}
		database.Unscoped().Where("id IN ?", prodIDs).Delete(&domain.Product{})
		database.Unscoped().Where("id = ?", user.ID).Delete(&domain.User{})
	}()

	getGlobalSaldo := func(pid uint) float64 {
		var p domain.Product
		database.Select("id, saldo").First(&p, pid)
		return p.Saldo
	}

	runOpname := func(loc string, catID uint, counts map[uint]float64) *domain.StockOpname {
		sess, err := opnameSvc.CreateSession(loc, &catID, "SUPERADMIN-HYB", "SO "+loc)
		if err != nil {
			t.Fatalf("CreateSession(%s) failed: %v", loc, err)
		}
		sessionIDs = append(sessionIDs, sess.ID)
		for pid, qty := range counts {
			if _, err := opnameSvc.UpdateCountItem(sess.ID, pid, domain.UpdateOpnameItemRequest{
				PhysicalQty: qty,
				Reason:      "Hitung Fisik " + loc,
			}); err != nil {
				t.Fatalf("UpdateCountItem(%s, %d) failed: %v", loc, pid, err)
			}
		}
		if _, err := opnameSvc.SubmitForApproval(sess.ID, "Submit "+loc); err != nil {
			t.Fatalf("SubmitForApproval(%s) failed: %v", loc, err)
		}
		approved, err := opnameSvc.ApproveSession(sess.ID, "SUPERADMIN-HYB")
		if err != nil {
			t.Fatalf("ApproveSession(%s) failed: %v", loc, err)
		}
		return approved
	}

	// =========================================================================
	// SKENARIO 1 & 2: SO Bertahap 5 Lokasi + Penjualan di Tengah SO
	// =========================================================================

	// 1. Lokasi 1: Toko Sudirman (A=2, B=2) -> Belum lengkap 5 lokasi, SUM <= Global -> Global A tetap 5, B tetap 15
	app1 := runOpname("Toko Sudirman", catAll5.ID, map[uint]float64{prodA.ID: 2, prodB.ID: 2})
	if got := getGlobalSaldo(prodA.ID); got != 5 {
		t.Errorf("after Sudirman SO: prodA global expected 5, got %.2f", got)
	}
	if got := getGlobalSaldo(prodB.ID); got != 15 {
		t.Errorf("after Sudirman SO: prodB global expected 15, got %.2f", got)
	}
	if app1.GlobalStockUpdatedCount != 0 {
		t.Errorf("after Sudirman SO: expected GlobalStockUpdatedCount 0, got %d", app1.GlobalStockUpdatedCount)
	}

	// 2. Penjualan di tengah proses SO pada Toko Sudirman (A laku 1 unit)
	// Sudirman A: 2 -> 1, Global A: 5 -> 4
	_, err := trxSvc.CreateSales(domain.CreateTransactionRequest{
		Invoice:       invMidSale,
		UserID:        user.ID,
		Location:      "Toko Sudirman",
		PaymentMethod: "Tunai",
		AmountPaid:    1200000,
		Items:         []domain.SalesItemRequest{{ProductSearch: prodA.Kode, Qty: 1, Price: 1200000}},
	})
	if err != nil {
		t.Fatalf("mid-SO CreateSales failed: %v", err)
	}
	if got := getGlobalSaldo(prodA.ID); got != 4 {
		t.Errorf("after mid-SO sale: prodA global expected 4, got %.2f", got)
	}
	var psSdrA domain.ProductStock
	database.Where("product_id = ? AND location = ?", prodA.ID, "Toko Sudirman").First(&psSdrA)
	if psSdrA.Saldo != 1 {
		t.Errorf("after mid-SO sale: prodA Sudirman expected 1, got %.2f", psSdrA.Saldo)
	}

	// 3. Lokasi 2: Toko Paye (A=3, B=3) -> SUM A = 1+3 = 4 (== Global 4), SUM B = 5 (< 15)
	runOpname("Toko Paye", catAll5.ID, map[uint]float64{prodA.ID: 3, prodB.ID: 3})
	if got := getGlobalSaldo(prodA.ID); got != 4 {
		t.Errorf("after Paye SO: prodA global expected 4, got %.2f", got)
	}
	if got := getGlobalSaldo(prodB.ID); got != 15 {
		t.Errorf("after Paye SO: prodB global expected 15, got %.2f", got)
	}

	// 4. Lokasi 3: Toko Utama (A=2, B=2) -> SUM A = 1+3+2 = 6 (> 4) => Global A OTOMATIS NAIK ke 6! SUM B = 7 (< 15) => tetap 15
	app3 := runOpname("Toko Utama", catAll5.ID, map[uint]float64{prodA.ID: 2, prodB.ID: 2})
	if got := getGlobalSaldo(prodA.ID); got != 6 {
		t.Errorf("after Utama SO: prodA global expected auto-raised to 6, got %.2f", got)
	}
	if got := getGlobalSaldo(prodB.ID); got != 15 {
		t.Errorf("after Utama SO: prodB global expected still 15, got %.2f", got)
	}
	if app3.GlobalStockRaisedCount != 1 || app3.GlobalStockFullSyncCount != 0 {
		t.Errorf("after Utama SO: expected Raised=1 FullSync=0, got Raised=%d FullSync=%d", app3.GlobalStockRaisedCount, app3.GlobalStockFullSyncCount)
	}

	// 5. Lokasi 4: Gudang Karangsokong (A=1, B=1) -> SUM A = 7 (> 6) => Global A naik ke 7; SUM B = 8 (< 15) => tetap 15
	runOpname("Gudang Karangsokong", catAll5.ID, map[uint]float64{prodA.ID: 1, prodB.ID: 1})
	if got := getGlobalSaldo(prodA.ID); got != 7 {
		t.Errorf("after Karangsokong SO: prodA global expected auto-raised to 7, got %.2f", got)
	}
	if got := getGlobalSaldo(prodB.ID); got != 15 {
		t.Errorf("after Karangsokong SO: prodB global expected still 15, got %.2f", got)
	}

	// 6. Lokasi 5: Gudang BTN (A=2, B=2) -> KE-5 LOKASI LENGKAP!
	// SUM A = 1 + 3 + 2 + 1 + 2 = 9 (akurat meski ada penjualan di tengah SO!)
	// SUM B = 2 + 3 + 2 + 1 + 2 = 10 -> Global B RESMI TURUN dari 15 ke 10!
	app5 := runOpname("Gudang BTN", catAll5.ID, map[uint]float64{prodA.ID: 2, prodB.ID: 2})
	if got := getGlobalSaldo(prodA.ID); got != 9 {
		t.Errorf("after BTN (5th) SO: prodA global expected 9, got %.2f", got)
	}
	if got := getGlobalSaldo(prodB.ID); got != 10 {
		t.Errorf("after BTN (5th) SO: prodB global expected full-synced down to 10, got %.2f", got)
	}
	if app5.GlobalStockFullSyncCount != 2 {
		t.Errorf("after BTN (5th) SO: expected GlobalStockFullSyncCount=2 (prodA & prodB), got %d", app5.GlobalStockFullSyncCount)
	}

	// =========================================================================
	// SKENARIO 3: SO Rutin Bulan Berikutnya (Single Branch) setelah 5 lokasi pernah Completed
	// =========================================================================
	// Re-SO Toko Sudirman: B turun 2 -> 1 (barang rusak/hilang) -> Global B otomatis turun 10 -> 9!
	appRoutine := runOpname("Toko Sudirman", catAll5.ID, map[uint]float64{prodA.ID: 1, prodB.ID: 1})
	if got := getGlobalSaldo(prodB.ID); got != 9 {
		t.Errorf("after routine Sudirman SO: prodB global expected auto-synced down to 9, got %.2f", got)
	}
	if appRoutine.GlobalStockFullSyncCount != 1 {
		t.Errorf("after routine Sudirman SO: expected GlobalStockFullSyncCount=1 (prodB), got %d", appRoutine.GlobalStockFullSyncCount)
	}

	// =========================================================================
	// SKENARIO 4: Preview & Manual SyncGlobalStockFromOpname (untuk produk baru 2 cabang SO)
	// =========================================================================
	runOpname("Toko Sudirman", catPartial.ID, map[uint]float64{prodC.ID: 2})
	runOpname("Toko Paye", catPartial.ID, map[uint]float64{prodC.ID: 4})
	// Baru 2 cabang (SUM=6 < Global 15) -> Global C harus tetap 15 sebelum sinkron manual
	if got := getGlobalSaldo(prodC.ID); got != 15 {
		t.Errorf("before manual sync: prodC global expected 15, got %.2f", got)
	}

	preview, err := opnameSvc.GetGlobalStockSyncPreview()
	if err != nil {
		t.Fatalf("GetGlobalStockSyncPreview failed: %v", err)
	}
	if len(preview.LocationStatuses) != 5 {
		t.Errorf("expected 5 location statuses, got %d", len(preview.LocationStatuses))
	}
	if preview.TotalLowered < 1 {
		t.Errorf("expected preview.TotalLowered >= 1 (at least prodC), got %d", preview.TotalLowered)
	}

	syncRes, err := opnameSvc.SyncGlobalStockFromOpname("SUPERADMIN-HYB")
	if err != nil {
		t.Fatalf("SyncGlobalStockFromOpname failed: %v", err)
	}
	if syncRes.TotalLowered < 1 {
		t.Errorf("expected syncRes.TotalLowered >= 1, got %d", syncRes.TotalLowered)
	}
	if got := getGlobalSaldo(prodC.ID); got != 6 {
		t.Errorf("after manual sync: prodC global expected 6 (2+4), got %.2f", got)
	}

	// =========================================================================
	// SKENARIO 5: Normalisasi "Pusat" pada AdjustProductStock -> "Toko Utama"
	// =========================================================================
	tx := database.Begin()
	if err := service.AdjustProductStock(tx, prodA.ID, "Pusat", -1, "SALE", "INV-NORM-"+suffix, "Test Normalisasi Pusat", "KASIR-1"); err != nil {
		tx.Rollback()
		t.Fatalf("AdjustProductStock('Pusat') failed: %v", err)
	}
	tx.Commit()

	var psUtama domain.ProductStock
	if err := database.Where("product_id = ? AND location = ?", prodA.ID, "Toko Utama").First(&psUtama).Error; err != nil {
		t.Fatalf("failed to fetch Toko Utama stock: %v", err)
	}
	if psUtama.Saldo != 1 {
		t.Errorf("expected Toko Utama saldo decremented from 2 to 1 via 'Pusat' normalization, got %.2f", psUtama.Saldo)
	}
	var pusatCount int64
	database.Model(&domain.ProductStock{}).Where("product_id = ? AND location = ?", prodA.ID, "Pusat").Count(&pusatCount)
	if pusatCount != 0 {
		t.Errorf("expected 0 'Pusat' rows in product_stocks after normalization, got %d", pusatCount)
	}

	// =========================================================================
	// SKENARIO 6: Lokasi non-resmi ("Toko Kota") diabaikan oleh breakdown & sync
	// =========================================================================
	if err := database.Create(&domain.ProductStock{
		ProductID: prodC.ID,
		Location:  "Toko Kota",
		Saldo:     99,
	}).Error; err != nil {
		t.Fatalf("failed inserting legacy Toko Kota stock: %v", err)
	}
	bdC, err := opnameSvc.GetProductStockBreakdown(prodC.ID)
	if err != nil {
		t.Fatalf("GetProductStockBreakdown failed: %v", err)
	}
	for _, st := range bdC {
		if st.Location == "Toko Kota" {
			t.Errorf("GetProductStockBreakdown must exclude non-official location 'Toko Kota'")
		}
	}
	// Jalankan manual sync lagi -> saldo prodC harus tetap 6 (2 Sudirman + 4 Paye), BUKAN 105!
	if _, err := opnameSvc.SyncGlobalStockFromOpname("SUPERADMIN-HYB"); err != nil {
		t.Fatalf("SyncGlobalStockFromOpname with ghost location failed: %v", err)
	}
	if got := getGlobalSaldo(prodC.ID); got != 6 {
		t.Errorf("ghost 'Toko Kota' stock must be ignored by SyncGlobalStockFromOpname: expected 6, got %.2f", got)
	}
}


func TestProductImagesIncludesOpnamePhotosWithMetadata(t *testing.T) {
	setupFeatureTestDB(t)
	database := db.Postgres.DB

	opnameSvc := service.NewOpnameService(database, service.NewStorageService())
	prodSvc := service.NewProductService(database)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
	var cat domain.Category
	database.FirstOrCreate(&cat, domain.Category{Name: "Cat SOPhoto " + suffix})
	var brand domain.Brand
	database.FirstOrCreate(&brand, domain.Brand{Name: "Brand SOPhoto " + suffix})
	var supp domain.Supplier
	database.FirstOrCreate(&supp, domain.Supplier{ID: "SUP-SOP-" + suffix, Name: "Supp SOPhoto"})

	prod := domain.Product{
		Nama: "TV SOPhoto Test " + suffix, Kode: "SOP-" + suffix,
		Saldo: 5, HBeli: 100000, HJual: 150000, Satuan: "UNIT",
		CategoryID: cat.ID, BrandID: brand.ID, SupplierID: supp.ID,
	}
	if err := database.Create(&prod).Error; err != nil {
		t.Fatalf("failed create product: %v", err)
	}

	var sessionIDs []uint
	defer func() {
		database.Unscoped().Where("product_id = ?", prod.ID).Delete(&domain.ProductImage{})
		database.Unscoped().Where("product_id = ?", prod.ID).Delete(&domain.StockMovement{})
		database.Unscoped().Where("product_id = ?", prod.ID).Delete(&domain.StockOpnameItem{})
		database.Unscoped().Where("product_id = ?", prod.ID).Delete(&domain.ProductStock{})
		if len(sessionIDs) > 0 {
			database.Unscoped().Where("id IN ?", sessionIDs).Delete(&domain.StockOpname{})
		}
		database.Unscoped().Where("id = ?", prod.ID).Delete(&domain.Product{})
	}()

	// 1. Sesi SO aktif (Toko Utama) dengan foto
	sess1, err := opnameSvc.CreateSession("Toko Utama", &cat.ID, "SUPERADMIN-1", "SO Foto Utama")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	sessionIDs = append(sessionIDs, sess1.ID)

	photoURL1 := "http://127.0.0.1:9000/stock-opname/opname/test_utama_" + suffix + ".jpg"
	if _, err := opnameSvc.UpdateCountItem(sess1.ID, prod.ID, domain.UpdateOpnameItemRequest{
		PhysicalQty: 5,
		Reason:      "Cek Fisik",
		Notes:       "Rak A1",
		PhotoURL:    photoURL1,
	}); err != nil {
		t.Fatalf("UpdateCountItem failed: %v", err)
	}

	// 2. Sesi SO yang dibatalkan (Toko Paye) -> fotonya TIDAK boleh muncul di galeri produk
	sess2, err := opnameSvc.CreateSession("Toko Paye", &cat.ID, "SUPERADMIN-1", "SO Batal")
	if err != nil {
		t.Fatalf("CreateSession sess2 failed: %v", err)
	}
	sessionIDs = append(sessionIDs, sess2.ID)
	photoURLCancelled := "http://127.0.0.1:9000/stock-opname/opname/test_cancelled_" + suffix + ".jpg"
	_, _ = opnameSvc.UpdateCountItem(sess2.ID, prod.ID, domain.UpdateOpnameItemRequest{
		PhysicalQty: 2,
		PhotoURL:    photoURLCancelled,
	})
	if _, err := opnameSvc.CancelSession(sess2.ID, "SUPERADMIN-1"); err != nil {
		t.Fatalf("CancelSession failed: %v", err)
	}

	// 3. Cek GetProductImages (semua lokasi) -> harus menemukan foto SO sess1 beserta metadata SO
	imgs, err := prodSvc.GetProductImages(prod.ID, "")
	if err != nil {
		t.Fatalf("GetProductImages failed: %v", err)
	}
	if len(imgs) != 1 {
		t.Fatalf("expected 1 image (only active SO, excluding Cancelled), got %d: %+v", len(imgs), imgs)
	}
	got := imgs[0]
	if got.Source != "opname" || got.OpnameID != sess1.ID || got.OpnameNo != sess1.OpnameNo || got.OpnameCreatedAt == nil || got.Location != "Toko Utama" || got.ImageURL != photoURL1 {
		t.Errorf("unexpected SO photo metadata: %+v", got)
	}

	// 4. Cek filter lokasi "Toko Sudirman" -> harus kosong
	imgsSdr, err := prodSvc.GetProductImages(prod.ID, "Toko Sudirman")
	if err != nil {
		t.Fatalf("GetProductImages filter failed: %v", err)
	}
	if len(imgsSdr) != 0 {
		t.Errorf("expected 0 images for Toko Sudirman, got %d", len(imgsSdr))
	}
}
