package domain

import (
	"time"

	"gorm.io/gorm"
)

// domain/purchase.go
type Purchase struct {
	gorm.Model
	Nota       string    `gorm:"index" json:"nota"`
	Tanggal    time.Time `gorm:"index" json:"tanggal"`
	ProductID  uint      `json:"product_id"`
	Product    Product   `gorm:"foreignKey:ProductID" json:"product"`
	SupplierID uint      `json:"supplier_id"`
	Supplier   Supplier  `gorm:"foreignKey:SupplierID" json:"supplier"`
	Qty        float64   `json:"qty"`
	HBeli      float64   `json:"h_beli"`
	HargaJual  float64   `gorm:"-" json:"harga_jual"`
	Total      float64   `json:"total"`
}
