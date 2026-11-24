package http

import (
	"backend-cashier/domain"
	"backend-cashier/service"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

type ExpenseHandler struct {
	Service     *service.ExpenseService
	AuthService *service.AuthService
}

func NewExpenseHandler(s *service.ExpenseService, auth ...*service.AuthService) *ExpenseHandler {
	h := &ExpenseHandler{Service: s}
	if len(auth) > 0 {
		h.AuthService = auth[0]
	}
	return h
}

func (h *ExpenseHandler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/api")
	api.Post("/expenses", h.Create)
	api.Get("/expenses", h.GetAll)
	api.Get("/expenses/:id", h.GetByID)
	api.Put("/expenses/:id", h.Update)
	api.Delete("/expenses/:id", h.Delete)

	// --- RUTE UNDO & CLOSE SHIFT MANUAL (SUPER ADMIN) ---
	api.Put("/expenses/undo-shift/transaction/:id", h.UndoShiftExpense)
	api.Put("/expenses/undo-shift/transaction", h.UndoShiftExpense)
	api.Put("/expenses/close-shift/transaction/:id", h.CloseShiftExpense)
	api.Put("/expenses/close-shift/transaction", h.CloseShiftExpense)
	api.Put("/expenses/close-shift/bulk", h.CloseShiftBulk)
}

func (h *ExpenseHandler) parseClaims(c *fiber.Ctx) (jwt.MapClaims, error) {
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

// checkSuperadmin memvalidasi bahwa pengguna yang mengakses endpoint memiliki role superadmin
func (h *ExpenseHandler) checkSuperadmin(c *fiber.Ctx) (jwt.MapClaims, error) {
	claims, err := h.parseClaims(c)
	if err != nil || claims == nil {
		return nil, errors.New("otentikasi diperlukan: silakan login terlebih dahulu")
	}
	role := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", claims["role"])))
	normRole := strings.ReplaceAll(strings.ReplaceAll(role, "-", ""), "_", "")
	if normRole != "superadmin" {
		return claims, errors.New("akses ditolak: hanya role Super Admin yang berhak memodifikasi tutup shift pengeluaran")
	}
	return claims, nil
}

func (h *ExpenseHandler) Create(c *fiber.Ctx) error {
	var req domain.ExpenseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"status": "error", "message": "Format data tidak valid"})
	}

	expense := &domain.Expense{
		Nota:      req.Nota,
		Tanggal:   req.Tanggal,
		Deskripsi: req.Deskripsi,
		Total:     req.Total,
		Kategori:  req.Kategori,
		UserID:    req.UserID,
		Location:  req.Location,
		Shift:     req.Shift,
	}

	if err := h.Service.CreateExpense(expense); err != nil {
		return c.Status(500).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{"status": "success", "data": expense})
}

func (h *ExpenseHandler) GetAll(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	search := c.Query("search", "")
	location := c.Query("location", "")
	shift := c.Query("shift", "")
	shiftStatus := c.Query("shift_status", "")

	expenses, total, err := h.Service.GetAll(page, limit, search, location, shift, shiftStatus)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}

	return c.JSON(fiber.Map{
		"status": "success",
		"data":   expenses,
		"meta": fiber.Map{
			"total": total,
			"page":  page,
			"limit": limit,
		},
	})
}

func (h *ExpenseHandler) GetByID(c *fiber.Ctx) error {
	id := c.Params("id")
	expense, err := h.Service.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"status": "error", "message": "Expense not found"})
	}
	return c.JSON(fiber.Map{"status": "success", "data": expense})
}

func (h *ExpenseHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var req domain.ExpenseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"status": "error", "message": "Format data tidak valid"})
	}

	input := &domain.Expense{
		Nota:      req.Nota,
		Tanggal:   req.Tanggal,
		Deskripsi: req.Deskripsi,
		Total:     req.Total,
		Kategori:  req.Kategori,
		UserID:    req.UserID,
		Location:  req.Location,
		Shift:     req.Shift,
	}

	if err := h.Service.UpdateExpense(id, input); err != nil {
		return c.Status(500).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "success", "message": "Expense updated"})
}

func (h *ExpenseHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.Service.DeleteExpense(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"status": "error", "message": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "success", "message": "Expense deleted"})
}

// UndoShiftExpense membatalkan status tutup shift pengeluaran (kembali ke Belum Tutup)
func (h *ExpenseHandler) UndoShiftExpense(c *fiber.Ctx) error {
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

	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		var bodyReq struct {
			ID   string `json:"id"`
			Nota string `json:"nota"`
		}
		_ = c.BodyParser(&bodyReq)
		if bodyReq.ID != "" {
			id = bodyReq.ID
		} else {
			id = bodyReq.Nota
		}
	}

	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "ID atau nomor nota pengeluaran tidak boleh kosong",
		})
	}

	if err := h.Service.UndoShiftByID(id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("Tutup shift pengeluaran #%s berhasil dibatalkan (Belum Tutup Shift).", id),
		"id":      id,
	})
}

// CloseShiftExpense melakukan tutup shift manual untuk satu pengeluaran
func (h *ExpenseHandler) CloseShiftExpense(c *fiber.Ctx) error {
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

	var req domain.CloseShiftExpenseRequest
	_ = c.BodyParser(&req)

	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		if req.ID > 0 {
			id = strconv.FormatUint(uint64(req.ID), 10)
		} else if req.Nota != "" {
			id = req.Nota
		}
	}

	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "ID atau nomor nota pengeluaran tidak boleh kosong",
		})
	}
	if req.Shift == 0 {
		req.Shift = 1
	}

	if err := h.Service.CloseShiftByID(id, req.Shift); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("Pengeluaran #%s berhasil di-close shift ke Shift %d.", id, req.Shift),
		"id":      id,
		"shift":   req.Shift,
	})
}

// CloseShiftBulk melakukan tutup shift manual secara bulk untuk banyak pengeluaran
func (h *ExpenseHandler) CloseShiftBulk(c *fiber.Ctx) error {
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

	var req domain.CloseShiftExpenseBulkRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Format data request tidak valid",
		})
	}

	if len(req.IDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Pilih minimal satu pengeluaran untuk di-close shift",
		})
	}
	if req.Shift == 0 {
		req.Shift = 1
	}

	affected, err := h.Service.CloseShiftBulk(req.IDs, req.Shift)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":          fmt.Sprintf("Berhasil melakukan close shift ke Shift %d untuk %d data pengeluaran.", req.Shift, affected),
		"affected_expense": affected,
		"shift":            req.Shift,
		"ids":              req.IDs,
	})
}
