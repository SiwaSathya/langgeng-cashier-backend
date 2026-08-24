package http

import (
	"backend-cashier/domain"
	"backend-cashier/service"

	"github.com/gofiber/fiber/v2"
)

// LoginRequest menampung input JSON dari frontend
type LoginRequest struct {
	Username string `json:"username" example:"kasir"`
	Password string `json:"password" example:"password123"`
}

type AuthHandler struct {
	Service *service.AuthService
}

func NewAuthHandler(s *service.AuthService) *AuthHandler {
	return &AuthHandler{Service: s}
}

func (h *AuthHandler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/api")

	// Rute Endpoint Auth & User Management
	api.Post("/auth/login", h.Login)
	api.Get("/users", h.GetAllUsers)
	api.Post("/users", h.CreateUser)
	api.Get("/users/:id", h.GetUserByID)
	api.Put("/users/:id", h.UpdateUser)
	api.Delete("/users/:id", h.DeleteUser)

	// Endpoint alias master employees untuk kompatibilitas frontend
	api.Get("/master/employees", h.GetAllUsers)
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Format data yang dikirim salah",
		})
	}

	if req.Username == "" || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Username dan password wajib diisi",
		})
	}

	token, err := h.Service.Login(req.Username, req.Password)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Login berhasil",
		"token":   token,
	})
}

func (h *AuthHandler) CreateUser(c *fiber.Ctx) error {
	var req domain.UserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Format request tidak valid",
		})
	}

	result, err := h.Service.CreateUser(req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "User berhasil ditambahkan",
		"data":    result,
	})
}

func (h *AuthHandler) GetAllUsers(c *fiber.Ctx) error {
	results, err := h.Service.GetAllUsers()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(results)
}

func (h *AuthHandler) GetUserByID(c *fiber.Ctx) error {
	id := c.Params("id")
	result, err := h.Service.GetUserByID(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(result)
}

func (h *AuthHandler) UpdateUser(c *fiber.Ctx) error {
	id := c.Params("id")
	var req domain.UserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Format request tidak valid",
		})
	}

	result, err := h.Service.UpdateUser(id, req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "User berhasil diperbarui",
		"data":    result,
	})
}

func (h *AuthHandler) DeleteUser(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.Service.DeleteUser(id); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(fiber.Map{
		"message": "User berhasil dihapus",
	})
}
