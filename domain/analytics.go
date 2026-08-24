package domain

type AnalyticsKPISummary struct {
	TotalOmset       float64 `json:"total_omset"`
	TotalPengeluaran float64 `json:"total_pengeluaran"`
	Profit           float64 `json:"profit"`
	TotalTransaksi   int64   `json:"total_transaksi"`
	TotalQty         float64 `json:"total_qty"`
}

type DailyTrendItem struct {
	Date        string  `json:"date"`
	DayName     string  `json:"day_name"`
	Penjualan   float64 `json:"penjualan"`
	Pengeluaran float64 `json:"pengeluaran"`
}

type PaymentMethodShare struct {
	Method string  `json:"method"`
	Total  float64 `json:"total"`
	Count  int64   `json:"count"`
}

type TopProductItem struct {
	ProductID   uint    `json:"product_id"`
	ProductName string  `json:"product_name"`
	ProductCode string  `json:"product_code"`
	TotalQty    float64 `json:"total_qty"`
	TotalOmset  float64 `json:"total_omset"`
}

type CriticalStockItem struct {
	ProductID   uint    `json:"product_id"`
	ProductName string  `json:"product_name"`
	ProductCode string  `json:"product_code"`
	Category    string  `json:"category"`
	Saldo       float64 `json:"saldo"`
	Status      string  `json:"status"` // KOSONG, CRITICAL, AMAN
}

type CategoryMarginItem struct {
	CategoryID   uint    `json:"category_id"`
	CategoryName string  `json:"category_name"`
	OmsetKotor   float64 `json:"omset_kotor"`
	HPP          float64 `json:"hpp"`
	Profit       float64 `json:"profit"`
	MarginPct    float64 `json:"margin_pct"`
}

type StaffPerformanceItem struct {
	UserID         string  `json:"user_id"`
	StaffName      string  `json:"staff_name"`
	TotalOmset     float64 `json:"total_omset"`
	TotalTransaksi int64   `json:"total_transaksi"`
}

type PeakHourItem struct {
	Hour  string `json:"hour"`  // "09:00", "11:00", etc.
	Count int64  `json:"count"` // transaction count
}

type AnalyticsReportResponse struct {
	Period           string                 `json:"period"`
	KPI              AnalyticsKPISummary    `json:"kpi"`
	Trends           []DailyTrendItem       `json:"trends"`
	PaymentMethods   []PaymentMethodShare   `json:"payment_methods"`
	TopProducts      []TopProductItem       `json:"top_products"`
	CriticalStocks   []CriticalStockItem    `json:"critical_stocks"`
	CategoryMargins  []CategoryMarginItem   `json:"category_margins"`
	StaffLeaderboard []StaffPerformanceItem `json:"staff_leaderboard"`
	PeakHours        []PeakHourItem         `json:"peak_hours"`
}
