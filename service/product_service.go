package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"backend-cashier/domain"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type ProductService struct {
	DB *gorm.DB
}

func NewProductService(db *gorm.DB) *ProductService {
	return &ProductService{DB: db}
}

// GET ALL: Ambil semua data produk dengan pagination dan search
func (s *ProductService) GetAll(search string, page int, limit int) ([]domain.Product, int64, error) {
	var products []domain.Product
	var total int64

	db := s.DB.Model(&domain.Product{}).
		Preload("Category").
		Preload("Brand").
		Preload("Supplier").
		Preload("Stocks").
		Order("id desc")

	if search != "" {
		db = db.Where("nama ILIKE ? OR kode ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	err := db.Offset(offset).Limit(limit).Find(&products).Error

	return products, total, err
}

// CREATE: Tambah produk baru
func (s *ProductService) Create(p domain.Product) error {
	if strings.TrimSpace(p.Kode) == "" {
		return errors.New("kode produk wajib diisi")
	}
	if strings.TrimSpace(p.Nama) == "" {
		return errors.New("nama produk wajib diisi")
	}
	if p.CategoryID == 0 {
		return errors.New("kategori produk wajib dipilih (category_id tidak boleh kosong)")
	}

	var catCount int64
	if err := s.DB.Model(&domain.Category{}).Where("id = ?", p.CategoryID).Count(&catCount).Error; err != nil || catCount == 0 {
		return fmt.Errorf("kategori dengan ID %d tidak ditemukan di database", p.CategoryID)
	}

	if p.BrandID > 0 {
		var brandCount int64
		if err := s.DB.Model(&domain.Brand{}).Where("id = ?", p.BrandID).Count(&brandCount).Error; err != nil || brandCount == 0 {
			return fmt.Errorf("brand dengan ID %d tidak ditemukan di database", p.BrandID)
		}
	}

	if p.SupplierID != "" {
		var suppCount int64
		if err := s.DB.Model(&domain.Supplier{}).Where("id = ?", p.SupplierID).Count(&suppCount).Error; err != nil || suppCount == 0 {
			return fmt.Errorf("supplier dengan ID '%s' tidak ditemukan di database", p.SupplierID)
		}
	}

	var count int64
	s.DB.Model(&domain.Product{}).Where("kode = ?", p.Kode).Count(&count)
	if count > 0 {
		return errors.New("kode produk sudah digunakan")
	}

	return s.DB.Create(&p).Error
}

// UPDATE: Edit data produk
func (s *ProductService) Update(id string, p domain.ProductRequestUpdate) error {
	var product domain.Product

	if err := s.DB.First(&product, id).Error; err != nil {
		return errors.New("produk tidak ditemukan")
	}

	if p.CategoryID > 0 {
		var catCount int64
		if err := s.DB.Model(&domain.Category{}).Where("id = ?", p.CategoryID).Count(&catCount).Error; err != nil || catCount == 0 {
			return fmt.Errorf("kategori dengan ID %d tidak ditemukan di database", p.CategoryID)
		}
	}

	if p.BrandID > 0 {
		var brandCount int64
		if err := s.DB.Model(&domain.Brand{}).Where("id = ?", p.BrandID).Count(&brandCount).Error; err != nil || brandCount == 0 {
			return fmt.Errorf("brand dengan ID %d tidak ditemukan di database", p.BrandID)
		}
	}

	if p.SupplierID != "" {
		var suppCount int64
		if err := s.DB.Model(&domain.Supplier{}).Where("id = ?", p.SupplierID).Count(&suppCount).Error; err != nil || suppCount == 0 {
			return fmt.Errorf("supplier dengan ID '%s' tidak ditemukan di database", p.SupplierID)
		}
	}

	if err := s.DB.Model(&product).Updates(p).Error; err != nil {
		return err
	}

	if globalSSE != nil {
		globalSSE.Broadcast("products:stock", "product_updated", map[string]interface{}{
			"product_id": product.ID,
			"nama":       product.Nama,
			"kode":       product.Kode,
			"saldo":      product.Saldo,
			"hjual":      product.HJual,
		})
	}

	return nil
}

// DELETE: Hapus produk berdasarkan ID
func (s *ProductService) Delete(id string) error {
	var product domain.Product

	if err := s.DB.First(&product, id).Error; err != nil {
		return errors.New("produk tidak ditemukan")
	}

	return s.DB.Delete(&product).Error
}

// GET BY ID: Ambil satu produk untuk diedit
func (s *ProductService) GetByID(id string) (domain.Product, error) {
	var product domain.Product
	err := s.DB.Preload("Category").Preload("Brand").Preload("Supplier").Preload("Stocks").First(&product, id).Error
	return product, err
}

// -------------------------------------------------------------
// HISTORICAL STOCK VALUATION REPORT (REQUEST KEMBAR & AKUNTANSI)
// -------------------------------------------------------------

func (s *ProductService) GetHistoricalStockReport(cutoffDateStr, location, search string) (*domain.HistoricalStockReport, error) {
	var cutoffTime time.Time
	var err error

	if cutoffDateStr != "" {
		cutoffTime, err = time.Parse("2006-01-02", cutoffDateStr)
		if err != nil {
			cutoffTime, _ = time.Parse(time.RFC3339, cutoffDateStr)
		}
	}
	if cutoffTime.IsZero() {
		cutoffTime = time.Now()
	}

	// Batas akhir hari tanggal cutoff
	cutoffEndOfDay := time.Date(cutoffTime.Year(), cutoffTime.Month(), cutoffTime.Day(), 23, 59, 59, 999999999, cutoffTime.Location())
	formattedCutoff := cutoffTime.Format("2006-01-02")

	// 1. Ambil seluruh master produk
	var products []domain.Product
	pQuery := s.DB.Model(&domain.Product{}).Preload("Category").Order("nama asc")
	if search != "" {
		pQuery = pQuery.Where("nama ILIKE ? OR kode ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if err := pQuery.Find(&products).Error; err != nil {
		return nil, err
	}

	// 2. Query total penjualan (sales) setelah cutoff
	type productQty struct {
		ProductID uint
		TotalQty  float64
	}
	var futureSales []productQty
	sq := s.DB.Table("sales").
		Select("product_id, COALESCE(SUM(qty), 0) as total_qty").
		Where("created_at > ?", cutoffEndOfDay).
		Group("product_id")
	if location != "" && !strings.EqualFold(location, "Semua Toko") {
		sq = sq.Where("location = ?", location)
	}
	sq.Scan(&futureSales)

	futureSalesMap := make(map[uint]float64)
	for _, item := range futureSales {
		futureSalesMap[item.ProductID] = item.TotalQty
	}

	// 3. Query total pembelian (purchases) setelah cutoff
	var futurePurchases []productQty
	pq := s.DB.Table("purchases").
		Select("product_id, COALESCE(SUM(qty), 0) as total_qty").
		Where("created_at > ? OR tanggal > ?", cutoffEndOfDay, cutoffEndOfDay).
		Group("product_id")
	if location != "" && !strings.EqualFold(location, "Semua Toko") {
		pq = pq.Where("location = ?", location)
	}
	pq.Scan(&futurePurchases)

	futurePurchasesMap := make(map[uint]float64)
	for _, item := range futurePurchases {
		futurePurchasesMap[item.ProductID] = item.TotalQty
	}

	// 4. Query total retur distributor setelah cutoff
	var futureReturs []productQty
	s.DB.Table("returs").
		Select("product_id, COALESCE(SUM(1), 0) as total_qty").
		Where("created_at > ? AND product_id IS NOT NULL", cutoffEndOfDay).
		Group("product_id").
		Scan(&futureReturs)

	futureRetursMap := make(map[uint]float64)
	for _, item := range futureReturs {
		futureRetursMap[item.ProductID] = item.TotalQty
	}

	// 5. Susun hasil valuasi stok per produk
	var items []domain.HistoricalStockItem
	var grandTotalQty float64
	var grandTotalNominal float64

	for _, p := range products {
		currentSaldo := p.Saldo
		salesAfterCutoff := futureSalesMap[p.ID]
		purchasesAfterCutoff := futurePurchasesMap[p.ID]
		retursAfterCutoff := futureRetursMap[p.ID]

		// Stok per tanggal cutoff = Stok Saat Ini + Penjualan Setelah Cutoff - Pembelian Setelah Cutoff + Retur Setelah Cutoff
		qtyAtCutoff := currentSaldo + salesAfterCutoff - purchasesAfterCutoff + retursAfterCutoff
		if qtyAtCutoff < 0 {
			qtyAtCutoff = 0
		}

		nominalAtCutoff := qtyAtCutoff * p.HBeli

		catName := "-"
		if p.Category.Name != "" {
			catName = p.Category.Name
		}

		items = append(items, domain.HistoricalStockItem{
			ProductID:    p.ID,
			Kode:         p.Kode,
			Nama:         p.Nama,
			Satuan:       p.Satuan,
			CategoryName: catName,
			HBeli:        p.HBeli,
			HJual:        p.HJual,
			QtyStok:      qtyAtCutoff,
			NominalStok:  nominalAtCutoff,
		})

		grandTotalQty += qtyAtCutoff
		grandTotalNominal += nominalAtCutoff
	}

	report := &domain.HistoricalStockReport{
		CutoffDate:   formattedCutoff,
		Location:     location,
		TotalItems:   len(items),
		TotalQty:     grandTotalQty,
		TotalNominal: grandTotalNominal,
		Items:        items,
	}

	return report, nil
}

// ExportHistoricalStockExcel menghasilkan file Excel (.xlsx dalam bentuk bytes)
func (s *ProductService) ExportHistoricalStockExcel(cutoffDateStr, location, search string) ([]byte, error) {
	report, err := s.GetHistoricalStockReport(cutoffDateStr, location, search)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	sheetName := "Valuasi Stok"
	f.SetSheetName("Sheet1", sheetName)

	// Judul Laporan
	f.SetCellValue(sheetName, "A1", "LAPORAN VALUASI & NOMINAL STOK AKHIR")
	f.SetCellValue(sheetName, "A2", fmt.Sprintf("Tanggal Cutoff: %s | Lokasi: %s | Total Nilai Persediaan: Rp %.0f", report.CutoffDate, report.Location, report.TotalNominal))

	// Header Tabel
	headers := []string{"No", "Kode Produk", "Nama Produk", "Kategori", "Satuan", "Harga Beli Satuan", "Harga Jual", "Qty Stok Akhir", "Nominal Stok (Rp)"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 4)
		f.SetCellValue(sheetName, cell, h)
	}

	// Baris Data
	for idx, item := range report.Items {
		row := idx + 5
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), idx+1)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), item.Kode)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), item.Nama)
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), item.CategoryName)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), item.Satuan)
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), item.HBeli)
		f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), item.HJual)
		f.SetCellValue(sheetName, fmt.Sprintf("H%d", row), item.QtyStok)
		f.SetCellValue(sheetName, fmt.Sprintf("I%d", row), item.NominalStok)
	}

	// Baris Total
	totalRow := len(report.Items) + 5
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", totalRow), "TOTAL")
	f.SetCellValue(sheetName, fmt.Sprintf("H%d", totalRow), report.TotalQty)
	f.SetCellValue(sheetName, fmt.Sprintf("I%d", totalRow), report.TotalNominal)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
