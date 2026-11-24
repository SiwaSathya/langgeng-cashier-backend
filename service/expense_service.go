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

type ExpenseService struct {
	DB            *gorm.DB
	AccountingSvc *AccountingService
}

func NewExpenseService(db *gorm.DB, accSvc *AccountingService) *ExpenseService {
	return &ExpenseService{
		DB:            db,
		AccountingSvc: accSvc,
	}
}

func (s *ExpenseService) CreateExpense(expense *domain.Expense) error {
	// Validasi & Fallback UserID agar tidak melanggar foreign key fk_expenses_user
	if expense.UserID != "" {
		var u domain.User
		if err := s.DB.Where("id = ?", expense.UserID).First(&u).Error; err == nil {
			if expense.Location == "" && u.Location != "" {
				expense.Location = u.Location
			}
		} else {
			var fallbackUser domain.User
			if err := s.DB.First(&fallbackUser).Error; err == nil {
				expense.UserID = fallbackUser.ID
				if expense.Location == "" && fallbackUser.Location != "" {
					expense.Location = fallbackUser.Location
				}
			}
		}
	} else {
		var fallbackUser domain.User
		if err := s.DB.First(&fallbackUser).Error; err == nil {
			expense.UserID = fallbackUser.ID
			if expense.Location == "" && fallbackUser.Location != "" {
				expense.Location = fallbackUser.Location
			}
		}
	}

	if expense.Location == "" {
		expense.Location = "Toko Utama"
	}
	if expense.Tanggal.IsZero() {
		expense.Tanggal = time.Now()
	}

	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(expense).Error; err != nil {
			return err
		}
		if s.AccountingSvc != nil {
			return s.AccountingSvc.AutoPostExpenseJournal(tx, expense.Nota, expense.Deskripsi, expense.Kategori, expense.Location, expense.Total, expense.Tanggal)
		}
		return nil
	})
}

func (s *ExpenseService) GetAll(page, limit int, search, location, shiftStr, shiftStatus string) ([]domain.Expense, int64, error) {
	var expenses []domain.Expense
	var total int64
	offset := (page - 1) * limit

	query := s.DB.Model(&domain.Expense{}).Preload("User")

	search = strings.TrimSpace(search)
	if search != "" {
		query = query.Where("deskripsi ILIKE ? OR nota ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	location = strings.TrimSpace(location)
	if location != "" && !strings.EqualFold(location, "Semua Toko") {
		if strings.EqualFold(location, "Pusat") || strings.EqualFold(location, "Toko Utama") {
			query = query.Where("location = 'Toko Utama' OR location = '' OR location IS NULL")
		} else {
			query = query.Where("location = ?", location)
		}
	}

	shiftStatus = strings.ToLower(strings.TrimSpace(shiftStatus))
	if shiftStatus == "unclosed" {
		query = query.Where("shift IS NULL OR shift = 0")
	} else if shiftStatus == "closed" {
		query = query.Where("shift IS NOT NULL AND shift > 0")
	}

	shiftStr = strings.TrimSpace(shiftStr)
	if shiftStr != "" && !strings.EqualFold(shiftStr, "all") && !strings.EqualFold(shiftStr, "semua") {
		if sNum, err := strconv.Atoi(shiftStr); err == nil && sNum > 0 {
			query = query.Where("shift = ?", sNum)
		}
	}

	query.Count(&total)
	err := query.Limit(limit).Offset(offset).Order("tanggal desc, id desc").Find(&expenses).Error

	return expenses, total, err
}

func (s *ExpenseService) GetByID(id string) (domain.Expense, error) {
	var expense domain.Expense
	err := s.DB.Preload("User").First(&expense, "id = ?", id).Error
	return expense, err
}

func (s *ExpenseService) UpdateExpense(id string, input *domain.Expense) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var expense domain.Expense
		if err := tx.First(&expense, "id = ?", id).Error; err != nil {
			return err
		}
		oldNota := expense.Nota
		if err := tx.Model(&expense).Updates(input).Error; err != nil {
			return err
		}

		tx.First(&expense, "id = ?", id)
		if s.AccountingSvc != nil {
			if oldNota != "" {
				var oldEntries []domain.JournalEntry
				tx.Where("reference = ? AND entry_number LIKE 'JV-EXP-%'", oldNota).Find(&oldEntries)
				for _, e := range oldEntries {
					tx.Select("Items").Delete(&e)
				}
			}
			return s.AccountingSvc.AutoPostExpenseJournal(tx, expense.Nota, expense.Deskripsi, expense.Kategori, expense.Location, expense.Total, expense.Tanggal)
		}
		return nil
	})
}

