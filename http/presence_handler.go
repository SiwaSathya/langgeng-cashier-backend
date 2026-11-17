package http

import (
	"backend-cashier/service"
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

type PresenceHandler struct {
	PresenceService *service.PresenceService
	SSEService      *service.SSEService
}

func NewPresenceHandler(presenceSvc *service.PresenceService, sseSvc *service.SSEService) *PresenceHandler {
	return &PresenceHandler{
		PresenceService: presenceSvc,
		SSEService:      sseSvc,
	}
}

func (h *PresenceHandler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/api/presence")

	api.Post("/ping", h.Ping)
	api.Post("/leave", h.Leave)
	api.Get("/online", h.GetOnlineUsers)
	api.Get("/summary", h.GetSummary)
	api.Get("/live", h.LivePresence)
}

func (h *PresenceHandler) parseClaims(c *fiber.Ctx) (jwt.MapClaims, error) {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		authHeader = c.Query("token")
	}
	if authHeader == "" {
		return nil, errors.New("token tidak ditemukan")
	}

	tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
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

func (h *PresenceHandler) Ping(c *fiber.Ctx) error {
	var req struct {
		SessionID string `json:"session_id"`
		Page      string `json:"page"`
	}
	_ = c.BodyParser(&req)

	claims, err := h.parseClaims(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{
			"error": "Otentikasi diperlukan: " + err.Error(),
		})
	}

	userID := fmt.Sprintf("%v", claims["user_id"])
	username := fmt.Sprintf("%v", claims["username"])
	name := fmt.Sprintf("%v", claims["name"])
	role := fmt.Sprintf("%v", claims["role"])
	location := fmt.Sprintf("%v", claims["location"])

	clientIP := c.IP()
	if fwd := c.Get("X-Forwarded-For"); fwd != "" {
		clientIP = strings.TrimSpace(strings.Split(fwd, ",")[0])
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("sess_%s", userID)
	}

	session := h.PresenceService.PingSession(service.OnlineSession{
		SessionID:   sessionID,
		UserID:      userID,
		Username:    username,
		Name:        name,
		Role:        role,
		Location:    location,
		CurrentPage: req.Page,
		IPAddress:   clientIP,
		UserAgent:   c.Get("User-Agent"),
	})

	return c.JSON(fiber.Map{
		"status":     "ok",
		"session_id": session.SessionID,
		"last_seen":  session.LastSeen,
	})
}

func (h *PresenceHandler) Leave(c *fiber.Ctx) error {
	var req struct {
		SessionID string `json:"session_id"`
	}
	_ = c.BodyParser(&req)

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = c.Query("session_id")
	}

	if sessionID != "" {
		h.PresenceService.RemoveSession(sessionID)
	}

	return c.JSON(fiber.Map{
		"status":  "ok",
		"message": "Sesi berhasil dihapus",
	})
}

func (h *PresenceHandler) GetOnlineUsers(c *fiber.Ctx) error {
	users := h.PresenceService.GetOnlineUsers()
	summary := h.PresenceService.GetSummary()

	return c.JSON(fiber.Map{
		"status":          "success",
		"total_online":    len(users),
		"users":           users,
		"summary":         summary,
		"location_counts": summary.LocationCount,
	})
}

func (h *PresenceHandler) GetSummary(c *fiber.Ctx) error {
	summary := h.PresenceService.GetSummary()
	return c.JSON(fiber.Map{
		"status":  "success",
		"summary": summary,
	})
}
func (h *PresenceHandler) LivePresence(c *fiber.Ctx) error {
	if _, err := h.parseClaims(c); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Otentikasi diperlukan: " + err.Error(),
		})
	}
	return StreamSSE(c, h.SSEService, "presence:live")
}
