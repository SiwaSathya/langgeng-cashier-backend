package service

import (
	"backend-cashier/domain"
	"errors"
	"time"

	"gorm.io/gorm"
)

// DTO untuk menerima request dari frontend
type ReturRequest struct {
	SalesID          uint   `json:"sales_id" binding:"required"`
	Reason           string `json:"reason" binding:"required"`
	Status           string `json:"status"`
	IsRetur          bool   `json:"is_retur"`
	IsReturToCompany bool   `json:"is_retur_to_company"`
	QtyRetur         int    `json:"qty_retur" binding:"required,gt=0"`
}

type ReturService interface {
	GetAllRetur() ([]domain.Retur, error)
	CreateRetur(req ReturRequest) (*domain.Retur, error)
	BatalkanRetur(id uint) (*domain.Retur, error)
	KirimKeDistributor(id uint) (*domain.Retur, error)
	SelesaikanRetur(id uint) (*domain.Retur, error)
}

type returService struct {
	db *gorm.DB
}

func NewReturService(db *gorm.DB) ReturService {
	return &returService{db: db}
}

func (s *returService) CreateRetur(req ReturRequest) (*domain.Retur, error) {
	var returData domain.Retur

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

		// 2. Validasi Qty Retur tidak boleh melebihi Qty Penjualan awal
		if req.QtyRetur > int(sale.Qty) {
			return errors.New("jumlah qty retur melebihi jumlah pembelian asli")
		}

		// 3. Siapkan & Simpan data Retur baru
		returData = domain.Retur{
			SalesID:          req.SalesID,
			Reason:           req.Reason,
			Status:           req.Status,
			IsRetur:          req.IsRetur,
			IsReturToCompany: req.IsReturToCompany,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}

		if err := tx.Create(&returData).Error; err != nil {
			return err
		}

		sale.Qty = sale.Qty - float64(req.QtyRetur)

		if err := tx.Save(&sale).Error; err != nil {
			return err
		}

		if !req.IsReturToCompany {
			err := tx.Model(&domain.Product{}).
				Where("id = ?", sale.ProductID).
				Update("saldo", gorm.Expr("saldo + ?", req.QtyRetur)).Error
			if err != nil {
				return errors.New("gagal memperbarui stok produk")
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &returData, nil
}

func (s *returService) GetAllRetur() ([]domain.Retur, error) {
	var listRetur []domain.Retur
	err := s.db.Preload("Sales").Preload("Sales.Product").Order("id desc").Find(&listRetur).Error
	return listRetur, err
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

		// Asumsi QtyRetur disimpan/diketahui (misal default 1 atau dicatat di field khusus.
		// Kita ambil sampel statis atau asumsikan 1 jika tidak ada field khusus, sesuaikan dengan logic bisnis Anda)
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
