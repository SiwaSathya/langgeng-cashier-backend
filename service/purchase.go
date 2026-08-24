package service

import (
	"backend-cashier/domain"
	"time"

	"gorm.io/gorm"
)

type PurchaseService struct {
	DB            *gorm.DB
	AccountingSvc *AccountingService
}

func NewPurchaseService(db *gorm.DB, accSvc *AccountingService) *PurchaseService {
	return &PurchaseService{
		DB:            db,
		AccountingSvc: accSvc,
	}
}

// CreatePurchase: Simpan pembelian dan update stok produk
func (s *PurchaseService) CreatePurchase(purchase *domain.Purchase) error {
	if purchase.Location == "" && purchase.UserID != "" {
		var u domain.User
		if err := s.DB.Where("id = ?", purchase.UserID).First(&u).Error; err == nil && u.Location != "" {
			purchase.Location = u.Location
		}
	}
	if purchase.Location == "" {
		purchase.Location = "Toko Utama"
	}
	if purchase.Tanggal.IsZero() {
		purchase.Tanggal = time.Now()
	}

	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(purchase).Error; err != nil {
			return err
		}

		// Update Saldo/Stok di tabel Product
		if purchase.ProductID > 0 && purchase.Qty > 0 {
			var product domain.Product
			if err := tx.First(&product, purchase.ProductID).Error; err == nil {
				tx.Model(&product).Update("saldo", product.Saldo+purchase.Qty)
			}
		}

		if s.AccountingSvc != nil {
			s.AccountingSvc.AutoPostPurchaseJournal(tx, purchase.Nota, purchase.Total, purchase.Tanggal)
		}

		return nil
	})
}

func (s *PurchaseService) GetAll(page, limit int, search string) ([]domain.Purchase, int64, error) {
	var purchases []domain.Purchase
	var total int64
	offset := (page - 1) * limit

	query := s.DB.Model(&domain.Purchase{}).Preload("Product").Preload("Supplier").Preload("User")

	if search != "" {
		query = query.Where("nota LIKE ?", "%"+search+"%")
	}

	query.Count(&total)
	err := query.Limit(limit).Offset(offset).Order("created_at desc").Find(&purchases).Error

	return purchases, total, err
}

func (s *PurchaseService) GetByID(id string) (domain.Purchase, error) {
	var purchase domain.Purchase
	err := s.DB.Preload("Product").Preload("Supplier").Preload("User").First(&purchase, "id = ?", id).Error
	return purchase, err
}

func (s *PurchaseService) DeletePurchase(id string) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var purchase domain.Purchase
		if err := tx.First(&purchase, "id = ?", id).Error; err != nil {
			return err
		}

		if purchase.ProductID > 0 && purchase.Qty > 0 {
			var product domain.Product
			if err := tx.First(&product, purchase.ProductID).Error; err == nil {
				tx.Model(&product).Update("saldo", product.Saldo-purchase.Qty)
			}
		}

		return tx.Delete(&purchase).Error
	})
}
