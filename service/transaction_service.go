package service

import (
	"backend-cashier/domain"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"
)

type TransactionService struct {
	DB *gorm.DB
}

func NewTransactionService(db *gorm.DB) *TransactionService {
	return &TransactionService{DB: db}
}

// CreateSales membuat transaksi penjualan per-nota yang dapat berisi banyak item produk.
// Jika transaksi berstatus DP, pencatatan customer dan status DP dilakukan per-nota.
func (s *TransactionService) CreateSales(req domain.CreateTransactionRequest) (*domain.SalesResponse, error) {
	if len(req.Items) == 0 {
		return nil, errors.New("daftar barang belanjaan tidak boleh kosong")
	}

	invoice := req.Invoice
	if invoice == "" {
		invoice = fmt.Sprintf("PJL-%d", time.Now().Unix())
	}

	var totalNettoAll float64
	var salesRecords []domain.Sales

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var customerID *uint

		// 1. Simpan data Customer sekali per nota jika transaksi DP atau data customer diisi
		if req.IsDp || req.Customer.Name != "" || req.Customer.PhoneNumber != "" {
			customer := domain.Customer{
				Name:           req.Customer.Name,
				Age:            req.Customer.Age,
				Address:        req.Customer.Address,
				PhoneNumber:    req.Customer.PhoneNumber,
				IdentityNumber: req.Customer.IdentityNumber,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}

			if err := tx.Create(&customer).Error; err != nil {
				return fmt.Errorf("gagal menyimpan data customer: %w", err)
			}
			customerID = &customer.ID
		}

		status := "Lunas"
		if req.IsDp {
			status = "DP"
		}

		// 2. Loop setiap item barang dalam nota
		for _, item := range req.Items {
			if item.Qty <= 0 {
				return fmt.Errorf("jumlah qty untuk barang %s harus lebih dari 0", item.ProductSearch)
			}

			var prod domain.Product
			if err := tx.Where("nama = ? OR kode = ?", item.ProductSearch, item.ProductSearch).First(&prod).Error; err != nil {
				return fmt.Errorf("barang %s tidak ditemukan", item.ProductSearch)
			}

			if prod.Saldo < item.Qty {
				return fmt.Errorf("stok %s tidak cukup, sisa: %.0f", prod.Nama, prod.Saldo)
			}

			price := item.Price
			if price == 0 {
				price = prod.HJual
			}

			subtotal := (price * item.Qty) - item.Discount
			totalNettoAll += subtotal

			sales := domain.Sales{
				Invoice:       invoice,
				ProductID:     prod.ID,
				UserID:        req.UserID,
				MemberName:    req.MemberName,
				Qty:           item.Qty,
				HBeli:         prod.HBeli,
				HJual:         price,
				Discount:      item.Discount,
				TotalNetto:    subtotal,
				PaymentMethod: req.PaymentMethod,
				AmountPaid:    req.AmountPaid,
				IsDp:          req.IsDp,
				CustomerID:    customerID,
				Status:        status,
				Shift:         nil,
			}

			if err := tx.Create(&sales).Error; err != nil {
				return err
			}

			// Potong Stok Produk
			if err := tx.Model(&prod).Update("saldo", prod.Saldo-item.Qty).Error; err != nil {
				return err
			}

			salesRecords = append(salesRecords, sales)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	change := req.AmountPaid - totalNettoAll

	return &domain.SalesResponse{
		Invoice:       invoice,
		MemberName:    req.MemberName,
		PaymentMethod: req.PaymentMethod,
		AmountPaid:    req.AmountPaid,
		TotalNetto:    totalNettoAll,
		Change:        change,
		Status:        salesRecords[0].Status,
		IsDp:          req.IsDp,
	}, nil
}

// CreateExpenditure Logic (Pembelian & Operasional)
func (s *TransactionService) CreateExpenditure(nota, kode, nama string, qty, harga, total float64) (interface{}, error) {
	// Jika ada Kode Barang, anggap sebagai Pembelian Stok
	if kode != "" && kode != "-" {
		var prod domain.Product
		if err := s.DB.Where("kode = ?", kode).First(&prod).Error; err != nil {
			return nil, errors.New("kode barang tidak terdaftar di master")
		}

		purchase := domain.Purchase{
			Nota:      nota,
			Tanggal:   time.Now(),
			ProductID: prod.ID,
			Qty:       qty,
			HBeli:     harga,
			Total:     total,
		}

		err := s.DB.Transaction(func(tx *gorm.DB) error {
			tx.Create(&purchase)
			return tx.Model(&prod).Update("saldo", prod.Saldo+qty).Error
		})
		return purchase, err
	}

	// Jika tidak ada kode, masuk ke Biaya Operasional
	expense := domain.Expense{
		Nota:      nota,
		Tanggal:   time.Now(),
		Deskripsi: nama,
		Total:     total,
	}
	err := s.DB.Create(&expense).Error
	return expense, err
}

// GroupSalesByInvoice mengelompokkan baris data Sales ke dalam grup transaksi per-nota
func GroupSalesByInvoice(sales []domain.Sales) []domain.SalesTransactionGroup {
	var groups []domain.SalesTransactionGroup
	groupMap := make(map[string]int)

	for _, s := range sales {
		inv := s.Invoice
		if inv == "" {
			inv = fmt.Sprintf("PJL-SINGLE-%d", s.ID)
		}

		idx, exists := groupMap[inv]
		if !exists {
			group := domain.SalesTransactionGroup{
				Invoice:       s.Invoice,
				CreatedAt:     s.CreatedAt,
				UpdatedAt:     s.UpdatedAt,
				MemberName:    s.MemberName,
				UserID:        s.UserID,
				User:          s.User,
				CustomerID:    s.CustomerID,
				Customer:      s.Customer,
				PaymentMethod: s.PaymentMethod,
				AmountPaid:    s.AmountPaid,
				IsDp:          s.IsDp,
				Status:        s.Status,
				Shift:         s.Shift,
				TotalNetto:    s.TotalNetto,
				TotalQty:      s.Qty,
				Items:         []domain.Sales{s},
			}
			groups = append(groups, group)
			groupMap[inv] = len(groups) - 1
		} else {
			groups[idx].Items = append(groups[idx].Items, s)
			groups[idx].TotalNetto += s.TotalNetto
			groups[idx].TotalQty += s.Qty
			if s.UpdatedAt.After(groups[idx].UpdatedAt) {
				groups[idx].UpdatedAt = s.UpdatedAt
			}
		}
	}

	for i := range groups {
		groups[i].Change = groups[i].AmountPaid - groups[i].TotalNetto
		if groups[i].Change < 0 {
			groups[i].RemainingAmount = -groups[i].Change
			groups[i].Change = 0
		} else {
			groups[i].RemainingAmount = 0
		}
	}

	return groups
}

func (s *TransactionService) GetAllSales(f domain.SalesFilter) ([]domain.SalesTransactionGroup, error) {
	var results []domain.Sales
	query := s.DB.Preload("Product").Preload("Customer").Preload("User")

	if f.StartDate != "" && f.EndDate != "" {
		query = query.Where("created_at BETWEEN ? AND ?", f.StartDate+" 00:00:00", f.EndDate+" 23:59:59")
	}
	if f.Member != "" {
		query = query.Where("member_name LIKE ?", "%"+f.Member+"%")
	}
	if f.Method != "" {
		query = query.Where("payment_method = ?", f.Method)
	}

	err := query.Order("created_at desc, id desc").Find(&results).Error
	if err != nil {
		return nil, err
	}

	return GroupSalesByInvoice(results), nil
}

// GetAllExpenditure logic yang sudah diperbaiki
func (s *TransactionService) GetAllExpenditure(f domain.ExpenseFilter) (map[string]interface{}, error) {
	var purchases []domain.Purchase
	var expenses []domain.Expense

	// Gunakan variabel terpisah agar tidak terjadi "leaking" kondisi query
	pQuery := s.DB.Preload("Product")
	eQuery := s.DB // Base query untuk expense

	// Apply Filter Tanggal jika ada
	if f.StartDate != "" && f.EndDate != "" {
		start := f.StartDate + " 00:00:00"
		end := f.EndDate + " 23:59:59"
		pQuery = pQuery.Where("created_at BETWEEN ? AND ?", start, end)
		eQuery = eQuery.Where("created_at BETWEEN ? AND ?", start, end)
	}

	// Ambil data Purchase (Stok Masuk)
	if f.Type == "purchase" || f.Type == "" {
		if err := pQuery.Find(&purchases).Error; err != nil {
			return nil, err
		}
	}

	// Ambil data Operational Expense (Biaya Operasional)
	if f.Type == "operational" || f.Type == "" {
		if err := eQuery.Find(&expenses).Error; err != nil {
			return nil, err
		}
	}

	return map[string]interface{}{
		"purchases":   purchases,
		"operational": expenses,
	}, nil
}

func (a *TransactionService) GetUserSales(userId uint) ([]domain.SalesTransactionGroup, error) {
	var sales []domain.Sales
	err := a.DB.Preload("Product").Preload("Customer").Preload("User").Where("user_id = ?", userId).Order("created_at desc, id desc").Find(&sales).Error
	if err != nil {
		return nil, err
	}
	return GroupSalesByInvoice(sales), nil
}

// PelunasanSales memproses pelunasan transaksi per-nota (bukan per produk).
// Parameter identifier dapat berupa ID Sales (uint/string) atau Nomor Invoice.
func (s *TransactionService) PelunasanSales(identifier string, req domain.PelunasanRequest) (*domain.PelunasanResponse, error) {
	var salesList []domain.Sales

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var invoice string

		// Cek apakah identifier adalah numeric ID atau Nomor Invoice
		if id, err := strconv.Atoi(identifier); err == nil && id > 0 {
			var singleSale domain.Sales
			if err := tx.First(&singleSale, id).Error; err != nil {
				return errors.New("data transaksi penjualan tidak ditemukan")
			}
			invoice = singleSale.Invoice
		} else {
			invoice = identifier
		}

		if invoice == "" {
			return errors.New("nomor nota / invoice tidak valid")
		}
		

		// Ambil semua item penjualan dalam nota tersebut
		if err := tx.Preload("Product").Preload("Customer").Where("invoice = ?", invoice).Find(&salesList).Error; err != nil || len(salesList) == 0 {
			return errors.New("data transaksi penjualan dengan nota tersebut tidak ditemukan")
		}

		// Validasi apakah transaksi ini sudah berstatus lunas
		if !salesList[0].IsDp && salesList[0].Status == "Lunas" {
			return errors.New("transaksi pada nota ini sudah berstatus lunas")
		}

		status := req.Status
		if status == "" {
			status = "Lunas"
		}

		paymentMethod := req.PaymentMethod
		if paymentMethod == "" {
			paymentMethod = salesList[0].PaymentMethod
		}

		amountPaid := req.AmountPaid
		if amountPaid <= 0 {
			var totalNetto float64
			for _, item := range salesList {
				totalNetto += item.TotalNetto
			}
			amountPaid = totalNetto
		}

		// Update semua item dalam nota tersebut menjadi Lunas
		updates := map[string]interface{}{
			"status":         status,
			"is_dp":          req.IsDp,
			"payment_method": paymentMethod,
			"amount_paid":    amountPaid,
			"updated_at":     time.Now(),
		}

		if err := tx.Model(&domain.Sales{}).Where("invoice = ?", invoice).Updates(updates).Error; err != nil {
			return err
		}

		// Refresh data setelah update
		if err := tx.Preload("Product").Preload("Customer").Where("invoice = ?", invoice).Find(&salesList).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	var totalNetto float64
	for _, item := range salesList {
		totalNetto += item.TotalNetto
	}

	return &domain.PelunasanResponse{
		ID:            salesList[0].ID,
		Invoice:       salesList[0].Invoice,
		Status:        salesList[0].Status,
		IsDp:          salesList[0].IsDp,
		PaymentMethod: salesList[0].PaymentMethod,
		AmountPaid:    salesList[0].AmountPaid,
		TotalNetto:    totalNetto,
		CustomerID:    salesList[0].CustomerID,
		Customer:      salesList[0].Customer,
		Sales:         salesList,
	}, nil
}

func (s *TransactionService) GetCurrentShift() uint {
	todayStart := time.Now().Format("2006-01-02") + " 00:00:00"
	todayEnd := time.Now().Format("2006-01-02") + " 23:59:59"

	var count int64
	s.DB.Model(&domain.Sales{}).
		Where(
			"created_at BETWEEN ? AND ? AND shift IS NOT NULL",
			todayStart,
			todayEnd,
		).
		Count(&count)

	if count == 0 {
		return 1
	}

	return 2
}

func (s *TransactionService) GetReceipt() (*domain.ReceiptResponse, error) {
	var sales []domain.Sales

	err := s.DB.
		Preload("Product").
		Preload("User").
		Preload("Customer").
		Where(
			"shift IS NULL",
		).
		Find(&sales).
		Error

	if err != nil {
		return nil, err
	}

	var expenses []domain.Expense

	err = s.DB.
		Where(
			"tanggal >= ?",
			time.Now().Format("2006-01-02"),
		).
		Find(&expenses).
		Error

	if err != nil {
		return nil, err
	}

	return &domain.ReceiptResponse{
		Category: "SHIFT_RECEIPT",
		Sales:    sales,
		Expenses: expenses,
	}, nil
}

func (s *TransactionService) UpdateSalesShift(shift uint) error {
	return s.DB.
		Model(&domain.Sales{}).
		Where(
			"shift IS NULL",
		).
		Update(
			"shift",
			shift,
		).
		Error
}
