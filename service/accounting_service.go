package service

import (
	"backend-cashier/domain"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

type AccountingService struct {
	DB *gorm.DB
}

func NewAccountingService(db *gorm.DB) *AccountingService {
	svc := &AccountingService{DB: db}
	svc.SeedDefaultAccounts()
	return svc
}

// SeedDefaultAccounts menginisialisasi 34 akun standar sesuai skema file Excel Buku Harian & Buku Besar
func (s *AccountingService) SeedDefaultAccounts() {
	var count int64
	s.DB.Model(&domain.Account{}).Count(&count)
	if count == 0 {
		defaultAccounts := []domain.Account{
			// --- ASSETS (ASET LANCAR & KAS/BANK) ---
			{Code: "1101", Name: "KAS", Type: "ASSET", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "1102", Name: "BCA", Type: "ASSET", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "1103", Name: "BRI 4560", Type: "ASSET", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "1104", Name: "BRI 5563", Type: "ASSET", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "1105", Name: "BNI 3815", Type: "ASSET", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "1106", Name: "BNI 9442", Type: "ASSET", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "1107", Name: "MANDIRI", Type: "ASSET", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "1108", Name: "PPN MASUKAN", Type: "ASSET", NormalPosition: "DEBET", InitialBalance: 0},

			// --- LIABILITIES (KEWAJIBAN & HUTANG) ---
			{Code: "2101", Name: "UTANG DAGANG", Type: "LIABILITY", NormalPosition: "KREDIT", InitialBalance: 0},
			{Code: "2102", Name: "HUTANG BANK BNI 6121", Type: "LIABILITY", NormalPosition: "KREDIT", InitialBalance: 0},
			{Code: "2103", Name: "PPN KELUARAN", Type: "LIABILITY", NormalPosition: "KREDIT", InitialBalance: 0},

			// --- EQUITY (EKUITAS & MODAL) ---
			{Code: "3101", Name: "MODAL PEMILIK", Type: "EQUITY", NormalPosition: "KREDIT", InitialBalance: 0},
			{Code: "3102", Name: "PRIVE", Type: "EQUITY", NormalPosition: "DEBET", InitialBalance: 0},

			// --- REVENUE (PENDAPATAN) ---
			{Code: "4101", Name: "PENJUALAN", Type: "REVENUE", NormalPosition: "KREDIT", InitialBalance: 0},
			{Code: "4102", Name: "PENDAPATAN INSENTIF", Type: "REVENUE", NormalPosition: "KREDIT", InitialBalance: 0},
			{Code: "4103", Name: "JASA GIRO", Type: "REVENUE", NormalPosition: "KREDIT", InitialBalance: 0},

			// --- EXPENSES (HPP & BIAYA OPERASIONAL) ---
			{Code: "5101", Name: "PEMBELIAN", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "5102", Name: "PEMBELIAN YANG TIDAK DIKREDITKAN", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6101", Name: "GAJI PEGAWAI", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6102", Name: "BIAYA LISTRIK", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6103", Name: "BIAYA ADMINISTRASI BANK", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6104", Name: "BIAYA BAHAN BAKAR", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6105", Name: "BIAYA KONSUMSI", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6106", Name: "BIAYA KEPERLUAN TOKO", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6107", Name: "BIAYA SAMSAT KENDARAAN", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6108", Name: "BIAYA EKSPEDISI", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6109", Name: "BIAYA INTERNET", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6110", Name: "IURAN SAMPAH", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6111", Name: "BIAYA OPERASIONAL KENDARAAN", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6112", Name: "BIAYA TELKOM", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6113", Name: "BIAYA BUNGA", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6114", Name: "BIAYA SUMBANGAN", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6115", Name: "PPH 21", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6116", Name: "PPH PS 23 FINAL", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
			{Code: "6199", Name: "BIAYA LAIN LAIN", Type: "EXPENSE", NormalPosition: "DEBET", InitialBalance: 0},
		}

		for _, acc := range defaultAccounts {
			s.DB.FirstOrCreate(&acc, domain.Account{Name: acc.Name})
		}
	}
}

// -------------------------------------------------------------
// ACCOUNT CRUD
// -------------------------------------------------------------

func (s *AccountingService) GetAllAccounts() ([]domain.Account, error) {
	var accounts []domain.Account
	if err := s.DB.Order("code asc, name asc").Find(&accounts).Error; err != nil {
		return nil, err
	}

	for i := range accounts {
		var debetSum, kreditSum float64
		s.DB.Model(&domain.JournalItem{}).Where("account_id = ?", accounts[i].ID).Select("COALESCE(SUM(debet), 0)").Scan(&debetSum)
		s.DB.Model(&domain.JournalItem{}).Where("account_id = ?", accounts[i].ID).Select("COALESCE(SUM(kredit), 0)").Scan(&kreditSum)

		if accounts[i].NormalPosition == "DEBET" {
			accounts[i].CurrentBalance = accounts[i].InitialBalance + debetSum - kreditSum
		} else {
			accounts[i].CurrentBalance = accounts[i].InitialBalance + kreditSum - debetSum
		}
	}

	return accounts, nil
}

func (s *AccountingService) CreateAccount(acc domain.Account) (*domain.Account, error) {
	if acc.Name == "" {
		return nil, errors.New("nama akun wajib diisi")
	}
	if acc.Code == "" {
		acc.Code = fmt.Sprintf("ACC-%d", time.Now().UnixNano()%100000)
	}
	if acc.NormalPosition == "" {
		if acc.Type == "LIABILITY" || acc.Type == "EQUITY" || acc.Type == "REVENUE" {
			acc.NormalPosition = "KREDIT"
		} else {
			acc.NormalPosition = "DEBET"
		}
	}

	if err := s.DB.Create(&acc).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

func (s *AccountingService) GetAccountByID(id uint) (*domain.Account, error) {
	var acc domain.Account
	if err := s.DB.First(&acc, id).Error; err != nil {
		return nil, errors.New("akun tidak ditemukan")
	}
	return &acc, nil
}

func (s *AccountingService) UpdateAccount(id uint, input domain.Account) (*domain.Account, error) {
	var acc domain.Account
	if err := s.DB.First(&acc, id).Error; err != nil {
		return nil, errors.New("akun tidak ditemukan")
	}

	updates := map[string]interface{}{}
	if input.Name != "" {
		updates["name"] = input.Name
	}
	if input.Code != "" {
		updates["code"] = input.Code
	}
	if input.Type != "" {
		updates["type"] = input.Type
	}
	if input.NormalPosition != "" {
		updates["normal_position"] = input.NormalPosition
	}
	updates["initial_balance"] = input.InitialBalance

	if err := s.DB.Model(&acc).Updates(updates).Error; err != nil {
		return nil, err
	}

	s.DB.First(&acc, id)
	return &acc, nil
}

func (s *AccountingService) DeleteAccount(id uint) error {
	var count int64
	s.DB.Model(&domain.JournalItem{}).Where("account_id = ?", id).Count(&count)
	if count > 0 {
		return errors.New("akun tidak dapat dihapus karena sudah memiliki catatan transaksi jurnal")
	}

	return s.DB.Delete(&domain.Account{}, id).Error
}

// -------------------------------------------------------------
// BUKU HARIAN (JOURNAL ENTRIES)
// -------------------------------------------------------------

func (s *AccountingService) CreateJournalEntry(req domain.CreateJournalRequest) (*domain.JournalEntry, error) {
	if len(req.Items) < 2 {
		return nil, errors.New("jurnal harus memiliki minimal 2 baris (Debet dan Kredit)")
	}

	var entryDate time.Time
	var err error
	if req.Date != "" {
		entryDate, err = time.Parse("2006-01-02", req.Date)
		if err != nil {
			entryDate, _ = time.Parse(time.RFC3339, req.Date)
		}
	}
	if entryDate.IsZero() {
		entryDate = time.Now()
	}

	entryNumber := fmt.Sprintf("JV-%s-%03d", entryDate.Format("20060102"), time.Now().UnixNano()%1000)

	var totalDebet, totalKredit float64
	var journalItems []domain.JournalItem

	for _, itemReq := range req.Items {
		var accountID = itemReq.AccountID
		if accountID == 0 && itemReq.AccountName != "" {
			var acc domain.Account
			err := s.DB.Where("LOWER(name) = ?", strings.ToLower(itemReq.AccountName)).First(&acc).Error
			if err == nil {
				accountID = acc.ID
			} else {
				acc = domain.Account{
					Code:           fmt.Sprintf("ACC-%d", time.Now().UnixNano()%10000),
					Name:           itemReq.AccountName,
					Type:           "EXPENSE",
					NormalPosition: "DEBET",
				}
				s.DB.Create(&acc)
				accountID = acc.ID
			}
		}

		if accountID == 0 {
			return nil, errors.New("setiap baris jurnal harus memiliki akun yang valid")
		}

		totalDebet += itemReq.Debet
		totalKredit += itemReq.Kredit

		journalItems = append(journalItems, domain.JournalItem{
			AccountID:   accountID,
			Description: itemReq.Description,
			Debet:       itemReq.Debet,
			Kredit:      itemReq.Kredit,
		})
	}

	diff := totalDebet - totalKredit
	if diff < -0.01 || diff > 0.01 {
		return nil, fmt.Errorf("jurnal tidak seimbang! Total Debet (IDR %.2f) != Total Kredit (IDR %.2f), Selisih: IDR %.2f", totalDebet, totalKredit, diff)
	}

	entry := domain.JournalEntry{
		EntryNumber: entryNumber,
		Date:        entryDate,
		Description: req.Description,
		Reference:   req.Reference,
		Items:       journalItems,
	}

	err = s.DB.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&entry).Error
	})
	if err != nil {
		return nil, err
	}

	s.DB.Preload("Items.Account").First(&entry, entry.ID)
	return &entry, nil
}

