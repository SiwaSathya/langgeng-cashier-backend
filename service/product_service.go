package service

import (
	"backend-cashier/domain"
	"errors"

	"gorm.io/gorm"
)

type ProductService struct {
	DB *gorm.DB
}

func NewProductService(db *gorm.DB) *ProductService {
	return &ProductService{DB: db}
}

// READ: Ambil semua produk dengan Pagination & Search
func (s *ProductService) GetAll(search string, page int, limit int) ([]domain.Product, int64, error) {
	var products []domain.Product
	var total int64

	db := s.DB.Model(&domain.Product{}).
		Preload("Category").
		Preload("Brand").
		Preload("Supplier")

	if search != "" {
		searchText := "%" + search + "%"
		db = db.Where("nama LIKE ? OR kode LIKE ?", searchText, searchText)
	}

	db.Count(&total)
	offset := (page - 1) * limit
	err := db.Offset(offset).Limit(limit).Order("id desc").Find(&products).Error
	return products, total, err
}

// CREATE: Simpan produk baru
func (s *ProductService) Create(p domain.Product) error {
	// Cek apakah kode sudah ada (karena kode harus unik)
	var count int64
	s.DB.Model(&domain.Product{}).Where("kode = ?", p.Kode).Count(&count)
	if count > 0 {
		return errors.New("kode produk sudah digunakan")
	}

	return s.DB.Create(&p).Error
}

// UPDATE: Edit data produk
func (s *ProductService) Update(id string, p domain.ProductRequestUpdate) error {
	var product domain.Product
	// Cek apakah barangnya ada

	if err := s.DB.First(&product, id).Error; err != nil {
		return errors.New("produk tidak ditemukan")
	}

	// Update data menggunakan Updates (hanya kolom yang berubah)
	return s.DB.Model(&product).Updates(p).Error
}

// DELETE: Hapus produk (Soft Delete jika ada DeletedAt di domain)
func (s *ProductService) Delete(id string) error {
	result := s.DB.Delete(&domain.Product{}, id)
	if result.RowsAffected == 0 {
		return errors.New("tidak ada produk yang dihapus (id tidak ditemukan)")
	}
	return result.Error
}

// GET BY ID: Untuk keperluan edit di frontend
func (s *ProductService) GetByID(id string) (domain.Product, error) {
	var p domain.Product
	err := s.DB.Preload("Category").Preload("Brand").Preload("Supplier").First(&p, id).Error
	return p, err
}
