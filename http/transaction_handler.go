package http

import (
	"backend-cashier/domain"
	"backend-cashier/service"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

// SalesRequest mendefinisikan input untuk kasir
type SalesRequest struct {
	ProductSearch string                 `json:"product_search" example:"POLY 32RG9059"`
	MemberName    string                 `json:"member_name" example:"Desak"`
	Qty           float64                `json:"qty" example:"1"`
	UserID        string                 `json:"user_id" example:"123"`
	Price         float64                `json:"price" example:"20000"`
	Discount      float64                `json:"discount" example:"0"`
	PaymentMethod string                 `json:"payment_method" example:"Bayar Tunai"`
	AmountPaid    float64                `json:"amount_paid" example:"2500000"`
	IsDp          bool                   `json:"is_dp" example:"false"`
	Customer      domain.CustomerRequest `json:"customer"`
}

// ExpenditureRequest mendefinisikan input untuk pengeluaran barang/operasional
type ExpenditureRequest struct {
	Nota  string  `json:"nota" example:"PBL20260501"`
	Kode  string  `json:"kode_barang" example:"101824"`
	Nama  string  `json:"nama" example:"BIAYA ANGKUT"`
	Qty   float64 `json:"qty" example:"1"`
	Harga float64 `json:"harga" example:"50000"`
	Total float64 `json:"total" example:"50000"`
}

type TransactionHandler struct {
	Service     *service.TransactionService
	AuthService *service.AuthService
}

func NewTransactionHandler(s *service.TransactionService, auth ...*service.AuthService) *TransactionHandler {
	h := &TransactionHandler{Service: s}
	if len(auth) > 0 {
		h.AuthService = auth[0]
	}
	return h
}

func (h *TransactionHandler) parseClaims(c *fiber.Ctx) (jwt.MapClaims, error) {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		authHeader = c.Query("token")
	}
	if authHeader == "" {
		return nil, errors.New("token otentikasi tidak ditemukan")
	}

	tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if h.AuthService != nil && h.AuthService.IsTokenRevoked(tokenStr) {
		return nil, errors.New("token telah dicabut (revoked), silakan login kembali")
	}

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("alg signing invalid")
		}
		return service.GetJWTSecret(), nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("token otentikasi tidak valid atau telah kedaluwarsa")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("claims token tidak valid")
	}
	return claims, nil
}

func (h *TransactionHandler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/api")
	api.Post("/sales", h.CreateSales)
	api.Get("/sales", h.GetSales)
	api.Get("/sales/live", h.LiveSales)

	api.Post("/expenditure", h.CreateExpenditure)
	api.Get("/expenditure", h.GetExpenditure)
	api.Put("/sales/:id", h.PutPelunasanSales)
	api.Put("/sales/pelunasan/:invoice", h.PutPelunasanSales)
	api.Put("/sales/invoice/:invoice/pelunasan", h.PutPelunasanSales)
	api.Delete("/sales/:id", h.DeleteSales)
	api.Delete("/sales/invoice/:invoice", h.DeleteSales)

	// --- RUTE SALES RETUR & USER SALES ---
	api.Get("/sales/user/:user_id", h.GetUserSales)
	api.Get("/shift/current", h.GetCurrentShift)
	api.Get(
		"/receipt",
		h.GetReceipt,
	)

	api.Put(
		"/sales/update/shift",
		h.UpdateSalesShift,
	)

	// --- RUTE UNDO SHIFT (SUPER ADMIN) ---
	api.Put("/sales/undo-shift/transaction/:invoice", h.UndoShiftTransaction)
	api.Put("/sales/undo-shift/transaction", h.UndoShiftTransaction)
	api.Get("/sales/shift-summary", h.GetShiftSummary)
	api.Put("/sales/undo-shift/batch", h.UndoShiftBatch)

	// --- RUTE CLOSE SHIFT MANUAL (SUPER ADMIN) ---
	api.Put("/sales/close-shift/transaction/:invoice", h.CloseShiftTransaction)
	api.Put("/sales/close-shift/transaction", h.CloseShiftTransaction)
	api.Put("/sales/close-shift/bulk", h.CloseShiftBulk)
}

