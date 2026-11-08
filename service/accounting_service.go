package service

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"backend-cashier/domain"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type AccountingService struct {
	DB *gorm.DB
}

func NewAccountingService(db *gorm.DB) *AccountingService {
	return &AccountingService{DB: db}
}

// -------------------------------------------------------------
// BAGAN AKUN (CHART OF ACCOUNTS)
// -------------------------------------------------------------

func (s *AccountingService) GetAllAccounts() ([]domain.Account, error) {
	var accounts []domain.Account
	err := s.DB.Order("code asc").Find(&accounts).Error
	return accounts, err
}

func (s *AccountingService) GetAccountByID(id uint) (*domain.Account, error) {
	var account domain.Account
	if err := s.DB.First(&account, id).Error; err != nil {
		return nil, errors.New("akun tidak ditemukan")
	}
	return &account, nil
}

func (s *AccountingService) CreateAccount(req domain.AccountRequest) (*domain.Account, error) {
	if req.Code == "" || req.Name == "" {
		return nil, errors.New("kode dan nama akun wajib diisi")
	}

	var existing domain.Account
	if err := s.DB.Where("code = ?", req.Code).First(&existing).Error; err == nil {
		return nil, fmt.Errorf("kode akun '%s' sudah terdaftar", req.Code)
	}

	accountType := strings.ToUpper(req.Type)
	normalPos := strings.ToUpper(req.NormalPosition)
	if normalPos == "" {
		if accountType == "ASSET" || accountType == "EXPENSE" {
			normalPos = "DEBET"
		} else {
			normalPos = "KREDIT"
		}
	}

	account := domain.Account{
		Code:           req.Code,
		Name:           req.Name,
		Type:           accountType,
		NormalPosition: normalPos,
		InitialBalance: req.InitialBalance,
		Description:    req.Description,
	}

	if err := s.DB.Create(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (s *AccountingService) UpdateAccount(id uint, req domain.AccountRequest) (*domain.Account, error) {
	var account domain.Account
	if err := s.DB.First(&account, id).Error; err != nil {
		return nil, errors.New("akun tidak ditemukan")
	}

	if req.Code != "" && req.Code != account.Code {
		var existing domain.Account
		if err := s.DB.Where("code = ? AND id != ?", req.Code, id).First(&existing).Error; err == nil {
			return nil, fmt.Errorf("kode akun '%s' sudah digunakan oleh akun lain", req.Code)
		}
		account.Code = req.Code
	}

	if req.Name != "" {
		account.Name = req.Name
	}
	if req.Type != "" {
		account.Type = strings.ToUpper(req.Type)
	}
	if req.NormalPosition != "" {
		account.NormalPosition = strings.ToUpper(req.NormalPosition)
	}
	account.InitialBalance = req.InitialBalance
	account.Description = req.Description

	if err := s.DB.Save(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (s *AccountingService) DeleteAccount(id uint) error {
	var account domain.Account
	if err := s.DB.First(&account, id).Error; err != nil {
		return errors.New("akun tidak ditemukan")
	}

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

	uuidPart := strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
	entryNumber := fmt.Sprintf("JV-%s-%s", entryDate.Format("20060102"), strings.ToUpper(uuidPart))

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

	diff := math.Abs(totalDebet - totalKredit)
	if diff > 0.01 {
		return nil, fmt.Errorf("jurnal tidak balance: total debet (%.2f) != total kredit (%.2f)", totalDebet, totalKredit)
	}

	location := strings.TrimSpace(req.Location)
	if location == "" {
		location = "Toko Utama"
	}

	entry := domain.JournalEntry{
		EntryNumber: entryNumber,
		Date:        entryDate,
		Description: req.Description,
		Reference:   req.Reference,
		Location:    location,
		Items:       journalItems,
	}

	if err := s.DB.Create(&entry).Error; err != nil {
		return nil, err
	}

	s.DB.Preload("Items.Account").First(&entry, entry.ID)
	return &entry, nil
}

func (s *AccountingService) UpdateJournalEntry(id uint, req domain.CreateJournalRequest) (*domain.JournalEntry, error) {
	var entry domain.JournalEntry
	if err := s.DB.First(&entry, id).Error; err != nil {
		return nil, errors.New("jurnal transaksi tidak ditemukan")
	}

	if len(req.Items) < 2 {
		return nil, errors.New("jurnal harus memiliki minimal 2 baris (Debet dan Kredit)")
	}

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
			JournalEntryID: entry.ID,
			AccountID:      accountID,
			Description:    itemReq.Description,
			Debet:          itemReq.Debet,
			Kredit:         itemReq.Kredit,
		})
	}

	diff := math.Abs(totalDebet - totalKredit)
	if diff > 0.01 {
		return nil, fmt.Errorf("jurnal tidak balance: total debet (%.2f) != total kredit (%.2f)", totalDebet, totalKredit)
	}

	var entryDate time.Time
	if req.Date != "" {
		if t, err := time.Parse("2006-01-02", req.Date); err == nil {
			entryDate = t
		} else if t, err := time.Parse(time.RFC3339, req.Date); err == nil {
			entryDate = t
		}
	}
	if !entryDate.IsZero() {
		entry.Date = entryDate
	}

	if req.Description != "" {
		entry.Description = req.Description
	}
	if req.Reference != "" {
		entry.Reference = req.Reference
	}
	if req.Location != "" {
		entry.Location = req.Location
	}

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&entry).Error; err != nil {
			return err
		}
		if err := tx.Where("journal_entry_id = ?", entry.ID).Delete(&domain.JournalItem{}).Error; err != nil {
			return err
		}
		for i := range journalItems {
			journalItems[i].JournalEntryID = entry.ID
			if err := tx.Create(&journalItems[i]).Error; err != nil {
				return err
			}
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

	if f.Month > 0 && f.Year > 0 && (f.StartDate == "" || f.EndDate == "") {
		firstDay := time.Date(f.Year, time.Month(f.Month), 1, 0, 0, 0, 0, time.Local)
		lastDay := firstDay.AddDate(0, 1, -1)
		f.StartDate = firstDay.Format("2006-01-02")
		f.EndDate = lastDay.Format("2006-01-02")
	} else if f.Year > 0 && (f.StartDate == "" || f.EndDate == "") {
		f.StartDate = fmt.Sprintf("%04d-01-01", f.Year)
		f.EndDate = fmt.Sprintf("%04d-12-31", f.Year)
	}

	query := s.DB.Model(&domain.JournalEntry{}).Preload("Items.Account")

	if f.StartDate != "" && f.EndDate != "" {
		query = query.Where("date BETWEEN ? AND ?", f.StartDate+" 00:00:00", f.EndDate+" 23:59:59")
	} else if f.StartDate != "" {
		query = query.Where("date >= ?", f.StartDate+" 00:00:00")
	} else if f.EndDate != "" {
		query = query.Where("date <= ?", f.EndDate+" 23:59:59")
	}

	if f.Search != "" {
		pattern := "%" + strings.ToLower(f.Search) + "%"
		query = query.Where("LOWER(description) LIKE ? OR LOWER(reference) LIKE ? OR LOWER(entry_number) LIKE ?", pattern, pattern, pattern)
	}
	if f.Location != "" && !strings.EqualFold(f.Location, "Semua Toko") {
		query = query.Where("location = ?", f.Location)
	}
	if strings.EqualFold(f.Status, "checked") {
		query = query.Where("is_checked = true")
	} else if strings.EqualFold(f.Status, "unchecked") {
		query = query.Where("is_checked = false")
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

// ExportJournalsExcel mengekspor data Buku Harian ke file Excel (.xlsx) sesuai filter periode (hari/bulan/tahun), lokasi toko, dan status cek
func (s *AccountingService) ExportJournalsExcel(f domain.JournalFilter) ([]byte, error) {
	if f.Month > 0 && f.Year > 0 && (f.StartDate == "" || f.EndDate == "") {
		firstDay := time.Date(f.Year, time.Month(f.Month), 1, 0, 0, 0, 0, time.Local)
		lastDay := firstDay.AddDate(0, 1, -1)
		f.StartDate = firstDay.Format("2006-01-02")
		f.EndDate = lastDay.Format("2006-01-02")
	} else if f.Year > 0 && (f.StartDate == "" || f.EndDate == "") {
		f.StartDate = fmt.Sprintf("%04d-01-01", f.Year)
		f.EndDate = fmt.Sprintf("%04d-12-31", f.Year)
	}

	query := s.DB.Model(&domain.JournalEntry{}).Preload("Items.Account")

	if f.StartDate != "" && f.EndDate != "" {
		query = query.Where("date BETWEEN ? AND ?", f.StartDate+" 00:00:00", f.EndDate+" 23:59:59")
	} else if f.StartDate != "" {
		query = query.Where("date >= ?", f.StartDate+" 00:00:00")
	} else if f.EndDate != "" {
		query = query.Where("date <= ?", f.EndDate+" 23:59:59")
	}

	if f.Search != "" {
		pattern := "%" + strings.ToLower(f.Search) + "%"
		query = query.Where("LOWER(description) LIKE ? OR LOWER(reference) LIKE ? OR LOWER(entry_number) LIKE ?", pattern, pattern, pattern)
	}
	if f.Location != "" && !strings.EqualFold(f.Location, "Semua Toko") {
		query = query.Where("location = ?", f.Location)
	}
	if strings.EqualFold(f.Status, "checked") {
		query = query.Where("is_checked = true")
	} else if strings.EqualFold(f.Status, "unchecked") {
		query = query.Where("is_checked = false")
	}

	var entries []domain.JournalEntry
	if err := query.Order("date asc, id asc").Find(&entries).Error; err != nil {
		return nil, err
	}

	fExcel := excelize.NewFile()
	sheetName := "Buku Harian"
	fExcel.SetSheetName("Sheet1", sheetName)

	locTitle := f.Location
	if locTitle == "" {
		locTitle = "Semua Toko"
	}
	periodeStr := "Semua Periode"
	if f.StartDate != "" && f.EndDate != "" {
		periodeStr = fmt.Sprintf("%s s/d %s", f.StartDate, f.EndDate)
	} else if f.StartDate != "" {
		periodeStr = fmt.Sprintf("Mulai %s", f.StartDate)
	} else if f.EndDate != "" {
		periodeStr = fmt.Sprintf("Hingga %s", f.EndDate)
	}

	fExcel.SetCellValue(sheetName, "A1", "LAPORAN BUKU HARIAN (JURNAL UMUM)")
	fExcel.SetCellValue(sheetName, "A2", fmt.Sprintf("Toko: %s | Periode: %s | Total Transaksi: %d", locTitle, periodeStr, len(entries)))

	headers := []string{"No", "Tanggal", "No. Bukti", "Ref / Bukti", "Toko", "Kode Akun", "Nama Akun", "Keterangan", "Debet (Rp)", "Kredit (Rp)", "Markpoint"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 4)
		fExcel.SetCellValue(sheetName, cell, h)
	}

	row := 5
	var totalDebet, totalKredit float64
	entryCounter := 1

	for _, entry := range entries {
		tglStr := entry.Date.Format("2006-01-02")
		checkStr := "Belum Dicek"
		if entry.IsChecked {
			checkStr = "Sudah Dicek"
		}

		for itemIdx, item := range entry.Items {
			if itemIdx == 0 {
				fExcel.SetCellValue(sheetName, fmt.Sprintf("A%d", row), entryCounter)
				fExcel.SetCellValue(sheetName, fmt.Sprintf("B%d", row), tglStr)
				fExcel.SetCellValue(sheetName, fmt.Sprintf("C%d", row), entry.EntryNumber)
				fExcel.SetCellValue(sheetName, fmt.Sprintf("D%d", row), entry.Reference)
				fExcel.SetCellValue(sheetName, fmt.Sprintf("E%d", row), entry.Location)
				fExcel.SetCellValue(sheetName, fmt.Sprintf("K%d", row), checkStr)
			}

			accCode := ""
			accName := ""
			if item.Account.Code != "" {
				accCode = item.Account.Code
				accName = item.Account.Name
			}
			desc := item.Description
			if desc == "" {
				desc = entry.Description
			}

			fExcel.SetCellValue(sheetName, fmt.Sprintf("F%d", row), accCode)
			fExcel.SetCellValue(sheetName, fmt.Sprintf("G%d", row), accName)
			fExcel.SetCellValue(sheetName, fmt.Sprintf("H%d", row), desc)
			fExcel.SetCellValue(sheetName, fmt.Sprintf("I%d", row), item.Debet)
			fExcel.SetCellValue(sheetName, fmt.Sprintf("J%d", row), item.Kredit)

			totalDebet += item.Debet
			totalKredit += item.Kredit
			row++
		}
		entryCounter++
	}

	// Baris Total
	fExcel.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "TOTAL")
	fExcel.SetCellValue(sheetName, fmt.Sprintf("I%d", row), totalDebet)
	fExcel.SetCellValue(sheetName, fmt.Sprintf("J%d", row), totalKredit)

	buf, err := fExcel.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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

func (s *AccountingService) ToggleCheckJournal(id uint) (*domain.JournalEntry, error) {
	var entry domain.JournalEntry
	if err := s.DB.First(&entry, id).Error; err != nil {
		return nil, errors.New("jurnal transaksi tidak ditemukan")
	}
	entry.IsChecked = !entry.IsChecked
	if err := s.DB.Model(&entry).Update("is_checked", entry.IsChecked).Error; err != nil {
		return nil, err
	}
	s.DB.Preload("Items.Account").First(&entry, id)
	return &entry, nil
}

// -------------------------------------------------------------
// REKAPITULASI BUKU HARIAN
// -------------------------------------------------------------

func (s *AccountingService) GetRekapitulasi(startDate, endDate, location string) (*domain.AccountingRecapResponse, error) {
	if startDate == "" || endDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	}

	var accounts []domain.Account
	if err := s.DB.Order("code asc").Find(&accounts).Error; err != nil {
		return nil, err
	}

	var recapItems []domain.AccountingRecapItem
	var grandDebet, grandKredit float64

	for _, acc := range accounts {
		var debet, kredit float64

		q := s.DB.Table("journal_items").
			Joins("JOIN journal_entries ON journal_entries.id = journal_items.journal_entry_id").
			Where("journal_items.account_id = ? AND journal_entries.date BETWEEN ? AND ?", acc.ID, startDate+" 00:00:00", endDate+" 23:59:59")

		if location != "" && !strings.EqualFold(location, "Semua Toko") {
			q = q.Where("journal_entries.location = ?", location)
		}

		q.Select("COALESCE(SUM(journal_items.debet), 0), COALESCE(SUM(journal_items.kredit), 0)").
			Row().Scan(&debet, &kredit)

		if debet > 0 || kredit > 0 {
			recapItems = append(recapItems, domain.AccountingRecapItem{
				AccountID:   acc.ID,
				AccountCode: acc.Code,
				AccountName: acc.Name,
				AccountType: acc.Type,
				TotalDebet:  debet,
				TotalKredit: kredit,
			})
			grandDebet += debet
			grandKredit += kredit
		}
	}

	diff := grandDebet - grandKredit
	isBalanced := diff >= -0.01 && diff <= 0.01

	return &domain.AccountingRecapResponse{
		StartDate:   startDate,
		EndDate:     endDate,
		Items:       recapItems,
		TotalDebet:  grandDebet,
		TotalKredit: grandKredit,
		Difference:  diff,
		IsBalanced:  isBalanced,
	}, nil
}

// -------------------------------------------------------------
// BUKU BESAR (GENERAL LEDGER)
// -------------------------------------------------------------

func (s *AccountingService) GetGeneralLedger(accountID uint, startDate, endDate, location string) ([]domain.LedgerAccountCard, error) {
	if startDate == "" || endDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	}

	var accounts []domain.Account
	if accountID > 0 {
		var a domain.Account
		if err := s.DB.First(&a, accountID).Error; err != nil {
			return nil, errors.New("akun tidak ditemukan")
		}
		accounts = append(accounts, a)
	} else {
		if err := s.DB.Order("code asc").Find(&accounts).Error; err != nil {
			return nil, err
		}
	}

	var cards []domain.LedgerAccountCard

	for _, acc := range accounts {
		var priorDebet, priorKredit float64
		priorQ := s.DB.Table("journal_items").
			Joins("JOIN journal_entries ON journal_entries.id = journal_items.journal_entry_id").
			Where("journal_items.account_id = ? AND journal_entries.date < ?", acc.ID, startDate+" 00:00:00")

		if location != "" && !strings.EqualFold(location, "Semua Toko") {
			priorQ = priorQ.Where("journal_entries.location = ?", location)
		}

		priorQ.Select("COALESCE(SUM(journal_items.debet), 0), COALESCE(SUM(journal_items.kredit), 0)").
			Row().Scan(&priorDebet, &priorKredit)

		initialBal := acc.InitialBalance
		if acc.NormalPosition == "DEBET" {
			initialBal += (priorDebet - priorKredit)
		} else {
			initialBal += (priorKredit - priorDebet)
		}

		type rowResult struct {
			ID          uint
			Date        time.Time
			EntryNumber string
			Description string
			Reference   string
			Debet       float64
			Kredit      float64
		}
		var rows []rowResult

		rowsQ := s.DB.Table("journal_items").
			Select("journal_items.id, journal_entries.date, journal_entries.entry_number, COALESCE(journal_items.description, journal_entries.description) as description, journal_entries.reference, journal_items.debet, journal_items.kredit").
			Joins("JOIN journal_entries ON journal_entries.id = journal_items.journal_entry_id").
			Where("journal_items.account_id = ? AND journal_entries.date BETWEEN ? AND ?", acc.ID, startDate+" 00:00:00", endDate+" 23:59:59")

		if location != "" && !strings.EqualFold(location, "Semua Toko") {
			rowsQ = rowsQ.Where("journal_entries.location = ?", location)
		}

		rowsQ.Order("journal_entries.date asc, journal_items.id asc").
			Scan(&rows)

		var txItems []domain.LedgerTransactionItem
		currentBal := initialBal
		var totalDebet, totalKredit float64

		for _, r := range rows {
			if acc.NormalPosition == "DEBET" {
				currentBal += (r.Debet - r.Kredit)
			} else {
				currentBal += (r.Kredit - r.Debet)
			}

			totalDebet += r.Debet
			totalKredit += r.Kredit

			txItems = append(txItems, domain.LedgerTransactionItem{
				ID:          r.ID,
				Date:        r.Date,
				EntryNumber: r.EntryNumber,
				Description: r.Description,
				Reference:   r.Reference,
				Debet:       r.Debet,
				Kredit:      r.Kredit,
				Balance:     currentBal,
			})
		}

		if len(txItems) == 0 && initialBal == 0 {
			continue
		}

		cards = append(cards, domain.LedgerAccountCard{
			AccountID:       acc.ID,
			AccountCode:     acc.Code,
			AccountName:     acc.Name,
			AccountType:     acc.Type,
			NormalPosition:  acc.NormalPosition,
			StartingBalance: initialBal,
			Transactions:    txItems,
			TotalDebet:      totalDebet,
			TotalKredit:     totalKredit,
			EndingBalance:   currentBal,
		})
	}

	return cards, nil
}

