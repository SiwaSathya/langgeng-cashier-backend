package service

import (
	"backend-cashier/domain"
	"errors"
	"fmt"
	"sort"
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
	_ = db.Exec("ALTER TABLE payment_methods ADD COLUMN IF NOT EXISTS shift bigint DEFAULT NULL;")
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
			req.AmountPaid = 0
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
			_ = AdjustProductStock(tx, prod.ID, location, -item.Qty, "SALE", invoice, fmt.Sprintf("Penjualan Kasir (#%s)", invoice), req.UserID)

			salesRecords = append(salesRecords, sales)
		}

		change := 0.0
		if !req.IsBon && !req.IsDp && req.AmountPaid > totalNettoAll {
			change = req.AmountPaid - totalNettoAll
		}
		if change > 0 {
			_ = tx.Model(&domain.Sales{}).Where("invoice = ?", invoice).Update("change", change).Error
		}

		// 3. Simpan Payment Method
		if len(salesRecords) > 0 {
			primarySalesID := salesRecords[0].ID

			if req.IsBon {
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
				actualPaid := req.AmountPaid
				if !req.IsDp && req.AmountPaid > totalNettoAll && totalNettoAll > 0 {
					actualPaid = totalNettoAll
				}
				record := domain.PaymentMethod{
					SalesID: primarySalesID,
					Type:    req.PaymentMethod,
					Amount:  fmt.Sprintf("%.0f", actualPaid),
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
			s.AccountingSvc.AutoPostExpenseJournal(tx, nota, nama, "", expense.Location, total, time.Now())
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

		// Deduplikasi PaymentMethods dan bentuk PaymentMethod gabungan jika multi-payment
		if len(groups[i].PaymentMethods) > 0 {
			var uniquePms []domain.PaymentMethod
			seenID := make(map[uint]bool)
			seenType := make(map[string]bool)
			var distinctTypes []string

			for _, pm := range groups[i].PaymentMethods {
				if pm.ID > 0 {
					if seenID[pm.ID] {
						continue
					}
					seenID[pm.ID] = true
				}
				uniquePms = append(uniquePms, pm)
				t := strings.TrimSpace(pm.Type)
				if t != "" && !seenType[strings.ToLower(t)] {
					seenType[strings.ToLower(t)] = true
					distinctTypes = append(distinctTypes, t)
				}
			}
			groups[i].PaymentMethods = uniquePms
			if len(distinctTypes) > 1 {
				groups[i].PaymentMethod = strings.Join(distinctTypes, ", ")
			}
		}
	}

	return groups
}

// applySalesFilters menerapkan filter query untuk transaksi penjualan
func applySalesFilters(query *gorm.DB, f domain.SalesFilter) *gorm.DB {
	query = query.Where("is_returs = ?", false)

	// 1. Filter Rentang Tanggal
	if f.StartDate != "" && f.EndDate != "" {
		query = query.Where("created_at BETWEEN ? AND ?", f.StartDate+" 00:00:00", f.EndDate+" 23:59:59")
	} else if f.StartDate != "" {
		query = query.Where("created_at >= ?", f.StartDate+" 00:00:00")
	} else if f.EndDate != "" {
		query = query.Where("created_at <= ?", f.EndDate+" 23:59:59")
	}

	// 2. Filter No. Invoice
	if f.Invoice != "" {
		query = query.Where("LOWER(invoice) LIKE ?", "%"+strings.ToLower(strings.TrimSpace(f.Invoice))+"%")
	}

	// 3. Filter Nama Pelanggan / Member
	if f.Member != "" {
		query = query.Where("LOWER(member_name) LIKE ?", "%"+strings.ToLower(strings.TrimSpace(f.Member))+"%")
	}

	// 4. Filter Metode Pembayaran
	if f.Method != "" {
		m := strings.TrimSpace(f.Method)
		if strings.EqualFold(m, "Tunai") || strings.EqualFold(m, "Bayar Tunai") {
			query = query.Where("LOWER(payment_method) LIKE ?", "%tunai%")
		} else if strings.EqualFold(m, "Bon") || strings.EqualFold(m, "Bon / Utang") {
			query = query.Where("is_bon = ? OR LOWER(payment_method) LIKE ?", true, "%bon%")
		} else {
			query = query.Where("LOWER(payment_method) = ?", strings.ToLower(m))
		}
	}

	// 5. Filter Status Pembayaran (Lunas, Belum Lunas / Bon, DP)
	if f.Status != "" {
		st := strings.ToLower(strings.TrimSpace(f.Status))
		switch st {
		case "lunas":
			query = query.Where("status = ? AND is_bon = ? AND is_dp = ?", "Lunas", false, false)
		case "belum lunas", "bon", "piutang":
			query = query.Where("status = ? OR is_bon = ?", "Belum Lunas", true)
		case "dp":
			query = query.Where("is_dp = ?", true)
		}
	}

	// 6. Filter Shift Kerja Kasir
	if f.Shift > 0 {
		query = query.Where("shift = ?", f.Shift)
	} else if f.ShiftStatus == "unclosed" || f.ShiftStatus == "open" || f.ShiftStatus == "belum" {
		query = query.Where("shift IS NULL")
	} else if f.ShiftStatus == "closed" || f.ShiftStatus == "sudah" {
		query = query.Where("shift IS NOT NULL")
	}

	// 7. Filter Kasir Tertentu
	if f.Kasir != "" {
		kasirPattern := "%" + strings.ToLower(strings.TrimSpace(f.Kasir)) + "%"
		query = query.Where("user_id = ? OR user_id IN (SELECT id FROM users WHERE LOWER(name) LIKE ?)", f.Kasir, kasirPattern)
	}

	// 8. Filter Produk Tertentu (Nama atau Kode Produk)
	if f.ProductSearch != "" {
		prodPattern := "%" + strings.ToLower(strings.TrimSpace(f.ProductSearch)) + "%"
		query = query.Where("product_id IN (SELECT id FROM products WHERE LOWER(nama) LIKE ? OR LOWER(kode) LIKE ?)", prodPattern, prodPattern)
	}

	// 9. Pencarian Global / Cepat (Multi-field: Invoice, Member, Kasir, Produk)
	if f.Search != "" {
		searchPattern := "%" + strings.ToLower(strings.TrimSpace(f.Search)) + "%"
		query = query.Where(`(
			LOWER(invoice) LIKE ? 
			OR LOWER(member_name) LIKE ? 
			OR user_id IN (SELECT id FROM users WHERE LOWER(name) LIKE ?)
			OR product_id IN (SELECT id FROM products WHERE LOWER(nama) LIKE ? OR LOWER(kode) LIKE ?)
		)`, searchPattern, searchPattern, searchPattern, searchPattern, searchPattern)
	}

	// 10. Filter Lokasi Toko Cabang
	uLoc := strings.TrimSpace(f.UserLocation)
	if uLoc != "" && !strings.EqualFold(uLoc, "Toko Utama") && !strings.EqualFold(uLoc, "Pusat") {
		query = query.Where("location = ? OR location = ''", uLoc)
	} else {
		if f.Location != "" && !strings.EqualFold(f.Location, "Semua Toko") && !strings.EqualFold(f.Location, "All") {
			query = query.Where("location = ?", f.Location)
		}
	}

	return query
}

// GetPaginatedSales mengambil transaksi penjualan terkelompok per-nota secara terpaginasi (hemat memori & cepat)
func (s *TransactionService) GetPaginatedSales(f domain.SalesFilter) ([]domain.SalesTransactionGroup, int64, error) {
	page := f.Page
	if page < 1 {
		page = 1
	}
	limit := f.Limit
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	var totalInvoices int64
	countQuery := s.DB.Model(&domain.Sales{})
	countQuery = applySalesFilters(countQuery, f)
	if err := countQuery.Select("COUNT(DISTINCT invoice)").Scan(&totalInvoices).Error; err != nil {
		return nil, 0, err
	}

	if totalInvoices == 0 {
		return []domain.SalesTransactionGroup{}, 0, nil
	}

	type invoiceRow struct {
		Invoice string    `gorm:"column:invoice"`
		MaxDate time.Time `gorm:"column:max_date"`
		MaxID   uint      `gorm:"column:max_id"`
	}

	var rows []invoiceRow
	invoiceQuery := s.DB.Model(&domain.Sales{})
	invoiceQuery = applySalesFilters(invoiceQuery, f)
	err := invoiceQuery.
		Select("invoice, MAX(created_at) as max_date, MAX(id) as max_id").
		Group("invoice").
		Order("MAX(created_at) desc, MAX(id) desc").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error

	if err != nil {
		return nil, 0, err
	}

	if len(rows) == 0 {
		return []domain.SalesTransactionGroup{}, totalInvoices, nil
	}

	var invoices []string
	invoiceOrderMap := make(map[string]int)
	for i, r := range rows {
		invoices = append(invoices, r.Invoice)
		invoiceOrderMap[r.Invoice] = i
	}

	var sales []domain.Sales
	err = s.DB.Preload("Product").
		Preload("Customer").
		Preload("User").
		Preload("PaymentMethods").
		Where("invoice IN ?", invoices).
		Where("is_returs = ?", false).
		Order("created_at desc, id desc").
		Find(&sales).Error

	if err != nil {
		return nil, 0, err
	}

	groups := GroupSalesByInvoice(sales)
	sort.SliceStable(groups, func(i, j int) bool {
		return invoiceOrderMap[groups[i].Invoice] < invoiceOrderMap[groups[j].Invoice]
	})

	return groups, totalInvoices, nil
}

func (s *TransactionService) GetAllSales(f domain.SalesFilter) ([]domain.SalesTransactionGroup, error) {
	if f.Limit > 0 {
		groups, _, err := s.GetPaginatedSales(f)
		return groups, err
	}

	var results []domain.Sales
	query := s.DB.Preload("Product").
		Preload("Customer").
		Preload("User").
		Preload("PaymentMethods")

	query = applySalesFilters(query, f)

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

func (s *TransactionService) GetSaleByIdentifier(identifier string) (*domain.Sales, error) {
	var sale domain.Sales
	if id, err := strconv.Atoi(identifier); err == nil && id > 0 {
		if err := s.DB.First(&sale, id).Error; err != nil {
			return nil, err
		}
		return &sale, nil
	}
	if err := s.DB.Where("invoice = ?", identifier).First(&sale).Error; err != nil {
		return nil, err
	}
	return &sale, nil
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

		// Tentukan nama metode pelunasan yang sebenarnya dibayarkan
		var pelunasanMethodName string
		if len(req.PaymentMethods) > 0 && req.PaymentMethods[0].Type != "" {
			pelunasanMethodName = strings.TrimSpace(req.PaymentMethods[0].Type)
		} else if idxStart := strings.Index(req.PaymentMethod, "[Pelunasan:"); idxStart != -1 {
			extracted := req.PaymentMethod[idxStart+len("[Pelunasan:"):]
			if idxEnd := strings.Index(extracted, "]"); idxEnd != -1 {
				pelunasanMethodName = strings.TrimSpace(extracted[:idxEnd])
			}
		}
		if pelunasanMethodName == "" && req.PaymentMethod != "" && !strings.Contains(req.PaymentMethod, "[Pelunasan:") {
			pelunasanMethodName = strings.TrimSpace(req.PaymentMethod)
		}
		if pelunasanMethodName == "" {
			pelunasanMethodName = "Tunai"
		}

		// 1. SIMPAN CATATAN PEMBAYARAN KE TABEL payment_methods DENGAN STATUS "Pelunasan"
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
			record := domain.PaymentMethod{
				SalesID: primarySalesID,
				Type:    pelunasanMethodName,
				Amount:  fmt.Sprintf("%.0f", pelunasanAmount),
				Status:  "Pelunasan",
			}
			if err := tx.Create(&record).Error; err != nil {
				return fmt.Errorf("gagal menyimpan metode pelunasan: %w", err)
			}
		}

		// Kumpulkan seluruh metode pembayaran unik untuk invoice ini (DP awal + Pelunasan)
		var allPms []domain.PaymentMethod
		_ = tx.Where("sales_id IN (SELECT id FROM sales WHERE invoice = ?)", invoice).Find(&allPms).Error
		var distinctMethods []string
		seenMethods := make(map[string]bool)
		for _, pm := range allPms {
			t := strings.TrimSpace(pm.Type)
			if t != "" && !seenMethods[strings.ToLower(t)] {
				seenMethods[strings.ToLower(t)] = true
				distinctMethods = append(distinctMethods, t)
			}
		}
		finalCombinedPaymentMethod := strings.Join(distinctMethods, ", ")
		if finalCombinedPaymentMethod == "" {
			finalCombinedPaymentMethod = pelunasanMethodName
		}

		// 2. UPDATE STATUS SALES & RESET FLAG DP SERTA BON
		updates := map[string]interface{}{
			"status":         status,
			"is_dp":          false,
			"is_bon":         false,
			"payment_method": finalCombinedPaymentMethod,
			"amount_paid":    amountPaid,
			"updated_at":     time.Now(),
		}

		if err := tx.Model(&domain.Sales{}).Where("invoice = ?", invoice).Updates(updates).Error; err != nil {
			return err
		}

		if err := tx.Preload("Product").Preload("Customer").Preload("PaymentMethods").Where("invoice = ?", invoice).Find(&salesList).Error; err != nil {
			return err
		}

		// 3. UPDATE JURNAL & SETTLEMENT PIUTANG (Gunakan pelunasanMethodName agar akurat masuk ke rekening pelunasan)
		if s.AccountingSvc != nil {
			s.AccountingSvc.AutoPostSalesJournal(tx, invoice, pelunasanMethodName, pelunasanAmount, pelunasanAmount, false, time.Now())
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

	// Pastikan kolom shift di tabel payment_methods ada secara aman
	_ = s.DB.Exec("ALTER TABLE payment_methods ADD COLUMN IF NOT EXISTS shift bigint DEFAULT NULL;")

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
	} else if strings.EqualFold(loc, "Toko Utama") {
		salesQuery = salesQuery.Where("(location = ? OR location = '' OR location IS NULL)", "Toko Utama").Where("is_returs = ?", false)
		expenseQuery = expenseQuery.Where("(location = ? OR location = '' OR location IS NULL)", "Toko Utama")
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

	// Ambil data penerimaan Pelunasan yang belum terikat shift
	var pelunasanMethods []domain.PaymentMethod
	pelQuery := s.DB.Preload("Sales").Preload("Sales.Customer").
		Where("payment_methods.status = ?", "Pelunasan").
		Where("payment_methods.shift IS NULL")

	if !strings.EqualFold(loc, "Semua Toko") {
		if strings.EqualFold(loc, "Toko Utama") {
			pelQuery = pelQuery.Where("payment_methods.sales_id IN (SELECT id FROM sales WHERE (location = 'Toko Utama' OR location = '' OR location IS NULL))")
		} else {
			pelQuery = pelQuery.Where("payment_methods.sales_id IN (SELECT id FROM sales WHERE location = ?)", loc)
		}
	}

	if err := pelQuery.Find(&pelunasanMethods).Error; err != nil {
		return nil, err
	}

	pelunasanItems := make([]domain.PelunasanReceiptItem, 0)
	for _, pm := range pelunasanMethods {
		amt, _ := strconv.ParseFloat(pm.Amount, 64)
		custName := "-"
		if pm.Sales.Customer.Name != "" {
		custName = pm.Sales.Customer.Name
		}
		pelunasanItems = append(pelunasanItems, domain.PelunasanReceiptItem{
			ID:            pm.ID,
			SalesID:       pm.SalesID,
			Invoice:       pm.Sales.Invoice,
			CustomerName:  custName,
			Type:          pm.Type,
			PaymentMethod: pm.Type,
			Amount:        amt,
			CreatedAt:     pm.CreatedAt,
		})
	}

	return &domain.ReceiptResponse{
		Category:   "SHIFT_RECEIPT",
		Location:   loc,
		Sales:      sales,
		Expenses:   expenses,
		Pelunasans: pelunasanItems,
	}, nil
}

func (s *TransactionService) UpdateSalesShift(shift uint, location string) error {
	loc := strings.TrimSpace(location)
	if loc == "" || strings.EqualFold(loc, "Pusat") {
		loc = "Toko Utama"
	}

	_ = s.DB.Exec("ALTER TABLE payment_methods ADD COLUMN IF NOT EXISTS shift bigint DEFAULT NULL;")

	return s.DB.Transaction(func(tx *gorm.DB) error {
		// 1. Update shift pada data Sales yang belum memiliki shift
		salesQuery := tx.Model(&domain.Sales{}).Where("shift IS NULL")
		if !strings.EqualFold(loc, "Semua Toko") {
			if strings.EqualFold(loc, "Toko Utama") {
				salesQuery = salesQuery.Where("(location = ? OR location = '' OR location IS NULL)", "Toko Utama")
			} else {
				salesQuery = salesQuery.Where("location = ?", loc)
			}
		}

		if err := salesQuery.Update("shift", shift).Error; err != nil {
			return fmt.Errorf("gagal memperbarui shift sales: %w", err)
		}

		// 2. Update shift pada data Expense (Pengeluaran) yang belum memiliki shift
		expenseQuery := tx.Model(&domain.Expense{}).Where("shift IS NULL")
		if !strings.EqualFold(loc, "Semua Toko") {
			if strings.EqualFold(loc, "Toko Utama") {
				expenseQuery = expenseQuery.Where("(location = ? OR location = '' OR location IS NULL)", "Toko Utama")
			} else {
				expenseQuery = expenseQuery.Where("location = ?", loc)
			}
		}

		if err := expenseQuery.Update("shift", shift).Error; err != nil {
			return fmt.Errorf("gagal memperbarui shift expense: %w", err)
		}

		// 3. Update shift pada data PaymentMethod Pelunasan yang belum memiliki shift
		if strings.EqualFold(loc, "Semua Toko") {
			if err := tx.Exec("UPDATE payment_methods SET shift = ? WHERE status = 'Pelunasan' AND shift IS NULL", shift).Error; err != nil {
				return fmt.Errorf("gagal memperbarui shift pelunasan: %w", err)
			}
		} else if strings.EqualFold(loc, "Toko Utama") {
			if err := tx.Exec("UPDATE payment_methods SET shift = ? WHERE status = 'Pelunasan' AND shift IS NULL AND sales_id IN (SELECT id FROM sales WHERE location = 'Toko Utama' OR location = '' OR location IS NULL)", shift).Error; err != nil {
				return fmt.Errorf("gagal memperbarui shift pelunasan: %w", err)
			}
		} else {
			if err := tx.Exec("UPDATE payment_methods SET shift = ? WHERE status = 'Pelunasan' AND shift IS NULL AND sales_id IN (SELECT id FROM sales WHERE location = ?)", shift, loc).Error; err != nil {
				return fmt.Errorf("gagal memperbarui shift pelunasan: %w", err)
			}
		}

		return nil
	})
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
					_ = AdjustProductStock(tx, prod.ID, item.Location, item.Qty, "SALE_ROLLBACK", invoice, fmt.Sprintf("Pembatalan Penjualan (#%s)", invoice), "SYSTEM")
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

// UndoShiftByInvoice membatalkan status tutup shift untuk nota transaksi tertentu (shift diset kembali ke NULL)
func (s *TransactionService) UndoShiftByInvoice(invoice string) error {
	invoice = strings.TrimSpace(invoice)
	if invoice == "" {
		return errors.New("nomor invoice tidak boleh kosong")
	}

	return s.DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&domain.Sales{}).Where("invoice = ?", invoice).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return errors.New("transaksi dengan nomor invoice tersebut tidak ditemukan")
		}

		// 1. Reset shift pada sales transaksi ini menjadi NULL
		if err := tx.Exec("UPDATE sales SET shift = NULL WHERE invoice = ?", invoice).Error; err != nil {
			return fmt.Errorf("gagal membatalkan shift transaksi: %w", err)
		}

		// 2. Reset shift pada payment_methods terkait transaksi ini menjadi NULL
		if err := tx.Exec("UPDATE payment_methods SET shift = NULL WHERE sales_id IN (SELECT id FROM sales WHERE invoice = ?)", invoice).Error; err != nil {
			return fmt.Errorf("gagal membatalkan shift pembayaran: %w", err)
		}

		return nil
	})
}

