package domain

import (
	"time"

	"gorm.io/gorm"
)

type Expense struct {
	gorm.Model
	Nota      string    `gorm:"index" json:"nota"`
	Tanggal   time.Time `gorm:"index" json:"tanggal"`
	Deskripsi string    `json:"deskripsi"` // Diambil dari NAMA
	Total     float64   `json:"total"`     // Diambil dari TOTAL
	Kategori  string    `json:"kategori"`  // Diambil dari GOL/KDGOL
	UserID    string    `gorm:"size:100;index" json:"user_id"`
	User      User      `gorm:"foreignKey:UserID" json:"user"`
	Location  string    `gorm:"size:100;index" json:"location"` // Toko Utama, Toko Sudirman, Toko Paye
}

type ExpenseFilter struct {
	StartDate string `query:"start_date"`
	EndDate   string `query:"end_date"`
	Type      string `query:"type"` // "purchase" atau "operational"
	Location  string `query:"location"`
}