func (s *AccountingService) UpdateJournalEntry(id uint, req domain.CreateJournalRequest) (*domain.JournalEntry, error) {
	var entry domain.JournalEntry
	if err := s.DB.Preload("Items").First(&entry, id).Error; err != nil {
		return nil, errors.New("jurnal transaksi tidak ditemukan")
	}

	if len(req.Items) < 2 {
		return nil, errors.New("jurnal harus memiliki minimal 2 baris (Debet dan Kredit)")
	}

	var entryDate time.Time
	var err error
	if req.Date != "" {
		entryDate, err = time.Parse("2006-01-02", req.Date)
		if err != nil {
			entryDate, _ = time.Parse(time.RFC3339, req.Date)
		}
	}
	if entryDate.IsZero() {
		entryDate = entry.Date
	}

	var totalDebet, totalKredit float64
	var newItems []domain.JournalItem

	for _, itemReq := range req.Items {
		var accountID = itemReq.AccountID
		if accountID == 0 && itemReq.AccountName != "" {
			var acc domain.Account
			if err := s.DB.Where("LOWER(name) = ?", strings.ToLower(itemReq.AccountName)).First(&acc).Error; err == nil {
				accountID = acc.ID
			}
		}

		if accountID == 0 {
			return nil, errors.New("setiap baris jurnal harus memiliki akun yang valid")
		}

		totalDebet += itemReq.Debet
		totalKredit += itemReq.Kredit

		newItems = append(newItems, domain.JournalItem{
			JournalEntryID: entry.ID,
			AccountID:      accountID,
			Description:    itemReq.Description,
			Debet:          itemReq.Debet,
			Kredit:         itemReq.Kredit,
		})
	}

	diff := totalDebet - totalKredit
	if diff < -0.01 || diff > 0.01 {
		return nil, fmt.Errorf("jurnal tidak seimbang! Total Debet (IDR %.2f) != Total Kredit (IDR %.2f), Selisih: IDR %.2f", totalDebet, totalKredit, diff)
	}

	err = s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&entry).Updates(map[string]interface{}{
			"date":        entryDate,
			"description": req.Description,
			"reference":   req.Reference,
		}).Error; err != nil {
			return err
		}

		if err := tx.Where("journal_entry_id = ?", entry.ID).Delete(&domain.JournalItem{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&newItems).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	s.DB.Preload("Items.Account").First(&entry, entry.ID)
	return &entry, nil
}

