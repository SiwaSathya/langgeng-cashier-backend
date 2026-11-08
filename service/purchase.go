package service

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"backend-cashier/domain"
	"gorm.io/gorm"
)

type PurchaseService struct {
	DB            *gorm.DB
	AccountingSvc *AccountingService
}

func NewPurchaseService(db *gorm.DB, accountingSvc *AccountingService) *PurchaseService {
	return &PurchaseService{
		DB:            db,
		AccountingSvc: accountingSvc,
	}
}

// CreateBatch membuat banyak item pembelian dalam 1 transaksi per-nota
func (s *PurchaseService) CreateBatch(req domain.CreatePurchaseBatchRequest) error {
	if len(req.Items) == 0 {
		return errors.New("daftar barang belanjaan (items) tidak boleh kosong")
	}

	tgl := time.Now()
	if req.Tanggal != "" {
		if t, err := time.Parse(time.RFC3339, req.Tanggal); err == nil {
			tgl = t
		} else if t, err := time.Parse("2006-01-02", req.Tanggal); err == nil {
			tgl = t
		}
	}

	loc := strings.TrimSpace(req.Location)
	if loc == "" {
		loc = "Toko Utama"
	}

	docType := strings.TrimSpace(req.DocType)
	if docType == "" {
		docType = "Nota Faktur"
	}

	// 1. Validasi & Sanitasi SupplierID (Nullable FK)
	var supplierIDPtr *string
	reqSupplierID := strings.TrimSpace(string(req.SupplierID))
	if reqSupplierID != "" && reqSupplierID != "0" && !strings.EqualFold(reqSupplierID, "null") {
		var suppCount int64
		s.DB.Model(&domain.Supplier{}).Where("id = ?", reqSupplierID).Count(&suppCount)
		if suppCount > 0 {
			supplierIDPtr = &reqSupplierID
		}
	}

	// 2. Validasi & Sanitasi UserID (Nullable FK / Fallback ke user valid)
	var userIDPtr *string
	reqUserID := strings.TrimSpace(string(req.UserID))
	if reqUserID != "" && reqUserID != "0" && !strings.EqualFold(reqUserID, "null") {
		var u domain.User
		if err := s.DB.Where("id = ?", reqUserID).First(&u).Error; err == nil {
			userIDPtr = &u.ID
			if (loc == "" || loc == "Toko Utama") && u.Location != "" {
				loc = u.Location
			}
		}
	}
	if userIDPtr == nil {
		var u domain.User
		if err := s.DB.First(&u).Error; err == nil {
			userIDPtr = &u.ID
			if (loc == "" || loc == "Toko Utama") && u.Location != "" {
				loc = u.Location
			}
		}
	}

	return s.DB.Transaction(func(tx *gorm.DB) error {
		var grandTotal float64

		for _, item := range req.Items {
			if item.ProductID == 0 || item.Qty <= 0 {
				return errors.New("ID produk dan Qty harus valid (lebih dari 0)")
			}

			var prod domain.Product
			if err := tx.First(&prod, item.ProductID).Error; err != nil {
				return fmt.Errorf("produk ID %d tidak ditemukan", item.ProductID)
			}

			subtotal := item.Qty * item.HBeli
			grandTotal += subtotal

			p := domain.Purchase{
				Nota:       req.Nota,
				Tanggal:    tgl,
				ProductID:  item.ProductID,
				SupplierID: supplierIDPtr,
				Qty:        item.Qty,
				HBeli:      item.HBeli,
				Total:      subtotal,
				Location:   loc,
				UserID:     userIDPtr,
				DocType:    docType,
			}

			if err := tx.Create(&p).Error; err != nil {
				return err
			}

			// Tambah saldo stok produk
			if err := tx.Model(&prod).Update("saldo", prod.Saldo+item.Qty).Error; err != nil {
				return err
			}

			// Perbarui harga beli master jika diisi
			if item.HBeli > 0 {
				tx.Model(&prod).Update("h_beli", item.HBeli)
			}
		}

		if s.AccountingSvc != nil {
			s.AccountingSvc.AutoPostPurchaseJournal(tx, req.Nota, grandTotal, tgl, loc)
		}

		return nil
	})
}

