package service

import (
	"backend-cashier/domain"
	"strings"

	"gorm.io/gorm"
)

type MasterService struct {
	DB *gorm.DB
}

func NewMasterService(db *gorm.DB) *MasterService {
	return &MasterService{DB: db}
}

func (s *MasterService) GetAllCategories() ([]domain.Category, error) {
	var results []domain.Category
	err := s.DB.Find(&results).Error
	return results, err
}

func (s *MasterService) GetAllProducts(search string) ([]domain.Product, error) {
	var results []domain.Product

	// 1. Aktifkan Debug mode agar SQL muncul di terminal Go
	dbDebug := s.DB.Debug()

	// 2. Mulai query dengan Preload
	query := dbDebug.Model(&domain.Product{}).Preload("Category")

	if search != "" {
		// 3. Bersihkan spasi dan buat jadi lowercase
		search = strings.TrimSpace(strings.ToLower(search))
		pattern := "%" + search + "%"

		// 4. Gunakan LOWER() agar pencarian tidak peduli huruf besar/kecil (Case Insensitive)
		// Ini bekerja di MySQL, PostgreSQL, dan SQLite
		query = query.Where("LOWER(nama) LIKE ? OR LOWER(kode) LIKE ?", pattern, pattern)
	}

	// 5. Eksekusi
	err := query.Find(&results).Error

	return results, err
}

func (s *MasterService) GetProductResources() (map[string]interface{}, error) {
	var categories []domain.Category
	var brands []domain.Brand
	var suppliers []domain.Supplier

	// Menggunakan Find untuk mengambil semua data master
	if err := s.DB.Order("name asc").Find(&categories).Error; err != nil {
		return nil, err
	}
	if err := s.DB.Order("name asc").Find(&brands).Error; err != nil {
		return nil, err
	}
	if err := s.DB.Order("name asc").Find(&suppliers).Error; err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"categories": categories,
		"brands":     brands,
		"suppliers":  suppliers,
	}, nil
}