// -------------------------------------------------------------
// AUTOMATIC JOURNAL POSTING HELPERS (SEAMLESS DOUBLE ENTRY)
// -------------------------------------------------------------

func (s *AccountingService) AutoPostSalesJournal(tx *gorm.DB, invoice string, paymentMethod string, totalNetto, amountPaid float64, isDp bool, date time.Time, location ...string) {
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

	loc := "Toko Utama"
	if len(location) > 0 && strings.TrimSpace(location[0]) != "" {
		loc = strings.TrimSpace(location[0])
	}

	entry := domain.JournalEntry{
		EntryNumber: entryNumber,
		Date:        date,
		Description: fmt.Sprintf("PENJUALAN NOTA %s (%s)", invoice, paymentMethod),
		Reference:   invoice,
		Location:    loc,
		Items:       items,
	}

	_ = tx.Create(&entry).Error
}

func (s *AccountingService) AutoPostExpenseJournal(tx *gorm.DB, nota, deskripsi, kategori, location string, total float64, date time.Time) error {
	if tx == nil {
		tx = s.DB
	}

	if total <= 0 {
		return nil
	}

	var kasAcc domain.Account
	// Prioritas: kode 1101 atau nama "KAS"
	if err := tx.Where("code = ? OR LOWER(name) = ?", "1101", "kas").First(&kasAcc).Error; err != nil || kasAcc.ID == 0 {
		if err := tx.Where("type = ?", "ASSET").First(&kasAcc).Error; err != nil || kasAcc.ID == 0 {
			return errors.New("akun kas/aset tidak ditemukan di sistem")
		}
	}

	expenseAccountName := "BIAYA LAIN LAIN"
	descUpper := strings.ToUpper(deskripsi + " " + kategori)

	if strings.Contains(descUpper, "LISTRIK") || strings.Contains(descUpper, "PLN") {
		expenseAccountName = "BIAYA LISTRIK"
	} else if strings.Contains(descUpper, "GAJI") || strings.Contains(descUpper, "UPAH") {
		expenseAccountName = "GAJI PEGAWAI"
	} else if strings.Contains(descUpper, "BBM") || strings.Contains(descUpper, "BAHAN BAKAR") || strings.Contains(descUpper, "BENSIN") {
		expenseAccountName = "BIAYA BAHAN BAKAR"
	} else if strings.Contains(descUpper, "KONSUMSI") || strings.Contains(descUpper, "MAKAN") || strings.Contains(descUpper, "MINUM") || strings.Contains(descUpper, "SATE") || strings.Contains(descUpper, "RUJAK") || strings.Contains(descUpper, "MIXUE") {
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
	if err := tx.Where("LOWER(name) = ?", strings.ToLower(expenseAccountName)).First(&expAcc).Error; err != nil || expAcc.ID == 0 {
		if err := tx.Where("code = ? OR LOWER(name) = ?", "6199", "biaya lain lain").First(&expAcc).Error; err != nil || expAcc.ID == 0 {
			expAcc = domain.Account{
				Code:           "6199",
				Name:           "BIAYA LAIN LAIN",
				Type:           "EXPENSE",
				NormalPosition: "DEBET",
			}
			if err := tx.Create(&expAcc).Error; err != nil {
				tx.Where("code = ?", "6199").First(&expAcc)
			}
		}
	}

	cleanNota := strings.TrimSpace(nota)
	if cleanNota == "" {
		cleanNota = "EXP"
	}
	if location == "" {
		location = "Toko Utama"
	}

	uuidPart := strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
	entryNumber := fmt.Sprintf("JV-EXP-%s-%s", date.Format("20060102"), strings.ToUpper(uuidPart))

	desc := strings.TrimSpace(deskripsi)
	if desc == "" {
		desc = kategori
	}

	entry := domain.JournalEntry{
		EntryNumber: entryNumber,
		Date:        date,
		Description: fmt.Sprintf("BIAYA OPERASIONAL: %s", desc),
		Reference:   cleanNota,
		Location:    location,
		Items: []domain.JournalItem{
			{
				AccountID:   expAcc.ID,
				Description: desc,
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
	return tx.Create(&entry).Error
}

func (s *AccountingService) AutoPostPurchaseJournal(tx *gorm.DB, nota string, total float64, date time.Time, location ...string) {
	if tx == nil {
		tx = s.DB
	}

	var buyAcc, kasAcc domain.Account
	tx.Where("LOWER(name) = ?", "pembelian").First(&buyAcc)
	tx.Where("LOWER(name) = ?", "kas").First(&kasAcc)

	if buyAcc.ID == 0 || kasAcc.ID == 0 {
		return
	}

	cleanNota := strings.TrimSpace(nota)
	if cleanNota == "" {
		cleanNota = "BUY"
	}

	loc := "Toko Utama"
	if len(location) > 0 && strings.TrimSpace(location[0]) != "" {
		loc = strings.TrimSpace(location[0])
	}

	uuidPart := strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
	entryNumber := fmt.Sprintf("JV-BUY-%s-%s", cleanNota, strings.ToUpper(uuidPart))

	entry := domain.JournalEntry{
		EntryNumber: entryNumber,
		Date:        date,
		Description: fmt.Sprintf("PEMBELIAN STOK BARANG NOTA %s", nota),
		Reference:   nota,
		Location:    loc,
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

// SyncMissingExpenseJournals memeriksa tabel expenses dan membuat jurnal buku harian bagi yang belum tercatat
func (s *AccountingService) SyncMissingExpenseJournals(optionalTx ...*gorm.DB) (int, error) {
	db := s.DB
	if len(optionalTx) > 0 && optionalTx[0] != nil {
		db = optionalTx[0]
	}

	var expenses []domain.Expense
	if err := db.Find(&expenses).Error; err != nil {
		return 0, err
	}

	syncedCount := 0
	for _, exp := range expenses {
		cleanNota := strings.TrimSpace(exp.Nota)
		if cleanNota == "" {
			cleanNota = fmt.Sprintf("EXP-%d", exp.ID)
		}

		var count int64
		db.Model(&domain.JournalEntry{}).Where("reference = ? OR entry_number LIKE ?", cleanNota, "%"+cleanNota+"%").Count(&count)
		if count == 0 && exp.Total > 0 {
			tgl := exp.Tanggal
			if tgl.IsZero() {
				tgl = exp.CreatedAt
			}
			loc := exp.Location
			if loc == "" {
				loc = "Toko Utama"
			}
			err := s.AutoPostExpenseJournal(db, cleanNota, exp.Deskripsi, exp.Kategori, loc, exp.Total, tgl)
			if err == nil {
				syncedCount++
			}
		}
	}
	return syncedCount, nil
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
		query = query.Where("LOWER(customer_name) LIKE ? OR LOWER(sales_invoice) LIKE ? OR LOWER(keterangan) LIKE ? OR customer_phone LIKE ?", pat, pat, pat, pat)
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

	orderClause := "tanggal desc, id desc"
	if strings.EqualFold(f.GroupBy, "phone") {
		orderClause = "customer_phone asc, tanggal asc, id asc"
	}

	err := query.Order(orderClause).Limit(limit).Offset(offset).Find(&records).Error
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
	return &piutang, nil
}

func (s *AccountingService) UpdatePiutang(id uint, req domain.PiutangDagangRequest) (*domain.PiutangDagang, error) {
	var piutang domain.PiutangDagang
	if err := s.DB.First(&piutang, id).Error; err != nil {
		return nil, errors.New("data piutang tidak ditemukan")
	}

	if req.CustomerName != "" {
		piutang.CustomerName = req.CustomerName
	}
	if req.CustomerPhone != "" {
		piutang.CustomerPhone = req.CustomerPhone
	}
	if req.SalesInvoice != "" {
		piutang.SalesInvoice = req.SalesInvoice
	}
	if req.Tanggal != "" {
		if t, err := time.Parse("2006-01-02", req.Tanggal); err == nil {
			piutang.Tanggal = t
		}
	}
	if req.Keterangan != "" {
		piutang.Keterangan = req.Keterangan
	}

	piutang.SaldoAwal = req.SaldoAwal
	piutang.Debet = req.Debet
	piutang.Kredit = req.Kredit
	piutang.SaldoAkhir = piutang.SaldoAwal + piutang.Debet - piutang.Kredit

	if req.Status != "" {
		piutang.Status = req.Status
	} else {
		if piutang.SaldoAkhir <= 0 {
			piutang.Status = "Lunas"
		} else {
			piutang.Status = "Belum Lunas"
		}
	}

	if err := s.DB.Save(&piutang).Error; err != nil {
		return nil, err
	}
	return &piutang, nil
}

func (s *AccountingService) DeletePiutang(id uint) error {
	return s.DB.Delete(&domain.PiutangDagang{}, id).Error
}

func (s *AccountingService) AutoRecordSalesPiutang(tx *gorm.DB, customerID *uint, custName, custPhone, invoice string, totalNetto, amountPaid float64, date time.Time) {
	if tx == nil {
		tx = s.DB
	}

	cleanInv := strings.TrimSpace(invoice)
	var existing domain.PiutangDagang
	if err := tx.Where("sales_invoice = ?", cleanInv).First(&existing).Error; err == nil {
		return
	}

	sisaPiutang := totalNetto - amountPaid
	status := "Belum Lunas"
	if sisaPiutang <= 0 {
		status = "Lunas"
	}

	var saldoAwal float64
	if custPhone != "" {
		var last domain.PiutangDagang
		if err := tx.Where("customer_phone = ?", custPhone).Order("tanggal desc, id desc").First(&last).Error; err == nil {
			saldoAwal = last.SaldoAkhir
		}
	}

	p := domain.PiutangDagang{
		CustomerID:    customerID,
		CustomerName:  custName,
		CustomerPhone: custPhone,
		SalesInvoice:  cleanInv,
		Tanggal:       date,
		SaldoAwal:     saldoAwal,
		Debet:         sisaPiutang,
		Kredit:        0,
		SaldoAkhir:    saldoAwal + sisaPiutang,
		Keterangan:    fmt.Sprintf("Piutang Penjualan Nota %s", invoice),
		Status:        status,
	}

	_ = tx.Create(&p).Error
}

func (s *AccountingService) AutoSettleSalesPiutang(tx *gorm.DB, invoice string, paymentAmount float64) {
	if tx == nil {
		tx = s.DB
	}

	cleanInv := strings.TrimSpace(invoice)
	var p domain.PiutangDagang
	if err := tx.Where("sales_invoice = ?", cleanInv).First(&p).Error; err != nil {
		return
	}

	p.Kredit += paymentAmount
	p.SaldoAkhir = p.SaldoAwal + p.Debet - p.Kredit
	if p.SaldoAkhir <= 0 {
		p.Status = "Lunas"
		p.SaldoAkhir = 0
	}

	_ = tx.Save(&p).Error
}

// EnsureStandardAccounts memastikan seluruh akun standar akuntansi tersedia di database
func (s *AccountingService) EnsureStandardAccounts(tx *gorm.DB) (int, error) {
	if tx == nil {
		tx = s.DB
	}

	standardAccounts := []domain.Account{
		{Code: "1101", Name: "KAS", Type: "ASSET", NormalPosition: "DEBET", Description: "Kas Tunai Toko"},
		{Code: "1102", Name: "BCA", Type: "ASSET", NormalPosition: "DEBET", Description: "Rekening Bank BCA"},
		{Code: "1103", Name: "BRI 4560", Type: "ASSET", NormalPosition: "DEBET", Description: "Rekening Bank BRI 4560"},
		{Code: "1104", Name: "BRI 5563", Type: "ASSET", NormalPosition: "DEBET", Description: "Rekening Bank BRI 5563"},
		{Code: "1105", Name: "BNI 3815", Type: "ASSET", NormalPosition: "DEBET", Description: "Rekening Bank BNI 3815"},
		{Code: "1106", Name: "MANDIRI", Type: "ASSET", NormalPosition: "DEBET", Description: "Rekening Bank Mandiri"},
		{Code: "1107", Name: "PIUTANG DAGANG", Type: "ASSET", NormalPosition: "DEBET", Description: "Piutang Usaha Pelanggan"},
		{Code: "1108", Name: "PERSEDIAAN BARANG DAGANG", Type: "ASSET", NormalPosition: "DEBET", Description: "Persediaan Stok Produk"},
		{Code: "2101", Name: "UTANG DAGANG", Type: "LIABILITY", NormalPosition: "KREDIT", Description: "Utang Pembelian Barang ke Supplier"},
		{Code: "2102", Name: "PPN KELUARAN", Type: "LIABILITY", NormalPosition: "KREDIT", Description: "Pajak Pertambahan Nilai Keluaran 11%"},
		{Code: "3101", Name: "MODAL PEMILIK", Type: "EQUITY", NormalPosition: "KREDIT", Description: "Modal Pemilik Usaha"},
		{Code: "4101", Name: "PENJUALAN", Type: "REVENUE", NormalPosition: "KREDIT", Description: "Pendapatan Hasil Penjualan Barang"},
		{Code: "5101", Name: "HARGA POKOK PENJUALAN", Type: "EXPENSE", NormalPosition: "DEBET", Description: "Harga Pokok Penjualan (HPP)"},
		{Code: "5102", Name: "PEMBELIAN", Type: "EXPENSE", NormalPosition: "DEBET", Description: "Biaya Pembelian Stok Barang"},
		{Code: "6101", Name: "GAJI PEGAWAI", Type: "EXPENSE", NormalPosition: "DEBET", Description: "Beban Gaji & Upah Karyawan"},
		{Code: "6102", Name: "BIAYA LISTRIK", Type: "EXPENSE", NormalPosition: "DEBET", Description: "Beban Listrik PLN"},
		{Code: "6103", Name: "BIAYA BAHAN BAKAR", Type: "EXPENSE", NormalPosition: "DEBET", Description: "Beban BBM Operasional"},
		{Code: "6104", Name: "BIAYA KONSUMSI", Type: "EXPENSE", NormalPosition: "DEBET", Description: "Beban Konsumsi & Makanan Karyawan"},
		{Code: "6105", Name: "BIAYA EKSPEDISI", Type: "EXPENSE", NormalPosition: "DEBET", Description: "Beban Ongkos Kirim / Ekspedisi"},
		{Code: "6106", Name: "BIAYA KEPERLUAN TOKO", Type: "EXPENSE", NormalPosition: "DEBET", Description: "Beban Perlengkapan & Operasional Toko"},
		{Code: "6107", Name: "BIAYA INTERNET", Type: "EXPENSE", NormalPosition: "DEBET", Description: "Beban Internet & WiFi"},
		{Code: "6108", Name: "BIAYA TELKOM", Type: "EXPENSE", NormalPosition: "DEBET", Description: "Beban Telepon & Pulsa"},
		{Code: "6109", Name: "IURAN SAMPAH", Type: "EXPENSE", NormalPosition: "DEBET", Description: "Iuran Kebersihan Lingkungan"},
		{Code: "6110", Name: "BIAYA SAMSAT KENDARAAN", Type: "EXPENSE", NormalPosition: "DEBET", Description: "Beban Pajak & Samsat Kendaraan Operasional"},
		{Code: "6111", Name: "BIAYA ADMINISTRASI BANK", Type: "EXPENSE", NormalPosition: "DEBET", Description: "Beban Administrasi Bank"},
		{Code: "6199", Name: "BIAYA LAIN LAIN", Type: "EXPENSE", NormalPosition: "DEBET", Description: "Beban Operasional Lain-lain"},
	}

	createdCount := 0
	for _, acc := range standardAccounts {
		var existing domain.Account
		err := tx.Where("code = ? OR LOWER(name) = ?", acc.Code, strings.ToLower(acc.Name)).First(&existing).Error
		if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
			if errCreate := tx.Create(&acc).Error; errCreate == nil {
				createdCount++
			}
		}
	}
	return createdCount, nil
}

// SyncAllAccounting melakukan sinkronisasi ulang secara menyeluruh:
// 1. Membersihkan (hard delete) seluruh data rincian jurnal, buku harian, dan piutang dagang lama
// 2. Memastikan seluruh Bagan Akun Standar (COA) tersedia
// 3. Menyelaraskan seluruh transaksi Penjualan (Sales) ke Buku Harian Jurnal & Piutang Dagang
// 4. Menyelaraskan seluruh transaksi Pembelian (Purchases) ke Buku Harian Jurnal
// 5. Menyelaraskan seluruh transaksi Pengeluaran (Expenses) ke Buku Harian Jurnal
func (s *AccountingService) SyncAllAccounting(tx *gorm.DB) (*domain.SyncAccountingSummary, error) {
	if tx == nil {
		tx = s.DB
	}

	summary := &domain.SyncAccountingSummary{}

	err := tx.Transaction(func(trx *gorm.DB) error {
		// 1. Hapus seluruh data transaksi akuntansi lama (items, entries, piutang) agar sinkronisasi bersih dari awal
		var countItems int64
		trx.Model(&domain.JournalItem{}).Unscoped().Count(&countItems)
		if err := trx.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Where("1 = 1").Delete(&domain.JournalItem{}).Error; err != nil {
			return fmt.Errorf("gagal membersihkan rincian jurnal lama: %w", err)
		}
		summary.DeletedItems = int(countItems)

		var countJournals int64
		trx.Model(&domain.JournalEntry{}).Unscoped().Count(&countJournals)
		if err := trx.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Where("1 = 1").Delete(&domain.JournalEntry{}).Error; err != nil {
			return fmt.Errorf("gagal membersihkan buku harian jurnal lama: %w", err)
		}
		summary.DeletedJournals = int(countJournals)

		var countPiutang int64
		trx.Model(&domain.PiutangDagang{}).Unscoped().Count(&countPiutang)
		if err := trx.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Where("1 = 1").Delete(&domain.PiutangDagang{}).Error; err != nil {
			return fmt.Errorf("gagal membersihkan piutang dagang lama: %w", err)
		}
		summary.DeletedPiutang = int(countPiutang)

		// 2. Akun Standar
		accCreated, err := s.EnsureStandardAccounts(trx)
		if err != nil {
			return fmt.Errorf("gagal memastikan akun standar: %w", err)
		}
		summary.AccountsCreated = accCreated

		// 3. Sinkronisasi Penjualan (Sales)
		var allSales []domain.Sales
		if err := trx.Preload("Customer").Order("created_at asc, id asc").Find(&allSales).Error; err != nil {
			return fmt.Errorf("gagal mengambil data penjualan: %w", err)
		}

		type invData struct {
			Invoice       string
			PaymentMethod string
			Location      string
			TotalNetto    float64
			AmountPaid    float64
			IsDp          bool
			IsBon         bool
			CustomerID    *uint
			CustomerName  string
			CustomerPhone string
			CreatedAt     time.Time
		}

		invMap := make(map[string]*invData)
		var invOrder []string

		for _, item := range allSales {
			inv := strings.TrimSpace(item.Invoice)
			if inv == "" {
				continue
			}

			if _, exists := invMap[inv]; !exists {
				custName := item.MemberName
				custPhone := ""
				if item.Customer.ID > 0 {
					custName = item.Customer.Name
					custPhone = item.Customer.PhoneNumber
				}

				loc := item.Location
				if loc == "" {
					loc = "Toko Utama"
				}

				invMap[inv] = &invData{
					Invoice:       inv,
					PaymentMethod: item.PaymentMethod,
					Location:      loc,
					TotalNetto:    0,
					AmountPaid:    item.AmountPaid,
					IsDp:          item.IsDp,
					IsBon:         item.IsBon,
					CustomerID:    item.CustomerID,
					CustomerName:  custName,
					CustomerPhone: custPhone,
					CreatedAt:     item.CreatedAt,
				}
				invOrder = append(invOrder, inv)
			}

			invMap[inv].TotalNetto += item.TotalNetto
			if item.AmountPaid > invMap[inv].AmountPaid {
				invMap[inv].AmountPaid = item.AmountPaid
			}
			if item.IsDp {
				invMap[inv].IsDp = true
			}
			if item.IsBon {
				invMap[inv].IsBon = true
			}
		}

		for _, inv := range invOrder {
			d := invMap[inv]
			entryNumber := fmt.Sprintf("JV-SALES-%s", d.Invoice)

			var count int64
			trx.Model(&domain.JournalEntry{}).Where("entry_number = ? OR reference = ?", entryNumber, d.Invoice).Count(&count)
			if count == 0 && d.TotalNetto > 0 {
				isCredit := d.IsDp || d.IsBon
				s.AutoPostSalesJournal(trx, d.Invoice, d.PaymentMethod, d.TotalNetto, d.AmountPaid, isCredit, d.CreatedAt, d.Location)
				summary.SalesSynced++
				summary.TotalNewJournals++
			}

			// Sinkronisasi Piutang jika bon/dp atau amountPaid < totalNetto
			isPiutang := d.IsDp || d.IsBon || (d.AmountPaid < d.TotalNetto)
			if isPiutang {
				var pCount int64
				trx.Model(&domain.PiutangDagang{}).Where("sales_invoice = ?", d.Invoice).Count(&pCount)
				if pCount == 0 {
					s.AutoRecordSalesPiutang(trx, d.CustomerID, d.CustomerName, d.CustomerPhone, d.Invoice, d.TotalNetto, d.AmountPaid, d.CreatedAt)
					summary.PiutangSynced++
				}
			}
		}

		// 4. Sinkronisasi Pembelian (Purchases)
		var allPurchases []domain.Purchase
		if err := trx.Order("tanggal asc, id asc").Find(&allPurchases).Error; err != nil {
			return fmt.Errorf("gagal mengambil data pembelian: %w", err)
		}

		type purData struct {
			Nota     string
			Location string
			Total    float64
			Tanggal  time.Time
		}

		purMap := make(map[string]*purData)
		var purOrder []string

		for _, item := range allPurchases {
			nota := strings.TrimSpace(item.Nota)
			if nota == "" {
				continue
			}

			if _, exists := purMap[nota]; !exists {
				loc := item.Location
				if loc == "" {
					loc = "Toko Utama"
				}
				tgl := item.Tanggal
				if tgl.IsZero() {
					tgl = item.CreatedAt
				}

				purMap[nota] = &purData{
					Nota:     nota,
					Location: loc,
					Total:    0,
					Tanggal:  tgl,
				}
				purOrder = append(purOrder, nota)
			}
			purMap[nota].Total += item.Total
		}

		for _, nota := range purOrder {
			d := purMap[nota]
			var count int64
			trx.Model(&domain.JournalEntry{}).Where("reference = ? OR entry_number LIKE ?", d.Nota, "%"+d.Nota+"%").Count(&count)
			if count == 0 && d.Total > 0 {
				s.AutoPostPurchaseJournal(trx, d.Nota, d.Total, d.Tanggal, d.Location)
				summary.PurchasesSynced++
				summary.TotalNewJournals++
			}
		}

		// 5. Sinkronisasi Pengeluaran (Expenses)
		expSynced, err := s.SyncMissingExpenseJournals(trx)
		if err != nil {
			return fmt.Errorf("gagal menyinkronkan pengeluaran: %w", err)
		}
		summary.ExpensesSynced = expSynced
		summary.TotalNewJournals += expSynced

		return nil
	})

	if err != nil {
		return nil, err
	}

	return summary, nil
}