func (s *AccountingService) GetAllJournalEntries(f domain.JournalFilter) ([]domain.JournalEntry, int64, error) {
	var entries []domain.JournalEntry
	var total int64

	query := s.DB.Model(&domain.JournalEntry{}).Preload("Items.Account")

	if f.StartDate != "" && f.EndDate != "" {
		query = query.Where("date BETWEEN ? AND ?", f.StartDate+" 00:00:00", f.EndDate+" 23:59:59")
	}
	if f.Search != "" {
		pattern := "%" + strings.ToLower(f.Search) + "%"
		query = query.Where("LOWER(description) LIKE ? OR LOWER(reference) LIKE ? OR LOWER(entry_number) LIKE ?", pattern, pattern, pattern)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := f.Page
	if page < 1 {
		page = 1
	}
	limit := f.Limit
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	err := query.Order("date desc, id desc").Limit(limit).Offset(offset).Find(&entries).Error
	return entries, total, err
}

func (s *AccountingService) GetJournalEntryByID(id uint) (*domain.JournalEntry, error) {
	var entry domain.JournalEntry
	if err := s.DB.Preload("Items.Account").First(&entry, id).Error; err != nil {
		return nil, errors.New("jurnal transaksi tidak ditemukan")
	}
	return &entry, nil
}

func (s *AccountingService) DeleteJournalEntry(id uint) error {
	var entry domain.JournalEntry
	if err := s.DB.First(&entry, id).Error; err != nil {
		return errors.New("jurnal transaksi tidak ditemukan")
	}
	return s.DB.Select("Items").Delete(&entry).Error
}

// -------------------------------------------------------------
// REKAPITULASI BUKU HARIAN
// -------------------------------------------------------------

func (s *AccountingService) GetRekapitulasi(startDate, endDate string) (*domain.AccountingRecapResponse, error) {
	if startDate == "" || endDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	}

	type QueryResult struct {
		AccountID   uint    `json:"account_id"`
		AccountCode string  `json:"account_code"`
		AccountName string  `json:"account_name"`
		AccountType string  `json:"account_type"`
		TotalDebet  float64 `json:"total_debet"`
		TotalKredit float64 `json:"total_kredit"`
	}

	var rawResults []QueryResult
	err := s.DB.Table("journal_items").
		Select("accounts.id as account_id, accounts.code as account_code, accounts.name as account_name, accounts.type as account_type, SUM(journal_items.debet) as total_debet, SUM(journal_items.kredit) as total_kredit").
		Joins("JOIN journal_entries ON journal_entries.id = journal_items.journal_entry_id").
		Joins("JOIN accounts ON accounts.id = journal_items.account_id").
		Where("journal_entries.date BETWEEN ? AND ?", startDate+" 00:00:00", endDate+" 23:59:59").
		Where("journal_entries.deleted_at IS NULL").
		Group("accounts.id, accounts.code, accounts.name, accounts.type").
		Order("accounts.code asc, accounts.name asc").
		Scan(&rawResults).Error

	if err != nil {
		return nil, err
	}

	var items []domain.AccountingRecapItem
	var grandTotalDebet, grandTotalKredit float64

	for _, r := range rawResults {
		grandTotalDebet += r.TotalDebet
		grandTotalKredit += r.TotalKredit
		items = append(items, domain.AccountingRecapItem{
			AccountID:   r.AccountID,
			AccountCode: r.AccountCode,
			AccountName: r.AccountName,
			AccountType: r.AccountType,
			TotalDebet:  r.TotalDebet,
			TotalKredit: r.TotalKredit,
		})
	}

	diff := grandTotalDebet - grandTotalKredit
	isBalanced := diff > -0.01 && diff < 0.01

	return &domain.AccountingRecapResponse{
		StartDate:   startDate,
		EndDate:     endDate,
		Items:       items,
		TotalDebet:  grandTotalDebet,
		TotalKredit: grandTotalKredit,
		Difference:  diff,
		IsBalanced:  isBalanced,
	}, nil
}

