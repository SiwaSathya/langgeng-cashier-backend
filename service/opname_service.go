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

// OfficialStockLocations adalah 5 titik lokasi fisik resmi (3 Toko + 2 Gudang)
var OfficialStockLocations = []string{
	"Toko Utama",
	"Toko Sudirman",
	"Toko Paye",
	"Gudang Karangsokong",
	"Gudang BTN",
}

// NormalizeStockLocation menyelaraskan nama lokasi (misal "Pusat" / "" -> "Toko Utama") agar tidak membuat baris lokasi ganda di product_stocks
func NormalizeStockLocation(location string) string {
	loc := strings.TrimSpace(location)
	if loc == "" {
		return "Toko Utama"
	}
	lower := strings.ToLower(loc)
	switch {
	case lower == "pusat" || lower == "utama" || lower == "toko utama":
		return "Toko Utama"
	case lower == "sudirman" || lower == "toko sudirman":
		return "Toko Sudirman"
	case lower == "paye" || lower == "toko paye":
		return "Toko Paye"
	case lower == "karangsokong" || lower == "gudang karangsokong":
		return "Gudang Karangsokong"
	case lower == "btn" || lower == "gudang btn":
		return "Gudang BTN"
	default:
		return loc
	}
}

func hasAllOfficialLocations(locSet map[string]bool) bool {
	if len(locSet) < len(OfficialStockLocations) {
		return false
	}
	for _, official := range OfficialStockLocations {
		if !locSet[official] {
			return false
		}
	}
	return true
}

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
	case strings.Contains(locUpper, "KARANGSOKONG"):
		locCode = "KRS"
	case strings.Contains(locUpper, "BTN"):
		locCode = "BTN"
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
	s.DB.Unscoped().Model(&domain.StockOpname{}).Where("opname_no LIKE ?", prefix+"%").Count(&count)

	return fmt.Sprintf("%s%03d", prefix, count+1)
}

// CreateSession membuka sesi Stock Opname baru untuk satu cabang fisik
func (s *OpnameService) CreateSession(location string, categoryID *uint, userID string, notes string) (*domain.StockOpname, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return nil, errors.New("lokasi cabang toko wajib diisi")
	}
	location = NormalizeStockLocation(location)

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