// UpdateSalesShift
// @Summary Update shift transaksi
// @Tags Sales
// @Router /api/sales/update-shift [put]
func (h *TransactionHandler) UpdateSalesShift(
	c *fiber.Ctx,
) error {
	var req domain.UpdateShiftRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(
			400,
		).JSON(
			fiber.Map{
				"error": "format request salah",
			},
		)
	}

	if req.Shift == 0 {
		return c.Status(
			400,
		).JSON(
			fiber.Map{
				"error": "shift tidak boleh kosong",
			},
		)
	}

	err := h.Service.UpdateSalesShift(req.Shift, req.Location)
	if err != nil {
		return c.Status(
			500,
		).JSON(
			fiber.Map{
				"error": err.Error(),
			},
		)
	}

	return c.JSON(
		fiber.Map{
			"message":  "Shift transaksi berhasil diperbarui",
			"shift":    req.Shift,
			"location": req.Location,
		},
	)
}

// GetReceipt mengambil transaksi yang belum mempunyai shift
// @Summary      Data Nota Shift
// @Tags         Receipt
// @Router       /api/receipt [get]
func (h *TransactionHandler) GetReceipt(
	c *fiber.Ctx,
) error {
	location := c.Query("location")
	results, err := h.Service.GetReceipt(location)

	if err != nil {
		return c.Status(500).JSON(
			fiber.Map{
				"error": err.Error(),
			},
		)
	}

	return c.JSON(results)
}

// CreateSales godoc
// @Summary      Input Penjualan Kasir (Per Nota / Invoice)
// @Description  Membuat transaksi penjualan per-nota. Untuk transaksi DP, status DP dan customer dicatat per-nota.
// @Tags         Transactions
// @Accept       json
// @Produce      json
// @Param        request body domain.CreateTransactionRequest true "Payload Penjualan Per-Nota"
// @Success      201  {object}  domain.SalesResponse
// @Failure      400  {object}  map[string]string
// @Router       /api/sales [post]
func (h *TransactionHandler) CreateSales(c *fiber.Ctx) error {
	var txReq domain.CreateTransactionRequest

	rawBody := c.Body()
	trimmed := bytes.TrimSpace(rawBody)

	if len(trimmed) > 0 && trimmed[0] == '[' {
		// Format legacy array []SalesRequest
		var items []domain.SalesRequest
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Format data array tidak valid: " + err.Error()})
		}
		if len(items) == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "Data transaksi kosong"})
		}

		// Header transaksi diambil dari elemen pertama
		txReq.MemberName = items[0].MemberName
		txReq.UserID = items[0].UserID
		txReq.PaymentMethod = items[0].PaymentMethod
		txReq.AmountPaid = items[0].AmountPaid
		txReq.IsDp = items[0].IsDp
		txReq.IsBon = items[0].IsBon
		txReq.Customer = items[0].Customer

		for _, it := range items {
			txReq.Items = append(txReq.Items, domain.SalesItemRequest{
				ProductSearch: it.ProductSearch,
				Qty:           it.Qty,
				Price:         it.Price,
				Discount:      it.Discount,
			})
		}
	} else {
		// Format JSON object CreateTransactionRequest
		if err := json.Unmarshal(trimmed, &txReq); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Format data JSON tidak valid: " + err.Error()})
		}

		// Jika dikirim sebagai single SalesRequest object tanpa `items`
		if len(txReq.Items) == 0 {
			var single domain.SalesRequest
			if err := json.Unmarshal(trimmed, &single); err == nil && single.ProductSearch != "" {
				txReq.MemberName = single.MemberName
				txReq.UserID = single.UserID
				txReq.PaymentMethod = single.PaymentMethod
				txReq.AmountPaid = single.AmountPaid
				txReq.IsDp = single.IsDp
				txReq.Customer = single.Customer
				txReq.IsBon = single.IsBon
				txReq.Items = append(txReq.Items, domain.SalesItemRequest{
					ProductSearch: single.ProductSearch,
					Qty:           single.Qty,
					Price:         single.Price,
					Discount:      single.Discount,
				})
			}
		}
	}

	result, err := h.Service.CreateSales(txReq)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(result)
}

