package service

import (
	"backend-cashier/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SupplierService struct {
	DB *gorm.DB
}

func NewSupplierService(db *gorm.DB) *SupplierService {
	return &SupplierService{
		DB: db,
	}
}

// CREATE
func (s *SupplierService) CreateSupplier(
	supplier *domain.Supplier,
) error {

	if supplier.ID == "" {
		supplier.ID = uuid.New().String()
	}

	return s.DB.Create(supplier).Error
}

// GET ALL
func (s *SupplierService) GetAllSupplier() (
	[]domain.Supplier,
	error,
) {

	var suppliers []domain.Supplier

	err := s.DB.Find(&suppliers).Error

	return suppliers, err
}

// GET BY ID
func (s *SupplierService) GetSupplierByID(
	id string,
) (
	*domain.Supplier,
	error,
) {

	var supplier domain.Supplier

	err := s.DB.
		Where("id = ?", id).
		First(&supplier).
		Error

	if err != nil {
		return nil, err
	}

	return &supplier, nil
}

// UPDATE
func (s *SupplierService) UpdateSupplier(
	supplier *domain.Supplier,
) error {

	return s.DB.
		Model(&domain.Supplier{}).
		Where("id = ?", supplier.ID).
		Updates(map[string]interface{}{

			"name": supplier.Name,
		}).
		Error
}

// DELETE
func (s *SupplierService) DeleteSupplier(
	id string,
) error {

	return s.DB.
		Delete(
			&domain.Supplier{},
			"id = ?",
			id,
		).
		Error
}