// -------------------------------------------------------------
// BUKU BESAR (GENERAL LEDGER)
// -------------------------------------------------------------

func (s *AccountingService) GetGeneralLedger(startDate, endDate string, accountID uint) ([]domain.LedgerAccountCard, error) {
	if startDate == "" || endDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	}

	accountQuery := s.DB.Model(&domain.Account{})
	if accountID > 0 {
		accountQuery = accountQuery.Where("id = ?", accountID)
	}
	var accounts []domain.Account
	if err := accountQuery.Order("code asc, name asc").Find(&accounts).Error; err != nil {
		return nil, err
	}

	// 1. Batch query Saldo Awal (sebelum startDate) untuk semua akun sekaligus
	type PrevBalance struct {
		AccountID  uint    `json:"account_id"`
		PrevDebet  float64 `json:"prev_debet"`
		PrevKredit float64 `json:"prev_kredit"`
	}
	var prevList []PrevBalance
	s.DB.Table("journal_items").
		Select("journal_items.account_id, COALESCE(SUM(journal_items.debet), 0) as prev_debet, COALESCE(SUM(journal_items.kredit), 0) as prev_kredit").
		Joins("JOIN journal_entries ON journal_entries.id = journal_items.journal_entry_id").
		Where("journal_entries.date < ?", startDate+" 00:00:00").
		Where("journal_entries.deleted_at IS NULL").
		Group("journal_items.account_id").
		Scan(&prevList)

	prevMap := make(map[uint]PrevBalance)
	for _, p := range prevList {
		prevMap[p.AccountID] = p
	}

	// 2. Batch query semua transaksi jurnal pada periode tanggal sekaligus
	type ItemRow struct {
		ID          uint      `json:"id"`
		AccountID   uint      `json:"account_id"`
		Date        time.Time `json:"date"`
		EntryNumber string    `json:"entry_number"`
		Description string    `json:"description"`
		Reference   string    `json:"reference"`
		Debet       float64   `json:"debet"`
		Kredit      float64   `json:"kredit"`
	}
	var allRows []ItemRow
	s.DB.Table("journal_items").
		Select("journal_items.id, journal_items.account_id, journal_entries.date, journal_entries.entry_number, journal_items.description, journal_entries.reference, journal_items.debet, journal_items.kredit").
		Joins("JOIN journal_entries ON journal_entries.id = journal_items.journal_entry_id").
		Where("journal_entries.date BETWEEN ? AND ?", startDate+" 00:00:00", endDate+" 23:59:59").
		Where("journal_entries.deleted_at IS NULL").
		Order("journal_entries.date asc, journal_entries.id asc").
		Scan(&allRows)

	rowsMap := make(map[uint][]ItemRow)
	for _, r := range allRows {
		rowsMap[r.AccountID] = append(rowsMap[r.AccountID], r)
	}

	var cards []domain.LedgerAccountCard

	for _, acc := range accounts {
		prev := prevMap[acc.ID]
		var initBalance float64
		if acc.NormalPosition == "DEBET" {
			initBalance = acc.InitialBalance + prev.PrevDebet - prev.PrevKredit
		} else {
			initBalance = acc.InitialBalance + prev.PrevKredit - prev.PrevDebet
		}

		rows := rowsMap[acc.ID]
		var txItems []domain.LedgerTransactionItem
		currentBal := initBalance
		var totalDebet, totalKredit float64

		for _, r := range rows {
			totalDebet += r.Debet
			totalKredit += r.Kredit

			if acc.NormalPosition == "DEBET" {
				currentBal = currentBal + r.Debet - r.Kredit
			} else {
				currentBal = currentBal + r.Kredit - r.Debet
			}

			desc := r.Description
			if desc == "" {
				desc = r.EntryNumber
			}

			txItems = append(txItems, domain.LedgerTransactionItem{
				ID:          r.ID,
				Date:        r.Date,
				EntryNumber: r.EntryNumber,
				Description: desc,
				Reference:   r.Reference,
				Debet:       r.Debet,
				Kredit:      r.Kredit,
				Balance:     currentBal,
			})
		}

		cards = append(cards, domain.LedgerAccountCard{
			AccountID:      acc.ID,
			AccountCode:    acc.Code,
			AccountName:    acc.Name,
			AccountType:    acc.Type,
			NormalPosition: acc.NormalPosition,
			InitialBalance: initBalance,
			TotalDebet:     totalDebet,
			TotalKredit:    totalKredit,
			EndingBalance:  currentBal,
			Transactions:   txItems,
		})
	}

	return cards, nil
}

