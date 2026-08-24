package http

import (
	"backend-cashier/domain"
	"backend-cashier/service"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type AttendanceHandler struct {
	Service *service.AttendanceService
}

func NewAttendanceHandler(s *service.AttendanceService) *AttendanceHandler {
	return &AttendanceHandler{Service: s}
}

func (h *AttendanceHandler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/api")

	api.Post("/attendance", h.CreateAttendance)
	api.Get("/attendance", h.GetAllAttendance)
	api.Get("/attendance/summary", h.GetSummary)
	api.Get("/attendance/:id", h.GetByID)
	api.Put("/attendance/:id", h.UpdateAttendance)
	api.Delete("/attendance/:id", h.DeleteAttendance)
}

func (h *AttendanceHandler) CreateAttendance(c *fiber.Ctx) error {
	var req domain.AttendanceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Format request tidak valid: " + err.Error(),
		})
	}

	result, err := h.Service.CreateAttendance(req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "Data absensi berhasil disimpan",
		"data":    result,
	})
}

func (h *AttendanceHandler) GetAllAttendance(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	shift, _ := strconv.Atoi(c.Query("shift", "0"))

	filter := domain.AttendanceFilter{
		Date:      c.Query("date"),
		StartDate: c.Query("start_date"),
		EndDate:   c.Query("end_date"),
		UserID:    c.Query("user_id"),
		Shift:     uint(shift),
		Status:    c.Query("status"),
		Search:    c.Query("search"),
		Page:      page,
		Limit:     limit,
	}

	records, total, err := h.Service.GetAllAttendance(filter)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"data":  records,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (h *AttendanceHandler) GetByID(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "ID absensi tidak valid",
		})
	}

	result, err := h.Service.GetAttendanceByID(uint(id))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(result)
}

func (h *AttendanceHandler) UpdateAttendance(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "ID absensi tidak valid",
		})
	}

	var req domain.AttendanceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Format request tidak valid",
		})
	}

	result, err := h.Service.UpdateAttendance(uint(id), req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Data absensi berhasil diperbarui",
		"data":    result,
	})
}

func (h *AttendanceHandler) DeleteAttendance(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "ID absensi tidak valid",
		})
	}

	if err := h.Service.DeleteAttendance(uint(id)); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Data absensi berhasil dihapus",
	})
}

func (h *AttendanceHandler) GetSummary(c *fiber.Ctx) error {
	dateStr := c.Query("date")
	summary, err := h.Service.GetSummary(dateStr)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(summary)
}
