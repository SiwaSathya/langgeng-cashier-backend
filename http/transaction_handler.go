package http

import (
	"backend-cashier/domain"
	"backend-cashier/service"
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/gofiber/fiber/v2"
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
	Service *service.TransactionService
}

func NewTransactionHandler(s *service.TransactionService) *TransactionHandler {
	return &TransactionHandler{Service: s}
}

func (h *TransactionHandler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/api")
	api.Post("/sales", h.CreateSales)
	api.Get("/sales", h.GetSales)

	api.Post("/expenditure", h.CreateExpenditure)
	api.Get("/expenditure", h.GetExpenditure)
	api.Put("/sales/:id", h.PutPelunasanSales)
	api.Put("/sales/pelunasan/:invoice", h.PutPelunasanSales)
	api.Put("/sales/invoice/:invoice/pelunasan", h.PutPelunasanSales)

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

	err := h.Service.UpdateSalesShift(
		req.Shift,
	)

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
			"message": "Shift transaksi berhasil diperbarui",
			"shift":   req.Shift,
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
	results, err := h.Service.GetReceipt()

	if err != nil {
		return c.Status(500).JSON(
			fiber.Map{
				"error": err.Error(),
			},
		)
	}

	return c.JSON(
		domain.ReceiptResponse{
			Category: results.Category,
			Sales:    results.Sales,
			Expenses: results.Expenses,
		},
	)
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
	start := c.Query("start_date")
	end := c.Query("end_date")
	member := c.Query("member")
	method := c.Query("method")

	filter := domain.SalesFilter{
		StartDate: start,
		EndDate:   end,
		Member:    member,
		Method:    method,
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
	shift := h.Service.GetCurrentShift()

	return c.JSON(
		fiber.Map{
			"shift": shift,
		},
	)
}