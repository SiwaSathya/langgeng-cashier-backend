package domain

type Brand struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"unique"`
}
