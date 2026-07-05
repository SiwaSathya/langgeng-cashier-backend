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
}

type ProductResponse struct {
	ID         uint       `json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	DeletedAt  *time.Time `json:"deleted_at"`
	Kode       string     `json:"kode"`
	Nama       string     `json:"nama"`
	Satuan     string     `json:"satuan"`
	Saldo      float64    `json:"saldo"`
	HBeli      float64
	HPokok     float64
	HJual      float64
	SupplierID string
	Supplier   Supplier
	CategoryID uint
	Category   Category
	BrandID    uint
	Brand      Brand
}

type ProductRequest struct {
	ID         uint       `json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	DeletedAt  *time.Time `json:"deleted_at"`
	Kode       string     `json:"kode"`
	Nama       string     `json:"nama"`
	Satuan     string     `json:"satuan"`
	Saldo      float64    `json:"saldo"`
	HBeli      float64
	HPokok     float64
	HJual      float64
	SupplierID string
	Supplier   Supplier
	CategoryID uint
	Category   Category
	BrandID    uint
	Brand      Brand
}

type ProductRequestUpdate struct {
	ID        uint       `json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at"`
	Kode      string     `json:"kode"`
	Nama      string     `json:"nama"`
	// Satuan     string     `json:"satuan"`
	Saldo      float64 `json:"saldo"`
	HBeli      float64
	HPokok     float64
	HJual      float64
	SupplierID string
	Supplier   Supplier
	CategoryID uint
	Category   Category
	BrandID    uint
	Brand      Brand
}
