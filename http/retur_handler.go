package http

import (
	"backend-cashier/service"
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
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
	api.Post("/retur/direct", h.PostDirectRetur) // Rute baru: Retur produk langsung tanpa nota
	api.Put("/retur/:id/batal", h.CancelRetur)
	api.Put("/retur/:id/distributor", h.ToDistributor)
	api.Put("/retur/:id/selesai", h.CompleteRetur)
}

// Helper untuk mengekstrak identitas user dari JWT Token atau custom header
func extractReturUser(c *fiber.Ctx) (*string, string, string) {
	// 1. Cek JWT Bearer Authorization header
	authHeader := c.Get("Authorization")
	if authHeader != "" {
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		tokenStr = strings.TrimSpace(tokenStr)
		if tokenStr != "" {
			token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
				return service.GetJWTSecret(), nil
			})
			if err == nil && token != nil {
				if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
					var userID *string
					if uid, exists := claims["user_id"]; exists && uid != nil {
						strUID := fmt.Sprintf("%v", uid)
						if strUID != "" && strUID != "<nil>" {
							userID = &strUID
						}
					}
					var userName string
					if n, exists := claims["name"]; exists && n != nil {
						userName = fmt.Sprintf("%v", n)
					}
					var userRole string
					if r, exists := claims["role"]; exists && r != nil {
						userRole = fmt.Sprintf("%v", r)
					}
					return userID, userName, userRole
				}
			}
		}
	}

	// 2. Cek Custom Request Headers
	if hID := c.Get("X-User-Id"); hID != "" {
		hName := c.Get("X-User-Name")
		hRole := c.Get("X-User-Role")
		return &hID, hName, hRole
	}

	return nil, "", ""
}

func (h *ReturHandler) PostRetur(c *fiber.Ctx) error {
	var req service.ReturRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Format data tidak valid: " + err.Error(),
		})
	}

	// Auto-enrich user identity jika belum disediakan di payload body
	tokenUID, tokenName, tokenRole := extractReturUser(c)
	if req.UserID == nil && tokenUID != nil {
		req.UserID = tokenUID
	}
	if req.UserName == "" && tokenName != "" {
		req.UserName = tokenName
	}
	if req.UserRole == "" && tokenRole != "" {
		req.UserRole = tokenRole
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

func (h *ReturHandler) PostDirectRetur(c *fiber.Ctx) error {
	var req service.DirectReturRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Format data tidak valid: " + err.Error(),
		})
	}

	// Auto-enrich user identity jika belum disediakan di payload body
	tokenUID, tokenName, tokenRole := extractReturUser(c)
	if req.UserID == nil && tokenUID != nil {
		req.UserID = tokenUID
	}
	if req.UserName == "" && tokenName != "" {
		req.UserName = tokenName
	}
	if req.UserRole == "" && tokenRole != "" {
		req.UserRole = tokenRole
	}

	if req.Status == "" {
		req.Status = "Retur Sukses"
	}

	result, err := h.returService.CreateDirectRetur(req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Retur langsung berhasil diproses",
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
