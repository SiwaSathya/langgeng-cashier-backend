package service

import (
	"testing"
	"time"
)

func TestPresenceServicePingAndCleanup(t *testing.T) {
	sse := NewSSEService()
	ps := NewPresenceService(sse)

	// 1. Test Ping baru
	sess1 := OnlineSession{
		SessionID:   "sess_123",
		UserID:      "USER-1",
		Username:    "admin",
		Name:        "Super Admin",
		Role:        "superadmin",
		Location:    "Toko Utama",
		CurrentPage: "dashboard.html",
		IPAddress:   "127.0.0.1",
		UserAgent:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	}

	res := ps.PingSession(sess1)
	if res.Device != "Chrome (macOS)" {
		t.Errorf("Expected Chrome (macOS), got %s", res.Device)
	}

	users := ps.GetOnlineUsers()
	if len(users) != 1 {
		t.Fatalf("Expected 1 online user, got %d", len(users))
	}

	summary := ps.GetSummary()
	if summary.TotalOnline != 1 || summary.RoleCount["superadmin"] != 1 {
		t.Errorf("Unexpected summary: %+v", summary)
	}

	// 2. Test update page ping yang sama
	sess1Update := sess1
	sess1Update.CurrentPage = "product.html"
	ps.PingSession(sess1Update)

	users = ps.GetOnlineUsers()
	if len(users) != 1 || users[0].CurrentPage != "product.html" {
		t.Errorf("Expected page updated to product.html, got %v", users[0].CurrentPage)
	}

	// 3. Test Tambah Sesi ke-2
	sess2 := OnlineSession{
		SessionID:   "sess_456",
		UserID:      "USER-2",
		Username:    "kasir1",
		Name:        "Desak Kasir",
		Role:        "kasir",
		Location:    "Sudirman",
		CurrentPage: "dashboard.html",
		IPAddress:   "127.0.0.1",
		UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Firefox/121.0",
	}
	ps.PingSession(sess2)

	summary = ps.GetSummary()
	if summary.TotalOnline != 2 || summary.RoleCount["kasir"] != 1 || summary.ActiveLocations != 2 {
		t.Errorf("Expected 2 online across 2 locations, got %+v", summary)
	}

	// 4. Test RemoveSession (Leave)
	ps.RemoveSession("sess_456")
	if len(ps.GetOnlineUsers()) != 1 {
		t.Errorf("Expected 1 user after remove, got %d", len(ps.GetOnlineUsers()))
	}

	// 5. Test cleanup expired
	ps.cleanupExpired(0 * time.Millisecond) // immediate expire
	if len(ps.GetOnlineUsers()) != 0 {
		t.Errorf("Expected 0 users after immediate cleanup, got %d", len(ps.GetOnlineUsers()))
	}
}