// GetAll mengambil data purchase dengan pagination berbasis 10 GRUP per halaman
func (s *PurchaseService) GetAll(page, limit int, groupBy, search, location string) ([]domain.PurchaseTransactionGroup, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if page <= 0 {
		page = 1
	}

	var purchases []domain.Purchase
	query := s.DB.Model(&domain.Purchase{}).
		Preload("Product").
		Preload("Supplier").
		Preload("User").
		Order("tanggal desc, id desc")

	if search != "" {
		query = query.Joins("LEFT JOIN products ON products.id = purchases.product_id").
			Joins("LEFT JOIN suppliers ON suppliers.id = purchases.supplier_id").
			Where("purchases.nota ILIKE ? OR products.nama ILIKE ? OR products.kode ILIKE ? OR suppliers.name ILIKE ?",
				"%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	if location != "" && !strings.EqualFold(location, "Semua Toko") {
		query = query.Where("purchases.location = ?", location)
	}

	// Ambil data untuk dikelompokkan
	if err := query.Find(&purchases).Error; err != nil {
		return nil, 0, err
	}

	// 1. Kelompokkan seluruh pembelian (Group by Nota atau Group by Supplier)
	var allGroups []domain.PurchaseTransactionGroup
	if strings.EqualFold(groupBy, "supplier") {
		allGroups = GroupPurchasesBySupplier(purchases)
	} else {
		allGroups = GroupPurchasesByNota(purchases)
	}

	totalGroups := int64(len(allGroups))

	// 2. Pagination terhadap hasil GRUP (10 grup per page)
	offset := (page - 1) * limit
	if offset >= len(allGroups) {
		return []domain.PurchaseTransactionGroup{}, totalGroups, nil
	}

	end := offset + limit
	if end > len(allGroups) {
		end = len(allGroups)
	}

	pagedGroups := allGroups[offset:end]
	return pagedGroups, totalGroups, nil
}

func GroupPurchasesByNota(purchases []domain.Purchase) []domain.PurchaseTransactionGroup {
	var groups []domain.PurchaseTransactionGroup
	groupMap := make(map[string]int)

	for _, p := range purchases {
		nota := p.Nota
		if nota == "" {
			nota = fmt.Sprintf("PBL-SINGLE-%d", p.ID)
		}

		suppID := ""
		if p.SupplierID != nil {
			suppID = *p.SupplierID
		}
		userID := ""
		if p.UserID != nil {
			userID = *p.UserID
		}

		docType := p.DocType
		if docType == "" {
			docType = "Nota Faktur"
		}

		idx, exists := groupMap[nota]
		if !exists {
			group := domain.PurchaseTransactionGroup{
				GroupKey:    nota,
				Nota:        p.Nota,
				SupplierID:  suppID,
				Supplier:    p.Supplier,
				Tanggal:     p.Tanggal,
				Location:    p.Location,
				UserID:      userID,
				User:        p.User,
				DocType:     docType,
				TotalQty:    p.Qty,
				TotalAmount: p.Total,
				TotalItems:  1,
				Items:       []domain.Purchase{p},
			}
			groups = append(groups, group)
			groupMap[nota] = len(groups) - 1
		} else {
			groups[idx].Items = append(groups[idx].Items, p)
			groups[idx].TotalQty += p.Qty
			groups[idx].TotalAmount += p.Total
			groups[idx].TotalItems++
			if groups[idx].DocType == "" {
				groups[idx].DocType = docType
			}
		}
	}

	// Hitung DPP dan PPN untuk setiap grup
	for i := range groups {
		groups[i].DPP = math.Round(groups[i].TotalAmount / 1.11)
		groups[i].PPN = groups[i].TotalAmount - groups[i].DPP
	}

	return groups
}

func GroupPurchasesBySupplier(purchases []domain.Purchase) []domain.PurchaseTransactionGroup {
	var groups []domain.PurchaseTransactionGroup
	groupMap := make(map[string]int)

	for _, p := range purchases {
		suppName := "Supplier Umum"
		if p.Supplier.Name != "" {
			suppName = p.Supplier.Name
		}

		suppID := ""
		if p.SupplierID != nil {
			suppID = *p.SupplierID
		}
		userID := ""
		if p.UserID != nil {
			userID = *p.UserID
		}

		docType := p.DocType
		if docType == "" {
			docType = "Nota Faktur"
		}

		idx, exists := groupMap[suppName]
		if !exists {
			group := domain.PurchaseTransactionGroup{
				GroupKey:    suppName,
				SupplierID:  suppID,
				Supplier:    p.Supplier,
				Tanggal:     p.Tanggal,
				Location:    p.Location,
				UserID:      userID,
				User:        p.User,
				DocType:     docType,
				TotalQty:    p.Qty,
				TotalAmount: p.Total,
				TotalItems:  1,
				Items:       []domain.Purchase{p},
			}
			groups = append(groups, group)
			groupMap[suppName] = len(groups) - 1
		} else {
			groups[idx].Items = append(groups[idx].Items, p)
			groups[idx].TotalQty += p.Qty
			groups[idx].TotalAmount += p.Total
			groups[idx].TotalItems++
		}
	}

	// Hitung DPP dan PPN untuk setiap grup
	for i := range groups {
		groups[i].DPP = math.Round(groups[i].TotalAmount / 1.11)
		groups[i].PPN = groups[i].TotalAmount - groups[i].DPP
	}

	return groups
}

// DeleteByNota membatalkan pembelian 1 nota dan mengembalikan stok produk (rollback)
func (s *PurchaseService) DeleteByNota(nota string) error {
	var items []domain.Purchase
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("nota = ?", nota).Find(&items).Error; err != nil || len(items) == 0 {
			return errors.New("data nota pembelian tidak ditemukan")
		}

		for _, item := range items {
			var prod domain.Product
			if err := tx.First(&prod, item.ProductID).Error; err == nil {
				tx.Model(&prod).Update("saldo", prod.Saldo-item.Qty)
			}
		}

		return tx.Where("nota = ?", nota).Delete(&domain.Purchase{}).Error
	})
}
