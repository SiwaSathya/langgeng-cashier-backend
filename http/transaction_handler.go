package http

import (
	"backend-cashier/domain"
	"backend-cashier/service"
	"fmt"
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

	// --- RUTE SALES RETUR & USER SALES ---
	api.Get("/sales/user/:user_id", h.GetUserSales)
	// api.Get("/sales/:id/is-retur", h.IsRetur)
	// api.Put("/sales/:id/is-retur", h.IsReturUpdate) // Menggunakan Query Param ?status=true
	// api.Get("/sales/:id/is-retur-company", h.IsReturToCompany)
	// api.Put("/sales/:id/is-retur-company", h.IsReturToCompanyUpdate) // Menggunakan Query Param ?status=true
}

// @Summary      Input Penjualan Kasir (Full Detail)
// @Tags         Transactions
// @Param        request body http.SalesRequest true "Payload Penjualan"
// @Success      201  {object}  domain.SalesResponse
// @Router       /api/sales [post]
func (h *TransactionHandler) CreateSales(c *fiber.Ctx) error {
	var req []SalesRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Format data salah"})
	}

	fmt.Println("Request sebelum mapping:", req)

	var salesRequests []domain.SalesRequest
	for _, item := range req {
		salesRequests = append(salesRequests, domain.SalesRequest{
			ProductSearch: item.ProductSearch,
			MemberName:    item.MemberName,
			Qty:           item.Qty,
			Price:         item.Price,
			Discount:      item.Discount,
			PaymentMethod: item.PaymentMethod,
			AmountPaid:    item.AmountPaid,
			IsDp:          item.IsDp,
			UserID:        item.UserID,
			Customer: domain.Customer{
				Name:           item.Customer.Name,
				Age:            item.Customer.Age,
				Address:        item.Customer.Address,
				PhoneNumber:    item.Customer.PhoneNumber,
				IdentityNumber: item.Customer.IdentityNumber,
			},
		})
	}

	fmt.Println("Request setelah mapping ke domain.SalesRequest:", salesRequests)
	result, err := h.Service.CreateSales(salesRequests)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(result)
}

// GetSales godoc
// @Summary      Laporan Penjualan (Filter)
// @Tags         Reports
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

// GetUserSales mengambil data penjualan berdasarkan ID User
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

func (h *TransactionHandler) PutPelunasanSales(c *fiber.Ctx) error {
	// 1. Ambil ID dari parameter URL (/api/sales/19 -> ID = 19)
	idParam := c.Params("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "ID transaksi tidak valid",
		})
	}

	// 2. Parse JSON body yang dikirim oleh SweetAlert / Frontend
	var req service.PelunasanRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Format data request pelunasan tidak valid: " + err.Error(),
		})
	}

	// Auto-fallback jika status dari frontend tidak terisi
	if req.Status == "" {
		req.Status = "Lunas"
	}

	// 3. Panggil fungsi PelunasanSales yang sudah kita buat di TransactionService
	result, err := h.Service.PelunasanSales(uint(id), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// 4. Kembalikan response sukses 200 OK ke frontend riwayat.html
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Pelunasan transaksi berhasil disimpan!",
		"data":    result,
	})
}