// -------------------------------------------------------------
// AUTOMATIC JOURNAL POSTING HELPERS (SEAMLESS DOUBLE ENTRY)
// -------------------------------------------------------------

func (s *AccountingService) AutoPostSalesJournal(tx *gorm.DB, invoice string, paymentMethod string, totalNetto, amountPaid float64, isDp bool, date time.Time) {
	if tx == nil {
		tx = s.DB
	}

	debetAccountName := "KAS"
	methodUpper := strings.ToUpper(paymentMethod)
	if strings.Contains(methodUpper, "BCA") {
		debetAccountName = "BCA"
	} else if strings.Contains(methodUpper, "BRI 4560") {
		debetAccountName = "BRI 4560"
	} else if strings.Contains(methodUpper, "BRI") {
		debetAccountName = "BRI 5563"
	} else if strings.Contains(methodUpper, "BNI 3815") || strings.Contains(methodUpper, "BNI") {
		debetAccountName = "BNI 3815"
	} else if strings.Contains(methodUpper, "MANDIRI") {
		debetAccountName = "MANDIRI"
	}

	var debetAcc, salesAcc, ppnAcc domain.Account
	tx.Where("LOWER(name) = ?", strings.ToLower(debetAccountName)).First(&debetAcc)
	tx.Where("LOWER(name) = ?", "penjualan").First(&salesAcc)
	tx.Where("LOWER(name) = ?", "ppn keluaran").First(&ppnAcc)

	if debetAcc.ID == 0 || salesAcc.ID == 0 {
		return
	}

	entryNumber := fmt.Sprintf("JV-SALES-%s", invoice)

	var existing domain.JournalEntry
	if err := tx.Where("entry_number = ?", entryNumber).First(&existing).Error; err == nil {
		return
	}

	paid := totalNetto
	if isDp && amountPaid > 0 {
		paid = amountPaid
	}

	// 4. Perhitungan PPN Keluaran (11%) & DPP Penjualan Dibulatkan:
	// Contoh Kas 100.000 -> Penjualan (Kas / 1.11) = 90.090, PPN Keluaran = 9.910
	dppPenjualan := math.Round(paid / 1.11)
	ppnKeluaran := paid - dppPenjualan

	items := []domain.JournalItem{
		{
			AccountID:   debetAcc.ID,
			Description: fmt.Sprintf("Penerimaan %s", debetAccountName),
			Debet:       paid,
			Kredit:      0,
		},
		{
			AccountID:   salesAcc.ID,
			Description: "Pendapatan Penjualan",
			Debet:       0,
			Kredit:      dppPenjualan,
		},
	}

	if ppnAcc.ID > 0 && ppnKeluaran > 0 {
		items = append(items, domain.JournalItem{
			AccountID:   ppnAcc.ID,
			Description: "PPN Keluaran (11%)",
			Debet:       0,
			Kredit:      ppnKeluaran,
		})
	} else {
		items[1].Kredit = paid
	}

	entry := domain.JournalEntry{
		EntryNumber: entryNumber,
		Date:        date,
		Description: fmt.Sprintf("PENJUALAN NOTA %s (%s)", invoice, paymentMethod),
		Reference:   invoice,
		Items:       items,
	}

	_ = tx.Create(&entry).Error
}

