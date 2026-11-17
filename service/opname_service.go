package service

import (
	"backend-cashier/domain"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OpnameService struct {
	DB      *gorm.DB
	Storage StorageService
	SSE     *SSEService
}

func NewOpnameService(db *gorm.DB, storage StorageService, sse ...*SSEService) *OpnameService {
	var sseSvc *SSEService
	if len(sse) > 0 {
		sseSvc = sse[0]
	}
	return &OpnameService{
		DB:      db,
		Storage: storage,
		SSE:     sseSvc,
	}
}

// GenerateOpnameNo membuat nomor dokumen otomatis seperti OPN-SDR-20261109-001
func (s *OpnameService) GenerateOpnameNo(location string) string {
	locCode := "PST"
	locUpper := strings.ToUpper(strings.TrimSpace(location))
	switch {
	case strings.Contains(locUpper, "SUDIRMAN"):
		locCode = "SDR"
	case strings.Contains(locUpper, "PAYE"):
		locCode = "PAY"
	case strings.Contains(locUpper, "KOTA"):
		locCode = "KOT"
	case strings.Contains(locUpper, "UTAMA"):
		locCode = "UTM"
	default:
		clean := strings.ReplaceAll(locUpper, "TOKO", "")
		clean = strings.TrimSpace(clean)
		if len(clean) >= 3 {
			locCode = clean[:3]
		}
	}

	today := time.Now().Format("20060102")
	prefix := fmt.Sprintf("OPN-%s-%s-", locCode, today)

	var count int64
	s.DB.Model(&domain.StockOpname{}).Where("opname_no LIKE ?", prefix+"%").Count(&count)

	return fmt.Sprintf("%s%03d", prefix, count+1)
}

// CreateSession membuka sesi Stock Opname baru untuk satu cabang fisik
func (s *OpnameService) CreateSession(location string, categoryID *uint, userID string, notes string) (*domain.StockOpname, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return nil, errors.New("lokasi cabang toko wajib diisi")
	}

	opnameNo := s.GenerateOpnameNo(location)

	// Normalisasi categoryID: jika nil atau 0, anggap opname semua kategori (NULL di database)
	if categoryID != nil && *categoryID == 0 {
		categoryID = nil
	}

	// Tarik seluruh produk aktif (bisa difilter per kategori)
	var products []domain.Product
	query := s.DB.Model(&domain.Product{})
	if categoryID != nil && *categoryID > 0 {
		query = query.Where("category_id = ?", *categoryID)
	}
	if err := query.Order("nama asc, id asc").Find(&products).Error; err != nil {
		return nil, fmt.Errorf("gagal mengambil data produk: %w", err)
	}

	if len(products) == 0 {
		return nil, errors.New("tidak ada produk yang ditemukan untuk opname")
	}

	tx := s.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	opname := domain.StockOpname{
		OpnameNo:   opnameNo,
		Location:   location,
		Status:     "Draft",
		CategoryID: categoryID,
		Notes:      notes,
		UserID:     userID,
		TotalItems: len(products),
	}

	if err := tx.Create(&opname).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal membuat sesi opname: %w", err)
	}

	// Tarik seluruh saldo cabang dalam 1 query batch efisien (O(1) database roundtrip)
	var existingStocks []domain.ProductStock
	tx.Where("location = ?", location).Find(&existingStocks)
	stockMap := make(map[uint]float64, len(existingStocks))
	for _, s := range existingStocks {
		stockMap[s.ProductID] = s.Saldo
	}

	// Inisialisasi batch record ProductStock = 0 untuk produk yang belum memiliki baris lokasi
	var newStocks []domain.ProductStock
	for _, prod := range products {
		if _, exists := stockMap[prod.ID]; !exists {
			newStocks = append(newStocks, domain.ProductStock{
				ProductID: prod.ID,
				Location:  location,
				Saldo:     0,
			})
			stockMap[prod.ID] = 0
		}
	}
	if len(newStocks) > 0 {
		if err := tx.CreateInBatches(&newStocks, 500).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("gagal menginisialisasi saldo stok cabang: %w", err)
		}
	}

	items := make([]domain.StockOpnameItem, 0, len(products))
	totalMatched := 0
	totalDeficit := 0
	totalSurplus := 0
	totalDiffQty := 0.0
	totalDiffValue := 0.0

	for _, prod := range products {
		// Saldo sistem diambil dari map cabang (default 0 jika belum ada record)
		systemQty := stockMap[prod.ID]
		physicalQty := systemQty
		discQty := physicalQty - systemQty
		discVal := discQty * prod.HBeli

		totalMatched++

		item := domain.StockOpnameItem{
			StockOpnameID:    opname.ID,
			ProductID:        prod.ID,
			SystemQty:        systemQty,
			PhysicalQty:      physicalQty,
			DiscrepancyQty:   discQty,
			HBeli:            prod.HBeli,
			DiscrepancyValue: discVal,
			Reason:           "Cocok",
			Notes:            "",
			PhotoURL:         "",
		}
		items = append(items, item)
	}

	// Bulk insert dalam batch 500 item untuk performa kilat
	if err := tx.CreateInBatches(&items, 500).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal membuat baris rincian opname: %w", err)
	}

	// Update rekap header
	opname.TotalMatched = totalMatched
	opname.TotalDeficit = totalDeficit
	opname.TotalSurplus = totalSurplus
	opname.TotalDiffQty = totalDiffQty
	opname.TotalDiffValue = totalDiffValue
	_ = tx.Save(&opname).Error

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("gagal commit sesi opname: %w", err)
	}

	opname.Items = items
	if s.SSE != nil {
		s.SSE.Broadcast("opname:all", "session_created", map[string]interface{}{
			"id":        opname.ID,
			"opname_no": opname.OpnameNo,
			"location":  opname.Location,
			"status":    opname.Status,
		})
	}
	return &opname, nil
}

