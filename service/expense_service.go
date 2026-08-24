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
	if expense.Location == "" && expense.UserID != "" {
		var u domain.User
		if err := s.DB.Where("id = ?", expense.UserID).First(&u).Error; err == nil && u.Location != "" {
			expense.Location = u.Location
		}
	}
	if expense.Location == "" {
		expense.Location = "Toko Utama"
	}
	if expense.Tanggal.IsZero() {
		expense.Tanggal = time.Now()
	}

	err := s.DB.Create(expense).Error
	if err == nil && s.AccountingSvc != nil {
		s.AccountingSvc.AutoPostExpenseJournal(s.DB, expense.Nota, expense.Deskripsi, expense.Kategori, expense.Total, expense.Tanggal)
	}
	return err
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
	var expense domain.Expense
	if err := s.DB.First(&expense, "id = ?", id).Error; err != nil {
		return err
	}
	return s.DB.Model(&expense).Updates(input).Error
}

func (s *ExpenseService) DeleteExpense(id string) error {
	return s.DB.Delete(&domain.Expense{}, "id = ?", id).Error
}