func (s *AccountingService) AutoPostExpenseJournal(tx *gorm.DB, nota, deskripsi, kategori string, total float64, date time.Time) {
	if tx == nil {
		tx = s.DB
	}

	var kasAcc domain.Account
	tx.Where("LOWER(name) = ?", "kas").First(&kasAcc)

	expenseAccountName := "BIAYA LAIN LAIN"
	descUpper := strings.ToUpper(deskripsi + " " + kategori)

	if strings.Contains(descUpper, "LISTRIK") || strings.Contains(descUpper, "PLN") {
		expenseAccountName = "BIAYA LISTRIK"
	} else if strings.Contains(descUpper, "GAJI") || strings.Contains(descUpper, "UPAH") {
		expenseAccountName = "GAJI PEGAWAI"
	} else if strings.Contains(descUpper, "BBM") || strings.Contains(descUpper, "BAHAN BAKAR") || strings.Contains(descUpper, "BENSIN") {
		expenseAccountName = "BIAYA BAHAN BAKAR"
	} else if strings.Contains(descUpper, "KONSUMSI") || strings.Contains(descUpper, "MAKAN") || strings.Contains(descUpper, "MINUM") {
		expenseAccountName = "BIAYA KONSUMSI"
	} else if strings.Contains(descUpper, "SAMSAT") || strings.Contains(descUpper, "PAJAK KENDARAAN") {
		expenseAccountName = "BIAYA SAMSAT KENDARAAN"
	} else if strings.Contains(descUpper, "EKSPEDISI") || strings.Contains(descUpper, "ONGKIR") || strings.Contains(descUpper, "ANGKUT") {
		expenseAccountName = "BIAYA EKSPEDISI"
	} else if strings.Contains(descUpper, "INTERNET") || strings.Contains(descUpper, "WIFI") || strings.Contains(descUpper, "INDIHOME") {
		expenseAccountName = "BIAYA INTERNET"
	} else if strings.Contains(descUpper, "TELKOM") || strings.Contains(descUpper, "PULSA") {
		expenseAccountName = "BIAYA TELKOM"
	} else if strings.Contains(descUpper, "SAMPAH") || strings.Contains(descUpper, "KEBERSIHAN") {
		expenseAccountName = "IURAN SAMPAH"
	} else if strings.Contains(descUpper, "BUNGA") {
		expenseAccountName = "BIAYA BUNGA"
	} else if strings.Contains(descUpper, "ADMIN") {
		expenseAccountName = "BIAYA ADMINISTRASI BANK"
	} else if strings.Contains(descUpper, "TOKO") || strings.Contains(descUpper, "PLASTIK") || strings.Contains(descUpper, "KERTAS") || strings.Contains(descUpper, "ATK") {
		expenseAccountName = "BIAYA KEPERLUAN TOKO"
	}

	var expAcc domain.Account
	tx.Where("LOWER(name) = ?", strings.ToLower(expenseAccountName)).First(&expAcc)
	if expAcc.ID == 0 {
		expAcc = kasAcc
	}

	entryNumber := fmt.Sprintf("JV-EXP-%s-%d", nota, time.Now().UnixNano()%1000)
	entry := domain.JournalEntry{
		EntryNumber: entryNumber,
		Date:        date,
		Description: fmt.Sprintf("BIAYA OPERASIONAL: %s", deskripsi),
		Reference:   nota,
		Items: []domain.JournalItem{
			{
				AccountID:   expAcc.ID,
				Description: deskripsi,
				Debet:       total,
				Kredit:      0,
			},
			{
				AccountID:   kasAcc.ID,
				Description: "Pengeluaran Kas",
				Debet:       0,
				Kredit:      total,
			},
		},
	}
	_ = tx.Create(&entry).Error
}

