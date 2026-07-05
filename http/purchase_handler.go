package http

import (
	"backend-cashier/domain"
	"backend-cashier/service"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type PurchaseHandler struct {
	Service *service.PurchaseService
}

func NewPurchaseHandler(s *service.PurchaseService) *PurchaseHandler {
	return &PurchaseHandler{Service: s}
}

func (h *PurchaseHandler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/api")

	api.Post("/purchases", h.Create)
	api.Get("/purchases", h.GetAll)
	api.Get("/purchases/:id", h.GetByID)
	api.Delete("/purchases/:id", h.Delete)
}

func (h *PurchaseHandler) Create(c *fiber.Ctx) error {
	purchase := new(domain.Purchase)
	if err := c.BodyParser(purchase); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"status":  "error",
			"message": "Format data tidak valid",
		})
	}

	// Pastikan service menerima pointer ke domain.Purchase
	if err := h.Service.CreatePurchase(purchase); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}

	return c.Status(201).JSON(fiber.Map{
		"status":  "success",
		"message": "Pembelian berhasil dicatat",
		"data":    purchase,
	})
}

func (h *PurchaseHandler) GetAll(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	search := c.Query("search", "")

	purchases, total, err := h.Service.GetAll(page, limit, search)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"status": "success",
		"data":   purchases,
		"meta": fiber.Map{
			"total": total,
			"page":  page,
			"limit": limit,
		},
	})
}

func (h *PurchaseHandler) GetByID(c *fiber.Ctx) error {
	id := c.Params("id")
	purchase, err := h.Service.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"status":  "error",
			"message": "Data pembelian tidak ditemukan",
		})
	}
	return c.JSON(fiber.Map{
		"status": "success",
		"data":   purchase,
	})
}

func (h *PurchaseHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.Service.DeletePurchase(id); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Data pembelian berhasil dihapus",
	})
}
