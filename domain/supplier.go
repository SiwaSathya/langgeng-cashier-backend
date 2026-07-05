package domain

type Supplier struct {
	ID   string `gorm:"primaryKey"`
	Name string
}