func (s *ExpenseService) DeleteExpense(id string) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var expense domain.Expense
		if err := tx.First(&expense, "id = ?", id).Error; err != nil {
			return err
		}
		if expense.Nota != "" {
			var entries []domain.JournalEntry
			tx.Where("reference = ? AND entry_number LIKE 'JV-EXP-%'", expense.Nota).Find(&entries)
			for _, e := range entries {
				tx.Select("Items").Delete(&e)
			}
		}
		return tx.Delete(&expense).Error
	})
}

// CloseShiftByID melakukan tutup shift manual untuk satu transaksi pengeluaran tertentu
func (s *ExpenseService) CloseShiftByID(identifier string, shift uint) error {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return errors.New("ID atau nomor nota pengeluaran tidak boleh kosong")
	}
	if shift == 0 {
		return errors.New("nomor shift harus lebih besar dari 0")
	}

	return s.DB.Transaction(func(tx *gorm.DB) error {
		var expense domain.Expense
		var err error
		if idNum, parseErr := strconv.ParseUint(identifier, 10, 64); parseErr == nil && idNum > 0 {
			err = tx.First(&expense, "id = ?", idNum).Error
		} else {
			err = tx.First(&expense, "nota = ?", identifier).Error
		}
		if err != nil {
			return errors.New("data pengeluaran tidak ditemukan")
		}

		if err := tx.Model(&expense).Update("shift", shift).Error; err != nil {
			return fmt.Errorf("gagal menutup shift pengeluaran: %w", err)
		}
		return nil
	})
}

// CloseShiftBulk melakukan tutup shift manual massal untuk daftar ID pengeluaran
func (s *ExpenseService) CloseShiftBulk(ids []uint, shift uint) (int64, error) {
	if len(ids) == 0 {
		return 0, errors.New("daftar ID pengeluaran tidak boleh kosong")
	}
	if shift == 0 {
		return 0, errors.New("nomor shift harus lebih besar dari 0")
	}

	// Filter ID duplikat / 0
	var cleanIDs []uint
	seen := make(map[uint]bool)
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			cleanIDs = append(cleanIDs, id)
		}
	}
	if len(cleanIDs) == 0 {
		return 0, errors.New("daftar ID pengeluaran tidak valid")
	}

	var affectedCount int64
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&domain.Expense{}).Where("id IN (?)", cleanIDs).Update("shift", shift)
		if res.Error != nil {
			return fmt.Errorf("gagal menutup shift pengeluaran: %w", res.Error)
		}
		affectedCount = res.RowsAffected
		return nil
	})

	return affectedCount, err
}

// UndoShiftByID membatalkan status tutup shift pengeluaran (kembali ke NULL)
func (s *ExpenseService) UndoShiftByID(identifier string) error {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return errors.New("ID atau nomor nota pengeluaran tidak boleh kosong")
	}

	return s.DB.Transaction(func(tx *gorm.DB) error {
		var expense domain.Expense
		var err error
		if idNum, parseErr := strconv.ParseUint(identifier, 10, 64); parseErr == nil && idNum > 0 {
			err = tx.First(&expense, "id = ?", idNum).Error
		} else {
			err = tx.First(&expense, "nota = ?", identifier).Error
		}
		if err != nil {
			return errors.New("data pengeluaran tidak ditemukan")
		}

		if err := tx.Model(&expense).Update("shift", gorm.Expr("NULL")).Error; err != nil {
			return fmt.Errorf("gagal membatalkan shift pengeluaran: %w", err)
		}
		return nil
	})
}
