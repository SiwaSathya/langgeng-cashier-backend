package domain

import (
	"time"

	"gorm.io/gorm"
)

// domain/expense.go
type Expense struct {
	gorm.Model
	Nota      string    `json:"nota"`
	Tanggal   time.Time `json:"tanggal"`
	Deskripsi string    `json:"deskripsi"` // Diambil dari NAMA
	Total     float64   `json:"total"`     // Diambil dari TOTAL
	Kategori  string    `json:"kategori"`  // Diambil dari GOL/KDGOL
}

// Struct untuk filter Expenditure
type ExpenseFilter struct {
	StartDate string `query:"start_date"`
	EndDate   string `query:"end_date"`
	Type      string `query:"type"` // "purchase" atau "operational"
}
