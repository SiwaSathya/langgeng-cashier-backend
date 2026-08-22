package domain

import (
	"time"

	"gorm.io/gorm"
)

// Sale untuk transaksi Kasir
type Sales struct {
	gorm.Model
	Invoice       string   `json:"invoice"`
	ProductID     uint     `json:"product_id"`
	Product       Product  `gorm:"foreignKey:ProductID"`
	MemberName    string   `json:"member_name"` // Dari field 'Member' di gambar
	Qty           float64  `json:"qty"`
	UserID        string   `json:"user_id"`
	HBeli         float64  `json:"h_beli"`
	HJual         float64  `json:"h_jual"`
	Discount      float64  `json:"discount"`    // Potongan harga
	TotalNetto    float64  `json:"total_netto"` // Harga setelah diskon
	PaymentMethod string   `json:"payment_method"`
	AmountPaid    float64  `json:"amount_paid"`
	Change        float64  `json:"change"`
	IsDp          bool     `json:"is_dp"`
	CustomerID    *uint    `gorm:"nullable" json:"customer_id"`
	Status        string   `json:"status"`
	Shift         *uint    `gorm:"nullable" json:"shift"`
	Customer      Customer `gorm:"foreignKey:CustomerID"`
	User          User     `gorm:"foreignKey:UserID"`
}

type SalesResponse struct {
	ID            uint            `json:"id"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	DeletedAt     *time.Time      `json:"deleted_at"`
	Invoice       string          `json:"invoice"`
	ProductID     uint            `json:"product_id"`
	Product       ProductResponse `gorm:"foreignKey:ProductID"`
	MemberName    string          `json:"member_name"` // Dari field 'Member' di gambar
	Qty           float64         `json:"qty"`
	HBeli         float64         `json:"h_beli"`
	HJual         float64         `json:"h_jual"`
	Discount      float64         `json:"discount"`    // Potongan harga
	TotalNetto    float64         `json:"total_netto"` // Harga setelah diskon
	PaymentMethod string          `json:"payment_method"`
	AmountPaid    float64         `json:"amount_paid"`
	Change        float64         `json:"change"`
}

type SalesRequest struct {
	ProductSearch string   `json:"product_search" example:"POLY 32RG9059"`
	MemberName    string   `json:"member_name" example:"Desak"`
	UserID        string   `json:"user_id" example:"123"`
	Qty           float64  `json:"qty" example:"1"`
	Price         float64  `json:"price" example:"20000"`
	Discount      float64  `json:"discount" example:"0"`
	PaymentMethod string   `json:"payment_method" example:"Bayar Tunai"`
	AmountPaid    float64  `json:"amount_paid" example:"2500000"`
	IsDp          bool     `json:"is_dp" example:"false"`
	Customer      Customer `json:"customer"`
}

// Struct untuk filter Sales
type SalesFilter struct {
	StartDate string `query:"start_date"`
	EndDate   string `query:"end_date"`
	Member    string `query:"member"`
	Method    string `query:"method"`
}

type UpdateShiftRequest struct {
	Shift uint `json:"shift"`
}

type ReceiptResponse struct {
	Category string `json:"category"`

	Sales []Sales `json:"sales"`

	Expenses []Expense `json:"expenses"`
}
