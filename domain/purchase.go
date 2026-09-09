package domain

import (
	"time"

	"gorm.io/gorm"
)

type Purchase struct {
	gorm.Model
	Nota       string    `gorm:"index" json:"nota"`
	Tanggal    time.Time `gorm:"index" json:"tanggal"`
	ProductID  uint      `json:"product_id"`
	Product    Product   `gorm:"foreignKey:ProductID" json:"product"`
	SupplierID string    `json:"supplier_id"`
	Supplier   Supplier  `gorm:"foreignKey:SupplierID" json:"supplier"`
	Qty        float64   `json:"qty"`
	HBeli      float64   `json:"h_beli"`
	Total      float64   `json:"total"`
	UserID     string    `gorm:"size:100;index" json:"user_id"`
	User       User      `gorm:"foreignKey:UserID" json:"user"`
	Location   string    `gorm:"size:100;index" json:"location"` // Toko Utama, Toko Sudirman, Toko Paye
}

type PurchaseRequest struct {
	Nota       string    `gorm:"index" json:"nota"`
	Tanggal    time.Time `gorm:"index" json:"tanggal"`
	ProductID  uint      `json:"product_id"`
	SupplierID string    `json:"supplier_id"`
	Qty        float64   `json:"qty"`
	HBeli      float64   `json:"h_beli"`
	Total      float64   `json:"total"`
	UserID     string    `gorm:"size:100;index" json:"user_id"`
	Location   string    `gorm:"size:100;index" json:"location"`
}
