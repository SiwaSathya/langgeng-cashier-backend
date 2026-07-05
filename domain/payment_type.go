package domain

import "gorm.io/gorm"

type PaymentType struct {
	gorm.Model
	Name string `gorm:"unique;not null"`
}
