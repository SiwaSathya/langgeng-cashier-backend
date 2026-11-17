package domain

type Supplier struct {
	ID   string `gorm:"primaryKey"`
	Name string
}
type SupplierRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
