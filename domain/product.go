package domain

import (
	"encoding/json"
	"fmt"
	"strconv"
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

func parseUintFlex(v interface{}) uint {
	switch val := v.(type) {
	case float64:
		return uint(val)
	case int:
		return uint(val)
	case int64:
		return uint(val)
	case string:
		n, _ := strconv.ParseUint(val, 10, 64)
		return uint(n)
	}
	return 0
}

func parseFloatFlex(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	}
	return 0
}

func parseStringFlex(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// UnmarshalJSON mendukung format snake_case, PascalCase, dan camelCase agar kompatibel 100%
func (p *ProductRequest) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	getVal := func(keys ...string) (interface{}, bool) {
		for _, k := range keys {
			if v, ok := raw[k]; ok && v != nil {
				return v, true
			}
		}
		return nil, false
	}

	if v, ok := getVal("kode", "Kode"); ok {
		p.Kode = parseStringFlex(v)
	}
	if v, ok := getVal("nama", "Nama"); ok {
		p.Nama = parseStringFlex(v)
	}
	if v, ok := getVal("satuan", "Satuan"); ok {
		p.Satuan = parseStringFlex(v)
	}
	if v, ok := getVal("saldo", "Saldo"); ok {
		p.Saldo = parseFloatFlex(v)
	}
	if v, ok := getVal("h_beli", "HBeli", "hbeli", "hBeli"); ok {
		p.HBeli = parseFloatFlex(v)
	}
	if v, ok := getVal("h_pokok", "HPokok", "hpokok", "hPokok"); ok {
		p.HPokok = parseFloatFlex(v)
	}
	if v, ok := getVal("h_jual", "HJual", "hjual", "hJual"); ok {
		p.HJual = parseFloatFlex(v)
	}
	if v, ok := getVal("supplier_id", "SupplierID", "supplierId", "SupplierId"); ok {
		p.SupplierID = parseStringFlex(v)
	}
	if v, ok := getVal("category_id", "CategoryID", "categoryId", "CategoryId"); ok {
		p.CategoryID = parseUintFlex(v)
	}
	if v, ok := getVal("brand_id", "BrandID", "brandId", "BrandId"); ok {
		p.BrandID = parseUintFlex(v)
	}

	return nil
}

// UnmarshalJSON mendukung format snake_case, PascalCase, dan camelCase agar kompatibel 100%
func (p *ProductRequestUpdate) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	getVal := func(keys ...string) (interface{}, bool) {
		for _, k := range keys {
			if v, ok := raw[k]; ok && v != nil {
				return v, true
			}
		}
		return nil, false
	}

	if v, ok := getVal("kode", "Kode"); ok {
		p.Kode = parseStringFlex(v)
	}
	if v, ok := getVal("nama", "Nama"); ok {
		p.Nama = parseStringFlex(v)
	}
	if v, ok := getVal("satuan", "Satuan"); ok {
		p.Satuan = parseStringFlex(v)
	}
	if v, ok := getVal("saldo", "Saldo"); ok {
		p.Saldo = parseFloatFlex(v)
	}
	if v, ok := getVal("h_beli", "HBeli", "hbeli", "hBeli"); ok {
		p.HBeli = parseFloatFlex(v)
	}
	if v, ok := getVal("h_pokok", "HPokok", "hpokok", "hPokok"); ok {
		p.HPokok = parseFloatFlex(v)
	}
	if v, ok := getVal("h_jual", "HJual", "hjual", "hJual"); ok {
		p.HJual = parseFloatFlex(v)
	}
	if v, ok := getVal("supplier_id", "SupplierID", "supplierId", "SupplierId"); ok {
		p.SupplierID = parseStringFlex(v)
	}
	if v, ok := getVal("category_id", "CategoryID", "categoryId", "CategoryId"); ok {
		p.CategoryID = parseUintFlex(v)
	}
	if v, ok := getVal("brand_id", "BrandID", "brandId", "BrandId"); ok {
		p.BrandID = parseUintFlex(v)
	}

	return nil
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