// GetSales godoc
// @Summary      Laporan Penjualan (Dikelompokkan Per-Nota)
// @Description  Mengambil data transaksi penjualan yang dikelompokkan berdasarkan nomor nota (invoice).
// @Tags         Reports
// @Produce      json
// @Param        start_date query     string false "Tanggal Mulai (YYYY-MM-DD)"
// @Param        end_date   query     string false "Tanggal Akhir (YYYY-MM-DD)"
// @Param        member     query     string false "Filter Nama Member"
// @Param        method     query     string false "Filter Metode Pembayaran"
// @Success      200        {array}   domain.SalesTransactionGroup
// @Failure      500        {object}  map[string]string
// @Router       /api/sales [get]
func (h *TransactionHandler) GetSales(c *fiber.Ctx) error {
	var filter domain.SalesFilter
	if err := c.QueryParser(&filter); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Parameter filter tidak valid"})
	}

	// Ambil query param manual jika query parser melewatkan string kosong/default
	if filter.StartDate == "" {
		filter.StartDate = c.Query("start_date")
	}
	if filter.EndDate == "" {
		filter.EndDate = c.Query("end_date")
	}
	if filter.Member == "" {
		filter.Member = c.Query("member")
	}
	if filter.Method == "" {
		filter.Method = c.Query("method")
	}
	if filter.Location == "" {
		filter.Location = c.Query("location")
	}
	if filter.UserLocation == "" {
		filter.UserLocation = c.Query("user_location")
	}
	if filter.Invoice == "" {
		filter.Invoice = c.Query("invoice")
	}
	if filter.Status == "" {
		filter.Status = c.Query("status")
	}
	if filter.ShiftStatus == "" {
		filter.ShiftStatus = c.Query("shift_status")
	}
	if sVal := c.Query("shift"); sVal != "" {
		if sVal == "unclosed" || sVal == "open" || sVal == "belum" {
			filter.ShiftStatus = "unclosed"
			filter.Shift = 0
		} else if filter.Shift == 0 {
			if sInt, err := strconv.Atoi(sVal); err == nil && sInt > 0 {
				filter.Shift = uint(sInt)
			}
		}
	}
	if filter.Kasir == "" {
		filter.Kasir = c.Query("kasir")
	}
	if filter.ProductSearch == "" {
		filter.ProductSearch = c.Query("product_search")
	}
	if filter.Search == "" {
		filter.Search = c.Query("search")
	}

	isPaginated := c.Query("page") != "" || c.Query("limit") != ""
	if isPaginated {
		if filter.Limit <= 0 {
			filter.Limit = 20
		}
		if filter.Page <= 0 {
			filter.Page = 1
		}
		groups, total, err := h.Service.GetPaginatedSales(filter)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		totalPages := int(math.Ceil(float64(total) / float64(filter.Limit)))
		return c.JSON(fiber.Map{
			"data":        groups,
			"total":       total,
			"page":        filter.Page,
			"limit":       filter.Limit,
			"total_pages": totalPages,
		})
	}

	results, err := h.Service.GetAllSales(filter)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(results)
}

