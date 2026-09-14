package domain

import (
	"gorm.io/gorm"
)

type PaymentMethod struct {
	gorm.Model
	SalesID uint   `json:"sales_id"`
	Type    string `json:"type"`
	Amount  string `json:"amount"`
	Status  string `json:"status"`
	Sales   Sales  `gorm:"foreignKey:SalesID"`
}

type PaymentMethodRequest struct {
	SalesID uint   `json:"sales_id"`
	Type    string `json:"type"`
	Amount  string `json:"amount"`
	Status  string `json:"status"`
}
