package service_test

import (
	"backend-cashier/db"
	"backend-cashier/domain"
	"backend-cashier/service"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"
)

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
		Saldo:      10, // Saldo global lama
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
		// Bersihkan data test
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

	// Buka Sesi Opname baru untuk Toko Sudirman
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
	// (Selisih: Fisik 3 - Sistem 0 = +3, Nilai Deviasi: 3 * 100.000 = Rp 300.000)
	updatedItem, err := opnameSvc.UpdateCountItem(session1.ID, dummyProduct.ID, domain.UpdateOpnameItemRequest{
		PhysicalQty: 3,
		Reason:      "Stok Awal Fisik",
		Notes:       "Rak Elektronik A1",
		PhotoURL:    "/uploads/opname/test_sudirman.jpg",
	})
	if err != nil {
		t.Fatalf("failed to update count item: %v", err)
	}

	if updatedItem.PhysicalQty != 3 {
		t.Errorf("expected physical_qty 3, got %f", updatedItem.PhysicalQty)
	}
	if updatedItem.DiscrepancyQty != 3 {
		t.Errorf("expected discrepancy_qty 3, got %f", updatedItem.DiscrepancyQty)
	}
	if updatedItem.DiscrepancyValue != 300000 {
		t.Errorf("expected discrepancy_value 300000, got %f", updatedItem.DiscrepancyValue)
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
	// 2. UJI COBA SESI OPNAME CABANG 2: TOKO PAYE (Independensi Antar Cabang)
	// =========================================================================
	locPaye := "Toko Paye"
	session2, err := opnameSvc.CreateSession(locPaye, &category.ID, "USER-AUDIT-2", "Opname Toko Paye")
	if err != nil {
		t.Fatalf("failed to create session for Paye: %v", err)
	}
	sessionIDs = append(sessionIDs, session2.ID)

	// Di Toko Paye, dihitung fisik ada 2 unit
	_, err = opnameSvc.UpdateCountItem(session2.ID, dummyProduct.ID, domain.UpdateOpnameItemRequest{
		PhysicalQty: 2,
		Reason:      "Stok Fisik Display",
		Notes:       "Etalase Depan",
		PhotoURL:    "/uploads/opname/test_paye.jpg",
	})
	if err != nil {
		t.Fatalf("failed to update count for Paye: %v", err)
	}

	// Approve sesi Paye
	_, err = opnameSvc.ApproveSession(session2.ID, "SUPERVISOR-PAY")
	if err != nil {
		t.Fatalf("failed to approve session Paye: %v", err)
	}

	// Verifikasi Saldo Toko Paye = 2, dan Toko Sudirman tetap 3 (tidak terganggu)
	var psPayeAfter domain.ProductStock
	database.Where("product_id = ? AND location = ?", dummyProduct.ID, locPaye).First(&psPayeAfter)
	if psPayeAfter.Saldo != 2 {
		t.Errorf("expected Paye saldo 2, got %f", psPayeAfter.Saldo)
	}

	database.Where("product_id = ? AND location = ?", dummyProduct.ID, locSudirman).First(&psSudirmanAfter)
	if psSudirmanAfter.Saldo != 3 {
		t.Errorf("expected Sudirman saldo remains 3, got %f", psSudirmanAfter.Saldo)
	}

	// Verifikasi Saldo Total di Master Produk = 3 + 2 = 5
	var prodAfterAll domain.Product
	database.First(&prodAfterAll, dummyProduct.ID)
	if prodAfterAll.Saldo != 5 {
		t.Errorf("expected master product total saldo 5 (3 Sudirman + 2 Paye), got %f", prodAfterAll.Saldo)
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
	// Kasir melakukan penjualan 1 unit di Toko Sudirman
	tx := database.Begin()
	errAdjust := service.AdjustProductStock(tx, dummyProduct.ID, locSudirman, -1, "SALE", "INV-TEST-OPN-1", "Penjualan Toko Sudirman", "KASIR-1")
	if errAdjust != nil {
		tx.Rollback()
		t.Fatalf("failed to adjust product stock on sale: %v", errAdjust)
	}
	tx.Commit()

	// Stok Toko Sudirman sekarang harus berkurang jadi 2 (3 - 1 = 2)
	database.Where("product_id = ? AND location = ?", dummyProduct.ID, locSudirman).First(&psSudirmanAfter)
	if psSudirmanAfter.Saldo != 2 {
		t.Errorf("expected Sudirman saldo 2 after sale, got %f", psSudirmanAfter.Saldo)
	}

	// Stok Toko Paye tetap 2
	database.Where("product_id = ? AND location = ?", dummyProduct.ID, locPaye).First(&psPayeAfter)
	if psPayeAfter.Saldo != 2 {
		t.Errorf("expected Paye saldo remains 2, got %f", psPayeAfter.Saldo)
	}

	// Verifikasi Kartu Stok Toko Sudirman memiliki 2 baris (OPNAME_ADJUST dan SALE)
	stockCardSdr, countSdr, err := opnameSvc.GetStockCard(dummyProduct.ID, locSudirman, 10, 0)
	if err != nil {
		t.Fatalf("failed to get stock card: %v", err)
	}
	if countSdr < 2 {
		t.Errorf("expected at least 2 stock movements for Sudirman, got %d", countSdr)
	}
	t.Logf("Sukses! Kartu stok Toko Sudirman tercatat %d mutasi, saldo akhir %.0f", countSdr, stockCardSdr[0].SaldoAfter)
}
