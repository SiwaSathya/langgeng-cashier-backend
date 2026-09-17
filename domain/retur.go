package domain

import (
	"time"

	"gorm.io/gorm"
)

type Retur struct {
	gorm.Model
	SalesID          *uint   `gorm:"nullable" json:"sales_id"`
	ProductID        *uint   `gorm:"nullable" json:"product_id"`
	Reason           string  `gorm:"nullable" json:"reason"`
	Status           string  `gorm:"nullable" json:"status"`
	IsRetur          bool    `gorm:"nullable" json:"is_retur"`
	IsReturToCompany bool    `gorm:"nullable" json:"is_retur_to_company"`
	Sales            Sales   `gorm:"foreignKey:SalesID"`
	Product          Product `gorm:"foreignKey:ProductID"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