// CreateExpenditure godoc
// @Summary      Input Pengeluaran (Barang/Operasional)
// @Tags         Transactions
// @Router       /api/expenditure [post]
func (h *TransactionHandler) CreateExpenditure(c *fiber.Ctx) error {
	var req ExpenditureRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	result, err := h.Service.CreateExpenditure(req.Nota, req.Kode, req.Nama, req.Qty, req.Harga, req.Total)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

// GetExpenditure godoc
// @Summary      Laporan Pengeluaran (Filter)
// @Tags         Reports
// @Router       /api/expenditure [get]
func (h *TransactionHandler) GetExpenditure(c *fiber.Ctx) error {
	filter := domain.ExpenseFilter{
		StartDate: c.Query("start_date"),
		EndDate:   c.Query("end_date"),
		Type:      c.Query("type"),
	}

	results, err := h.Service.GetAllExpenditure(filter)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(results)
}

// GetUserSales godoc
// @Summary      Laporan Penjualan User (Dikelompokkan Per-Nota)
// @Tags         Reports
// @Produce      json
// @Param        user_id path      int    true "User ID"
// @Success      200     {array}   domain.SalesTransactionGroup
// @Failure      400     {object}  map[string]string
// @Failure      500     {object}  map[string]string
// @Router       /api/sales/user/{user_id} [get]
func (h *TransactionHandler) GetUserSales(c *fiber.Ctx) error {
	userIdStr := c.Params("user_id")
	userId, err := strconv.ParseUint(userIdStr, 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID User tidak valid"})
	}

	results, err := h.Service.GetUserSales(uint(userId))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(results)
}

// PutPelunasanSales godoc
// @Summary      Pelunasan Transaksi Penjualan (Per-Nota)
// @Description  Melunasi transaksi penjualan berstatus DP untuk satu nota secara utuh.
// @Tags         Transactions
// @Accept       json
// @Produce      json
// @Param        id       path      string                  true  "Sales ID atau Nomor Invoice (e.g. PJL-...)"
// @Param        request  body      domain.PelunasanRequest true  "Payload Pelunasan"
// @Success      200      {object}  map[string]interface{}
// @Failure      400      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /api/sales/{id} [put]
func (h *TransactionHandler) PutPelunasanSales(c *fiber.Ctx) error {
	identifier := c.Params("id")
	if identifier == "" {
		identifier = c.Params("invoice")
	}
	if identifier == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "ID atau Nomor Invoice transaksi tidak boleh kosong",
		})
	}

	// Verifikasi Keamanan IDOR / BOLA (Broken Object Level Authorization)
	if claims, err := h.parseClaims(c); err == nil && claims != nil {
		userRole := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", claims["role"])))
		normalizedRole := strings.ReplaceAll(strings.ReplaceAll(userRole, "-", ""), "_", "")
		if normalizedRole != "superadmin" && normalizedRole != "admin" {
			userLocation := strings.TrimSpace(fmt.Sprintf("%v", claims["location"]))
			if userLocation == "" || strings.EqualFold(userLocation, "Pusat") {
				userLocation = "Toko Utama"
			}

			sale, errSale := h.Service.GetSaleByIdentifier(identifier)
			if errSale == nil && sale != nil {
				saleLoc := strings.TrimSpace(sale.Location)
				if saleLoc == "" || strings.EqualFold(saleLoc, "Pusat") {
					saleLoc = "Toko Utama"
				}

				if !strings.EqualFold(saleLoc, userLocation) {
					return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
						"error": fmt.Sprintf("Akses Ditolak (BOLA/IDOR Prevention): Anda tidak memiliki izin untuk melunasi nota transaksi dari lokasi cabang lain (%s)", sale.Location),
						"code":  "BOLA_IDOR_VIOLATION",
					})
				}
			}
		}
	}

	var req domain.PelunasanRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Format data request pelunasan tidak valid: " + err.Error(),
		})
	}

	if req.Status == "" {
		req.Status = "Lunas"
	}

	result, err := h.Service.PelunasanSales(identifier, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Pelunasan transaksi nota berhasil disimpan!",
		"data":    result,
	})
}

func (h *TransactionHandler) GetCurrentShift(
	c *fiber.Ctx,
) error {
	location := c.Query("location")
	shift := h.Service.GetCurrentShift(location)

	return c.JSON(
		fiber.Map{
			"shift":    shift,
			"location": location,
		},
	)
}

