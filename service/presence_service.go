package service

import (
	"strings"
	"sync"
	"time"
)

// OnlineSession merepresentasikan satu sesi aktif pengguna di browser
type OnlineSession struct {
	SessionID   string    `json:"session_id"`
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	Name        string    `json:"name"`
	Role        string    `json:"role"`
	Location    string    `json:"location"`
	CurrentPage string    `json:"current_page"`
	IPAddress   string    `json:"ip_address"`
	UserAgent   string    `json:"user_agent"`
	Device      string    `json:"device"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
}

// PresenceSummary merepresentasikan statistik ringkas pengguna online
type PresenceSummary struct {
	TotalOnline     int            `json:"total_online"`
	RoleCount       map[string]int `json:"role_count"`
	LocationCount   map[string]int `json:"location_count"`
	ActiveLocations int            `json:"active_locations"`
}

type PresenceService struct {
	mu         sync.RWMutex
	sessions   map[string]*OnlineSession
	sseService *SSEService
	stopChan   chan struct{}
}

func NewPresenceService(sseService *SSEService) *PresenceService {
	ps := &PresenceService{
		sessions:   make(map[string]*OnlineSession),
		sseService: sseService,
		stopChan:   make(chan struct{}),
	}

	// Background ticker untuk membersihkan sesi yang timeout (> 45 detik tanpa ping)
	go ps.startCleanupTicker(10*time.Second, 45*time.Second)

	return ps
}

func (s *PresenceService) startCleanupTicker(interval time.Duration, ttl time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.cleanupExpired(ttl)
		}
	}
}

func (s *PresenceService) cleanupExpired(ttl time.Duration) {
	s.mu.Lock()
	now := time.Now()
	removed := false

	for id, sess := range s.sessions {
		if now.Sub(sess.LastSeen) > ttl {
			delete(s.sessions, id)
			removed = true
		}
	}
	s.mu.Unlock()

	if removed && s.sseService != nil {
		s.broadcastPresenceUpdate()
	}
}

// PingSession memperbarui atau mendaftarkan sesi pengguna secara in-memory (sangat cepat & hemat resource)
func (s *PresenceService) PingSession(sess OnlineSession) *OnlineSession {
	s.mu.Lock()
	existing, ok := s.sessions[sess.SessionID]
	now := time.Now()

	if ok {
		existing.CurrentPage = sess.CurrentPage
		existing.LastSeen = now
		if sess.IPAddress != "" {
			existing.IPAddress = sess.IPAddress
		}
		if sess.UserAgent != "" {
			existing.UserAgent = sess.UserAgent
			existing.Device = parseDevice(sess.UserAgent)
		}
	} else {
		sess.FirstSeen = now
		sess.LastSeen = now
		sess.Device = parseDevice(sess.UserAgent)
		s.sessions[sess.SessionID] = &sess
		existing = &sess
	}
	s.mu.Unlock()

	if s.sseService != nil {
		s.broadcastPresenceUpdate()
	}

	return existing
}

// RemoveSession menghapus sesi seketika saat pengguna menutup tab atau logout
func (s *PresenceService) RemoveSession(sessionID string) {
	s.mu.Lock()
	_, ok := s.sessions[sessionID]
	if ok {
		delete(s.sessions, sessionID)
	}
	s.mu.Unlock()

	if ok && s.sseService != nil {
		s.broadcastPresenceUpdate()
	}
}

// GetOnlineUsers mengembalikan daftar seluruh sesi aktif yang belum timeout
func (s *PresenceService) GetOnlineUsers() []OnlineSession {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]OnlineSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		list = append(list, *sess)
	}
	return list
}

// GetSummary menghitung ringkasan statistik online per role dan per lokasi
func (s *PresenceService) GetSummary() PresenceSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	roleCount := make(map[string]int)
	locationCount := make(map[string]int)

	for _, sess := range s.sessions {
		r := strings.ToLower(strings.TrimSpace(sess.Role))
		if r == "" {
			r = "kasir"
		}
		roleCount[r]++

		loc := strings.TrimSpace(sess.Location)
		if loc == "" {
			loc = "Pusat"
		}
		locationCount[loc]++
	}

	return PresenceSummary{
		TotalOnline:     len(s.sessions),
		RoleCount:       roleCount,
		LocationCount:   locationCount,
		ActiveLocations: len(locationCount),
	}
}

// broadcastPresenceUpdate memancarkan snapshot user online ke channel SSE "presence:online"
func (s *PresenceService) broadcastPresenceUpdate() {
	if s.sseService == nil {
		return
	}
	data := map[string]interface{}{
		"summary": s.GetSummary(),
		"users":   s.GetOnlineUsers(),
		"time":    time.Now().Format(time.RFC3339),
	}
	s.sseService.Broadcast("presence:online", "presence_changed", data)
}

// parseDevice helper untuk membaca platform & browser dari User-Agent secara ringkas
func parseDevice(ua string) string {
	if ua == "" {
		return "Web Browser"
	}
	uaLower := strings.ToLower(ua)

	// Deteksi OS
	os := "Unknown OS"
	if strings.Contains(uaLower, "windows") {
		os = "Windows"
	} else if strings.Contains(uaLower, "macintosh") || strings.Contains(uaLower, "mac os") {
		os = "macOS"
	} else if strings.Contains(uaLower, "iphone") || strings.Contains(uaLower, "ipad") {
		os = "iOS"
	} else if strings.Contains(uaLower, "android") {
		os = "Android"
	} else if strings.Contains(uaLower, "linux") {
		os = "Linux"
	}

	// Deteksi Browser
	browser := "Browser"
	if strings.Contains(uaLower, "edg/") {
		browser = "Edge"
	} else if strings.Contains(uaLower, "chrome/") && !strings.Contains(uaLower, "chromium") {
		browser = "Chrome"
	} else if strings.Contains(uaLower, "firefox/") {
		browser = "Firefox"
	} else if strings.Contains(uaLower, "safari/") && !strings.Contains(uaLower, "chrome") {
		browser = "Safari"
	}

	return browser + " (" + os + ")"
}
