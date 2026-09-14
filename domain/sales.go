package domain

import (
	"time"

	"gorm.io/gorm"
)

// Sale untuk transaksi Kasir
type Sales struct {
	gorm.Model
	Invoice        string          `gorm:"index" json:"invoice"`
	ProductID      uint            `json:"product_id"`
	Product        Product         `gorm:"foreignKey:ProductID" json:"product"`
	MemberName     string          `json:"member_name"`
	Qty            float64         `json:"qty"`
	UserID         string          `gorm:"size:100;index" json:"user_id"`
	User           User            `gorm:"foreignKey:UserID" json:"user"`
	Location       string          `gorm:"size:100;index" json:"location"` // Toko Utama, Toko Sudirman, Toko Paye
	HBeli          float64         `json:"h_beli"`
	HJual          float64         `json:"h_jual"`
	Discount       float64         `json:"discount"`    // Potongan harga
	TotalNetto     float64         `json:"total_netto"` // Harga setelah diskon
	PaymentMethod  string          `json:"payment_method"`
	AmountPaid     float64         `json:"amount_paid"`
	Change         float64         `json:"change"`
	IsDp           bool            `json:"is_dp"`
	IsReturs       bool            `gorm:"default:false" json:"is_returs"`
	CustomerID     *uint           `gorm:"nullable" json:"customer_id"`
	Status         string          `json:"status"`
	Shift          *uint           `gorm:"nullable" json:"shift"`
	IsBon          bool            `json:"is_bon"` // <-- Tambahkan ini
	PaymentMethods []PaymentMethod `gorm:"foreignKey:SalesID" json:"payment_methods,omitempty"`
	Customer       Customer        `gorm:"foreignKey:CustomerID" json:"customer"`
}

