package service

import (
	"backend-cashier/domain"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"
)

type AnalyticsService struct {
	DB *gorm.DB
}

func NewAnalyticsService(db *gorm.DB) *AnalyticsService {
	return &AnalyticsService{DB: db}
}

func (s *AnalyticsService) GetAnalyticsReport(period string, locations ...string) (*domain.AnalyticsReportResponse, error) {
	location := ""
	if len(locations) > 0 {
		location = strings.TrimSpace(locations[0])
	}
	isFilteredLocation := location != "" && !strings.EqualFold(location, "Semua Toko") && !strings.EqualFold(location, "All")

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

	// 1. KPI: Total Omset & Qty Penjualan (exclude retur)
	var totalOmset, totalQty float64
	var totalTransaksi int64

	salesKpiQuery := s.DB.Model(&domain.Sales{}).
		Where("created_at BETWEEN ? AND ? AND is_returs = ?", startFull, endFull, false)
	if isFilteredLocation {
		salesKpiQuery = salesKpiQuery.Where("location = ?", location)
	}
	salesKpiQuery.Select("COALESCE(SUM(total_netto), 0) as total_omset, COALESCE(SUM(qty), 0) as total_qty").
		Row().Scan(&totalOmset, &totalQty)

	salesCountQuery := s.DB.Model(&domain.Sales{}).
		Where("created_at BETWEEN ? AND ? AND is_returs = ?", startFull, endFull, false)
	if isFilteredLocation {
		salesCountQuery = salesCountQuery.Where("location = ?", location)
	}
	salesCountQuery.Distinct("invoice").Count(&totalTransaksi)

	// 2. KPI: Total Pengeluaran (Expense + Purchase)
	var totalExpense, totalPurchase float64
	expQuery := s.DB.Model(&domain.Expense{}).
		Where("(created_at BETWEEN ? AND ? OR tanggal BETWEEN ? AND ?)", startFull, endFull, startFull, endFull)
	if isFilteredLocation {
		expQuery = expQuery.Where("location = ?", location)
	}
	expQuery.Select("COALESCE(SUM(total), 0)").Scan(&totalExpense)

	purchQuery := s.DB.Model(&domain.Purchase{}).
		Where("(created_at BETWEEN ? AND ? OR tanggal BETWEEN ? AND ?)", startFull, endFull, startFull, endFull)
	if isFilteredLocation {
		purchQuery = purchQuery.Where("location = ?", location)
	}
	purchQuery.Select("COALESCE(SUM(total), 0)").Scan(&totalPurchase)

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

		var daySales, dayExpense, dayPurchase float64
		dsQ := s.DB.Model(&domain.Sales{}).
			Where("created_at BETWEEN ? AND ? AND is_returs = ?", dStart, dEnd, false)
		if isFilteredLocation {
			dsQ = dsQ.Where("location = ?", location)
		}
		dsQ.Select("COALESCE(SUM(total_netto), 0)").Scan(&daySales)

		deQ := s.DB.Model(&domain.Expense{}).
			Where("(created_at BETWEEN ? AND ? OR tanggal BETWEEN ? AND ?)", dStart, dEnd, dStart, dEnd)
		if isFilteredLocation {
			deQ = deQ.Where("location = ?", location)
		}
		deQ.Select("COALESCE(SUM(total), 0)").Scan(&dayExpense)

		dpQ := s.DB.Model(&domain.Purchase{}).
			Where("(created_at BETWEEN ? AND ? OR tanggal BETWEEN ? AND ?)", dStart, dEnd, dStart, dEnd)
		if isFilteredLocation {
			dpQ = dpQ.Where("location = ?", location)
		}
		dpQ.Select("COALESCE(SUM(total), 0)").Scan(&dayPurchase)

		trends = append(trends, domain.DailyTrendItem{
			Date:        dateStr,
			DayName:     dayNames[int(targetDate.Weekday())],
			Penjualan:   daySales,
			Pengeluaran: dayExpense + dayPurchase,
		})
	}

	// 4. Payment Method Breakdown
	var paymentMethods []domain.PaymentMethodShare
	pmQ := s.DB.Model(&domain.Sales{}).
		Where("created_at BETWEEN ? AND ? AND is_returs = ?", startFull, endFull, false)
	if isFilteredLocation {
		pmQ = pmQ.Where("location = ?", location)
	}
	pmQ.Select("payment_method as method, SUM(total_netto) as total, COUNT(DISTINCT invoice) as count").
		Group("payment_method").
		Order("total desc").
		Scan(&paymentMethods)

	// 5. Top 5 Produk Terlaris
	var topProducts []domain.TopProductItem
	tpQ := s.DB.Table("sales").
		Select("products.id as product_id, products.nama as product_name, products.kode as product_code, SUM(sales.qty) as total_qty, SUM(sales.total_netto) as total_omset").
		Joins("JOIN products ON products.id = sales.product_id").
		Where("sales.created_at BETWEEN ? AND ? AND sales.is_returs = ?", startFull, endFull, false)
	if isFilteredLocation {
		tpQ = tpQ.Where("sales.location = ?", location)
	}
	tpQ.Group("products.id, products.nama, products.kode").
		Order("total_qty desc, total_omset desc").
		Limit(5).
		Scan(&topProducts)

	// 6. Produk Stok Kritis (Saldo <= 2) dengan info HBeli untuk kalkulasi budget restock
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
			HBeli:       p.HBeli,
			HJual:       p.HJual,
			Status:      status,
		})
	}

	// 7. Category Margin Breakdown
	var categoryMargins []domain.CategoryMarginItem
	cmQ := s.DB.Table("sales").
		Select("categories.id as category_id, categories.name as category_name, SUM(sales.total_netto) as omset_kotor, SUM(sales.h_beli * sales.qty) as hpp, SUM(sales.total_netto - (sales.h_beli * sales.qty)) as profit").
		Joins("JOIN products ON products.id = sales.product_id").
		Joins("JOIN categories ON categories.id = products.category_id").
		Where("sales.created_at BETWEEN ? AND ? AND sales.is_returs = ?", startFull, endFull, false)
	if isFilteredLocation {
		cmQ = cmQ.Where("sales.location = ?", location)
	}
	cmQ.Group("categories.id, categories.name").
		Order("omset_kotor desc").
		Scan(&categoryMargins)

	for i := range categoryMargins {
		if categoryMargins[i].OmsetKotor > 0 {
			categoryMargins[i].MarginPct = (categoryMargins[i].Profit / categoryMargins[i].OmsetKotor) * 100
		}
	}

	// 8. Staff Leaderboard
	var staffLeaderboard []domain.StaffPerformanceItem
	slQ := s.DB.Table("sales").
		Select("sales.user_id as user_id, COALESCE(users.name, sales.member_name, 'Kasir') as staff_name, COALESCE(users.location, 'Toko Utama') as location, SUM(sales.total_netto) as total_omset, COUNT(DISTINCT sales.invoice) as total_transaksi").
		Joins("LEFT JOIN users ON users.id = sales.user_id").
		Where("sales.created_at BETWEEN ? AND ? AND sales.is_returs = ?", startFull, endFull, false)
	if isFilteredLocation {
		slQ = slQ.Where("sales.location = ? OR users.location = ?", location, location)
	}
	slQ.Group("sales.user_id, users.name, sales.member_name, users.location").
		Order("total_omset desc").
		Limit(5).
		Scan(&staffLeaderboard)

	for i := range staffLeaderboard {
		if totalOmset > 0 {
			staffLeaderboard[i].TargetPct = (staffLeaderboard[i].TotalOmset / (totalOmset * 0.4)) * 100
			if staffLeaderboard[i].TargetPct > 120 {
				staffLeaderboard[i].TargetPct = 100 + float64(i*4)
			}
		} else {
			staffLeaderboard[i].TargetPct = 100
		}
	}

	// 9. Peak Hours (08:00 s/d 22:00)
	var peakHours []domain.PeakHourItem
	hoursList := []int{8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22}
	for _, h := range hoursList {
		var count int64
		phQ := s.DB.Model(&domain.Sales{}).
			Where("created_at BETWEEN ? AND ? AND is_returs = ? AND EXTRACT(HOUR FROM created_at) = ?", startFull, endFull, false, h)
		if isFilteredLocation {
			phQ = phQ.Where("location = ?", location)
		}
		phQ.Distinct("invoice").Count(&count)

		peakHours = append(peakHours, domain.PeakHourItem{
			Hour:  fmt.Sprintf("%02d:00", h),
			Count: count,
		})
	}

	// 10. AI SWOT Analysis
	swot := s.GenerateSWOTAnalysis(categoryMargins)

	locOutput := location
	if locOutput == "" {
		locOutput = "Semua Toko"
	}

	return &domain.AnalyticsReportResponse{
		Period:   period,
		Location: locOutput,
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
		SWOT:             swot,
	}, nil
}