// ApproveSession menyetujui dokumen dan menerapkan penyesuaian stok cabang serta sinkronisasi stok global Hybrid secara atomik
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

	loc := NormalizeStockLocation(opname.Location)

	var items []domain.StockOpnameItem
	if err := tx.Where("stock_opname_id = ?", opnameID).Find(&items).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal mengambil rincian opname: %w", err)
	}

	now := time.Now()
	productIDs := make([]uint, 0, len(items))
	productIDSet := make(map[uint]bool, len(items))
	var minCountedTime time.Time

	for _, item := range items {
		if !productIDSet[item.ProductID] {
			productIDSet[item.ProductID] = true
			productIDs = append(productIDs, item.ProductID)
		}
		ct := item.UpdatedAt
		if ct.IsZero() {
			ct = item.CreatedAt
		}
		if minCountedTime.IsZero() || ct.Before(minCountedTime) {
			minCountedTime = ct
		}
	}

	// 1. Batch tarik penjualan kasir yang terjadi SETELAH item mulai dihitung di cabang terkait (O(1) query)
	type recentSaleRow struct {
		ProductID uint
		Qty       float64
		CreatedAt time.Time
	}
	salesByProd := make(map[uint][]recentSaleRow)
	if !minCountedTime.IsZero() && len(productIDs) > 0 {
		var recentSales []recentSaleRow
		salesQuery := tx.Model(&domain.Sales{}).
			Select("product_id, qty, created_at").
			Where("created_at > ?", minCountedTime)
		if strings.EqualFold(loc, "Toko Utama") {
			salesQuery = salesQuery.Where("location = ? OR location = ? OR LOWER(location) = 'pusat' OR location = '' OR location IS NULL", loc, strings.TrimPrefix(loc, "Toko "))
		} else {
			salesQuery = salesQuery.Where("location = ? OR location = ?", loc, strings.TrimPrefix(loc, "Toko "))
		}
		_ = salesQuery.Find(&recentSales).Error
		for _, sr := range recentSales {
			if productIDSet[sr.ProductID] {
				salesByProd[sr.ProductID] = append(salesByProd[sr.ProductID], sr)
			}
		}
	}

	// 2. Lock & ambil seluruh ProductStock cabang terkait dalam 1 batch query
	var existingPS []domain.ProductStock
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("location = ?", loc).
		Find(&existingPS).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal mengunci saldo cabang: %w", err)
	}
	psMap := make(map[uint]*domain.ProductStock, len(existingPS))
	for i := range existingPS {
		psMap[existingPS[i].ProductID] = &existingPS[i]
	}

	newStocksToCreate := make([]domain.ProductStock, 0)
	movementsToCreate := make([]domain.StockMovement, 0, len(items))

	for _, item := range items {
		// Hitung mutasi penjualan kasir yang terjadi SETELAH item ini dihitung fisik (counted_at / item.UpdatedAt)
		countedTime := item.UpdatedAt
		if countedTime.IsZero() {
			countedTime = item.CreatedAt
		}

		var salesAfterCount float64 = 0
		for _, sr := range salesByProd[item.ProductID] {
			if sr.CreatedAt.After(countedTime) {
				salesAfterCount += sr.Qty
			}
		}

		// Saldo sesudah = Hasil hitung fisik dikurangi penjualan yang baru saja laku setelah dihitung
		saldoSesudah := item.PhysicalQty - salesAfterCount
		if saldoSesudah < 0 {
			saldoSesudah = 0
		}

		saldoSebelum := 0.0
		if ps, exists := psMap[item.ProductID]; exists {
			saldoSebelum = ps.Saldo
			if math.Abs(ps.Saldo-saldoSesudah) > 0.0001 {
				if err := tx.Model(ps).Update("saldo", saldoSesudah).Error; err != nil {
					tx.Rollback()
					return nil, fmt.Errorf("gagal mengupdate saldo cabang: %w", err)
				}
				ps.Saldo = saldoSesudah
			}
		} else {
			newPS := domain.ProductStock{
				ProductID: item.ProductID,
				Location:  loc,
				Saldo:     saldoSesudah,
			}
			newStocksToCreate = append(newStocksToCreate, newPS)
		}

		diff := saldoSesudah - saldoSebelum
		var qtyIn float64 = 0
		var qtyOut float64 = 0
		if diff > 0 {
			qtyIn = diff
		} else if diff < 0 {
			qtyOut = -diff
		}

		movementNotes := fmt.Sprintf("Penyesuaian Opname: %s", item.Reason)
		if item.Notes != "" {
			movementNotes += fmt.Sprintf(" (%s)", item.Notes)
		}

		movementsToCreate = append(movementsToCreate, domain.StockMovement{
			ProductID:    item.ProductID,
			Location:     loc,
			MovementType: "OPNAME_ADJUST",
			ReferenceNo:  opname.OpnameNo,
			QtyIn:        qtyIn,
			QtyOut:       qtyOut,
			SaldoBefore:  saldoSebelum,
			SaldoAfter:   saldoSesudah,
			Notes:        movementNotes,
			UserID:       approverID,
		})
	}

	if len(newStocksToCreate) > 0 {
		if err := tx.CreateInBatches(&newStocksToCreate, 500).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("gagal inisialisasi product stock cabang: %w", err)
		}
	}

	if len(movementsToCreate) > 0 {
		if err := tx.CreateInBatches(&movementsToCreate, 500).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("gagal mencatat mutasi kartu stok: %w", err)
		}
	}

	// 3. Hybrid Sinkronisasi Stok Global (products.saldo):
	// - Jika seluruh 5 titik lokasi resmi sudah selesai SO untuk suatu produk -> sinkron penuh products.saldo = max(SUM(product_stocks), 0)
	// - Jika masih masa transisi SO sebagian lokasi -> hanya NAIKKAN products.saldo jika SUM(product_stocks) > products.saldo
	//   (tidak pernah menurunkan stok global sebelum ke-5 lokasi selesai agar penjualan kasir cabang lain tidak terblokir).
	globalUpdatedCount := 0
	globalRaisedCount := 0
	globalFullSyncCount := 0

	if len(productIDs) > 0 {
		type prodLocRow struct {
			ProductID uint
			Location  string
		}
		var completedRows []prodLocRow
		_ = tx.Table("stock_opname_items AS soi").
			Select("DISTINCT soi.product_id, so.location").
			Joins("JOIN stock_opnames AS so ON so.id = soi.stock_opname_id").
			Where("so.status = ? AND so.deleted_at IS NULL AND soi.deleted_at IS NULL", "Completed").
			Scan(&completedRows).Error

		completedLocs := make(map[uint]map[string]bool, len(productIDs))
		for _, r := range completedRows {
			if !productIDSet[r.ProductID] {
				continue
			}
			if completedLocs[r.ProductID] == nil {
				completedLocs[r.ProductID] = make(map[string]bool)
			}
			completedLocs[r.ProductID][NormalizeStockLocation(r.Location)] = true
		}
		for _, pid := range productIDs {
			if completedLocs[pid] == nil {
				completedLocs[pid] = make(map[string]bool)
			}
			completedLocs[pid][loc] = true
		}

		type stockSumRow struct {
			ProductID uint
			SumAll    float64
		}
		var sumRows []stockSumRow
		_ = tx.Table("product_stocks").
			Select("product_id, COALESCE(SUM(saldo), 0) AS sum_all").
			Where("deleted_at IS NULL AND location IN ?", OfficialStockLocations).
			Group("product_id").
			Scan(&sumRows).Error

		sumMap := make(map[uint]float64, len(sumRows))
		for _, r := range sumRows {
			sumMap[r.ProductID] = r.SumAll
		}

		prodSaldoMap := make(map[uint]float64, len(productIDs))
		for start := 0; start < len(productIDs); start += 1000 {
			end := start + 1000
			if end > len(productIDs) {
				end = len(productIDs)
			}
			var prods []domain.Product
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id, saldo").
				Where("id IN ?", productIDs[start:end]).
				Find(&prods).Error; err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("gagal mengunci data produk global: %w", err)
			}
			for _, p := range prods {
				prodSaldoMap[p.ID] = p.Saldo
			}
		}

		for _, pid := range productIDs {
			curGlobal, exists := prodSaldoMap[pid]
			if !exists {
				continue
			}
			sumAll := sumMap[pid]
			if sumAll < 0 {
				sumAll = 0
			}

			if hasAllOfficialLocations(completedLocs[pid]) {
				if math.Abs(sumAll-curGlobal) > 0.0001 {
					if err := tx.Model(&domain.Product{}).Where("id = ?", pid).Update("saldo", sumAll).Error; err != nil {
						tx.Rollback()
						return nil, fmt.Errorf("gagal menyinkronkan saldo global produk #%d: %w", pid, err)
					}
					globalUpdatedCount++
					globalFullSyncCount++
				}
			} else {
				// Masa transisi SO sebagian cabang: naikkan jika total fisik cabang > saldo global lama
				if sumAll > curGlobal+0.0001 {
					if err := tx.Model(&domain.Product{}).Where("id = ?", pid).Update("saldo", sumAll).Error; err != nil {
						tx.Rollback()
						return nil, fmt.Errorf("gagal menaikkan saldo global produk #%d: %w", pid, err)
					}
					globalUpdatedCount++
					globalRaisedCount++
				}
			}
		}
	}

	// 4. Kunci Sesi Dokumen
	opname.Location = loc
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

	opname.GlobalStockUpdatedCount = globalUpdatedCount
	opname.GlobalStockRaisedCount = globalRaisedCount
	opname.GlobalStockFullSyncCount = globalFullSyncCount

	if s.SSE != nil {
		payload := map[string]interface{}{
			"stock_opname_id":            opnameID,
			"status":                     opname.Status,
			"approver_id":                approverID,
			"global_stock_updated_count": globalUpdatedCount,
			"global_stock_raised_count":  globalRaisedCount,
			"global_stock_full_sync":     globalFullSyncCount,
		}
		s.SSE.Broadcast(fmt.Sprintf("opname:%d", opnameID), "status_changed", payload)
		s.SSE.Broadcast("opname:all", "status_changed", payload)
	}

	return &opname, nil
}

