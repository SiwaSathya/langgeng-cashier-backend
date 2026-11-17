package domain

import "time"

// Konstanta Tipe Logout
const (
	LogoutTypeActive    = "ACTIVE"            // Sedang Aktif / Belum Logout
	LogoutTypeWithShift = "LOGOUT_WITH_SHIFT" // Logout & Selesaikan Shift
	LogoutTypeOnly      = "LOGOUT_ONLY"       // Logout Saja (Tanpa Selesaikan Shift)
	LogoutTypeExpired   = "EXPIRED"           // Sesi Berakhir Otomatis
)

type UserActivityLog struct {
	ID          uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID   string     `gorm:"size:100;index" json:"session_id"`
	UserID      string     `gorm:"size:50;index" json:"user_id"`
	Username    string     `gorm:"size:100;index" json:"username"`
	Name        string     `gorm:"size:100" json:"name"`
	Role        string     `gorm:"size:50;index" json:"role"`
	Location    string     `gorm:"size:100;index" json:"location"`
	LoginAt     time.Time  `gorm:"index" json:"login_at"`
	LogoutAt    *time.Time `json:"logout_at"`
	LogoutType  string     `gorm:"size:50;default:'ACTIVE';index" json:"logout_type"` // ACTIVE, LOGOUT_WITH_SHIFT, LOGOUT_ONLY, EXPIRED
	ShiftNumber *int       `json:"shift_number"`                                      // 1, 2, 3 jika menyelesaikan shift
	IPAddress   string     `gorm:"size:50" json:"ip_address"`
	UserAgent   string     `gorm:"size:255" json:"user_agent"`
	Device      string     `gorm:"size:100" json:"device"`
	DurationSec int64      `json:"duration_seconds"` // Durasi aktif dalam detik
	Notes       string     `gorm:"type:text" json:"notes"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type LogoutRequest struct {
	SessionID  string `json:"session_id"`
	LogoutType string `json:"logout_type"` // "LOGOUT_WITH_SHIFT" atau "LOGOUT_ONLY"
	Shift      *int   `json:"shift"`
	Location   string `json:"location"`
	Notes      string `json:"notes"`
}

type UserLogFilter struct {
	Page       int
	Limit      int
	UserID     string
	Username   string
	Role       string
	Location   string
	LogoutType string
	StartDate  string
	EndDate    string
	Search     string
}