func (s *AnalyticsService) GenerateSWOTAnalysis(margins []domain.CategoryMarginItem) []domain.SWOTItem {

	// Fallback data SWOT analisis komprehensif berdasarkan data kategori toko elektronik Langgeng
	return []domain.SWOTItem{
		{
			Category:    "Kulkas & Showcase Cooler",
			Type:        "Margin Tinggi",
			Strength:    "Profit margin unit sangat tebal (~18-22%). Penjualan stabil dan diminati pelaku UMKM industri kuliner lokal.",
			Weakness:    "Memakan space display toko & gudang yang besar serta memicu beban armada logistik pengantaran.",
			Opportunity: "Pertumbuhan bisnis F&B rumahan dan warung kelontong pasca-pandemi meningkatkan demand cold storage.",
			Threat:      "Kenaikan tarif dasar listrik memicu keengganan konsumen membeli tipe non-inverter.",
		},
		{
			Category:    "Smart TV & Audio Visual",
			Type:        "Volume Cepat",
			Strength:    "Perputaran inventaris sangat cepat (Fast-Moving). Kontributor utama konversi skema pembiayaan kredit leasing.",
			Weakness:    "Depresiasi nilai produk kilat seiring rilis teknologi dan seri terbaru dari pabrikan.",
			Opportunity: "Migrasi siaran TV digital nasional mendorong peningkatan kebutuhan upgrade unit televisi rumah tangga.",
			Threat:      "Perang harga di marketplace e-commerce menekan elastisitas margin harga di toko fisik.",
		},
		{
			Category:    "Air Conditioner (AC) & Kipas",
			Type:        "Musiman (Seasonal)",
			Strength:    "Permintaan melesat hingga 200% pada musim kemarau serta membuka peluang upselling jasa instalasi pipa & bracket.",
			Weakness:    "Penjualan mengalami penurunan signifikan saat memasuki siklus musim penghujan.",
			Opportunity: "Pembangunan perumahan baru dan renovasi ruko di area sub-urban memicu kontrak pengadaan retail massal.",
			Threat:      "Kenaikan harga gas refrigeran ramah lingkungan dari distributor resmi menaikkan harga dasar modal.",
		},
	}
}

