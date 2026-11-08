package service_test

import (
	"backend-cashier/db"
	"backend-cashier/domain"
	"backend-cashier/service"
	"fmt"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

func setupFeatureTestDB(t *testing.T) {
	_ = godotenv.Load("../.env")
	db.New()
	if db.Postgres.DB == nil {
		t.Skip("Database not available, skipping integration test")
	}
	db.RegisterTableToMigrate(db.Postgres.DB)
}

func TestUserAndAttendanceService(t *testing.T) {
	setupFeatureTestDB(t)
	database := db.Postgres.DB

	authSvc := service.NewAuthService(database)
	attSvc := service.NewAttendanceService(database)

	// 1. Test Create User with role
	testUsername := fmt.Sprintf("testuser_%d", time.Now().UnixNano()%100000)
	userReq := domain.UserRequest{
		Name:     "Test Employee",
		Username: testUsername,
		Password: "password123",
		Location: "Pusat",
		Role:     "kasir",
	}

	userResp, err := authSvc.CreateUser(userReq)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if userResp.ID == "" || userResp.Role != "kasir" {
		t.Fatalf("expected role kasir, got %s", userResp.Role)
	}

	defer func() {
		database.Unscoped().Where("username = ?", testUsername).Delete(&domain.User{})
	}()

	// 2. Test Login
	token, err := authSvc.Login(testUsername, "password123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if token == "" {
		t.Fatalf("expected non-empty token")
	}

	// 3. Test Create Attendance
	todayStr := time.Now().Format("2006-01-02")
	attReq := domain.AttendanceRequest{
		UserID:   userResp.ID,
		Date:     todayStr,
		Shift:    1,
		Status:   "Pagi",
		CheckIn:  "08:00",
		CheckOut: "16:00",
		Notes:    "Hadir tepat waktu",
	}

	att, err := attSvc.CreateAttendance(attReq)
	if err != nil {
		t.Fatalf("CreateAttendance failed: %v", err)
	}
	if att.ID == 0 || att.Status != "Pagi" {
		t.Fatalf("expected status Pagi, got %s", att.Status)
	}

	defer func() {
		database.Unscoped().Delete(&domain.Attendance{}, att.ID)
	}()

	// 4. Test Get Attendance Summary
	summary, err := attSvc.GetSummary(todayStr)
	if err != nil {
		t.Fatalf("GetSummary failed: %v", err)
	}
	if summary.Pagi < 1 {
		t.Fatalf("expected at least 1 Pagi in summary, got %d", summary.Pagi)
	}

	// 5. Test Get Attendance List
	list, total, err := attSvc.GetAllAttendance(domain.AttendanceFilter{Date: todayStr})
	if err != nil {
		t.Fatalf("GetAllAttendance failed: %v", err)
	}
	if total < 1 || len(list) < 1 {
		t.Fatalf("expected at least 1 attendance item, got %d", len(list))
	}

	// 6. Test Live Clock-In and Clock-Out flow
	clockUserReq := domain.UserRequest{
		Name:     "Clockin Employee",
		Username: fmt.Sprintf("clockuser_%d", time.Now().UnixNano()%100000),
		Password: "password123",
		Location: "Toko Utama",
		Role:     "kasir",
	}
	clockUser, err := authSvc.CreateUser(clockUserReq)
	if err != nil {
		t.Fatalf("CreateUser for clockin failed: %v", err)
	}
	defer func() {
		database.Unscoped().Where("user_id = ?", clockUser.ID).Delete(&domain.Attendance{})
		database.Unscoped().Where("username = ?", clockUserReq.Username).Delete(&domain.User{})
	}()

	// Status before clock-in
	statusBefore, err := attSvc.GetTodayAttendance(clockUser.ID)
	if err != nil {
		t.Fatalf("GetTodayAttendance before clockin failed: %v", err)
	}
	if statusBefore.HasAttended {
		t.Fatalf("expected HasAttended = false before clock-in")
	}

	// Perform Clock-In
	clockedIn, err := attSvc.ClockIn(domain.ClockInRequest{
		UserID: clockUser.ID,
		Shift:  1,
		Notes:  "Mulai shift pagi",
	})
	if err != nil {
		t.Fatalf("ClockIn failed: %v", err)
	}
	if clockedIn.CheckIn == nil {
		t.Fatalf("expected non-nil CheckIn after clock-in")
	}

	// Status after clock-in
	statusAfterIn, err := attSvc.GetTodayAttendance(clockUser.ID)
	if err != nil {
		t.Fatalf("GetTodayAttendance after clockin failed: %v", err)
	}
	if !statusAfterIn.HasAttended || statusAfterIn.HasClockedOut {
		t.Fatalf("expected HasAttended = true, HasClockedOut = false after clock-in")
	}

	// Perform Clock-Out
	clockedOut, err := attSvc.ClockOut(domain.ClockOutRequest{
		UserID: clockUser.ID,
		Notes:  "Shift selesai",
	})
	if err != nil {
		t.Fatalf("ClockOut failed: %v", err)
	}
	if clockedOut.CheckOut == nil {
		t.Fatalf("expected non-nil CheckOut after clock-out")
	}

	// Status after clock-out
	statusAfterOut, err := attSvc.GetTodayAttendance(clockUser.ID)
	if err != nil {
		t.Fatalf("GetTodayAttendance after clockout failed: %v", err)
	}
	if !statusAfterOut.HasAttended || !statusAfterOut.HasClockedOut {
		t.Fatalf("expected HasAttended = true, HasClockedOut = true after clock-out")
	}
}

func TestAccountingService(t *testing.T) {
	setupFeatureTestDB(t)
	database := db.Postgres.DB

	accSvc := service.NewAccountingService(database)

	// 1. Verify default accounts exist
	accounts, err := accSvc.GetAllAccounts()
	if err != nil {
		t.Fatalf("GetAllAccounts failed: %v", err)
	}
	if len(accounts) < 10 {
		t.Fatalf("expected seeded accounts, got %d", len(accounts))
	}

	// Find KAS and PENJUALAN accounts
	var kasAcc, salesAcc *domain.Account
	for i := range accounts {
		if accounts[i].Name == "KAS" {
			kasAcc = &accounts[i]
		}
		if accounts[i].Name == "PENJUALAN" {
			salesAcc = &accounts[i]
		}
	}
	if kasAcc == nil || salesAcc == nil {
		t.Fatalf("expected KAS and PENJUALAN accounts to exist")
	}

	// 2. Create Balanced Journal Entry
	todayStr := time.Now().Format("2006-01-02")
	jReq := domain.CreateJournalRequest{
		Date:        todayStr,
		Description: "Penjualan Tunai Harian Testing",
		Reference:   "REF-TEST-001",
		Items: []domain.CreateJournalItemRequest{
			{
				AccountID:   kasAcc.ID,
				Description: "Penerimaan Kas",
				Debet:       500000,
				Kredit:      0,
			},
			{
				AccountID:   salesAcc.ID,
				Description: "Pendapatan Penjualan",
				Debet:       0,
				Kredit:      500000,
			},
		},
	}

	entry, err := accSvc.CreateJournalEntry(jReq)
	if err != nil {
		t.Fatalf("CreateJournalEntry failed: %v", err)
	}
	if entry.ID == 0 {
		t.Fatalf("expected created journal entry ID")
	}

	defer func() {
		database.Unscoped().Where("journal_entry_id = ?", entry.ID).Delete(&domain.JournalItem{})
		database.Unscoped().Delete(&domain.JournalEntry{}, entry.ID)
	}()

	// 3. Test Unbalanced Journal Fails
	unbalancedReq := domain.CreateJournalRequest{
		Date:        todayStr,
		Description: "Unbalanced",
		Items: []domain.CreateJournalItemRequest{
			{AccountID: kasAcc.ID, Debet: 100000, Kredit: 0},
			{AccountID: salesAcc.ID, Debet: 0, Kredit: 50000},
		},
	}
	_, err = accSvc.CreateJournalEntry(unbalancedReq)
	if err == nil {
		t.Fatalf("expected error for unbalanced journal entry")
	}

	// 4. Test Rekapitulasi
	recap, err := accSvc.GetRekapitulasi(todayStr, todayStr, "")
	if err != nil {
		t.Fatalf("GetRekapitulasi failed: %v", err)
	}
	if !recap.IsBalanced {
		t.Fatalf("expected rekapitulasi to be balanced, difference: %f", recap.Difference)
	}

	// 5. Test General Ledger
	ledger, err := accSvc.GetGeneralLedger(kasAcc.ID, todayStr, todayStr, "")
	if err != nil {
		t.Fatalf("GetGeneralLedger failed: %v", err)
	}
	if len(ledger) == 0 {
		t.Fatalf("expected ledger card for KAS")
	}
}

// func TestShiftIsolationPerStore(t *testing.T) {
// 	setupFeatureTestDB(t)
// 	database := db.Postgres.DB

// 	accSvc := service.NewAccountingService(database)
// 	trxSvc := service.NewTransactionService(database, accSvc)

// 	var supplier domain.Supplier
// 	database.FirstOrCreate(&supplier, domain.Supplier{ID: "SUPP-ISO", Name: "Supplier ISO"})

// 	var category domain.Category
// 	database.FirstOrCreate(&category, domain.Category{Name: "Category ISO"})

// 	var brand domain.Brand
// 	database.FirstOrCreate(&brand, domain.Brand{Name: "Brand ISO"})

// 	var user domain.User
// 	database.FirstOrCreate(&user, domain.User{ID: "USER-ISO-1", Name: "Kasir ISO", Username: "kasir_iso", Role: "kasir", Location: "Toko Utama"})

// 	// Create test products
// 	prod := domain.Product{
// 		Kode:       fmt.Sprintf("PROD-ISO-%d", time.Now().UnixNano()%100000),
// 		Nama:       fmt.Sprintf("Produk Isolation Test %d", time.Now().UnixNano()%100000),
// 		Saldo:      100,
// 		HBeli:      10000,
// 		HJual:      20000,
// 		SupplierID: supplier.ID,
// 		CategoryID: category.ID,
// 		BrandID:    brand.ID,
// 	}
// 	if err := if err := database.Create(&prod).Error; err != nil { t.Fatalf("failed to create prod: %v", err) }.Error; err != nil {
// 		t.Fatalf("failed to create test product: %v", err)
// 	}
// 	defer database.Unscoped().Delete(&prod)

// 	// 1. Create Sales in Toko Utama (shift IS NULL)
// 	resUtama, err := trxSvc.CreateSales(domain.CreateTransactionRequest{
// 		Location:      "Toko Utama",
// 		UserID:        user.ID,
// 		MemberName:    "Pelanggan Utama",
// 		PaymentMethod: "Tunai",
// 		AmountPaid:    20000,
// 		Items: []domain.SalesItemRequest{
// 			{ProductSearch: prod.Kode, Qty: 1, Price: 20000},
// 		},
// 	})
// 	if err != nil {
// 		t.Fatalf("CreateSales Toko Utama failed: %v", err)
// 	}
// 	defer database.Unscoped().Where("invoice = ?", resUtama.Invoice).Delete(&domain.Sales{})

// 	// 2. Create Sales in Toko Sudirman (shift IS NULL)
// 	resSudirman, err := trxSvc.CreateSales(domain.CreateTransactionRequest{
// 		Location:      "Toko Sudirman",
// 		UserID:        user.ID,
// 		MemberName:    "Pelanggan Sudirman",
// 		PaymentMethod: "Tunai",
// 		AmountPaid:    20000,
// 		Items: []domain.SalesItemRequest{
// 			{ProductSearch: prod.Kode, Qty: 1, Price: 20000},
// 		},
// 	})
// 	if err != nil {
// 		t.Fatalf("CreateSales Toko Sudirman failed: %v", err)
// 	}
// 	defer database.Unscoped().Where("invoice = ?", resSudirman.Invoice).Delete(&domain.Sales{})

// 	var checkSudirman domain.Sales
// 	database.Where("invoice = ?", resSudirman.Invoice).First(&checkSudirman)
// 	if checkSudirman.Location != "Toko Sudirman" {
// 		t.Fatalf("expected checkSudirman location 'Toko Sudirman', got '%s'", checkSudirman.Location)
// 	}

// 	// 3. Close Shift ONLY in Toko Utama -> Shift 1
// 	err = trxSvc.UpdateSalesShift(1, "Toko Utama")
// 	if err != nil {
// 		t.Fatalf("UpdateSalesShift Toko Utama failed: %v", err)
// 	}

// 	// 4. Verify Toko Utama sales has shift = 1
// 	var sUtama domain.Sales
// 	database.Where("invoice = ?", resUtama.Invoice).First(&sUtama)
// 	if sUtama.Shift == nil || *sUtama.Shift != 1 {
// 		t.Fatalf("expected Toko Utama sales shift to be 1, got %v", sUtama.Shift)
// 	}

// 	// 5. Verify Toko Sudirman sales STILL HAS shift IS NULL (isolated and unaffected!)
// 	var sSudirman domain.Sales
// 	database.Where("invoice = ?", resSudirman.Invoice).First(&sSudirman)
// 	if sSudirman.Shift != nil {
// 		t.Fatalf("expected Toko Sudirman sales shift to still be nil (unaffected), but got %v", *sSudirman.Shift)
// 	}

// 	// 6. Verify GetReceipt for Toko Sudirman still returns its unshifted transaction
// 	receiptSudirman, err := trxSvc.GetReceipt("Toko Sudirman")
// 	if err != nil {
// 		t.Fatalf("GetReceipt Toko Sudirman failed: %v", err)
// 	}
// 	foundSudirman := false
// 	for _, s := range receiptSudirman.Sales {
// 		if s.Invoice == resSudirman.Invoice {
// 			foundSudirman = true
// 			break
// 		}
// 	}
// 	if !foundSudirman {
// 		t.Fatalf("expected unshifted transaction to be in Toko Sudirman receipt")
// 	}

// 	// 7. Verify GetReceipt for Toko Utama does NOT contain the closed shift transactions
// 	receiptUtama, err := trxSvc.GetReceipt("Toko Utama")
// 	if err != nil {
// 		t.Fatalf("GetReceipt Toko Utama failed: %v", err)
// 	}
// 	for _, s := range receiptUtama.Sales {
// 		if s.Invoice == resUtama.Invoice {
// 			t.Fatalf("expected closed Toko Utama transaction NOT to be in unshifted receipt")
// 		}
// 	}
// }

func TestAnalyticsService(t *testing.T) {
	setupFeatureTestDB(t)
	database := db.Postgres.DB

	analyticsSvc := service.NewAnalyticsService(database)

	report, err := analyticsSvc.GetAnalyticsReport("this-month")
	if err != nil {
		t.Fatalf("GetAnalyticsReport failed: %v", err)
	}

	if len(report.Trends) == 0 {
		t.Fatalf("expected trends in report")
	}
	if len(report.PeakHours) == 0 {
		t.Fatalf("expected peak hours in report")
	}
}

func TestSalesDeleteWithStockRollback(t *testing.T) {
	setupFeatureTestDB(t)
	database := db.Postgres.DB

	accSvc := service.NewAccountingService(database)
	trxSvc := service.NewTransactionService(database, accSvc)

	var supplier domain.Supplier
	database.FirstOrCreate(&supplier, domain.Supplier{ID: "SUPP-DEL", Name: "Supplier Delete"})

	var category domain.Category
	database.FirstOrCreate(&category, domain.Category{Name: "Category Delete"})

	var brand domain.Brand
	database.FirstOrCreate(&brand, domain.Brand{Name: "Brand Delete"})

	var user domain.User
	database.FirstOrCreate(&user, domain.User{ID: "USER-DEL-1", Name: "Admin Del", Username: "admin_del", Role: "admin", Location: "Toko Utama"})

	initialStock := float64(50)
	prod := domain.Product{
		Kode:       fmt.Sprintf("PROD-DEL-%d", time.Now().UnixNano()),
		Nama:       "Produk Rollback Stock Test",
		Saldo:      initialStock,
		HBeli:      10000,
		HJual:      25000,
		SupplierID: supplier.ID,
		CategoryID: category.ID,
		BrandID:    brand.ID,
	}
	if err := database.Create(&prod).Error; err != nil { t.Fatalf("failed to create prod: %v", err) }
	defer func() {
		database.Unscoped().Where("product_id = ?", prod.ID).Delete(&domain.Sales{})
		database.Unscoped().Delete(&prod)
	}()

	// 1. Create Sales of 5 items -> stock should become 45
	res, err := trxSvc.CreateSales(domain.CreateTransactionRequest{
		Location:      "Toko Utama",
		UserID:        user.ID,
		MemberName:    "Pelanggan Rollback",
		PaymentMethod: "Tunai",
		AmountPaid:    125000,
		Items: []domain.SalesItemRequest{
			{ProductSearch: prod.Kode, Qty: 5, Price: 25000},
		},
	})
	if err != nil {
		t.Fatalf("CreateSales failed: %v", err)
	}

	var prodAfterSale domain.Product
	database.First(&prodAfterSale, prod.ID)
	if prodAfterSale.Saldo != 45 {
		t.Fatalf("expected stock 45 after sale of 5, got %f", prodAfterSale.Saldo)
	}

	// 2. Delete the sales transaction -> stock should be automatically restored to 50
	err = trxSvc.DeleteSales(res.Invoice)
	if err != nil {
		t.Fatalf("DeleteSales failed: %v", err)
	}

	var prodAfterRollback domain.Product
	database.First(&prodAfterRollback, prod.ID)
	if prodAfterRollback.Saldo != initialStock {
		t.Fatalf("expected stock restored to %f, but got %f", initialStock, prodAfterRollback.Saldo)
	}

	// 3. Verify sales records are deleted
	var count int64
	database.Model(&domain.Sales{}).Where("invoice = ?", res.Invoice).Count(&count)
	if count != 0 {
		t.Fatalf("expected 0 sales rows after delete, found %d", count)
	}
}

// func TestPPNFormulaAndPiutangDagang(t *testing.T) {
// 	setupFeatureTestDB(t)
// 	database := db.Postgres.DB

// 	accSvc := service.NewAccountingService(database)
// 	trxSvc := service.NewTransactionService(database, accSvc)

// 	var supplier domain.Supplier
// 	database.FirstOrCreate(&supplier, domain.Supplier{ID: "SUPP-PPN", Name: "Supplier PPN"})

// 	var category domain.Category
// 	database.FirstOrCreate(&category, domain.Category{Name: "Category PPN"})

// 	var brand domain.Brand
// 	database.FirstOrCreate(&brand, domain.Brand{Name: "Brand PPN"})

// 	var user domain.User
// 	database.FirstOrCreate(&user, domain.User{ID: "USER-PPN-1", Name: "Kasir PPN", Username: "kasir_ppn", Role: "kasir", Location: "Toko Utama"})

// 	prod := domain.Product{
// 		Kode:       fmt.Sprintf("PROD-PPN-%d", time.Now().UnixNano()%100000),
// 		Nama:       "Produk PPN 100K",
// 		Saldo:      10,
// 		HBeli:      50000,
// 		HJual:      100000,
// 		SupplierID: supplier.ID,
// 		CategoryID: category.ID,
// 		BrandID:    brand.ID,
// 	}
// 	if err := database.Create(&prod).Error; err != nil { t.Fatalf("failed to create prod: %v", err) }
// 	defer database.Unscoped().Delete(&prod)

// 	// 1. Create Sales of Rp 100.000 with DP Rp 40.000
// 	res, err := trxSvc.CreateSales(domain.CreateTransactionRequest{
// 		Location:      "Toko Utama",
// 		UserID:        user.ID,
// 		MemberName:    "Pelanggan PPN",
// 		PaymentMethod: "Tunai",
// 		AmountPaid:    40000,
// 		IsDp:          true,
// 		Customer: domain.CustomerRequest{
// 			Name:        "Customer Piutang",
// 			PhoneNumber: "081299999",
// 		},
// 		Items: []domain.SalesItemRequest{
// 			{ProductSearch: prod.Kode, Qty: 1, Price: 100000},
// 		},
// 	})
// 	if err != nil {
// 		t.Fatalf("CreateSales failed: %v", err)
// 	}
// 	defer database.Unscoped().Where("invoice = ?", res.Invoice).Delete(&domain.Sales{})
// 	defer database.Unscoped().Where("sales_invoice = ?", res.Invoice).Delete(&domain.PiutangDagang{})

// 	// 2. Verify Piutang Dagang was automatically recorded
// 	var p domain.PiutangDagang
// 	if err := database.Where("sales_invoice = ?", res.Invoice).First(&p).Error; err != nil {
// 		t.Fatalf("expected PiutangDagang auto created for DP sale: %v", err)
// 	}
// 	if p.SaldoAkhir != 60000 {
// 		t.Fatalf("expected sisa piutang 60000, got %f", p.SaldoAkhir)
// 	}
// 	if p.Status != "Belum Lunas" {
// 		t.Fatalf("expected status Belum Lunas, got %s", p.Status)
// 	}

// 	// 3. Pelunasan Sales
// 	_, err = trxSvc.PelunasanSales(res.Invoice, domain.PelunasanRequest{
// 		PaymentMethod: "Transfer BCA",
// 		AmountPaid:    100000,
// 		IsDp:          false,
// 		Status:        "Lunas",
// 	})
// 	if err != nil {
// 		t.Fatalf("PelunasanSales failed: %v", err)
// 	}

// 	// 4. Verify Piutang settled
// 	var pSettled domain.PiutangDagang
// 	database.Where("sales_invoice = ?", res.Invoice).First(&pSettled)
// 	if pSettled.SaldoAkhir != 0 || pSettled.Status != "Lunas" {
// 		t.Fatalf("expected piutang saldo akhir 0 and status Lunas, got %f (%s)", pSettled.SaldoAkhir, pSettled.Status)
// 	}
// }

func TestSalesCrucialFilters(t *testing.T) {
	setupFeatureTestDB(t)
	database := db.Postgres.DB

	accSvc := service.NewAccountingService(database)
	trxSvc := service.NewTransactionService(database, accSvc)

	var supplier domain.Supplier
	database.FirstOrCreate(&supplier, domain.Supplier{ID: "SUPP-FLT", Name: "Supplier Filter"})

	var category domain.Category
	database.FirstOrCreate(&category, domain.Category{Name: "Kategori Filter"})

	var brand domain.Brand
	database.FirstOrCreate(&brand, domain.Brand{Name: "Brand Filter"})

	kasirID := fmt.Sprintf("USER-KASIR-%d", time.Now().UnixNano()%100000)
	kasirName := fmt.Sprintf("Kasir Handal %d", time.Now().UnixNano()%10000)
	user := domain.User{
		ID:       kasirID,
		Name:     kasirName,
		Username: fmt.Sprintf("kasirflt_%d", time.Now().UnixNano()%100000),
		Role:     "kasir",
		Location: "Toko Utama",
	}
	database.Create(&user)
	defer database.Unscoped().Delete(&user)

	prod := domain.Product{
		Kode:       fmt.Sprintf("PROD-FLT-%d", time.Now().UnixNano()),
		Nama:       "TV LED Smart 43 Inch Filter Test",
		Saldo:      50,
		HBeli:      2000000,
		HJual:      3500000,
		SupplierID: supplier.ID,
		CategoryID: category.ID,
		BrandID:    brand.ID,
	}
	if err := database.Create(&prod).Error; err != nil {
		t.Fatalf("failed to create test prod: %v", err)
	}
	defer database.Unscoped().Delete(&prod)

	// Buat transaksi 1: Shift 1, Lunas, Tunai, Member "Budi Santoso"
	trx1, err := trxSvc.CreateSales(domain.CreateTransactionRequest{
		Location:      "Toko Utama",
		UserID:        user.ID,
		MemberName:    "Budi Santoso",
		PaymentMethod: "Tunai",
		AmountPaid:    3500000,
		Items: []domain.SalesItemRequest{
			{ProductSearch: prod.Kode, Qty: 1, Price: 3500000},
		},
	})
	if err != nil {
		t.Fatalf("CreateSales 1 failed: %v", err)
	}
	defer func() {
		database.Unscoped().Where("sales_id IN (SELECT id FROM sales WHERE invoice = ?)", trx1.Invoice).Delete(&domain.PaymentMethod{})
		database.Unscoped().Where("invoice = ?", trx1.Invoice).Delete(&domain.Sales{})
		database.Unscoped().Where("entry_number = ?", "JV-SALES-"+trx1.Invoice).Delete(&domain.JournalEntry{})
	}()

	// Update Shift menjadi 1 untuk trx1
	shift1 := uint(1)
	database.Model(&domain.Sales{}).Where("invoice = ?", trx1.Invoice).Update("shift", shift1)

	// 1. Test Filter Invoice Spesifik
	fInv := domain.SalesFilter{Invoice: trx1.Invoice, Page: 1, Limit: 10}
	resInv, totalInv, err := trxSvc.GetPaginatedSales(fInv)
	if err != nil {
		t.Fatalf("GetPaginatedSales with invoice filter failed: %v", err)
	}
	if totalInv != 1 || len(resInv) != 1 || resInv[0].Invoice != trx1.Invoice {
		t.Fatalf("expected 1 result for invoice filter, got %d", totalInv)
	}

	// 2. Test Global Search (Mencocokkan nama barang)
	fSearchProd := domain.SalesFilter{Search: "LED Smart 43", Page: 1, Limit: 10}
	resSearchProd, totalSearchProd, err := trxSvc.GetPaginatedSales(fSearchProd)
	if err != nil {
		t.Fatalf("GetPaginatedSales with search prod failed: %v", err)
	}
	if totalSearchProd < 1 || len(resSearchProd) < 1 {
		t.Fatalf("expected search to find transaction by product name, got %d", totalSearchProd)
	}

	// 3. Test Global Search (Mencocokkan nama pelanggan)
	fSearchMember := domain.SalesFilter{Search: "Budi Santoso", Page: 1, Limit: 10}
	resSearchMember, totalSearchMember, err := trxSvc.GetPaginatedSales(fSearchMember)
	if err != nil {
		t.Fatalf("GetPaginatedSales with search member failed: %v", err)
	}
	if totalSearchMember < 1 || len(resSearchMember) < 1 {
		t.Fatalf("expected search to find transaction by member name, got %d", totalSearchMember)
	}

	// 4. Test Filter Shift Kasir (Shift 1 harus match, Shift 2 tidak)
	fShift1 := domain.SalesFilter{Invoice: trx1.Invoice, Shift: 1}
	_, totalShift1, err := trxSvc.GetPaginatedSales(fShift1)
	if err != nil || totalShift1 != 1 {
		t.Fatalf("expected total 1 for shift 1, got %d (err: %v)", totalShift1, err)
	}

	fShift2 := domain.SalesFilter{Invoice: trx1.Invoice, Shift: 2}
	_, totalShift2, err := trxSvc.GetPaginatedSales(fShift2)
	if err != nil || totalShift2 != 0 {
		t.Fatalf("expected total 0 for shift 2, got %d (err: %v)", totalShift2, err)
	}

	// 5. Test Filter Status Pembayaran
	fStatusLunas := domain.SalesFilter{Invoice: trx1.Invoice, Status: "Lunas"}
	_, totalLunas, err := trxSvc.GetPaginatedSales(fStatusLunas)
	if err != nil || totalLunas != 1 {
		t.Fatalf("expected total 1 for status Lunas, got %d (err: %v)", totalLunas, err)
	}

	fStatusBon := domain.SalesFilter{Invoice: trx1.Invoice, Status: "Belum Lunas"}
	_, totalBon, err := trxSvc.GetPaginatedSales(fStatusBon)
	if err != nil || totalBon != 0 {
		t.Fatalf("expected total 0 for status Belum Lunas, got %d (err: %v)", totalBon, err)
	}

	// 6. Test Filter Metode Pembayaran
	fMethod := domain.SalesFilter{Invoice: trx1.Invoice, Method: "Tunai"}
	_, totalMethod, err := trxSvc.GetPaginatedSales(fMethod)
	if err != nil || totalMethod != 1 {
		t.Fatalf("expected total 1 for method Tunai, got %d (err: %v)", totalMethod, err)
	}
}