// buildLocationOpnameStatuses mengecek kelengkapan SO Completed untuk seluruh 5 lokasi resmi
func (s *OpnameService) buildLocationOpnameStatuses(dbConn *gorm.DB) ([]domain.LocationOpnameStatus, bool) {
	type locAgg struct {
		Location        string
		Count           int64
		LastCompletedAt *time.Time
	}
	var rows []locAgg
	_ = dbConn.Table("stock_opnames").
		Select("location, COUNT(*) AS count, MAX(completed_at) AS last_completed_at").
		Where("status = ? AND deleted_at IS NULL", "Completed").
		Group("location").
		Scan(&rows).Error

	aggMap := make(map[string]locAgg)
	for _, r := range rows {
		norm := NormalizeStockLocation(r.Location)
		existing, ok := aggMap[norm]
		if !ok {
			aggMap[norm] = locAgg{Location: norm, Count: r.Count, LastCompletedAt: r.LastCompletedAt}
		} else {
			existing.Count += r.Count
			if r.LastCompletedAt != nil && (existing.LastCompletedAt == nil || r.LastCompletedAt.After(*existing.LastCompletedAt)) {
				existing.LastCompletedAt = r.LastCompletedAt
			}
			aggMap[norm] = existing
		}
	}

	statuses := make([]domain.LocationOpnameStatus, 0, len(OfficialStockLocations))
	all5Done := true
	for _, official := range OfficialStockLocations {
		agg, ok := aggMap[official]
		hasDone := ok && agg.Count > 0
		if !hasDone {
			all5Done = false
		}
		statuses = append(statuses, domain.LocationOpnameStatus{
			Location:          official,
			HasCompletedSO:    hasDone,
			CompletedSessions: agg.Count,
			LastCompletedAt:   agg.LastCompletedAt,
		})
	}
	return statuses, all5Done
}