func (s *AccountingService) AutoPostPurchaseJournal(tx *gorm.DB, nota string, total float64, date time.Time) {
	if tx == nil {
		tx = s.DB
	}

	var buyAcc, kasAcc domain.Account
	tx.Where("LOWER(name) = ?", "pembelian").First(&buyAcc)
	tx.Where("LOWER(name) = ?", "kas").First(&kasAcc)

	if buyAcc.ID == 0 || kasAcc.ID == 0 {
		return
	}

	entryNumber := fmt.Sprintf("JV-BUY-%s-%d", nota, time.Now().UnixNano()%1000)
	entry := domain.JournalEntry{
		EntryNumber: entryNumber,
		Date:        date,
		Description: fmt.Sprintf("PEMBELIAN STOK BARANG NOTA %s", nota),
		Reference:   nota,
		Items: []domain.JournalItem{
			{
				AccountID:   buyAcc.ID,
				Description: "Pembelian Barang Dagang",
				Debet:       total,
				Kredit:      0,
			},
			{
				AccountID:   kasAcc.ID,
				Description: "Kas Pembelian",
				Debet:       0,
				Kredit:      total,
			},
		},
	}
	_ = tx.Create(&entry).Error
}

// -------------------------------------------------------------
// PIUTANG DAGANG (ACCOUNTS RECEIVABLE)
// -------------------------------------------------------------