func (h *TransactionHandler) DeleteSales(c *fiber.Ctx) error {
	identifier := c.Params("id")
	if identifier == "" {
		identifier = c.Params("invoice")
	}
	if identifier == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Nomor invoice atau ID transaksi tidak boleh kosong",
		})
	}

	// 1. Verifikasi Keamanan IDOR / BOLA (Broken Object Level Authorization)
	claims, errClaims := h.parseClaims(c)
	if h.AuthService != nil && errClaims != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Otentikasi diperlukan untuk membatalkan nota transaksi: " + errClaims.Error(),
			"code":  "AUTH_REQUIRED",
		})
	}

	if claims != nil {
		userRole := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", claims["role"])))
		normalizedRole := strings.ReplaceAll(strings.ReplaceAll(userRole, "-", ""), "_", "")
		if normalizedRole != "superadmin" && normalizedRole != "admin" {
			userLocation := strings.TrimSpace(fmt.Sprintf("%v", claims["location"]))
			if userLocation == "" || strings.EqualFold(userLocation, "Pusat") {
				userLocation = "Toko Utama"
			}

			sale, errSale := h.Service.GetSaleByIdentifier(identifier)
			if errSale != nil {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error": "Data transaksi tidak ditemukan",
				})
			}

			saleLoc := strings.TrimSpace(sale.Location)
			if saleLoc == "" || strings.EqualFold(saleLoc, "Pusat") {
				saleLoc = "Toko Utama"
			}

			if !strings.EqualFold(saleLoc, userLocation) {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error": fmt.Sprintf("Akses Ditolak (BOLA/IDOR Prevention): Anda tidak memiliki izin untuk membatalkan nota dari lokasi cabang lain (%s)", sale.Location),
					"code":  "BOLA_IDOR_VIOLATION",
				})
			}
		}
	}

	if err := h.Service.DeleteSales(identifier); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Transaksi berhasil dibatalkan dan seluruh stok produk telah dikembalikan",
	})
}

// LiveSales menyiarkan event transaksi penjualan dan retur secara real-time via SSE dengan isolasi outlet (Cross-Store Data Leakage Prevention)
func (h *TransactionHandler) LiveSales(c *fiber.Ctx) error {
	topic := "sales:live"
	locationQuery := strings.TrimSpace(c.Query("location"))

	if claims, err := h.parseClaims(c); err == nil && claims != nil {
		userRole := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", claims["role"])))
		normalizedRole := strings.ReplaceAll(strings.ReplaceAll(userRole, "-", ""), "_", "")

		if normalizedRole != "superadmin" && normalizedRole != "admin" {
			// Kasir cabang hanya boleh menerima stream penjualan di cabangnya sendiri
			userLoc := strings.TrimSpace(fmt.Sprintf("%v", claims["location"]))
			if userLoc == "" || strings.EqualFold(userLoc, "Pusat") {
				userLoc = "Toko Utama"
			}
			topic = "sales:live:" + userLoc
		} else if locationQuery != "" && !strings.EqualFold(locationQuery, "Semua Toko") {
			// Admin / Super Admin memilih filter outlet tertentu
			if strings.EqualFold(locationQuery, "Pusat") {
				locationQuery = "Toko Utama"
			}
			topic = "sales:live:" + locationQuery
		}
	}

	return StreamSSE(c, service.GetGlobalSSE(), topic)
}

// checkSuperadmin memvalidasi bahwa pengguna yang mengakses endpoint memiliki role superadmin
func (h *TransactionHandler) checkSuperadmin(c *fiber.Ctx) (jwt.MapClaims, error) {
	claims, err := h.parseClaims(c)
	if err != nil || claims == nil {
		return nil, errors.New("otentikasi diperlukan: silakan login terlebih dahulu")
	}
	role := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", claims["role"])))
	normRole := strings.ReplaceAll(strings.ReplaceAll(role, "-", ""), "_", "")
	if normRole != "superadmin" {
		return claims, errors.New("akses ditolak: hanya role Super Admin yang berhak membatalkan (undo) tutup shift")
	}
	return claims, nil
}

