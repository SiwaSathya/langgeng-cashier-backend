package service

import (
	"backend-cashier/domain"

	"gorm.io/gorm"
)

type ExpenseService struct {
	DB *gorm.DB
}

func NewExpenseService(db *gorm.DB) *ExpenseService {
	return &ExpenseService{DB: db}
}

func (s *ExpenseService) CreateExpense(expense *domain.Expense) error {
	return s.DB.Create(expense).Error
}

func (s *ExpenseService) GetAll(page, limit int, search string) ([]domain.Expense, int64, error) {
	var expenses []domain.Expense
	var total int64
	offset := (page - 1) * limit

	query := s.DB.Model(&domain.Expense{})

	if search != "" {
		// Mencari berdasarkan keterangan pengeluaran
		query = query.Where("keterangan LIKE ?", "%"+search+"%")
	}

	query.Count(&total)
	err := query.Limit(limit).Offset(offset).Order("created_at desc").Find(&expenses).Error

	return expenses, total, err
}

func (s *ExpenseService) GetByID(id string) (domain.Expense, error) {
	var expense domain.Expense
	err := s.DB.First(&expense, "id = ?", id).Error
	return expense, err
}

func (s *ExpenseService) UpdateExpense(id string, input *domain.Expense) error {
	var expense domain.Expense
	if err := s.DB.First(&expense, "id = ?", id).Error; err != nil {
		return err
	}
	// Update field yang diperlukan
	return s.DB.Model(&expense).Updates(input).Error
}

func (s *ExpenseService) DeleteExpense(id string) error {
	return s.DB.Delete(&domain.Expense{}, "id = ?", id).Error
}
