package service

import (
	"backend-cashier/domain"
	"errors"
	"fmt"
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

	// Hitung saldo berjalan untuk masing-masing akun
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
	// Cek apakah ada jurnal yang memakai akun ini
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
				// Buat akun baru otomatis jika belum ada
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

	// Validasi Keseimbangan Debet == Kredit
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

	var cards []domain.LedgerAccountCard

	for _, acc := range accounts {
		// 1. Hitung Saldo Awal sebelum startDate
		var prevDebet, prevKredit float64
		s.DB.Table("journal_items").
			Joins("JOIN journal_entries ON journal_entries.id = journal_items.journal_entry_id").
			Where("journal_items.account_id = ?", acc.ID).
			Where("journal_entries.date < ?", startDate+" 00:00:00").
			Where("journal_entries.deleted_at IS NULL").
			Select("COALESCE(SUM(journal_items.debet), 0) as prev_debet, COALESCE(SUM(journal_items.kredit), 0) as prev_kredit").
			Row().Scan(&prevDebet, &prevKredit)

		var initBalance float64
		if acc.NormalPosition == "DEBET" {
			initBalance = acc.InitialBalance + prevDebet - prevKredit
		} else {
			initBalance = acc.InitialBalance + prevKredit - prevDebet
		}

		// 2. Ambil transaksi pada periode startDate s/d endDate
		type ItemRow struct {
			ID          uint      `json:"id"`
			Date        time.Time `json:"date"`
			EntryNumber string    `json:"entry_number"`
			Description string    `json:"description"`
			Reference   string    `json:"reference"`
			Debet       float64   `json:"debet"`
			Kredit      float64   `json:"kredit"`
		}

		var rows []ItemRow
		s.DB.Table("journal_items").
			Select("journal_items.id, journal_entries.date, journal_entries.entry_number, journal_items.description, journal_entries.reference, journal_items.debet, journal_items.kredit").
			Joins("JOIN journal_entries ON journal_entries.id = journal_items.journal_entry_id").
			Where("journal_items.account_id = ?", acc.ID).
			Where("journal_entries.date BETWEEN ? AND ?", startDate+" 00:00:00", endDate+" 23:59:59").
			Where("journal_entries.deleted_at IS NULL").
			Order("journal_entries.date asc, journal_entries.id asc").
			Scan(&rows)

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
