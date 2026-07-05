package domain

import (
	"time"

	"gorm.io/gorm"
)

type SalesRecap struct {
	gorm.Model
	Tanggal       time.Time
	Jumlah        float64 `gorm:"default:0"`
	Nilai         float64 `gorm:"default:0"`
	SBGN          float64 `gorm:"default:0"`
	PaymentTypeID uint
	PaymentType   PaymentType `gorm:"foreignKey:PaymentTypeID"`
	CashierID     uint
	Cashier       Cashier `gorm:"foreignKey:CashierID"`
}
