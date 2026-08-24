package service

import (
	"backend-cashier/domain"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type AnalyticsService struct {
	DB *gorm.DB
}

func NewAnalyticsService(db *gorm.DB) *AnalyticsService {
	return &AnalyticsService{DB: db}
}

func (s *AnalyticsService) GetAnalyticsReport(period string) (*domain.AnalyticsReportResponse, error) {
	now := time.Now()
	var startDate, endDate string

	switch period {
	case "last-month":
		firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
		lastOfLastMonth := firstOfThisMonth.AddDate(0, 0, -1)
		firstOfLastMonth := time.Date(lastOfLastMonth.Year(), lastOfLastMonth.Month(), 1, 0, 0, 0, 0, time.Local)
		startDate = firstOfLastMonth.Format("2006-01-02")
		endDate = lastOfLastMonth.Format("2006-01-02")
	case "this-year":
		firstOfYear := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.Local)
		startDate = firstOfYear.Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	case "this-month":
		fallthrough
	default:
		period = "this-month"
		firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
		startDate = firstOfMonth.Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	}

	startFull := startDate + " 00:00:00"
	endFull := endDate + " 23:59:59"

	// 1. KPI: Total Omset & Qty Penjualan
	var totalOmset, totalQty float64
	var totalTransaksi int64

	s.DB.Model(&domain.Sales{}).
		Where("created_at BETWEEN ? AND ?", startFull, endFull).
		Select("COALESCE(SUM(total_netto), 0) as total_omset, COALESCE(SUM(qty), 0) as total_qty").
		Row().Scan(&totalOmset, &totalQty)

	s.DB.Model(&domain.Sales{}).
		Where("created_at BETWEEN ? AND ?", startFull, endFull).
		Distinct("invoice").
		Count(&totalTransaksi)

	// 2. KPI: Total Pengeluaran (Expense + Purchase)
	var totalExpense, totalPurchase float64
	s.DB.Model(&domain.Expense{}).
		Where("created_at BETWEEN ? AND ? OR tanggal BETWEEN ? AND ?", startFull, endFull, startFull, endFull).
		Select("COALESCE(SUM(total), 0)").
		Scan(&totalExpense)

	s.DB.Model(&domain.Purchase{}).
		Where("created_at BETWEEN ? AND ? OR tanggal BETWEEN ? AND ?", startFull, endFull, startFull, endFull).
		Select("COALESCE(SUM(total), 0)").
		Scan(&totalPurchase)

	totalPengeluaran := totalExpense + totalPurchase
	profit := totalOmset - totalPengeluaran

	// 3. Trends: Tren Harian 7 Hari Terakhir
	var trends []domain.DailyTrendItem
	dayNames := []string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}

	for i := 6; i >= 0; i-- {
		targetDate := now.AddDate(0, 0, -i)
		dateStr := targetDate.Format("2006-01-02")
		dStart := dateStr + " 00:00:00"
		dEnd := dateStr + " 23:59:59"

		var daySales, dayExpense float64
		s.DB.Model(&domain.Sales{}).
			Where("created_at BETWEEN ? AND ?", dStart, dEnd).
			Select("COALESCE(SUM(total_netto), 0)").
			Scan(&daySales)

		s.DB.Model(&domain.Expense{}).
			Where("created_at BETWEEN ? AND ? OR tanggal BETWEEN ? AND ?", dStart, dEnd, dStart, dEnd).
			Select("COALESCE(SUM(total), 0)").
			Scan(&dayExpense)

		var dayPurchase float64
		s.DB.Model(&domain.Purchase{}).
			Where("created_at BETWEEN ? AND ? OR tanggal BETWEEN ? AND ?", dStart, dEnd, dStart, dEnd).
			Select("COALESCE(SUM(total), 0)").
			Scan(&dayPurchase)

		trends = append(trends, domain.DailyTrendItem{
			Date:        dateStr,
			DayName:     dayNames[int(targetDate.Weekday())],
			Penjualan:   daySales,
			Pengeluaran: dayExpense + dayPurchase,
		})
	}

	// 4. Payment Method Breakdown
	var paymentMethods []domain.PaymentMethodShare
	s.DB.Model(&domain.Sales{}).
		Where("created_at BETWEEN ? AND ?", startFull, endFull).
		Select("payment_method as method, SUM(total_netto) as total, COUNT(DISTINCT invoice) as count").
		Group("payment_method").
		Order("total desc").
		Scan(&paymentMethods)

	// 5. Top 5 Produk Terlaris
	var topProducts []domain.TopProductItem
	s.DB.Table("sales").
		Select("products.id as product_id, products.nama as product_name, products.kode as product_code, SUM(sales.qty) as total_qty, SUM(sales.total_netto) as total_omset").
		Joins("JOIN products ON products.id = sales.product_id").
		Where("sales.created_at BETWEEN ? AND ?", startFull, endFull).
		Group("products.id, products.nama, products.kode").
		Order("total_qty desc, total_omset desc").
		Limit(5).
		Scan(&topProducts)

	// 6. Produk Stok Kritis (Saldo <= 2)
	var criticalStocks []domain.CriticalStockItem
	var products []domain.Product
	s.DB.Preload("Category").Where("saldo <= ?", 2).Order("saldo asc").Limit(10).Find(&products)
	for _, p := range products {
		status := "CRITICAL"
		if p.Saldo <= 0 {
			status = "KOSONG"
		}
		catName := "Uncategorized"
		if p.Category.Name != "" {
			catName = p.Category.Name
		}
		criticalStocks = append(criticalStocks, domain.CriticalStockItem{
			ProductID:   p.ID,
			ProductName: p.Nama,
			ProductCode: p.Kode,
			Category:    catName,
			Saldo:       p.Saldo,
			Status:      status,
		})
	}

	// 7. Category Margin Breakdown
	var categoryMargins []domain.CategoryMarginItem
	s.DB.Table("sales").
		Select("categories.id as category_id, categories.name as category_name, SUM(sales.total_netto) as omset_kotor, SUM(sales.h_beli * sales.qty) as hpp, SUM(sales.total_netto - (sales.h_beli * sales.qty)) as profit").
		Joins("JOIN products ON products.id = sales.product_id").
		Joins("JOIN categories ON categories.id = products.category_id").
		Where("sales.created_at BETWEEN ? AND ?", startFull, endFull).
		Group("categories.id, categories.name").
		Order("omset_kotor desc").
		Scan(&categoryMargins)

	for i := range categoryMargins {
		if categoryMargins[i].OmsetKotor > 0 {
			categoryMargins[i].MarginPct = (categoryMargins[i].Profit / categoryMargins[i].OmsetKotor) * 100
		}
	}

	// 8. Staff Leaderboard
	var staffLeaderboard []domain.StaffPerformanceItem
	s.DB.Table("sales").
		Select("sales.user_id as user_id, COALESCE(users.name, sales.member_name, 'Kasir') as staff_name, SUM(sales.total_netto) as total_omset, COUNT(DISTINCT sales.invoice) as total_transaksi").
		Joins("LEFT JOIN users ON users.id = sales.user_id").
		Where("sales.created_at BETWEEN ? AND ?", startFull, endFull).
		Group("sales.user_id, users.name, sales.member_name").
		Order("total_omset desc").
		Limit(5).
		Scan(&staffLeaderboard)

	// 9. Peak Hours (08:00 s/d 22:00)
	var peakHours []domain.PeakHourItem
	hoursList := []int{8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22}
	for _, h := range hoursList {
		var count int64
		s.DB.Model(&domain.Sales{}).
			Where("created_at BETWEEN ? AND ? AND EXTRACT(HOUR FROM created_at) = ?", startFull, endFull, h).
			Distinct("invoice").
			Count(&count)

		peakHours = append(peakHours, domain.PeakHourItem{
			Hour:  fmt.Sprintf("%02d:00", h),
			Count: count,
		})
	}

	return &domain.AnalyticsReportResponse{
		Period: period,
		KPI: domain.AnalyticsKPISummary{
			TotalOmset:       totalOmset,
			TotalPengeluaran: totalPengeluaran,
			Profit:           profit,
			TotalTransaksi:   totalTransaksi,
			TotalQty:         totalQty,
		},
		Trends:           trends,
		PaymentMethods:   paymentMethods,
		TopProducts:      topProducts,
		CriticalStocks:   criticalStocks,
		CategoryMargins:  categoryMargins,
		StaffLeaderboard: staffLeaderboard,
		PeakHours:        peakHours,
	}, nil
}
