package http

import (
	"backend-cashier/service"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type ReturHandler struct {
	returService service.ReturService
}

func NewReturHandler(returService service.ReturService) *ReturHandler {
	return &ReturHandler{returService: returService}
}

func (h *ReturHandler) RegisterRoutes(router fiber.Router) {
	api := router.Group("/api")
	api.Post("/retur", h.PostRetur)
	api.Get("/retur", h.GetReturList)
	api.Put("/retur/:id/batal", h.CancelRetur)
	api.Put("/retur/:id/distributor", h.ToDistributor)
	api.Put("/retur/:id/selesai", h.CompleteRetur)
}

func (h *ReturHandler) PostRetur(c *fiber.Ctx) error {
	var req service.ReturRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Format data tidak valid: " + err.Error(),
		})
	}

	if req.Status == "" {
		req.Status = "Retur Sukses"
	}

	result, err := h.returService.CreateRetur(req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Retur berhasil diproses",
		"data":    result,
	})
}

func (h *ReturHandler) GetReturList(c *fiber.Ctx) error {
	results, err := h.returService.GetAllRetur()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(results)
}

func (h *ReturHandler) CancelRetur(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	result, err := h.returService.BatalkanRetur(uint(id))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Retur berhasil dibatalkan", "data": result})
}

func (h *ReturHandler) ToDistributor(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	result, err := h.returService.KirimKeDistributor(uint(id))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Status berubah: Dikirim ke distributor", "data": result})
}

func (h *ReturHandler) CompleteRetur(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	result, err := h.returService.SelesaikanRetur(uint(id))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Retur distributor selesai, stok bertambah", "data": result})
}