// GetGlobalStockSyncPreview menghitung dampak sinkronisasi stok global dari hasil SO tanpa mengubah database
func (s *OpnameService) GetGlobalStockSyncPreview() (*domain.GlobalStockSyncResult, error) {
	locStatuses, all5Done := s.buildLocationOpnameStatuses(s.DB)

	var productIDs []uint
	if err := s.DB.Table("stock_opname_items AS soi").
		Select("DISTINCT soi.product_id").
		Joins("JOIN stock_opnames AS so ON so.id = soi.stock_opname_id").
		Where("so.status = ? AND so.deleted_at IS NULL AND soi.deleted_at IS NULL", "Completed").
		Pluck("soi.product_id", &productIDs).Error; err != nil {
		return nil, fmt.Errorf("gagal memeriksa produk hasil SO: %w", err)
	}

	res := &domain.GlobalStockSyncResult{
		TotalProductsWithSO: len(productIDs),
		LocationStatuses:    locStatuses,
		All5LocationsDone:   all5Done,
	}
	if len(productIDs) == 0 {
		return res, nil
	}

	type stockSumRow struct {
		ProductID uint
		SumAll    float64
	}
	var sumRows []stockSumRow
	_ = s.DB.Table("product_stocks").
		Select("product_id, COALESCE(SUM(saldo), 0) AS sum_all").
		Where("deleted_at IS NULL AND location IN ?", OfficialStockLocations).
		Group("product_id").
		Scan(&sumRows).Error
	sumMap := make(map[uint]float64, len(sumRows))
	for _, r := range sumRows {
		sumMap[r.ProductID] = r.SumAll
	}

	for start := 0; start < len(productIDs); start += 1000 {
		end := start + 1000
		if end > len(productIDs) {
			end = len(productIDs)
		}
		var prods []domain.Product
		if err := s.DB.Select("id, saldo").Where("id IN ?", productIDs[start:end]).Find(&prods).Error; err != nil {
			return nil, err
		}
		for _, p := range prods {
			target := sumMap[p.ID]
			if target < 0 {
				target = 0
			}
			if target > p.Saldo+0.0001 {
				res.TotalChanged++
				res.TotalRaised++
			} else if target < p.Saldo-0.0001 {
				res.TotalChanged++
				res.TotalLowered++
			} else {
				res.TotalUnchanged++
			}
		}
	}

	return res, nil
}