func (s *AccountingService) GetAllPiutang(f domain.PiutangFilter) ([]domain.PiutangDagang, int64, error) {
	var records []domain.PiutangDagang
	var total int64

	query := s.DB.Model(&domain.PiutangDagang{}).Preload("Customer")

	if f.StartDate != "" && f.EndDate != "" {
		query = query.Where("tanggal BETWEEN ? AND ?", f.StartDate+" 00:00:00", f.EndDate+" 23:59:59")
	}
	if f.Status != "" {
		query = query.Where("LOWER(status) = ?", strings.ToLower(f.Status))
	}
	if f.Search != "" {
		pat := "%" + strings.ToLower(f.Search) + "%"
		query = query.Where("LOWER(customer_name) LIKE ? OR LOWER(sales_invoice) LIKE ? OR LOWER(keterangan) LIKE ?", pat, pat, pat)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := f.Page
	if page < 1 {
		page = 1
	}
	limit := f.Limit
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	err := query.Order("tanggal desc, id desc").Limit(limit).Offset(offset).Find(&records).Error
	return records, total, err
}

func (s *AccountingService) CreatePiutang(req domain.PiutangDagangRequest) (*domain.PiutangDagang, error) {
	if req.CustomerName == "" {
		return nil, errors.New("nama pelanggan wajib diisi")
	}

	var tgl time.Time
	var err error
	if req.Tanggal != "" {
		tgl, err = time.Parse("2006-01-02", req.Tanggal)
		if err != nil {
			tgl = time.Now()
		}
	} else {
		tgl = time.Now()
	}

	saldoAkhir := req.SaldoAwal + req.Debet - req.Kredit
	status := req.Status
	if status == "" {
		if saldoAkhir <= 0 {
			status = "Lunas"
		} else {
			status = "Belum Lunas"
		}
	}

	piutang := domain.PiutangDagang{
		CustomerID:    req.CustomerID,
		CustomerName:  req.CustomerName,
		CustomerPhone: req.CustomerPhone,
		SalesInvoice:  req.SalesInvoice,
		Tanggal:       tgl,
		SaldoAwal:     req.SaldoAwal,
		Debet:         req.Debet,
		Kredit:        req.Kredit,
		SaldoAkhir:    saldoAkhir,
		Keterangan:    req.Keterangan,
		Status:        status,
	}

	if err := s.DB.Create(&piutang).Error; err != nil {
		return nil, err
	}

	s.DB.Preload("Customer").First(&piutang, piutang.ID)
	return &piutang, nil
}

func (s *AccountingService) GetPiutangByID(id uint) (*domain.PiutangDagang, error) {
	var p domain.PiutangDagang
	if err := s.DB.Preload("Customer").First(&p, id).Error; err != nil {
		return nil, errors.New("data piutang tidak ditemukan")
	}
	return &p, nil
}

func (s *AccountingService) UpdatePiutang(id uint, req domain.PiutangDagangRequest) (*domain.PiutangDagang, error) {
	var p domain.PiutangDagang
	if err := s.DB.First(&p, id).Error; err != nil {
		return nil, errors.New("data piutang tidak ditemukan")
	}

	updates := map[string]interface{}{}
	if req.CustomerName != "" {
		updates["customer_name"] = req.CustomerName
	}
	if req.CustomerPhone != "" {
		updates["customer_phone"] = req.CustomerPhone
	}
	if req.SalesInvoice != "" {
		updates["sales_invoice"] = req.SalesInvoice
	}
	if req.Tanggal != "" {
		d, err := time.Parse("2006-01-02", req.Tanggal)
		if err == nil {
			updates["tanggal"] = d
		}
	}
	updates["saldo_awal"] = req.SaldoAwal
	updates["debet"] = req.Debet
	updates["kredit"] = req.Kredit

	saldoAkhir := req.SaldoAwal + req.Debet - req.Kredit
	updates["saldo_akhir"] = saldoAkhir

	if req.Status != "" {
		updates["status"] = req.Status
	} else {
		if saldoAkhir <= 0 {
			updates["status"] = "Lunas"
		} else {
			updates["status"] = "Belum Lunas"
		}
	}
	updates["keterangan"] = req.Keterangan

	if err := s.DB.Model(&p).Updates(updates).Error; err != nil {
		return nil, err
	}

	s.DB.Preload("Customer").First(&p, id)
	return &p, nil
}

func (s *AccountingService) DeletePiutang(id uint) error {
	return s.DB.Delete(&domain.PiutangDagang{}, id).Error
}

func (s *AccountingService) AutoRecordSalesPiutang(tx *gorm.DB, customerID *uint, customerName, customerPhone, invoice string, totalNetto, amountPaid float64, date time.Time) {
	if tx == nil {
		tx = s.DB
	}

	sisaPiutang := totalNetto - amountPaid
	status := "Belum Lunas"
	if sisaPiutang <= 0 {
		status = "Lunas"
	}

	p := domain.PiutangDagang{
		CustomerID:    customerID,
		CustomerName:  customerName,
		CustomerPhone: customerPhone,
		SalesInvoice:  invoice,
		Tanggal:       date,
		SaldoAwal:     0,
		Debet:         totalNetto,
		Kredit:        amountPaid,
		SaldoAkhir:    sisaPiutang,
		Keterangan:    fmt.Sprintf("Transaksi DP Nota %s", invoice),
		Status:        status,
	}

	_ = tx.Create(&p).Error
}

func (s *AccountingService) AutoSettleSalesPiutang(tx *gorm.DB, invoice string, amountPaid float64) {
	if tx == nil {
		tx = s.DB
	}

	var p domain.PiutangDagang
	if err := tx.Where("sales_invoice = ?", invoice).First(&p).Error; err == nil {
		tx.Model(&p).Updates(map[string]interface{}{
			"kredit":      p.Debet,
			"saldo_akhir": 0,
			"status":      "Lunas",
			"keterangan":  p.Keterangan + " [LUNAS]",
		})
	}
}