// UpdateCountItem memperbarui kuantitas fisik, alasan selisih, dan foto bukti untuk satu barang
func (s *OpnameService) UpdateCountItem(opnameID uint, productID uint, req domain.UpdateOpnameItemRequest) (*domain.StockOpnameItem, error) {
	var opname domain.StockOpname
	if err := s.DB.First(&opname, opnameID).Error; err != nil {
		return nil, errors.New("sesi opname tidak ditemukan")
	}

	if opname.Status == "Completed" || opname.Status == "Cancelled" {
		return nil, fmt.Errorf("sesi opname berstatus %s dan sudah terkunci", opname.Status)
	}

	var item domain.StockOpnameItem
	if err := s.DB.Where("stock_opname_id = ? AND product_id = ?", opnameID, productID).First(&item).Error; err != nil {
		return nil, errors.New("barang tidak ditemukan dalam sesi opname ini")
	}

	item.PhysicalQty = req.PhysicalQty
	item.DiscrepancyQty = item.PhysicalQty - item.SystemQty
	item.DiscrepancyValue = item.DiscrepancyQty * item.HBeli

	if req.Reason != "" {
		item.Reason = req.Reason
	} else {
		if item.DiscrepancyQty == 0 {
			item.Reason = "Cocok"
		} else if item.DiscrepancyQty < 0 {
			item.Reason = "Selisih Kurang"
		} else {
			item.Reason = "Selisih Lebih"
		}
	}

	item.Notes = req.Notes
	if req.PhotoURL != "" {
		item.PhotoURL = req.PhotoURL
	}

	if err := s.DB.Save(&item).Error; err != nil {
		return nil, fmt.Errorf("gagal menyimpan hasil hitung item: %w", err)
	}

	// Recalculate summary pada sesi opname
	s.RecalculateSessionSummary(opnameID)

	if s.SSE != nil {
		s.SSE.Broadcast(fmt.Sprintf("opname:%d", opnameID), "item_updated", map[string]interface{}{
			"stock_opname_id":   opnameID,
			"product_id":        item.ProductID,
			"system_qty":        item.SystemQty,
			"physical_qty":      item.PhysicalQty,
			"discrepancy_qty":   item.DiscrepancyQty,
			"discrepancy_value": item.DiscrepancyValue,
			"reason":            item.Reason,
			"notes":             item.Notes,
			"photo_url":         item.PhotoURL,
		})
	}

	return &item, nil
}