// GetShiftSummaryByDate mengambil daftar shift yang sudah ditutup pada tanggal dan lokasi tertentu
func (s *TransactionService) GetShiftSummaryByDate(dateStr, location string) ([]domain.ShiftSummaryItem, error) {
	loc := strings.TrimSpace(location)
	if loc == "" || strings.EqualFold(loc, "Pusat") {
		loc = "Toko Utama"
	}
	parsedDate := strings.TrimSpace(dateStr)
	if parsedDate == "" {
		parsedDate = time.Now().Format("2006-01-02")
	}
	startDay := parsedDate + " 00:00:00"
	endDay := parsedDate + " 23:59:59"

	type shiftRow struct {
		Shift      uint    `gorm:"column:shift"`
		TotalSales int64   `gorm:"column:total_sales"`
		TotalNetto float64 `gorm:"column:total_netto"`
	}
	var rows []shiftRow

	query := s.DB.Table("sales").
		Select("shift, COUNT(DISTINCT invoice) as total_sales, COALESCE(SUM(total_netto), 0) as total_netto").
		Where("created_at BETWEEN ? AND ? AND shift IS NOT NULL AND shift > 0", startDay, endDay)

	if !strings.EqualFold(loc, "Semua Toko") {
		query = query.Where("(location = ? OR (location = '' AND ? = 'Toko Utama'))", loc, loc)
	}

	if err := query.Group("shift").Order("shift ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}

	// Ambil juga pengeluaran per shift
	type expenseRow struct {
		Shift        uint    `gorm:"column:shift"`
		TotalExpense float64 `gorm:"column:total_expense"`
	}
	var expRows []expenseRow
	expQuery := s.DB.Table("expenses").
		Select("shift, COALESCE(SUM(total), 0) as total_expense").
		Where("tanggal BETWEEN ? AND ? AND shift IS NOT NULL AND shift > 0", startDay, endDay)

	if !strings.EqualFold(loc, "Semua Toko") {
		expQuery = expQuery.Where("(location = ? OR (location = '' AND ? = 'Toko Utama'))", loc, loc)
	}
	_ = expQuery.Group("shift").Scan(&expRows)
	expMap := make(map[uint]float64)
	for _, er := range expRows {
		expMap[er.Shift] = er.TotalExpense
	}

	result := make([]domain.ShiftSummaryItem, 0, len(rows))
	for _, r := range rows {
		result = append(result, domain.ShiftSummaryItem{
			Shift:        r.Shift,
			TotalSales:   r.TotalSales,
			TotalNetto:   r.TotalNetto,
			TotalExpense: expMap[r.Shift],
		})
	}

	return result, nil
}

// UndoShiftBatch membatalkan seluruh tutup shift pada tanggal, lokasi, dan shift tertentu (diset kembali ke NULL)
func (s *TransactionService) UndoShiftBatch(dateStr string, shift uint, location string) (int64, error) {
	if shift == 0 {
		return 0, errors.New("nomor shift tidak valid")
	}
	loc := strings.TrimSpace(location)
	if loc == "" || strings.EqualFold(loc, "Pusat") {
		loc = "Toko Utama"
	}
	parsedDate := strings.TrimSpace(dateStr)
	if parsedDate == "" {
		parsedDate = time.Now().Format("2006-01-02")
	}
	startDay := parsedDate + " 00:00:00"
	endDay := parsedDate + " 23:59:59"

	var affectedSales int64
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		// 1. Reset shift pada sales
		var salesSQL string
		var salesArgs []interface{}
		if strings.EqualFold(loc, "Semua Toko") {
			salesSQL = "UPDATE sales SET shift = NULL WHERE shift = ? AND created_at BETWEEN ? AND ?"
			salesArgs = []interface{}{shift, startDay, endDay}
		} else if strings.EqualFold(loc, "Toko Utama") {
			salesSQL = "UPDATE sales SET shift = NULL WHERE shift = ? AND created_at BETWEEN ? AND ? AND (location = 'Toko Utama' OR location = '' OR location IS NULL)"
			salesArgs = []interface{}{shift, startDay, endDay}
		} else {
			salesSQL = "UPDATE sales SET shift = NULL WHERE shift = ? AND created_at BETWEEN ? AND ? AND location = ?"
			salesArgs = []interface{}{shift, startDay, endDay, loc}
		}
		res := tx.Exec(salesSQL, salesArgs...)
		if res.Error != nil {
			return fmt.Errorf("gagal membatalkan shift penjualan: %w", res.Error)
		}
		affectedSales = res.RowsAffected

		// 2. Reset shift pada payment_methods (pelunasan)
		if strings.EqualFold(loc, "Semua Toko") {
			if err := tx.Exec("UPDATE payment_methods SET shift = NULL WHERE shift = ? AND created_at BETWEEN ? AND ?", shift, startDay, endDay).Error; err != nil {
				return fmt.Errorf("gagal membatalkan shift pelunasan: %w", err)
			}
		} else if strings.EqualFold(loc, "Toko Utama") {
			if err := tx.Exec("UPDATE payment_methods SET shift = NULL WHERE shift = ? AND created_at BETWEEN ? AND ? AND sales_id IN (SELECT id FROM sales WHERE location = 'Toko Utama' OR location = '' OR location IS NULL)", shift, startDay, endDay).Error; err != nil {
				return fmt.Errorf("gagal membatalkan shift pelunasan: %w", err)
			}
		} else {
			if err := tx.Exec("UPDATE payment_methods SET shift = NULL WHERE shift = ? AND created_at BETWEEN ? AND ? AND sales_id IN (SELECT id FROM sales WHERE location = ?)", shift, startDay, endDay, loc).Error; err != nil {
				return fmt.Errorf("gagal membatalkan shift pelunasan: %w", err)
			}
		}

		// 3. Reset shift pada expenses (menggunakan kolom tanggal)
		if strings.EqualFold(loc, "Semua Toko") {
			if err := tx.Exec("UPDATE expenses SET shift = NULL WHERE shift = ? AND tanggal BETWEEN ? AND ?", shift, startDay, endDay).Error; err != nil {
				return fmt.Errorf("gagal membatalkan shift pengeluaran: %w", err)
			}
		} else if strings.EqualFold(loc, "Toko Utama") {
			if err := tx.Exec("UPDATE expenses SET shift = NULL WHERE shift = ? AND tanggal BETWEEN ? AND ? AND (location = 'Toko Utama' OR location = '' OR location IS NULL)", shift, startDay, endDay).Error; err != nil {
				return fmt.Errorf("gagal membatalkan shift pengeluaran: %w", err)
			}
		} else {
			if err := tx.Exec("UPDATE expenses SET shift = NULL WHERE shift = ? AND tanggal BETWEEN ? AND ? AND location = ?", shift, startDay, endDay, loc).Error; err != nil {
				return fmt.Errorf("gagal membatalkan shift pengeluaran: %w", err)
			}
		}

		return nil
	})

	return affectedSales, err
}
