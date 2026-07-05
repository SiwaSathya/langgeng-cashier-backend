package http

import (
	"backend-cashier/service"

	"github.com/gofiber/fiber/v2"
)

// LoginRequest menampung input JSON dari frontend
type LoginRequest struct {
	Username string `json:"username" example:"kasir_utama"`
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

	// Rute Endpoint Login
	api.Post("/auth/login", h.Login)
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest

	// Parsing JSON body
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Format data yang dikirim salah",
		})
	}

	// Validasi input kosong
	if req.Username == "" || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Username dan password wajib diisi",
		})
	}

	// Panggil service login
	token, err := h.Service.Login(req.Username, req.Password)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Kembalikan token jika sukses
	return c.JSON(fiber.Map{
		"message": "Login berhasil",
		"token":   token,
	})
}
