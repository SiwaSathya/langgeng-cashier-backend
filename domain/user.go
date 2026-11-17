package domain

import "time"

type User struct {
	ID       string `gorm:"primaryKey" json:"id"`
	Name     string `gorm:"size:100" json:"name"`
	Username string `gorm:"size:100;unique" json:"username"`
	Password string `gorm:"size:100" json:"-"` // Cegah information leakage ke JSON response / DevTools
	Location string `gorm:"size:100" json:"location"`
	Role     string `gorm:"size:50" json:"role"` // admin, kasir, super-kasir, akuntan, superadmin
}

type UserRequest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
	Location string `json:"location"`
	Role     string `json:"role"`
}

type UserResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Location string `json:"location"`
	Role     string `json:"role"`
}

// RevokedToken mencatat token yang telah dibatalkan saat logout (Server-Side Blacklist anti-replay)
type RevokedToken struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	TokenHash string    `gorm:"size:64;uniqueIndex;not null" json:"token_hash"`
	UserID    string    `gorm:"size:64;index" json:"user_id"`
	ExpiresAt time.Time `gorm:"index;not null" json:"expires_at"`
	RevokedAt time.Time `gorm:"not null" json:"revoked_at"`
}
