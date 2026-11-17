package service_test

import (
	"backend-cashier/db"
	"backend-cashier/domain"
	"backend-cashier/service"
	"fmt"
	"testing"
	"time"
)

func TestUserLogService(t *testing.T) {
	setupFeatureTestDB(t)
	database := db.Postgres.DB

	logService := service.NewUserLogService(database)

	testSuffix := fmt.Sprintf("%d", time.Now().UnixNano()%10000000)
	sessID1 := "sess_test_" + testSuffix
	userID1 := "USER-LOG-1-" + testSuffix

	// 1. Test LogLogin
	now := time.Now()
	logEntry, err := logService.LogLogin(domain.UserActivityLog{
		SessionID: sessID1,
		UserID:    userID1,
		Username:  "kasir_test_" + testSuffix,
		Name:      "Kasir Satu",
		Role:      "kasir",
		Location:  "Toko Utama",
		IPAddress: "127.0.0.1",
		UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/120.0",
		LoginAt:   now,
	})
	if err != nil {
		t.Fatalf("LogLogin failed: %v", err)
	}
	if logEntry.LogoutType != domain.LogoutTypeActive {
		t.Errorf("Expected ACTIVE logout_type, got %s", logEntry.LogoutType)
	}

	// 2. Test LogLogout: LOGOUT_WITH_SHIFT
	shift := 1
	loggedOut, err := logService.LogLogout(domain.LogoutRequest{
		SessionID:  sessID1,
		LogoutType: domain.LogoutTypeWithShift,
		Shift:      &shift,
		Location:   "Toko Utama",
		Notes:      "Tutup shift sore",
	}, userID1)
	if err != nil {
		t.Fatalf("LogLogout failed: %v", err)
	}
	if loggedOut.LogoutType != domain.LogoutTypeWithShift {
		t.Errorf("Expected LOGOUT_WITH_SHIFT, got %s", loggedOut.LogoutType)
	}
	if loggedOut.ShiftNumber == nil || *loggedOut.ShiftNumber != 1 {
		t.Errorf("Expected Shift 1, got %v", loggedOut.ShiftNumber)
	}
	if loggedOut.LogoutAt == nil {
		t.Errorf("Expected non-nil LogoutAt")
	}

	// 3. Test Sesi Baru: LOGOUT_ONLY
	sessID2 := "sess_test2_" + testSuffix
	userID2 := "USER-LOG-2-" + testSuffix
	_, err = logService.LogLogin(domain.UserActivityLog{
		SessionID: sessID2,
		UserID:    userID2,
		Username:  "admin_test_" + testSuffix,
		Name:      "Admin Toko",
		Role:      "admin",
		Location:  "Sudirman",
		IPAddress: "127.0.0.1",
		LoginAt:   time.Now(),
	})
	if err != nil {
		t.Fatalf("LogLogin 2 failed: %v", err)
	}

	loggedOut2, err := logService.LogLogout(domain.LogoutRequest{
		SessionID:  sessID2,
		LogoutType: domain.LogoutTypeOnly,
		Location:   "Sudirman",
		Notes:      "Logout istirahat",
	}, userID2)
	if err != nil {
		t.Fatalf("LogLogout 2 failed: %v", err)
	}
	if loggedOut2.LogoutType != domain.LogoutTypeOnly {
		t.Errorf("Expected LOGOUT_ONLY, got %s", loggedOut2.LogoutType)
	}

	// 4. Test GetLogs & Summary
	logs, total, err := logService.GetLogs(domain.UserLogFilter{
		UserID: userID1,
		Page:   1,
		Limit:  10,
	})
	if err != nil || total != 1 || len(logs) != 1 {
		t.Errorf("GetLogs filter by userID failed: total=%d, len=%d, err=%v", total, len(logs), err)
	}

	summary, err := logService.GetSummary("", "")
	if err != nil {
		t.Fatalf("GetSummary failed: %v", err)
	}
	if summary["total_logout_with_shift"].(int64) < 1 || summary["total_logout_only"].(int64) < 1 {
		t.Errorf("Unexpected summary: %+v", summary)
	}
}