// UndoShiftTransaction membatalkan tutup shift untuk satu nota transaksi tertentu
func (h *TransactionHandler) UndoShiftTransaction(c *fiber.Ctx) error {
	claims, err := h.checkSuperadmin(c)
	if err != nil {
		status := fiber.StatusForbidden
		if claims == nil {
			status = fiber.StatusUnauthorized
		}
		return c.Status(status).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	invoice := strings.TrimSpace(c.Params("invoice"))
	if invoice == "" {
		var req domain.UndoShiftTransactionRequest
		if err := c.BodyParser(&req); err == nil && req.Invoice != "" {
			invoice = strings.TrimSpace(req.Invoice)
		}
	}

	if invoice == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Nomor invoice tidak boleh kosong",
		})
	}

	if err := h.Service.UndoShiftByInvoice(invoice); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("Tutup shift untuk nota #%s berhasil dibatalkan. Transaksi kembali berstatus Belum Tutup Shift.", invoice),
		"invoice": invoice,
	})
}

// GetShiftSummary mengambil ringkasan shift yang telah ditutup pada tanggal & lokasi tertentu
func (h *TransactionHandler) GetShiftSummary(c *fiber.Ctx) error {
	dateStr := strings.TrimSpace(c.Query("date"))
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}
	location := strings.TrimSpace(c.Query("location"))

	summary, err := h.Service.GetShiftSummaryByDate(dateStr, location)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"date":     dateStr,
		"location": location,
		"shifts":   summary,
	})
}

// UndoShiftBatch membatalkan tutup shift secara massal untuk shift tertentu pada tanggal & lokasi yang dipilih
func (h *TransactionHandler) UndoShiftBatch(c *fiber.Ctx) error {
	claims, err := h.checkSuperadmin(c)
	if err != nil {
		status := fiber.StatusForbidden
		if claims == nil {
			status = fiber.StatusUnauthorized
		}
		return c.Status(status).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var req domain.UndoShiftBatchRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Format data request tidak valid",
		})
	}

	if req.Shift == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Pilih shift yang ingin dibatalkan (Shift 1, Shift 2, dst)",
		})
	}

	affected, err := h.Service.UndoShiftBatch(req.Date, req.Shift, req.Location)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":        fmt.Sprintf("Berhasil membatalkan tutup Shift %d. %d transaksi penjualan dan pengeluaran terkait telah dikembalikan ke status Belum Tutup Shift.", req.Shift, affected),
		"affected_sales": affected,
		"shift":          req.Shift,
		"date":           req.Date,
	})
}


// CloseShiftTransaction melakukan close shift manual untuk satu transaksi tertentu
func (h *TransactionHandler) CloseShiftTransaction(c *fiber.Ctx) error {
	claims, err := h.checkSuperadmin(c)
	if err != nil {
		status := fiber.StatusForbidden
		if claims == nil {
			status = fiber.StatusUnauthorized
		}
		return c.Status(status).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var req domain.CloseShiftTransactionRequest
	_ = c.BodyParser(&req)
	if req.Invoice == "" {
		req.Invoice = c.Params("invoice")
	}

	invoice := strings.TrimSpace(req.Invoice)
	if invoice == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Nomor invoice tidak boleh kosong",
		})
	}
	if req.Shift == 0 {
		req.Shift = 1
	}

	if err := h.Service.CloseShiftByInvoice(invoice, req.Shift); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("Transaksi nota #%s berhasil di-close shift ke Shift %d.", invoice, req.Shift),
		"invoice": invoice,
		"shift":   req.Shift,
	})
}

// CloseShiftBulk melakukan close shift manual secara bulk untuk banyak transaksi sekaligus
func (h *TransactionHandler) CloseShiftBulk(c *fiber.Ctx) error {
	claims, err := h.checkSuperadmin(c)
	if err != nil {
		status := fiber.StatusForbidden
		if claims == nil {
			status = fiber.StatusUnauthorized
		}
		return c.Status(status).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var req domain.CloseShiftBulkRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Format data request tidak valid",
		})
	}

	if len(req.Invoices) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Pilih minimal satu transaksi untuk di-close shift",
		})
	}
	if req.Shift == 0 {
		req.Shift = 1
	}

	affected, err := h.Service.CloseShiftBulk(req.Invoices, req.Shift)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":        fmt.Sprintf("Berhasil melakukan close shift ke Shift %d untuk %d transaksi penjualan.", req.Shift, affected),
		"affected_sales": affected,
		"shift":          req.Shift,
		"invoices":       req.Invoices,
	})
}
