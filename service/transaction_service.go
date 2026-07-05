package service

import (
	"backend-cashier/domain"

	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type TransactionService struct {
	DB *gorm.DB
}

type PelunasanRequest struct {
	PaymentMethod string  `json:"payment_method"`
	AmountPaid    float64 `json:"amount_paid"`
	IsDp          bool    `json:"is_dp"`
	Status        string  `json:"status"`
}

func NewTransactionService(db *gorm.DB) *TransactionService {
	return &TransactionService{DB: db}
}

func (s *TransactionService) CreateSales(requests []domain.SalesRequest) (*domain.SalesResponse, error) {
	var totalNettoAll float64
	var invoice = fmt.Sprintf("PJL-%d", time.Now().Unix())
	var salesRecords []domain.Sales
	fmt.Println(requests)

	// Gunakan Transaction untuk membungkus seluruh loop
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		for _, req := range requests {

			var prod domain.Product
			fmt.Println("is_dp:", req.IsDp)
			fmt.Println("Mencari produk:", req.ProductSearch)
			if err := tx.Where("nama = ?", req.ProductSearch).First(&prod).Error; err != nil {
				return fmt.Errorf("barang %s tidak ditemukank", req.ProductSearch)
			}

			if prod.Saldo < req.Qty {
				return fmt.Errorf("stok %s tidak cukup, sisa: %.0f", prod.Nama, prod.Saldo)
			}

			subtotal := (prod.HJual * req.Qty) - req.Discount
			totalNettoAll += req.Price * req.Qty

			var customer *domain.Customer
			if req.IsDp {
				customer = &domain.Customer{
					Name:           req.Customer.Name,
					Age:            req.Customer.Age,
					Address:        req.Customer.Address,
					PhoneNumber:    req.Customer.PhoneNumber,
					IdentityNumber: req.Customer.IdentityNumber,
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
				}

				tx.Create(&customer)
			}

			fmt.Println(customer)
			sales := domain.Sales{
				Invoice:       invoice,
				ProductID:     prod.ID,
				UserID:        req.UserID,
				Qty:           req.Qty,
				HBeli:         prod.HBeli,
				HJual:         req.Price,
				TotalNetto:    subtotal,
				PaymentMethod: req.PaymentMethod,
				AmountPaid:    req.AmountPaid,
				IsDp:          req.IsDp,
			}

			if req.IsDp {
				sales.CustomerID = &customer.ID
				sales.Status = "DP"
			} else {
				sales.Status = "Lunas"
				// sales.CustomerID = nil

			}

			if err := tx.Create(&sales).Error; err != nil {
				return err
			}

			// Potong Stok
			if err := tx.Model(&prod).Update("saldo", prod.Saldo-req.Qty).Error; err != nil {
				return err
			}

			salesRecords = append(salesRecords, sales)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Kembalikan response berupa kembalian berdasarkan total semua item
	return &domain.SalesResponse{
		Invoice: invoice,
		Change:  requests[0].AmountPaid - totalNettoAll,
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

func (s *TransactionService) GetAllSales(f domain.SalesFilter) ([]domain.Sales, error) {
	var results []domain.Sales
	query := s.DB.Preload("Product")

	if f.StartDate != "" && f.EndDate != "" {
		query = query.Where("created_at BETWEEN ? AND ?", f.StartDate+" 00:00:00", f.EndDate+" 23:59:59")
	}
	if f.Member != "" {
		query = query.Where("member_name LIKE ?", "%"+f.Member+"%")
	}
	if f.Method != "" {
		query = query.Where("payment_method = ?", f.Method)
	}

	err := query.Find(&results).Error
	return results, err
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

func (a *TransactionService) GetUserSales(userId uint) ([]domain.Sales, error) {
	var sales []domain.Sales
	err := a.DB.Where("user_id = ?", userId).Find(&sales).Error
	return sales, err
}

// func (s *TransactionService) IsReturToCompany(saleID uint) (bool, error) {
// 	var sale domain.Sales
// 	if err := s.DB.Where("id = ?", saleID).First(&sale).Error; err != nil {
// 		return false, err
// 	}
// 	return sale.IsReturToCompany, nil
// }

// func (s *TransactionService) IsRetur(saleID uint) (bool, error) {
// 	var sale domain.Sales
// 	if err := s.DB.Where("id = ?", saleID).First(&sale).Error; err != nil {
// 		return false, err
// 	}
// 	return sale.IsRetur, nil
// }

// func (s *TransactionService) IsReturUpdate(saleID uint, isRetur bool) error {
// 	var sale domain.Sales
// 	if err := s.DB.Where("id = ?", saleID).First(&sale).Error; err != nil {
// 		return err
// 	}
// 	sale.IsRetur = isRetur
// 	return s.DB.Save(&sale).Error
// }

// func (s *TransactionService) IsReturToCompanyUpdate(saleID uint, isReturToCompany bool) error {
// 	var sale domain.Sales
// 	if err := s.DB.Where("id = ?", saleID).First(&sale).Error; err != nil {
// 		return err
// 	}
// 	sale.IsReturToCompany = isReturToCompany
// 	return s.DB.Save(&sale).Error
// }

func (s *TransactionService) PelunasanSales(id uint, req PelunasanRequest) (*domain.Sales, error) {
	var sale domain.Sales

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&sale, id).Error; err != nil {
			return errors.New("data transaksi penjualan tidak ditemukan")
		}

		if !sale.IsDp && sale.Status == "Lunas" {
			return errors.New("transaksi ini sudah berstatus lunas")
		}

		sale.PaymentMethod = req.PaymentMethod
		sale.AmountPaid = req.AmountPaid
		sale.IsDp = req.IsDp
		sale.Status = req.Status
		sale.UpdatedAt = time.Now()

		return tx.Save(&sale).Error
	})

	if err != nil {
		return nil, err
	}
	return &sale, nil
}
