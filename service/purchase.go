package service

import (
	"backend-cashier/domain"

	// sesuaikan path model Anda
	"gorm.io/gorm"
)

type PurchaseService struct {
	DB *gorm.DB
}

func NewPurchaseService(db *gorm.DB) *PurchaseService {
	return &PurchaseService{DB: db}
}

// CreatePurchase: Simpan pembelian dan update stok produk
func (s *PurchaseService) CreatePurchase(purchase *domain.Purchase) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		// 1. Simpan data Purchase
		if err := tx.Create(purchase).Error; err != nil {
			return err
		}

		// // 2. Update Saldo/Stok di tabel Product
		// var product domain.Product
		// if err := tx.First(&product, "id = ?", purchase.ProductID).Error; err != nil {
		// 	return errors.New("produk tidak ditemukan")
		// }

		// newSaldo := product.Saldo + purchase.Total
		// if err := tx.Model(&product).Update("saldo", newSaldo).Error; err != nil {
		// 	return err
		// }

		return nil
	})
}

func (s *PurchaseService) GetAll(page, limit int, search string) ([]domain.Purchase, int64, error) {
	var purchases []domain.Purchase
	var total int64
	offset := (page - 1) * limit

	query := s.DB.Model(&domain.Purchase{}).Preload("Product").Preload("Supplier")

	if search != "" {
		// Asumsi pencarian berdasarkan Invoice atau Nama Supplier
		query = query.Where("invoice LIKE ?", "%"+search+"%")
	}

	query.Count(&total)
	err := query.Limit(limit).Offset(offset).Order("created_at desc").Find(&purchases).Error

	return purchases, total, err
}

func (s *PurchaseService) GetByID(id string) (domain.Purchase, error) {
	var purchase domain.Purchase
	err := s.DB.Preload("Product").Preload("Supplier").First(&purchase, "id = ?", id).Error
	return purchase, err
}

func (s *PurchaseService) DeletePurchase(id string) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var purchase domain.Purchase
		if err := tx.First(&purchase, "id = ?", id).Error; err != nil {
			return err
		}

		// Opsional: Kurangi stok kembali jika pembelian dibatalkan/dihapus
		var product domain.Product
		tx.First(&product, "id = ?", purchase.ProductID)
		tx.Model(&product).Update("saldo", product.Saldo-purchase.Total)

		return tx.Delete(&purchase).Error
	})
}