// RecalculateSessionSummary menghitung ulang total kecocokan dan deviasi pada sesi opname
func (s *OpnameService) RecalculateSessionSummary(opnameID uint) {
	var items []domain.StockOpnameItem
	s.DB.Where("stock_opname_id = ?", opnameID).Find(&items)

	matched := 0
	deficit := 0
	surplus := 0
	var totalDiffQty float64 = 0
	var totalDiffValue float64 = 0

	for _, it := range items {
		totalDiffQty += it.DiscrepancyQty
		totalDiffValue += it.DiscrepancyValue
		if math.Abs(it.DiscrepancyQty) < 0.0001 {
			matched++
		} else if it.DiscrepancyQty < 0 {
			deficit++
		} else {
			surplus++
		}
	}

	s.DB.Model(&domain.StockOpname{}).Where("id = ?", opnameID).Updates(map[string]interface{}{
		"total_matched":    matched,
		"total_deficit":    deficit,
		"total_surplus":    surplus,
		"total_diff_qty":   totalDiffQty,
		"total_diff_value": totalDiffValue,
	})
}

// SubmitForApproval mengubah status dari Draft menjadi Waiting Approval
func (s *OpnameService) SubmitForApproval(opnameID uint, notes string) (*domain.StockOpname, error) {
	var opname domain.StockOpname
	if err := s.DB.First(&opname, opnameID).Error; err != nil {
		return nil, errors.New("sesi opname tidak ditemukan")
	}

	if opname.Status != "Draft" {
		return nil, fmt.Errorf("hanya dokumen berstatus Draft yang dapat diajukan (status saat ini: %s)", opname.Status)
	}

	updates := map[string]interface{}{
		"status": "Waiting Approval",
	}
	if notes != "" {
		updates["notes"] = notes
	}

	if err := s.DB.Model(&opname).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("gagal mengajukan approval: %w", err)
	}

	opname.Status = "Waiting Approval"
	if s.SSE != nil {
		payload := map[string]interface{}{
			"stock_opname_id": opnameID,
			"status":          opname.Status,
		}
		s.SSE.Broadcast(fmt.Sprintf("opname:%d", opnameID), "status_changed", payload)
		s.SSE.Broadcast("opname:all", "status_changed", payload)
	}
	return &opname, nil
}

