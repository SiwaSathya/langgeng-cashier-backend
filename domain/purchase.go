package domain

import (
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
)

// FlexibleID mendukung unmarshal JSON dari string, number, maupun null.
type FlexibleID string

func (fi *FlexibleID) UnmarshalJSON(b []byte) error {
	raw := strings.TrimSpace(string(b))
	if raw == "null" || raw == `""` || raw == "" {
		*fi = ""
		return nil
	}
	if strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`) {
		var s string
		if err := json.Unmarshal(b, &s); err == nil {
			*fi = FlexibleID(strings.TrimSpace(s))
			return nil
		}
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err == nil {
		*fi = FlexibleID(n.String())
		return nil
	}
	*fi = FlexibleID(strings.Trim(raw, `"`))
	return nil
}

func (fi FlexibleID) String() string {
	return string(fi)
}

type Purchase struct {
	gorm.Model
	Nota       string    `gorm:"index" json:"nota"`
	Tanggal    time.Time `gorm:"index" json:"tanggal"`
	ProductID  uint      `json:"product_id"`
	Product    Product   `gorm:"foreignKey:ProductID" json:"product"`
	SupplierID *string   `json:"supplier_id"`
	Supplier   Supplier  `gorm:"foreignKey:SupplierID" json:"supplier"`
	Qty        float64   `json:"qty"`
	HBeli      float64   `json:"h_beli"`
	Total      float64   `json:"total"`
	UserID     *string   `gorm:"size:100;index" json:"user_id"`
	User       User      `gorm:"foreignKey:UserID" json:"user"`
	Location   string    `gorm:"size:100;index" json:"location"`                // Toko Utama, Toko Sudirman, Toko Paye
	DocType    string    `gorm:"size:50;default:'Nota Faktur'" json:"doc_type"` // Nota Faktur, Surat Jalan
}

type PurchaseRequest struct {
	Nota       string     `gorm:"index" json:"nota"`
	Tanggal    time.Time  `gorm:"index" json:"tanggal"`
	ProductID  uint       `json:"product_id"`
	SupplierID FlexibleID `json:"supplier_id"`
	Qty        float64    `json:"qty"`
	HBeli      float64    `json:"h_beli"`
	Total      float64    `json:"total"`
	UserID     FlexibleID `gorm:"size:100;index" json:"user_id"`
	Location   string     `gorm:"size:100;index" json:"location"`
	DocType    string     `json:"doc_type"`
}

type PurchaseItemRequest struct {
	ProductID uint    `json:"product_id"`
	Qty       float64 `json:"qty"`
	HBeli     float64 `json:"h_beli"`
}

type CreatePurchaseBatchRequest struct {
	Nota       string                `json:"nota"`
	Tanggal    string                `json:"tanggal"`
	SupplierID FlexibleID            `json:"supplier_id"`
	Location   string                `json:"location"`
	UserID     FlexibleID            `json:"user_id"`
	DocType    string                `json:"doc_type"` // "Nota Faktur" (default) atau "Surat Jalan"
	Items      []PurchaseItemRequest `json:"items"`
	ProductID  uint                  `json:"product_id,omitempty"`
	Qty        float64               `json:"qty,omitempty"`
	HBeli      float64               `json:"h_beli,omitempty"`
}

type PurchaseTransactionGroup struct {
	GroupKey    string     `json:"group_key"` // Nomor Nota atau Nama Supplier
	Nota        string     `json:"nota,omitempty"`
	SupplierID  string     `json:"supplier_id,omitempty"`
	Supplier    Supplier   `json:"supplier,omitempty"`
	Tanggal     time.Time  `json:"tanggal"`
	Location    string     `json:"location"`
	UserID      string     `json:"user_id"`
	User        User       `json:"user"`
	DocType     string     `json:"doc_type,omitempty"`
	TotalQty    float64    `json:"total_qty"`
	TotalAmount float64    `json:"total_amount"`
	DPP         float64    `json:"dpp"`
	PPN         float64    `json:"ppn"`
	TotalItems  int        `json:"total_items"`
	Items       []Purchase `json:"items"`
}
