package domain

import (
	"time"

	"gorm.io/gorm"
)

// ProductStock mencatat kuantitas stok produk spesifik per cabang/lokasi fisik
type ProductStock struct {
	gorm.Model
	ProductID uint    `gorm:"index:idx_prod_location,unique;not null" json:"product_id"`
	Product   Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Location  string  `gorm:"size:100;index:idx_prod_location,unique;not null" json:"location"`
	Saldo     float64 `gorm:"default:0" json:"saldo"`
}

// StockMovement mencatat kartu stok / audit trail setiap pergerakan barang per cabang
type StockMovement struct {
	gorm.Model
	ProductID    uint    `gorm:"index;not null" json:"product_id"`
	Product      Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Location     string  `gorm:"size:100;index;not null" json:"location"`
	MovementType string  `gorm:"size:50;index;not null" json:"movement_type"` // SALE, PURCHASE, RETUR, OPNAME_ADJUST, TRANSFER_IN, TRANSFER_OUT
	ReferenceNo  string  `gorm:"size:100;index" json:"reference_no"`
	QtyIn        float64 `gorm:"default:0" json:"qty_in"`
	QtyOut       float64 `gorm:"default:0" json:"qty_out"`
	SaldoBefore  float64 `json:"saldo_before"`
	SaldoAfter   float64 `json:"saldo_after"`
	Notes        string  `gorm:"size:255" json:"notes"`
	UserID       string  `gorm:"size:100" json:"user_id"`
}

// StockOpname mencatat sesi dokumen Stock Opname per cabang
type StockOpname struct {
	gorm.Model
	OpnameNo       string            `gorm:"size:100;uniqueIndex;not null" json:"opname_no"` // Contoh: OPN-SDR-20261109-001
	Location       string            `gorm:"size:100;index;not null" json:"location"`
	Status         string            `gorm:"size:50;index;default:'Draft'" json:"status"` // Draft, Waiting Approval, Completed, Cancelled
	CategoryID     *uint             `json:"category_id,omitempty"`
	Category       *Category         `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	Notes          string            `gorm:"size:255" json:"notes"`
	UserID         string            `gorm:"size:100;not null" json:"user_id"`      // Petugas penghitung
	ApproverID     *string           `gorm:"size:100" json:"approver_id,omitempty"` // Supervisor / Admin yang menyetujui
	TotalItems     int               `json:"total_items"`
	TotalMatched   int               `json:"total_matched"` // Selisih 0
	TotalDeficit   int               `json:"total_deficit"` // Selisih negatif (hilang/rusak)
	TotalSurplus   int               `json:"total_surplus"` // Selisih positif (lebih)
	TotalDiffQty   float64           `json:"total_diff_qty"`
	TotalDiffValue float64           `json:"total_diff_value"` // Total nominal (Rp) deviasi berdasarkan HBeli
	CompletedAt    *time.Time        `json:"completed_at,omitempty"`
	Items          []StockOpnameItem `gorm:"foreignKey:StockOpnameID" json:"items,omitempty"`

	// Metadata ringkasan sinkronisasi stok global saat ApproveSession (non-DB)
	GlobalStockUpdatedCount  int `gorm:"-" json:"global_stock_updated_count,omitempty"`
	GlobalStockRaisedCount   int `gorm:"-" json:"global_stock_raised_count,omitempty"`
	GlobalStockFullSyncCount int `gorm:"-" json:"global_stock_full_sync_count,omitempty"`
}

// StockOpnameItem mencatat baris rincian hitungan fisik per barang dalam sesi opname
type StockOpnameItem struct {
	gorm.Model
	StockOpnameID    uint    `gorm:"index;not null" json:"stock_opname_id"`
	ProductID        uint    `gorm:"index;not null" json:"product_id"`
	Product          Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	SystemQty        float64 `json:"system_qty"`             // Saldo sistem saat sesi dibuka
	PhysicalQty      float64 `json:"physical_qty"`           // Saldo hasil hitung fisik di rak
	DiscrepancyQty   float64 `json:"discrepancy_qty"`        // PhysicalQty - SystemQty
	HBeli            float64 `json:"h_beli"`                 // Harga beli satuan master produk
	DiscrepancyValue float64 `json:"discrepancy_value"`      // DiscrepancyQty * HBeli
	Reason           string  `gorm:"size:100" json:"reason"` // Barang Rusak / Pecah, Hilang, Salah Hitung Nota, Cocok, dll.
	Notes            string  `gorm:"size:255" json:"notes"`
	PhotoURL         string  `gorm:"size:500" json:"photo_url"` // URL foto bukti di MinIO / storage
}

// Request & DTO structs
type CreateOpnameSessionRequest struct {
	Location   string `json:"location"`
	CategoryID *uint  `json:"category_id,omitempty"`
	Notes      string `json:"notes"`
}

type UpdateOpnameItemRequest struct {
	PhysicalQty float64 `json:"physical_qty"`
	Reason      string  `json:"reason"`
	Notes       string  `json:"notes"`
	PhotoURL    string  `json:"photo_url"`
}

type SubmitOpnameRequest struct {
	Notes string `json:"notes"`
}

type ApproveOpnameRequest struct {
	Notes string `json:"notes"`
}

// LocationOpnameStatus merangkum status kelengkapan SO per titik lokasi resmi
type LocationOpnameStatus struct {
	Location          string     `json:"location"`
	HasCompletedSO    bool       `json:"has_completed_so"`
	CompletedSessions int64      `json:"completed_sessions"`
	LastCompletedAt   *time.Time `json:"last_completed_at,omitempty"`
}

// GlobalStockSyncResult merangkum preview maupun hasil eksekusi sinkronisasi stok global dari hasil SO
type GlobalStockSyncResult struct {
	TotalProductsWithSO int                    `json:"total_products_with_so"`
	TotalChanged        int                    `json:"total_changed"`
	TotalRaised         int                    `json:"total_raised"`
	TotalLowered        int                    `json:"total_lowered"`
	TotalUnchanged      int                    `json:"total_unchanged"`
	LocationStatuses    []LocationOpnameStatus `json:"location_statuses"`
	All5LocationsDone   bool                   `json:"all_5_locations_done"`
}
