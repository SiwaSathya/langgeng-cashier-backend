package domain

import (
	"time"

	"gorm.io/gorm"
)

type Retur struct {
	gorm.Model
	SalesID          uint   `json:"sales_id"`
	Reason           string `json:"reason"`
	Status           string `json:"status"`
	IsRetur          bool   `json:"is_retur"`
	IsReturToCompany bool   `json:"is_retur_to_company"`
	Sales            Sales  `gorm:"foreignKey:SalesID"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
