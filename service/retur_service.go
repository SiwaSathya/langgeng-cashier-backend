package service

import (
	"backend-cashier/domain"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// DTO untuk menerima request dari frontend
type ReturRequest struct {
	SalesID          uint    `json:"sales_id" binding:"required"`
	Reason           string  `json:"reason" binding:"required"`
	Status           string  `json:"status"`
	IsRetur          bool    `json:"is_retur"`
	IsReturToCompany bool    `json:"is_retur_to_company"`
	QtyRetur         int     `json:"qty_retur" binding:"required,gt=0"`
	UserID           *string `json:"user_id"`
	UserName         string  `json:"user_name"`
	UserRole         string  `json:"user_role"`
}

type DirectReturRequest struct {
	ProductID        uint    `json:"product_id"`
	QtyRetur         int     `json:"qty_retur"`
	Reason           string  `json:"reason"`
	Status           string  `json:"status"`
	IsRetur          bool    `json:"is_retur"`
	IsReturToCompany bool    `json:"is_retur_to_company"`
	UserID           *string `json:"user_id"`
	UserName         string  `json:"user_name"`
	UserRole         string  `json:"user_role"`
}

type ReturService interface {
	GetAllRetur() ([]domain.Retur, error)
	CreateRetur(req ReturRequest) (*domain.Retur, error)
	BatalkanRetur(id uint) (*domain.Retur, error)
	KirimKeDistributor(id uint) (*domain.Retur, error)
	SelesaikanRetur(id uint) (*domain.Retur, error)
	CreateDirectRetur(req DirectReturRequest) (*domain.Retur, error)
}

type returService struct {
	db *gorm.DB
}

func NewReturService(db *gorm.DB) ReturService {
	return &returService{db: db}
}

func (s *returService) CreateRetur(req ReturRequest) (*domain.Retur, error) {
	var returData domain.Retur
	var salesInvoice string

	// Menggunakan DB Transaction agar aman (ACID)
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 1. Cari data Sales (Penjualan) terlebih dahulu
		var sale domain.Sales // Pastikan nama struct Sales Anda sesuai
		if err := tx.Preload("Product").First(&sale, req.SalesID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("data transaksi penjualan tidak ditemukan")
			}
			return err
		}
		salesInvoice = sale.Invoice

		// 2. Validasi Qty Retur tidak boleh melebihi Qty Penjualan awal
		if req.QtyRetur > int(sale.Qty) {
			return errors.New("jumlah qty retur melebihi jumlah pembelian asli")
		}

		// Resolusi User / Petugas Penanggung Jawab
		var resolvedUserID *string = req.UserID
		var resolvedUserName string = req.UserName
		var resolvedUserRole string = req.UserRole

		if resolvedUserName == "" && resolvedUserID != nil && *resolvedUserID != "" {
			var u domain.User
			if err := tx.Where("id = ? OR username = ?", *resolvedUserID, *resolvedUserID).First(&u).Error; err == nil {
				resolvedUserName = u.Name
				resolvedUserRole = u.Role
			}
		}

		if resolvedUserName == "" {
			if sale.UserID != "" {
				var u domain.User
				if err := tx.Where("id = ? OR username = ?", sale.UserID, sale.UserID).First(&u).Error; err == nil {
					resolvedUserName = u.Name
					resolvedUserRole = u.Role
					resolvedUserID = &u.ID
				}
			}
			if resolvedUserName == "" && sale.MemberName != "" {
				resolvedUserName = sale.MemberName
			}
		}

		// 3. Siapkan & Simpan data Retur baru
		returData = domain.Retur{
			SalesID:          &req.SalesID,
			ProductID:        &sale.ProductID,
			Reason:           req.Reason,
			Status:           req.Status,
			IsRetur:          req.IsRetur,
			IsReturToCompany: req.IsReturToCompany,
			UserID:           resolvedUserID,
			UserName:         resolvedUserName,
			UserRole:         resolvedUserRole,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}

		if err := tx.Create(&returData).Error; err != nil {
			return err
		}

		err := tx.Model(&sale).Update("is_returs", true).Error
		if err != nil {
			return errors.New("gagal memperbarui status retur pada transaksi penjualan")
		}

		if !req.IsReturToCompany {
			err := tx.Model(&domain.Product{}).
				Where("id = ?", sale.ProductID).
				Update("saldo", gorm.Expr("saldo + ?", req.QtyRetur)).Error
			if err != nil {
				return errors.New("gagal memperbarui stok produk")
			}
			userStr := ""
			if resolvedUserID != nil {
				userStr = *resolvedUserID
			}
			_ = AdjustProductStock(tx, sale.ProductID, sale.Location, float64(req.QtyRetur), "RETUR_CUSTOMER", fmt.Sprintf("Retur Pelanggan (#%s)", sale.Invoice), req.Reason, userStr)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if globalSSE != nil {
		globalSSE.Broadcast("sales:live", "retur_created", map[string]interface{}{
			"sales_invoice": salesInvoice,
			"product_id":    returData.ProductID,
			"qty":           req.QtyRetur,
			"reason":        returData.Reason,
			"status":        returData.Status,
			"user_name":     returData.UserName,
			"created_at":    returData.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return &returData, nil
}

func (s *returService) CreateDirectRetur(req DirectReturRequest) (*domain.Retur, error) {
	if req.ProductID == 0 {
		return nil, errors.New("product_id tidak boleh kosong")
	}

	if req.QtyRetur <= 0 {
		return nil, errors.New("jumlah qty retur harus lebih dari 0")
	}

	var returData domain.Retur

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 1. Cek keberadaan produk & stok saat ini
		var product domain.Product
		if err := tx.First(&product, req.ProductID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("data produk tidak ditemukan")
			}
			return err
		}

		// 2. Validasi stok toko mencukupi untuk dikirim ke pabrik
		if product.Saldo < float64(req.QtyRetur) {
			return fmt.Errorf("stok %s tidak mencukupi untuk diretur (sisa stok: %.0f)", product.Nama, product.Saldo)
		}

		// Resolusi User / Petugas Penanggung Jawab
		var resolvedUserID *string = req.UserID
		var resolvedUserName string = req.UserName
		var resolvedUserRole string = req.UserRole

		if resolvedUserName == "" && resolvedUserID != nil && *resolvedUserID != "" {
			var u domain.User
			if err := tx.Where("id = ? OR username = ?", *resolvedUserID, *resolvedUserID).First(&u).Error; err == nil {
				resolvedUserName = u.Name
				resolvedUserRole = u.Role
			}
		}

		// 3. Simpan data retur (Status langsung ke distributor)
		returData = domain.Retur{
			SalesID:          nil,
			ProductID:        &req.ProductID,
			Reason:           req.Reason,
			Status:           "Retur ke Distributor",
			IsRetur:          true,
			IsReturToCompany: true,
			UserID:           resolvedUserID,
			UserName:         resolvedUserName,
			UserRole:         resolvedUserRole,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}

		if err := tx.Create(&returData).Error; err != nil {
			return errors.New("gagal menyimpan data retur")
		}

		// 4. POTONG STOK TOKO karena barang fisik keluar dikirim ke pabrik
		err := tx.Model(&domain.Product{}).
			Where("id = ?", req.ProductID).
			Update("saldo", gorm.Expr("saldo - ?", req.QtyRetur)).Error
		if err != nil {
			return errors.New("gagal memotong stok produk")
		}
		userStr := ""
		if resolvedUserID != nil {
			userStr = *resolvedUserID
		}
		_ = AdjustProductStock(tx, req.ProductID, "Toko Utama", -float64(req.QtyRetur), "RETUR_DISTRIBUTOR", fmt.Sprintf("Retur ke Pabrik (#%d)", returData.ID), req.Reason, userStr)

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &returData, nil
}

func (s *returService) GetAllRetur() ([]domain.Retur, error) {
	var results []domain.Retur

	err := s.db.
		Preload("Product").
		Preload("Sales.Product").
		Preload("User").
		Order("created_at desc, id desc").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	for i := range results {
		if results[i].UserName == "" {
			if results[i].User != nil && results[i].User.Name != "" {
				results[i].UserName = results[i].User.Name
				results[i].UserRole = results[i].User.Role
			} else if results[i].Sales.UserID != "" {
				var u domain.User
				if err := s.db.Where("id = ? OR username = ?", results[i].Sales.UserID, results[i].Sales.UserID).First(&u).Error; err == nil {
					results[i].UserName = u.Name
					results[i].UserRole = u.Role
				}
			}
			if results[i].UserName == "" {
				results[i].UserName = "Kasir (Sistem)"
			}
		}
	}

	return results, nil
}

// 1. BATALKAN RETUR: Sales Qty bertambah lagi, Stok Toko berkurang lagi (karena barang ditarik balik oleh konsumen)
func (s *returService) BatalkanRetur(id uint) (*domain.Retur, error) {
	var retur domain.Retur
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Preload("Sales").First(&retur, id).Error; err != nil {
			return errors.New("data retur tidak ditemukan")
		}
		if retur.Status == "Retur Dibatalkan" || retur.Status == "Retur ke Distributor" || retur.Status == "Selesai (Distributor)" {
			return errors.New("status retur saat ini tidak dapat dibatalkan")
		}

		qtyDibatalkan := 1

		// Kembalikan quantity ke transaksi Sales semula
		if err := tx.Model(&domain.Sales{}).Where("id = ?", retur.SalesID).Update("qty", gorm.Expr("qty + ?", qtyDibatalkan)).Error; err != nil {
			return err
		}

		// Kurangi stok product di toko (karena barang dibawa pulang lagi oleh customer)
		if err := tx.Model(&domain.Product{}).Where("id = ?", retur.Sales.ProductID).Update("saldo", gorm.Expr("saldo - ?", qtyDibatalkan)).Error; err != nil {
			return err
		}

		retur.Status = "Retur Dibatalkan"
		retur.UpdatedAt = time.Now()
		return tx.Save(&retur).Error
	})
	return &retur, err
}

// 2. RETUR KE DISTRIBUTOR: Mengurangi kuantitas stok di toko karena dikirim keluar
func (s *returService) KirimKeDistributor(id uint) (*domain.Retur, error) {
	var retur domain.Retur
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Preload("Sales").First(&retur, id).Error; err != nil {
			return errors.New("data retur tidak ditemukan")
		}
		if retur.Status != "Retur Sukses" {
			return errors.New("hanya retur berstatus sukses yang bisa dikirim ke distributor")
		}

		qtyKirim := 1

		// Mengurangi stok produk di toko karena barang fisik dikirim ke suplier/distributor
		if err := tx.Model(&domain.Product{}).Where("id = ?", retur.Sales.ProductID).Update("saldo", gorm.Expr("saldo - ?", qtyKirim)).Error; err != nil {
			return err
		}

		retur.Status = "Retur ke Distributor"
		retur.IsReturToCompany = true
		retur.UpdatedAt = time.Now()
		return tx.Save(&retur).Error
	})
	return &retur, err
}

// 3. SELESAIKAN RETUR: Barang dari distributor telah kembali masuk ke gudang/toko
func (s *returService) SelesaikanRetur(id uint) (*domain.Retur, error) {
	var retur domain.Retur
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Preload("Sales").First(&retur, id).Error; err != nil {
			return errors.New("data retur tidak ditemukan")
		}
		if retur.Status != "Retur ke Distributor" {
			return errors.New("hanya retur yang sedang digantung di distributor yang bisa diselesaikan")
		}

		qtyKembali := 1

		// Tambahkan kembali stok produk toko karena barang baru pengganti/perbaikan dari distributor sudah datang
		if err := tx.Model(&domain.Product{}).Where("id = ?", retur.Sales.ProductID).Update("saldo", gorm.Expr("saldo + ?", qtyKembali)).Error; err != nil {
			return err
		}

		retur.Status = "Selesai (Distributor)"
		retur.UpdatedAt = time.Now()
		return tx.Save(&retur).Error
	})
	return &retur, err
}
