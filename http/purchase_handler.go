package http

import (
	"backend-cashier/domain"
	"backend-cashier/service"
	"math"
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
	api.Delete("/purchases/nota/:nota", h.DeleteByNota)
}

// Create menangani input pembelian batch per-nota (banyak item produk sekaligus)
// dan tetap kompatibel dengan request single item lama sebagai fallback.
func (h *PurchaseHandler) Create(c *fiber.Ctx) error {
	var req domain.CreatePurchaseBatchRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "Format data tidak valid: " + err.Error(),
		})
	}

	// Fallback kompatibilitas: jika payload dikirim format single item tanpa array `items`
	if len(req.Items) == 0 && req.ProductID > 0 {
		req.Items = append(req.Items, domain.PurchaseItemRequest{
			ProductID: req.ProductID,
			Qty:       req.Qty,
			HBeli:     req.HBeli,
		})
	}

	if err := h.Service.CreateBatch(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"status":  "success",
		"message": "Pembelian berhasil dicatat dan stok produk telah bertambah",
		"data":    req,
	})
}

// GetAll mengambil data purchase yang dikelompokkan (Group by Nota / Group by Supplier)
func (h *PurchaseHandler) GetAll(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	groupBy := c.Query("group_by", "nota")
	search := c.Query("search", "")
	location := c.Query("location", "")

	results, totalGroups, err := h.Service.GetAll(page, limit, groupBy, search, location)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}

	totalPages := int(math.Ceil(float64(totalGroups) / float64(limit)))

	return c.JSON(fiber.Map{
		"status": "success",
		"data":   results,
		"meta": fiber.Map{
			"page":        page,
			"limit":       limit,
			"total":       totalGroups,
			"total_pages": totalPages,
		},
	})
}

// DeleteByNota membatalkan pembelian 1 nota utuh sekaligus me-rollback stok seluruh barang di dalamnya
func (h *PurchaseHandler) DeleteByNota(c *fiber.Ctx) error {
	nota := c.Params("nota")
	if nota == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "Nomor nota tidak boleh kosong",
		})
	}

	if err := h.Service.DeleteByNota(nota); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"status":  "error",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Transaksi nota pembelian berhasil dibatalkan dan stok telah dikembalikan",
	})
}
