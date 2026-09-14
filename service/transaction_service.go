package service

import (
	"backend-cashier/domain"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type TransactionService struct {
	DB            *gorm.DB
	AccountingSvc *AccountingService
}

func NewTransactionService(db *gorm.DB, accountingSvc *AccountingService) *TransactionService {
	return &TransactionService{
		DB:            db,
		AccountingSvc: accountingSvc,
	}
}

// CreateSales membuat transaksi penjualan per-nota yang dapat berisi banyak item produk.
func (s *TransactionService) CreateSales(req domain.CreateTransactionRequest) (*domain.SalesResponse, error) {
	if len(req.Items) == 0 {
		return nil, errors.New("daftar barang belanjaan tidak boleh kosong")
	}

	// Validasi khusus transaksi Bon / Utang
	if req.IsBon {
		if req.Customer.Name == "" || req.Customer.PhoneNumber == "" {
			return nil, errors.New("transaksi bon/utang wajib mengisi nama dan no. telepon customer")
		}
	}

	invoice := req.Invoice
	if invoice == "" {
		invoice = fmt.Sprintf("PJL-%d", time.Now().UnixNano())
	}

	location := req.Location
	if location == "" && req.UserID != "" {
		var user domain.User
		if err := s.DB.Where("id = ?", req.UserID).First(&user).Error; err == nil && user.Location != "" {
			location = user.Location
		}
	}
	if location == "" {
		location = "Toko Utama"
	}

	var totalPaidFromMethods float64
	var methodNames []string
	for _, pm := range req.PaymentMethods {
		if amt, err := strconv.ParseFloat(pm.Amount, 64); err == nil {
			totalPaidFromMethods += amt
		}
		if pm.Type != "" {
			methodNames = append(methodNames, pm.Type)
		}
	}

	if req.AmountPaid == 0 && totalPaidFromMethods > 0 {
		req.AmountPaid = totalPaidFromMethods
	}
	if req.PaymentMethod == "" {
		if len(methodNames) > 0 {
			req.PaymentMethod = strings.Join(methodNames, ", ")
		} else if req.IsBon {
			req.PaymentMethod = "Bon / Utang"
		}
	}

	var totalNettoAll float64
	var salesRecords []domain.Sales
	var createdPaymentMethods []domain.PaymentMethod

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var customerID *uint

		// 1. Simpan data Customer (wajib untuk DP dan Bon)
		if req.IsDp || req.IsBon || req.Customer.Name != "" || req.Customer.PhoneNumber != "" {
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

		// Tentukan status nota
		status := "Lunas"
		if req.IsBon {
			status = "Bon"
		} else if req.IsDp {
			status = "DP"
		}

		// 2. Simpan setiap item produk
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
				Location:      location,
				MemberName:    req.MemberName,
				Qty:           item.Qty,
				HBeli:         prod.HBeli,
				HJual:         price,
				Discount:      item.Discount,
				TotalNetto:    subtotal,
				PaymentMethod: req.PaymentMethod,
				AmountPaid:    req.AmountPaid,
				IsDp:          req.IsDp,
				IsBon:         req.IsBon,
				CustomerID:    customerID,
				Status:        status,
				Shift:         nil,
			}

			if err := tx.Create(&sales).Error; err != nil {
				return err
			}

			if err := tx.Model(&prod).Update("saldo", prod.Saldo-item.Qty).Error; err != nil {
				return err
			}

			salesRecords = append(salesRecords, sales)
		}

		// 3. Simpan Payment Method
		if len(salesRecords) > 0 {
			primarySalesID := salesRecords[0].ID

			if req.IsBon && len(req.PaymentMethods) == 0 {
				// Catat transaksi bon nominal 0
				record := domain.PaymentMethod{
					SalesID: primarySalesID,
					Type:    "Bon / Utang",
					Amount:  "0",
					Status:  "Belum Lunas",
				}
				if err := tx.Create(&record).Error; err != nil {
					return err
				}
				createdPaymentMethods = append(createdPaymentMethods, record)
			} else if len(req.PaymentMethods) > 0 {
				for _, pm := range req.PaymentMethods {
					pmStatus := pm.Status
					if pmStatus == "" {
						pmStatus = status
					}
					record := domain.PaymentMethod{
						SalesID: primarySalesID,
						Type:    pm.Type,
						Amount:  pm.Amount,
						Status:  pmStatus,
					}
					if err := tx.Create(&record).Error; err != nil {
						return err
					}
					createdPaymentMethods = append(createdPaymentMethods, record)
				}
			} else {
				record := domain.PaymentMethod{
					SalesID: primarySalesID,
					Type:    req.PaymentMethod,
					Amount:  fmt.Sprintf("%.0f", req.AmountPaid),
					Status:  status,
				}
				if err := tx.Create(&record).Error; err != nil {
					return err
				}
				createdPaymentMethods = append(createdPaymentMethods, record)
			}
		}

		// 4. Catat Jurnal & Piutang Dagang jika DP atau Bon
		if s.AccountingSvc != nil {
			isCredit := req.IsDp || req.IsBon
			s.AccountingSvc.AutoPostSalesJournal(tx, invoice, req.PaymentMethod, totalNettoAll, req.AmountPaid, isCredit, time.Now())
			if isCredit {
				s.AccountingSvc.AutoRecordSalesPiutang(tx, customerID, req.Customer.Name, req.Customer.PhoneNumber, invoice, totalNettoAll, req.AmountPaid, time.Now())
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	change := req.AmountPaid - totalNettoAll
	if change < 0 {
		change = 0
	}

	return &domain.SalesResponse{
		Invoice:        invoice,
		MemberName:     req.MemberName,
		UserID:         req.UserID,
		Location:       location,
		PaymentMethod:  req.PaymentMethod,
		AmountPaid:     req.AmountPaid,
		TotalNetto:     totalNettoAll,
		Change:         change,
		Status:         salesRecords[0].Status,
		IsDp:           req.IsDp,
		IsBon:          req.IsBon,
		PaymentMethods: createdPaymentMethods,
	}, nil
}

// CreateExpenditure Logic (Pembelian & Operasional)
func (s *TransactionService) CreateExpenditure(nota, kode, nama string, qty, harga, total float64) (interface{}, error) {
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
			Location:  "Toko Utama",
		}

		err := s.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&purchase).Error; err != nil {
				return err
			}
			if err := tx.Model(&prod).Update("saldo", prod.Saldo+qty).Error; err != nil {
				return err
			}
			if s.AccountingSvc != nil {
				s.AccountingSvc.AutoPostPurchaseJournal(tx, nota, total, time.Now())
			}
			return nil
		})
		return purchase, err
	}

	expense := domain.Expense{
		Nota:      nota,
		Tanggal:   time.Now(),
		Deskripsi: nama,
		Total:     total,
		Location:  "Toko Utama",
	}
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&expense).Error; err != nil {
			return err
		}
		if s.AccountingSvc != nil {
			s.AccountingSvc.AutoPostExpenseJournal(tx, nota, nama, "", total, time.Now())
		}
		return nil
	})
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
			loc := s.Location
			if loc == "" && s.User.Location != "" {
				loc = s.User.Location
			}
			if loc == "" {
				loc = "Toko Utama"
			}

			group := domain.SalesTransactionGroup{
				Invoice:        s.Invoice,
				CreatedAt:      s.CreatedAt,
				UpdatedAt:      s.UpdatedAt,
				MemberName:     s.MemberName,
				UserID:         s.UserID,
				User:           s.User,
				Location:       loc,
				CustomerID:     s.CustomerID,
				Customer:       s.Customer,
				PaymentMethod:  s.PaymentMethod,
				AmountPaid:     s.AmountPaid,
				IsDp:           s.IsDp,
				Status:         s.Status,
				Shift:          s.Shift,
				TotalNetto:     s.TotalNetto,
				TotalQty:       s.Qty,
				IsBon:          s.IsBon,
				Items:          []domain.Sales{s},
				PaymentMethods: append([]domain.PaymentMethod{}, s.PaymentMethods...),
			}
			groups = append(groups, group)
			groupMap[inv] = len(groups) - 1
		} else {
			groups[idx].Items = append(groups[idx].Items, s)
			groups[idx].TotalNetto += s.TotalNetto
			groups[idx].TotalQty += s.Qty
			if len(s.PaymentMethods) > 0 {
				groups[idx].PaymentMethods = append(groups[idx].PaymentMethods, s.PaymentMethods...)
			}
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
	query := s.DB.Preload("Product").
		Preload("Customer").
		Preload("User").
		Preload("PaymentMethods").
		Where("is_returs = ?", false)

	if f.StartDate != "" && f.EndDate != "" {
		query = query.Where("created_at BETWEEN ? AND ?", f.StartDate+" 00:00:00", f.EndDate+" 23:59:59")
	}
	if f.Member != "" {
		query = query.Where("member_name LIKE ?", "%"+f.Member+"%")
	}
	if f.Method != "" {
		query = query.Where("payment_method = ?", f.Method)
	}

	uLoc := strings.TrimSpace(f.UserLocation)
	if uLoc != "" && !strings.EqualFold(uLoc, "Toko Utama") && !strings.EqualFold(uLoc, "Pusat") {
		query = query.Where("location = ? OR location = ''", uLoc)
	} else {
		if f.Location != "" && !strings.EqualFold(f.Location, "Semua Toko") && !strings.EqualFold(f.Location, "All") {
			query = query.Where("location = ?", f.Location)
		}
	}

	err := query.Order("created_at desc, id desc").Find(&results).Error
	if err != nil {
		return nil, err
	}

	return GroupSalesByInvoice(results), nil
}