// SyncGlobalStockFromOpname mengeksekusi finalisasi & sinkronisasi products.saldo = max(SUM(product_stocks.saldo), 0)
// untuk seluruh produk yang sudah memiliki minimal 1 sesi Stock Opname berstatus Completed
func (s *OpnameService) SyncGlobalStockFromOpname(userID string) (*domain.GlobalStockSyncResult, error) {
	tx := s.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	locStatuses, all5Done := s.buildLocationOpnameStatuses(tx)

	var productIDs []uint
	if err := tx.Table("stock_opname_items AS soi").
		Select("DISTINCT soi.product_id").
		Joins("JOIN stock_opnames AS so ON so.id = soi.stock_opname_id").
		Where("so.status = ? AND so.deleted_at IS NULL AND soi.deleted_at IS NULL", "Completed").
		Pluck("soi.product_id", &productIDs).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal mengambil daftar produk hasil SO: %w", err)
	}

	res := &domain.GlobalStockSyncResult{
		TotalProductsWithSO: len(productIDs),
		LocationStatuses:    locStatuses,
		All5LocationsDone:   all5Done,
	}

	if len(productIDs) == 0 {
		tx.Rollback()
		return res, errors.New("belum ada sesi Stock Opname berstatus Completed untuk disinkronkan")
	}

	type stockSumRow struct {
		ProductID uint
		SumAll    float64
	}
	var sumRows []stockSumRow
	if err := tx.Table("product_stocks").
		Select("product_id, COALESCE(SUM(saldo), 0) AS sum_all").
		Where("deleted_at IS NULL AND location IN ?", OfficialStockLocations).
		Group("product_id").
		Scan(&sumRows).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal menghitung total stok cabang: %w", err)
	}
	sumMap := make(map[uint]float64, len(sumRows))
	for _, r := range sumRows {
		sumMap[r.ProductID] = r.SumAll
	}

	for start := 0; start < len(productIDs); start += 1000 {
		end := start + 1000
		if end > len(productIDs) {
			end = len(productIDs)
		}
		var prods []domain.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id, saldo").
			Where("id IN ?", productIDs[start:end]).
			Find(&prods).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("gagal mengunci produk: %w", err)
		}

		for _, p := range prods {
			target := sumMap[p.ID]
			if target < 0 {
				target = 0
			}
			if target > p.Saldo+0.0001 {
				if err := tx.Model(&domain.Product{}).Where("id = ?", p.ID).Update("saldo", target).Error; err != nil {
					tx.Rollback()
					return nil, fmt.Errorf("gagal memperbarui saldo global produk #%d: %w", p.ID, err)
				}
				res.TotalChanged++
				res.TotalRaised++
			} else if target < p.Saldo-0.0001 {
				if err := tx.Model(&domain.Product{}).Where("id = ?", p.ID).Update("saldo", target).Error; err != nil {
					tx.Rollback()
					return nil, fmt.Errorf("gagal memperbarui saldo global produk #%d: %w", p.ID, err)
				}
				res.TotalChanged++
				res.TotalLowered++
			} else {
				res.TotalUnchanged++
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("gagal commit sinkronisasi stok global: %w", err)
	}

	if s.SSE != nil {
		s.SSE.Broadcast("opname:all", "global_stock_synced", map[string]interface{}{
			"user_id":       userID,
			"total_changed": res.TotalChanged,
			"total_raised":  res.TotalRaised,
			"total_lowered": res.TotalLowered,
			"timestamp":     time.Now().Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return res, nil
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
		query = query.Where("location = ?", NormalizeStockLocation(location))
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
	if err := s.DB.Where("product_id = ? AND location IN ?", productID, OfficialStockLocations).Order("location asc").Find(&stocks).Error; err != nil {
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
		query = query.Where("location = ?", NormalizeStockLocation(location))
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
	location = NormalizeStockLocation(location)

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
