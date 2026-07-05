package http

import (
	"backend-cashier/domain"
	"backend-cashier/service"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type ExpenseHandler struct {
	Service *service.ExpenseService
}

func NewExpenseHandler(s *service.ExpenseService) *ExpenseHandler {
	return &ExpenseHandler{Service: s}
}

func (h *ExpenseHandler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/api")
	api.Post("/expenses", h.Create)
	api.Get("/expenses", h.GetAll)
	api.Get("/expenses/:id", h.GetByID)
	api.Put("/expenses/:id", h.Update)
	api.Delete("/expenses/:id", h.Delete)
}

func (h *ExpenseHandler) Create(c *fiber.Ctx) error {
	expense := new(domain.Expense)
	if err := c.BodyParser(expense); err != nil {
		return c.Status(400).JSON(fiber.Map{"status": "error", "message": "Bad Request"})
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

	expenses, total, err := h.Service.GetAll(page, limit, search)
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
	input := new(domain.Expense)
	if err := c.BodyParser(input); err != nil {
		return c.Status(400).JSON(fiber.Map{"status": "error", "message": "Bad Request"})
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