func (s *TransactionService) GetAllExpenditure(f domain.ExpenseFilter) (map[string]interface{}, error) {
	var purchases []domain.Purchase
	var expenses []domain.Expense

	pQuery := s.DB.Preload("Product")
	eQuery := s.DB

	if f.StartDate != "" && f.EndDate != "" {
		start := f.StartDate + " 00:00:00"
		end := f.EndDate + " 23:59:59"
		pQuery = pQuery.Where("created_at BETWEEN ? AND ?", start, end)
		eQuery = eQuery.Where("created_at BETWEEN ? AND ?", start, end)
	}

	if f.Location != "" && !strings.EqualFold(f.Location, "Semua Toko") {
		pQuery = pQuery.Where("location = ?", f.Location)
		eQuery = eQuery.Where("location = ?", f.Location)
	}

	if f.Type == "purchase" || f.Type == "" {
		if err := pQuery.Find(&purchases).Error; err != nil {
			return nil, err
		}
	}

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
	err := a.DB.Preload("Product").
		Preload("Customer").
		Preload("User").
		Preload("PaymentMethods").
		Where("user_id = ?", userId).
		Order("created_at desc, id desc").
		Find(&sales).Error
	if err != nil {
		return nil, err
	}
	return GroupSalesByInvoice(sales), nil
}

