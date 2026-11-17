package domain

import (
	"time"

	"gorm.io/gorm"
)

type Product struct {
	gorm.Model
	Kode       string `gorm:"unique"`
	Nama       string
	Satuan     string
	Saldo      float64
	HBeli      float64
	HPokok     float64
	HJual      float64
	SupplierID string
	Supplier   Supplier
	CategoryID uint
	Category   Category
	BrandID    uint
	Brand      Brand
	Stocks     []ProductStock `gorm:"foreignKey:ProductID" json:"stocks,omitempty"`
}

type ProductResponse struct {
	ID         uint           `json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  *time.Time     `json:"deleted_at"`
	Kode       string         `json:"kode"`
	Nama       string         `json:"nama"`
	Satuan     string         `json:"satuan"`
	Saldo      float64        `json:"saldo"`
	HBeli      float64        `json:"h_beli,omitempty"`
	HPokok     float64        `json:"h_pokok,omitempty"`
	HJual      float64        `json:"h_jual,omitempty"`
	SupplierID string         `json:"supplier_id,omitempty"`
	Supplier   Supplier       `json:"supplier,omitempty"`
	CategoryID uint           `json:"category_id,omitempty"`
	Category   Category       `json:"category,omitempty"`
	BrandID    uint           `json:"brand_id,omitempty"`
	Brand      Brand          `json:"brand,omitempty"`
	Stocks     []ProductStock `json:"stocks,omitempty"`
}

type ProductRequest struct {
	Kode       string  `json:"kode"`
	Nama       string  `json:"nama"`
	Satuan     string  `json:"satuan"`
	Saldo      float64 `json:"saldo"`
	HBeli      float64 `json:"h_beli"`
	HPokok     float64 `json:"h_pokok"`
	HJual      float64 `json:"h_jual"`
	SupplierID string  `json:"supplier_id"`
	CategoryID uint    `json:"category_id"`
	BrandID    uint    `json:"brand_id"`
}

type ProductRequestUpdate struct {
	Kode       string  `json:"kode"`
	Nama       string  `json:"nama"`
	Satuan     string  `json:"satuan"`
	Saldo      float64 `json:"saldo"`
	HBeli      float64 `json:"h_beli"`
	HPokok     float64 `json:"h_pokok"`
	HJual      float64 `json:"h_jual"`
	SupplierID string  `json:"supplier_id"`
	CategoryID uint    `json:"category_id"`
	BrandID    uint    `json:"brand_id"`
}

// DTO untuk Laporan Nilai & Valuasi Stok per Tanggal Tertentu (Historical Cutoff)
type HistoricalStockItem struct {
	ProductID    uint    `json:"product_id"`
	Kode         string  `json:"kode"`
	Nama         string  `json:"nama"`
	Satuan       string  `json:"satuan"`
	CategoryName string  `json:"category_name"`
	HBeli        float64 `json:"h_beli"`       // Harga beli satuan master
	HJual        float64 `json:"h_jual"`       // Harga jual satuan master
	QtyStok      float64 `json:"qty_stok"`     // Sisa stok (QTY) pada tanggal cutoff
	NominalStok  float64 `json:"nominal_stok"` // QtyStok * HBeli (Nilai persediaan aset untuk Laba Rugi)
}

type HistoricalStockReport struct {
	CutoffDate   string                `json:"cutoff_date"` // YYYY-MM-DD
	Location     string                `json:"location"`    // Semua Toko / Nama Cabang
	TotalItems   int                   `json:"total_items"`
	TotalQty     float64               `json:"total_qty"`
	TotalNominal float64               `json:"total_nominal"` // Total nilai persediaan toko
	Items        []HistoricalStockItem `json:"items"`
}