// ApproveSession menyetujui dokumen dan menerapkan penyesuaian stok secara atomik
func (s *OpnameService) ApproveSession(opnameID uint, approverID string) (*domain.StockOpname, error) {
	tx := s.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var opname domain.StockOpname
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&opname, opnameID).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("sesi opname tidak ditemukan")
	}

	if opname.Status == "Completed" {
		tx.Rollback()
		return nil, errors.New("dokumen opname ini sudah selesai disetujui sebelumnya")
	}
	if opname.Status == "Cancelled" {
		tx.Rollback()
		return nil, errors.New("dokumen opname ini telah dibatalkan")
	}

	var items []domain.StockOpnameItem
	if err := tx.Where("stock_opname_id = ?", opnameID).Find(&items).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal mengambil rincian opname: %w", err)
	}

	now := time.Now()

	for _, item := range items {
		// Lock dan ambil ProductStock cabang terkait
		var ps domain.ProductStock
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("product_id = ? AND location = ?", item.ProductID, opname.Location).
			First(&ps).Error

		saldoSebelum := 0.0
		if err == nil {
			saldoSebelum = ps.Saldo
		} else {
			ps = domain.ProductStock{
				ProductID: item.ProductID,
				Location:  opname.Location,
				Saldo:     0,
			}
			if errCreate := tx.Create(&ps).Error; errCreate != nil {
				tx.Rollback()
				return nil, fmt.Errorf("gagal inisialisasi product stock: %w", errCreate)
			}
		}

		saldoSesudah := item.PhysicalQty
		diff := saldoSesudah - saldoSebelum

		var qtyIn float64 = 0
		var qtyOut float64 = 0
		if diff > 0 {
			qtyIn = diff
		} else if diff < 0 {
			qtyOut = -diff
		}

		// Catat satu baris ke Kartu Stok / Audit Trail (StockMovement)
		movementNotes := fmt.Sprintf("Penyesuaian Opname: %s", item.Reason)
		if item.Notes != "" {
			movementNotes += fmt.Sprintf(" (%s)", item.Notes)
		}

		movement := domain.StockMovement{
			ProductID:    item.ProductID,
			Location:     opname.Location,
			MovementType: "OPNAME_ADJUST",
			ReferenceNo:  opname.OpnameNo,
			QtyIn:        qtyIn,
			QtyOut:       qtyOut,
			SaldoBefore:  saldoSebelum,
			SaldoAfter:   saldoSesudah,
			Notes:        movementNotes,
			UserID:       approverID,
		}
		if err := tx.Create(&movement).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("gagal mencatat mutasi kartu stok: %w", err)
		}

		// 1. Update Saldo Toko Terkait (product_stocks)
		if err := tx.Model(&ps).Update("saldo", saldoSesudah).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("gagal mengupdate saldo cabang: %w", err)
		}

		// 2. Sinkronisasi Saldo Total di Master Produk (products.saldo = SUM seluruh cabang)
		var totalLocationStock float64
		if err := tx.Model(&domain.ProductStock{}).
			Where("product_id = ?", item.ProductID).
			Select("COALESCE(SUM(saldo), 0)").
			Scan(&totalLocationStock).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("gagal menghitung total saldo master produk: %w", err)
		}

		if err := tx.Model(&domain.Product{}).
			Where("id = ?", item.ProductID).
			Update("saldo", totalLocationStock).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("gagal mengupdate saldo total master produk: %w", err)
		}
	}

	// 3. Kunci Sesi Dokumen
	opname.Status = "Completed"
	opname.ApproverID = &approverID
	opname.CompletedAt = &now

	if err := tx.Save(&opname).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal menyelesaikan dokumen opname: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("gagal commit approval opname: %w", err)
	}

	if s.SSE != nil {
		payload := map[string]interface{}{
			"stock_opname_id": opnameID,
			"status":          opname.Status,
			"approver_id":     approverID,
		}
		s.SSE.Broadcast(fmt.Sprintf("opname:%d", opnameID), "status_changed", payload)
		s.SSE.Broadcast("opname:all", "status_changed", payload)
	}

	return &opname, nil
}

// CancelSession membatalkan sesi dokumen opname
func (s *OpnameService) CancelSession(opnameID uint, userID string) (*domain.StockOpname, error) {
	var opname domain.StockOpname
	if err := s.DB.First(&opname, opnameID).Error; err != nil {
		return nil, errors.New("sesi opname tidak ditemukan")
	}

	if opname.Status == "Completed" {
		return nil, errors.New("dokumen opname yang sudah selesai tidak dapat dibatalkan")
	}

	if err := s.DB.Model(&opname).Updates(map[string]interface{}{
		"status": "Cancelled",
	}).Error; err != nil {
		return nil, fmt.Errorf("gagal membatalkan sesi: %w", err)
	}

	opname.Status = "Cancelled"
	if s.SSE != nil {
		payload := map[string]interface{}{
			"stock_opname_id": opnameID,
			"status":          opname.Status,
			"user_id":         userID,
		}
		s.SSE.Broadcast(fmt.Sprintf("opname:%d", opnameID), "status_changed", payload)
		s.SSE.Broadcast("opname:all", "status_changed", payload)
	}
	return &opname, nil
}

// GetSession mengambil rincian sesi dokumen opname lengkap dengan produk
func (s *OpnameService) GetSession(opnameID uint) (*domain.StockOpname, error) {
	var opname domain.StockOpname
	if err := s.DB.
		Preload("Category").
		Preload("Items.Product").
		First(&opname, opnameID).Error; err != nil {
		return nil, errors.New("sesi opname tidak ditemukan")
	}
	return &opname, nil
}

