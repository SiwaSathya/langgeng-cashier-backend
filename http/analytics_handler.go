package http

import (
	"backend-cashier/service"

	"github.com/gofiber/fiber/v2"
)

type AnalyticsHandler struct {
	Service *service.AnalyticsService
}

func NewAnalyticsHandler(s *service.AnalyticsService) *AnalyticsHandler {
	return &AnalyticsHandler{Service: s}
}

func (h *AnalyticsHandler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/api")
	api.Get("/analytics/report", h.GetReport)
}

func (h *AnalyticsHandler) GetReport(c *fiber.Ctx) error {
	period := c.Query("period", "this-month")
	result, err := h.Service.GetAnalyticsReport(period)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(result)
}