func (s *TransactionService) PelunasanSales(identifier string, req domain.PelunasanRequest) (*domain.PelunasanResponse, error) {
	var salesList []domain.Sales

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var invoice string

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

		if err := tx.Preload("Product").Preload("Customer").Where("invoice = ?", invoice).Find(&salesList).Error; err != nil || len(salesList) == 0 {
			return errors.New("data transaksi penjualan dengan nota tersebut tidak ditemukan")
		}

		// Cek apakah transaksi memang belum lunas (bisa dari DP atau Bon)
		if !salesList[0].IsDp && !salesList[0].IsBon && salesList[0].Status == "Lunas" {
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

		// Hitung nominal pelunasan yang baru dibayarkan
		previousPaid := salesList[0].AmountPaid
		amountPaid := req.AmountPaid
		if amountPaid <= 0 {
			var totalNetto float64
			for _, item := range salesList {
				totalNetto += item.TotalNetto
			}
			amountPaid = totalNetto
		}

		pelunasanAmount := amountPaid - previousPaid
		if pelunasanAmount <= 0 {
			pelunasanAmount = amountPaid
		}

		// 1. UPDATE STATUS SALES & RESET FLAG DP SERTA BON
		updates := map[string]interface{}{
			"status":         status,
			"is_dp":          false,
			"is_bon":         false,
			"payment_method": paymentMethod,
			"amount_paid":    amountPaid,
			"updated_at":     time.Now(),
		}

		if err := tx.Model(&domain.Sales{}).Where("invoice = ?", invoice).Updates(updates).Error; err != nil {
			return err
		}

		// 2. SIMPAN CATATAN PEMBAYARAN KE TABEL payment_methods DENGAN STATUS "Pelunasan"
		primarySalesID := salesList[0].ID

		if len(req.PaymentMethods) > 0 {
			for _, pm := range req.PaymentMethods {
				record := domain.PaymentMethod{
					SalesID: primarySalesID,
					Type:    pm.Type,
					Amount:  pm.Amount,
					Status:  "Pelunasan",
				}
				if err := tx.Create(&record).Error; err != nil {
					return fmt.Errorf("gagal menyimpan metode pelunasan: %w", err)
				}
			}
		} else {
			// Fallback parsing nama metode pelunasan dari string gabungan
			methodType := paymentMethod
			if idxStart := strings.Index(paymentMethod, "[Pelunasan:"); idxStart != -1 {
				extracted := paymentMethod[idxStart+len("[Pelunasan:"):]
				if idxEnd := strings.Index(extracted, "]"); idxEnd != -1 {
					methodType = strings.TrimSpace(extracted[:idxEnd])
				}
			}

			record := domain.PaymentMethod{
				SalesID: primarySalesID,
				Type:    methodType,
				Amount:  fmt.Sprintf("%.0f", pelunasanAmount),
				Status:  "Pelunasan",
			}
			if err := tx.Create(&record).Error; err != nil {
				return fmt.Errorf("gagal menyimpan metode pelunasan: %w", err)
			}
		}

		if err := tx.Preload("Product").Preload("Customer").Preload("PaymentMethods").Where("invoice = ?", invoice).Find(&salesList).Error; err != nil {
			return err
		}

		// 3. UPDATE JURNAL & SETTLEMENT PIUTANG
		if s.AccountingSvc != nil {
			s.AccountingSvc.AutoPostSalesJournal(tx, invoice, paymentMethod, pelunasanAmount, pelunasanAmount, false, time.Now())
			s.AccountingSvc.AutoSettleSalesPiutang(tx, invoice, pelunasanAmount)
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

func (s *TransactionService) GetCurrentShift(location string) uint {
	todayStart := time.Now().Format("2006-01-02") + " 00:00:00"
	todayEnd := time.Now().Format("2006-01-02") + " 23:59:59"

	loc := strings.TrimSpace(location)
	if loc == "" || strings.EqualFold(loc, "Pusat") {
		loc = "Toko Utama"
	}

	var count int64
	q := s.DB.Model(&domain.Sales{}).
		Where("created_at BETWEEN ? AND ? AND shift IS NOT NULL", todayStart, todayEnd)

	if !strings.EqualFold(loc, "Semua Toko") {
		q = q.Where("(location = ? OR (location = '' AND ? = 'Toko Utama'))", loc, loc)
	}

	q.Count(&count)

	if count == 0 {
		return 1
	}

	return 2
}

func (s *TransactionService) GetReceipt(location string) (*domain.ReceiptResponse, error) {
	loc := strings.TrimSpace(location)
	if loc == "" || strings.EqualFold(loc, "Pusat") {
		loc = "Toko Utama"
	}

	var sales []domain.Sales
	var expenses []domain.Expense

	// Tambahkan .Preload("PaymentMethods") di sini
	salesQuery := s.DB.Preload("Product").
		Preload("User").
		Preload("Customer").
		Preload("PaymentMethods").
		Where("shift IS NULL")

	expenseQuery := s.DB.Where("tanggal >= ?", time.Now().Format("2006-01-02"))

	if strings.EqualFold(loc, "Semua Toko") {
		salesQuery = salesQuery.Where("is_returs = ?", false)
	} else {
		salesQuery = salesQuery.Where("location = ?", loc).Where("is_returs = ?", false)
		expenseQuery = expenseQuery.Where("location = ?", loc)
	}

	if err := salesQuery.Find(&sales).Error; err != nil {
		return nil, err
	}

	if err := expenseQuery.Where("shift IS NULL").Find(&expenses).Error; err != nil {
		return nil, err
	}

	return &domain.ReceiptResponse{
		Category: "SHIFT_RECEIPT",
		Location: loc,
		Sales:    sales,
		Expenses: expenses,
	}, nil
}

func (s *TransactionService) UpdateSalesShift(shift uint, location string) error {
	loc := strings.TrimSpace(location)
	if loc == "" || strings.EqualFold(loc, "Pusat") {
		loc = "Toko Utama"
	}

	q := s.DB.Model(&domain.Sales{}).Where("shift IS NULL")

	if !strings.EqualFold(loc, "Semua Toko") {
		q = q.Where("(location = ? OR (location = '' AND ? = 'Toko Utama'))", loc, loc)
	}

	return q.Update("shift", shift).Error
}

func (s *TransactionService) DeleteSales(identifier string) error {
	var salesList []domain.Sales

	return s.DB.Transaction(func(tx *gorm.DB) error {
		var invoice string

		if id, err := strconv.Atoi(identifier); err == nil && id > 0 {
			var singleSale domain.Sales
			if err := tx.First(&singleSale, id).Error; err != nil {
				return errors.New("data transaksi tidak ditemukan")
			}
			invoice = singleSale.Invoice
		} else {
			invoice = identifier
		}

		if invoice == "" {
			return errors.New("nomor nota / invoice tidak valid")
		}

		if err := tx.Where("invoice = ?", invoice).Find(&salesList).Error; err != nil || len(salesList) == 0 {
			return errors.New("data transaksi penjualan tidak ditemukan")
		}

		var salesIDs []uint
		for _, item := range salesList {
			salesIDs = append(salesIDs, item.ID)
			if item.ProductID > 0 && item.Qty > 0 {
				var prod domain.Product
				if err := tx.First(&prod, item.ProductID).Error; err == nil {
					tx.Model(&prod).Update("saldo", prod.Saldo+item.Qty)
				}
			}
		}

		// 1. Hapus payment methods terkait
		if len(salesIDs) > 0 {
			if err := tx.Where("sales_id IN ?", salesIDs).Delete(&domain.PaymentMethod{}).Error; err != nil {
				return err
			}
		}

		// 2. Hapus jurnal otomatis
		entryNumber := fmt.Sprintf("JV-SALES-%s", invoice)
		var journal domain.JournalEntry
		if err := tx.Where("entry_number = ?", entryNumber).First(&journal).Error; err == nil {
			tx.Where("journal_entry_id = ?", journal.ID).Delete(&domain.JournalItem{})
			tx.Delete(&journal)
		}

		// 3. Hapus piutang
		tx.Where("sales_invoice = ?", invoice).Delete(&domain.PiutangDagang{})

		// 4. Hapus baris penjualan
		return tx.Where("invoice = ?", invoice).Delete(&domain.Sales{}).Error
	})
}
