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
		Status:   "Hadir",
		CheckIn:  "08:00",
		CheckOut: "16:00",
		Notes:    "Hadir tepat waktu",
	}

	att, err := attSvc.CreateAttendance(attReq)
	if err != nil {
		t.Fatalf("CreateAttendance failed: %v", err)
	}
	if att.ID == 0 || att.Status != "Hadir" {
		t.Fatalf("expected status Hadir, got %s", att.Status)
	}

	defer func() {
		database.Unscoped().Delete(&domain.Attendance{}, att.ID)
	}()

	// 4. Test Get Attendance Summary
	summary, err := attSvc.GetSummary(todayStr)
	if err != nil {
		t.Fatalf("GetSummary failed: %v", err)
	}
	if summary.Hadir < 1 {
		t.Fatalf("expected at least 1 Hadir in summary, got %d", summary.Hadir)
	}

	// 5. Test Get Attendance List
	list, total, err := attSvc.GetAllAttendance(domain.AttendanceFilter{Date: todayStr})
	if err != nil {
		t.Fatalf("GetAllAttendance failed: %v", err)
	}
	if total < 1 || len(list) < 1 {
		t.Fatalf("expected at least 1 attendance item, got %d", len(list))
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
	recap, err := accSvc.GetRekapitulasi(todayStr, todayStr)
	if err != nil {
		t.Fatalf("GetRekapitulasi failed: %v", err)
	}
	if !recap.IsBalanced {
		t.Fatalf("expected rekapitulasi to be balanced, difference: %f", recap.Difference)
	}

	// 5. Test General Ledger
	ledger, err := accSvc.GetGeneralLedger(todayStr, todayStr, kasAcc.ID)
	if err != nil {
		t.Fatalf("GetGeneralLedger failed: %v", err)
	}
	if len(ledger) == 0 {
		t.Fatalf("expected ledger card for KAS")
	}
}

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