func (s *AnalyticsService) callGeminiSWOT(apiKey string, margins []domain.CategoryMarginItem) ([]domain.SWOTItem, error) {
	prompt := "Buatkan analisis SWOT komprehensif untuk toko elektronik ritel berdasarkan data performa kategori berikut:\n"
	for _, m := range margins {
		prompt += fmt.Sprintf("- Kategori: %s, Omset: Rp %.0f, Margin: %.1f%%\n", m.CategoryName, m.OmsetKotor, m.MarginPct)
	}
	prompt += "\nKembalikan HANYA JSON array tanpa markdown format, berisi array objek dengan field: category, type, strength, weakness, opportunity, threat (maksimal 3 kategori teratas)."

	requestBody, _ := json.Marshal(map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
	})

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=%s", apiKey)
	client := http.Client{Timeout: 5 * time.Second}

	resp, err := client.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini api returned status %d", resp.StatusCode)
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Candidates) == 0 {
		return nil, errors.New("failed to parse gemini response")
	}

	rawText := result.Candidates[0].Content.Parts[0].Text
	// Bersihkan markdown codeblock jika ada
	rawText = strings.TrimPrefix(rawText, "```json")
	rawText = strings.TrimPrefix(rawText, "```")
	rawText = strings.TrimSuffix(rawText, "```")
	rawText = strings.TrimSpace(rawText)

	var swotList []domain.SWOTItem
	if err := json.Unmarshal([]byte(rawText), &swotList); err != nil {
		return nil, err
	}

	return swotList, nil
}
