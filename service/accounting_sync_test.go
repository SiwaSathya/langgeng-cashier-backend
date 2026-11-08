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

func TestSyncAllAccountingWipeAndRecreate(t *testing.T) {
	setupFeatureTestDB(t)
	database := db.Postgres.DB

	// Bersihkan jika ada sisa data test sebelumnya
	database.Unscoped().Where("sales_invoice LIKE ?", "%TEST-SYNC%").Delete(&domain.PiutangDagang{})
	database.Unscoped().Where("reference LIKE ?", "%TEST-SYNC%").Delete(&domain.JournalEntry{})
	database.Unscoped().Where("nota LIKE ?", "%TEST-SYNC%").Delete(&domain.Expense{})
	database.Unscoped().Where("nota LIKE ?", "%TEST-SYNC%").Delete(&domain.Purchase{})
	database.Unscoped().Where("invoice LIKE ?", "%TEST-SYNC%").Delete(&domain.Sales{})
	database.Unscoped().Where("kode LIKE ?", "%TEST-SYNC%").Delete(&domain.Product{})
	database.Unscoped().Where("id LIKE ?", "%USER-SYNC%").Delete(&domain.User{})

	accountingSvc := service.NewAccountingService(database)

	testSuffix := fmt.Sprintf("%d", time.Now().UnixNano())
	testInvoice := fmt.Sprintf("INV-TEST-SYNC-%s", testSuffix)
	testNotaPurchase := fmt.Sprintf("NOTA-TEST-SYNC-%s", testSuffix)
	testNotaExpense := fmt.Sprintf("EXP-TEST-SYNC-%s", testSuffix)

	var supplier domain.Supplier
	database.FirstOrCreate(&supplier, domain.Supplier{ID: "SUPP-SYNC", Name: "Supplier Sync"})

	var category domain.Category
	database.FirstOrCreate(&category, domain.Category{Name: "Kategori Sync"})

	var brand domain.Brand
	database.FirstOrCreate(&brand, domain.Brand{Name: "Brand Sync"})

	user := domain.User{
		ID:       fmt.Sprintf("USER-SYNC-%s", testSuffix),
		Name:     "User Sync",
		Username: fmt.Sprintf("usersync_%s", testSuffix),
		Role:     "kasir",
		Location: "Toko Utama",
	}
	if err := database.Create(&user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Buat produk dummy
	dummyProduct := domain.Product{
		Model:      gorm.Model{CreatedAt: time.Now()},
		Nama:       "Produk Sync Test " + testSuffix,
		Kode:       "PROD-SYNC-" + testSuffix,
		Saldo:      10,
		HBeli:      100000,
		HJual:      150000,
		Satuan:     "PCS",
		SupplierID: supplier.ID,
		CategoryID: category.ID,
		BrandID:    brand.ID,
	}
	if err := database.Create(&dummyProduct).Error; err != nil {
		database.Unscoped().Delete(&user)
		t.Fatalf("failed to create dummy product: %v", err)
	}

	// Buat transaksi Sales dummy (Bon / DP)
	dummySale := domain.Sales{
		Model:         gorm.Model{CreatedAt: time.Now()},
		Invoice:       testInvoice,
		ProductID:     dummyProduct.ID,
		UserID:        user.ID,
		MemberName:    "Customer Sync Test",
		Qty:           1,
		HBeli:         100000,
		HJual:         150000,
		TotalNetto:    150000,
		AmountPaid:    50000,
		PaymentMethod: "Tunai",
		IsDp:          true,
		Status:        "Belum Lunas",
		Location:      "Toko Utama",
	}
	if err := database.Create(&dummySale).Error; err != nil {
		database.Unscoped().Where("kode = ?", dummyProduct.Kode).Delete(&domain.Product{})
		database.Unscoped().Delete(&user)
		t.Fatalf("failed to create dummy sale: %v", err)
	}

	// Buat transaksi Purchase dummy
	dummyPurchase := domain.Purchase{
		Model:     gorm.Model{CreatedAt: time.Now()},
		Nota:      testNotaPurchase,
		ProductID: dummyProduct.ID,
		Total:     200000,
		Location:  "Toko Utama",
		Tanggal:   time.Now(),
	}
	if err := database.Create(&dummyPurchase).Error; err != nil {
		database.Unscoped().Where("invoice = ?", testInvoice).Delete(&domain.Sales{})
		database.Unscoped().Where("kode = ?", dummyProduct.Kode).Delete(&domain.Product{})
		database.Unscoped().Delete(&user)
		t.Fatalf("failed to create dummy purchase: %v", err)
	}

	// Buat pengeluaran Expense dummy
	dummyExpense := domain.Expense{
		Model:     gorm.Model{CreatedAt: time.Now()},
		Nota:      testNotaExpense,
		Deskripsi: "Beban Listrik Operasional Toko Test",
		Kategori:  "Listrik",
		Total:     75000,
		Location:  "Toko Utama",
		Tanggal:   time.Now(),
		UserID:    user.ID,
	}
	if err := database.Create(&dummyExpense).Error; err != nil {
		database.Unscoped().Where("nota = ?", testNotaPurchase).Delete(&domain.Purchase{})
		database.Unscoped().Where("invoice = ?", testInvoice).Delete(&domain.Sales{})
		database.Unscoped().Where("kode = ?", dummyProduct.Kode).Delete(&domain.Product{})
		database.Unscoped().Delete(&user)
		t.Fatalf("failed to create dummy expense: %v", err)
	}

	defer func() {
		database.Unscoped().Where("sales_invoice = ?", testInvoice).Delete(&domain.PiutangDagang{})
		database.Unscoped().Where("reference IN ?", []string{testInvoice, testNotaPurchase, testNotaExpense}).Delete(&domain.JournalEntry{})
		database.Unscoped().Where("nota = ?", testNotaExpense).Delete(&domain.Expense{})
		database.Unscoped().Where("nota = ?", testNotaPurchase).Delete(&domain.Purchase{})
		database.Unscoped().Where("invoice = ?", testInvoice).Delete(&domain.Sales{})
		database.Unscoped().Where("kode = ?", dummyProduct.Kode).Delete(&domain.Product{})
		database.Unscoped().Delete(&user)
	}()

	// 1. Eksekusi pertama sinkronisasi akuntansi
	summary1, err := accountingSvc.SyncAllAccounting(database)
	if err != nil {
		t.Fatalf("SyncAllAccounting first run failed: %v", err)
	}

	if summary1.SalesSynced < 1 {
		t.Errorf("expected at least 1 sales synced, got %d", summary1.SalesSynced)
	}
	if summary1.PurchasesSynced < 1 {
		t.Errorf("expected at least 1 purchase synced, got %d", summary1.PurchasesSynced)
	}
	if summary1.ExpensesSynced < 1 {
		t.Errorf("expected at least 1 expense synced, got %d", summary1.ExpensesSynced)
	}
	if summary1.PiutangSynced < 1 {
		t.Errorf("expected at least 1 piutang synced, got %d", summary1.PiutangSynced)
	}

	// Verifikasi entri jurnal test benar-benar tersimpan
	var jvSales domain.JournalEntry
	if err := database.Where("reference = ?", testInvoice).First(&jvSales).Error; err != nil {
		t.Fatalf("expected sales journal entry created, err: %v", err)
	}

	// 2. Eksekusi kedua sinkronisasi akuntansi (Mekanisme Wipe-First & Re-Create)
	summary2, err := accountingSvc.SyncAllAccounting(database)
	if err != nil {
		t.Fatalf("SyncAllAccounting second run (wipe & recreate) failed: %v", err)
	}

	// Pada eksekusi kedua, data lama harus terhapus lebih dulu
	if summary2.DeletedJournals == 0 {
		t.Errorf("expected summary2.DeletedJournals > 0, got %d", summary2.DeletedJournals)
	}
	if summary2.DeletedItems == 0 {
		t.Errorf("expected summary2.DeletedItems > 0, got %d", summary2.DeletedItems)
	}

	// Dan jurnal berhasil di-generate kembali secara bersih
	var jvSalesRecreated domain.JournalEntry
	if err := database.Where("reference = ?", testInvoice).First(&jvSalesRecreated).Error; err != nil {
		t.Fatalf("expected sales journal entry recreated after wipe, err: %v", err)
	}
}