type SalesResponse struct {
	ID             uint            `json:"id,omitempty"`
	CreatedAt      time.Time       `json:"created_at,omitempty"`
	UpdatedAt      time.Time       `json:"updated_at,omitempty"`
	DeletedAt      *time.Time      `json:"deleted_at,omitempty"`
	Invoice        string          `json:"invoice"`
	ProductID      uint            `json:"product_id,omitempty"`
	Product        ProductResponse `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	MemberName     string          `json:"member_name,omitempty"`
	UserID         string          `json:"user_id,omitempty"`
	Location       string          `json:"location,omitempty"`
	Qty            float64         `json:"qty,omitempty"`
	HBeli          float64         `json:"h_beli,omitempty"`
	HJual          float64         `json:"h_jual,omitempty"`
	Discount       float64         `json:"discount,omitempty"`
	TotalNetto     float64         `json:"total_netto,omitempty"`
	PaymentMethod  string          `json:"payment_method,omitempty"`
	AmountPaid     float64         `json:"amount_paid"`
	Change         float64         `json:"change"`
	Status         string          `json:"status,omitempty"`
	IsDp           bool            `json:"is_dp,omitempty"`
	IsBon          bool            `json:"is_bon"` // <-- Tambahkan ini
	PaymentMethods []PaymentMethod `gorm:"foreignKey:SalesID" json:"payment_methods,omitempty"`
}

type SalesItemRequest struct {
	ProductSearch string  `json:"product_search" example:"POLY 32RG9059"`
	Qty           float64 `json:"qty" example:"1"`
	Price         float64 `json:"price" example:"20000"`
	Discount      float64 `json:"discount" example:"0"`
}

type CreateTransactionRequest struct {
	Invoice        string             `json:"invoice,omitempty" example:"PJL-123456"`
	MemberName     string             `json:"member_name" example:"Desak"`
	UserID         string             `json:"user_id" example:"123"`
	Location       string             `json:"location" example:"Toko Utama"`
	PaymentMethod  string             `json:"payment_method" example:"Bayar Tunai"`
	AmountPaid     float64            `json:"amount_paid" example:"2500000"`
	IsDp           bool               `json:"is_dp" example:"false"`
	IsBon          bool               `json:"is_bon"` // <-- Tambahkan ini
	Customer       CustomerRequest    `json:"customer"`
	PaymentMethods []PaymentMethod    `gorm:"foreignKey:SalesID" json:"payment_methods,omitempty"`
	Items          []SalesItemRequest `json:"items"`
}

type SalesRequest struct {
	ProductSearch  string                 `json:"product_search" example:"POLY 32RG9059"`
	MemberName     string                 `json:"member_name" example:"Desak"`
	UserID         string                 `json:"user_id" example:"123"`
	Location       string                 `json:"location" example:"Toko Utama"`
	Qty            float64                `json:"qty" example:"1"`
	Price          float64                `json:"price" example:"20000"`
	Discount       float64                `json:"discount" example:"0"`
	PaymentMethod  string                 `json:"payment_method" example:"Bayar Tunai"`
	AmountPaid     float64                `json:"amount_paid" example:"2500000"`
	IsDp           bool                   `json:"is_dp" example:"false"`
	IsBon          bool                   `json:"is_bon"` // <-- Tambahkan ini
	Customer       CustomerRequest        `json:"customer"`
	PaymentMethods []PaymentMethodRequest `json:"payment_methods"`
}

type PelunasanRequest struct {
	PaymentMethod  string                 `json:"payment_method" example:"Bayar Tunai"`
	AmountPaid     float64                `json:"amount_paid" example:"500000"`
	IsDp           bool                   `json:"is_dp" example:"false"`
	Status         string                 `json:"status" example:"Lunas"`
	PaymentMethods []PaymentMethodRequest `json:"payment_methods"`
}

type PelunasanResponse struct {
	ID            uint     `json:"id,omitempty"`
	Invoice       string   `json:"invoice"`
	Status        string   `json:"status"`
	IsDp          bool     `json:"is_dp"`
	PaymentMethod string   `json:"payment_method"`
	AmountPaid    float64  `json:"amount_paid"`
	TotalNetto    float64  `json:"total_netto"`
	CustomerID    *uint    `json:"customer_id,omitempty"`
	Customer      Customer `json:"customer"`
	Sales         []Sales  `json:"sales"`
}

// SalesTransactionGroup mengelompokkan item penjualan berdasarkan transaksi / nota (invoice)
type SalesTransactionGroup struct {
	Invoice         string          `json:"invoice"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	MemberName      string          `json:"member_name"`
	UserID          string          `json:"user_id"`
	User            User            `json:"user"`
	Location        string          `json:"location"`
	CustomerID      *uint           `json:"customer_id"`
	Customer        Customer        `json:"customer"`
	PaymentMethod   string          `json:"payment_method"`
	AmountPaid      float64         `json:"amount_paid"`
	TotalNetto      float64         `json:"total_netto"`
	Change          float64         `json:"change"`
	RemainingAmount float64         `json:"remaining_amount"`
	IsDp            bool            `json:"is_dp"`
	Status          string          `json:"status"`
	Shift           *uint           `json:"shift"`
	TotalQty        float64         `json:"total_qty"`
	IsBon           bool            `json:"is_bon"` // <-- Tambahkan ini
	Items           []Sales         `json:"items"`
	PaymentMethods  []PaymentMethod `gorm:"foreignKey:SalesID" json:"payment_methods,omitempty"`
}

// Struct untuk filter Sales
type SalesFilter struct {
	StartDate    string `query:"start_date"`
	EndDate      string `query:"end_date"`
	Member       string `query:"member"`
	Method       string `query:"method"`
	Location     string `query:"location"`
	UserLocation string `query:"user_location"`
}

type UpdateShiftRequest struct {
	Shift    uint   `json:"shift"`
	Location string `json:"location"`
}

type ReceiptResponse struct {
	Category string    `json:"category"`
	Location string    `json:"location,omitempty"`
	Sales    []Sales   `json:"sales"`
	Expenses []Expense `json:"expenses"`
}
