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
	UserID           *string `gorm:"type:varchar(100);nullable" json:"user_id"`
	UserName         string  `gorm:"type:varchar(100);nullable" json:"user_name"`
	UserRole         string  `gorm:"type:varchar(50);nullable" json:"user_role"`
	Sales            Sales   `gorm:"foreignKey:SalesID"`
	Product          Product `gorm:"foreignKey:ProductID"`
	User             *User   `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