// ListSessions menampilkan daftar dokumen Stock Opname
func (s *OpnameService) ListSessions(location string, status string, limit int, offset int) ([]domain.StockOpname, int64, error) {
	var opnames []domain.StockOpname
	var total int64

	query := s.DB.Model(&domain.StockOpname{})
	if location != "" && location != "Semua Toko" {
		query = query.Where("location = ?", location)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	if err := query.
		Preload("Category").
		Order("created_at desc, id desc").
		Limit(limit).
		Offset(offset).
		Find(&opnames).Error; err != nil {
		return nil, 0, err
	}

	return opnames, total, nil
}

// GetProductStockBreakdown mengambil rincian stok produk di seluruh cabang/lokasi
func (s *OpnameService) GetProductStockBreakdown(productID uint) ([]domain.ProductStock, error) {
	var stocks []domain.ProductStock
	if err := s.DB.Where("product_id = ?", productID).Order("location asc").Find(&stocks).Error; err != nil {
		return nil, err
	}
	return stocks, nil
}

// GetStockCard mengambil riwayat mutasi / kartu stok produk
func (s *OpnameService) GetStockCard(productID uint, location string, limit int, offset int) ([]domain.StockMovement, int64, error) {
	var movements []domain.StockMovement
	var total int64

	query := s.DB.Model(&domain.StockMovement{}).Where("product_id = ?", productID)
	if location != "" && location != "Semua Toko" {
		query = query.Where("location = ?", location)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	if err := query.
		Order("created_at desc, id desc").
		Limit(limit).
		Offset(offset).
		Find(&movements).Error; err != nil {
		return nil, 0, err
	}

	return movements, total, nil
}

// AdjustProductStock adalah fungsi pembantu universal untuk memotong/menambah stok cabang saat transaksi Penjualan, Pembelian, atau Retur
func AdjustProductStock(tx *gorm.DB, productID uint, location string, delta float64, movementType string, refNo string, notes string, userID string) error {
	location = strings.TrimSpace(location)
	if location == "" {
		location = "Toko Utama"
	}

	var ps domain.ProductStock
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("product_id = ? AND location = ?", productID, location).
		First(&ps).Error

	saldoSebelum := 0.0
	if err == nil {
		saldoSebelum = ps.Saldo
	} else {
		ps = domain.ProductStock{
			ProductID: productID,
			Location:  location,
			Saldo:     0,
		}
		if errCreate := tx.Create(&ps).Error; errCreate != nil {
			return fmt.Errorf("gagal membuat product_stock: %w", errCreate)
		}
	}

	saldoSesudah := saldoSebelum + delta
	var qtyIn float64 = 0
	var qtyOut float64 = 0
	if delta > 0 {
		qtyIn = delta
	} else if delta < 0 {
		qtyOut = -delta
	}

	// Catat riwayat ke StockMovement
	movement := domain.StockMovement{
		ProductID:    productID,
		Location:     location,
		MovementType: movementType,
		ReferenceNo:  refNo,
		QtyIn:        qtyIn,
		QtyOut:       qtyOut,
		SaldoBefore:  saldoSebelum,
		SaldoAfter:   saldoSesudah,
		Notes:        notes,
		UserID:       userID,
	}
	if err := tx.Create(&movement).Error; err != nil {
		return fmt.Errorf("gagal mencatat stock_movement: %w", err)
	}

	// Update saldo di ProductStock
	if err := tx.Model(&ps).Update("saldo", saldoSesudah).Error; err != nil {
		return fmt.Errorf("gagal mengupdate saldo product_stock: %w", err)
	}

	if globalSSE != nil {
		var totalSaldo float64
		_ = tx.Model(&domain.Product{}).Where("id = ?", productID).Select("saldo").Scan(&totalSaldo)
		stockPayload := map[string]interface{}{
			"product_id":    productID,
			"location":      location,
			"saldo_before":  saldoSebelum,
			"saldo_after":   saldoSesudah,
			"total_saldo":   totalSaldo,
			"delta":         delta,
			"movement_type": movementType,
			"ref_no":        refNo,
			"user_id":       userID,
			"timestamp":     time.Now().Format("2006-01-02T15:04:05Z07:00"),
		}
		globalSSE.Broadcast("products:stock", "stock_changed", stockPayload)
		if location != "" {
			globalSSE.Broadcast("products:stock:"+location, "stock_changed", stockPayload)
		}
	}

	return nil
}
