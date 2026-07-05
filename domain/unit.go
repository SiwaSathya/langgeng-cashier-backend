package domain

// domain/unit.go
type Unit struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"unique;not null"` // Contoh: "PCS", "SET", "UNIT"
}
