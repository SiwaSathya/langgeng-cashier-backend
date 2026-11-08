package service

import (
	"backend-cashier/domain"
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

func (s *ExpenseService) GetAll(page, limit int, search string) ([]domain.Expense, int64, error) {
	var expenses []domain.Expense
	var total int64
	offset := (page - 1) * limit

	query := s.DB.Model(&domain.Expense{}).Preload("User")

	if search != "" {
		query = query.Where("deskripsi LIKE ? OR nota LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	query.Count(&total)
	err := query.Limit(limit).Offset(offset).Order("created_at desc").Find(&expenses).Error

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
