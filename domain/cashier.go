package domain

import "gorm.io/gorm"

type Cashier struct {
	gorm.Model
	Name string `gorm:"unique;not null"`
}
