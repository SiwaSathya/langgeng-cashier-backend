package http

import (
	"backend-cashier/domain"
	"backend-cashier/service"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

// LoginRequest menampung input JSON dari frontend
type LoginRequest struct {
	Username string `json:"username" example:"kasir"`
	Password string `json:"password" example:"password123"`
}

type AuthHandler struct {
	Service         *service.AuthService
	LogService      *service.UserLogService
	PresenceService *service.PresenceService
}

func NewAuthHandler(s *service.AuthService, logSvc *service.UserLogService, presenceSvc *service.PresenceService) *AuthHandler {
	return &AuthHandler{
		Service:         s,
		LogService:      logSvc,
		PresenceService: presenceSvc,
	}
}

func (h *AuthHandler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/api")

	// Rute Endpoint Auth & User Management
	api.Post("/auth/login", h.Login)
	api.Post("/auth/logout", h.Logout)
	api.Post("/auth/refresh", h.RefreshToken)
	api.Get("/auth/verify", h.VerifyToken)
	api.Get("/users", RequireRoles(h.Service), h.GetAllUsers)
	api.Post("/users", RequireRoles(h.Service, "superadmin", "admin"), h.CreateUser)
	api.Get("/users/:id", RequireRoles(h.Service), h.GetUserByID)
	api.Put("/users/:id", RequireRoles(h.Service, "superadmin", "admin"), h.UpdateUser)
	api.Delete("/users/:id", RequireRoles(h.Service, "superadmin", "admin"), h.DeleteUser)

	// Endpoint User Activity Logs (Dilindungi Otorisasi Super Admin & Admin)
	api.Get("/user-logs", RequireRoles(h.Service, "superadmin", "admin"), h.GetUserLogs)
	api.Get("/user-logs/summary", RequireRoles(h.Service, "superadmin", "admin"), h.GetUserLogSummary)

	// Endpoint alias master employees untuk dropdown kasir/staf
	api.Get("/master/employees", RequireRoles(h.Service), h.GetAllUsers)
}

func (h *AuthHandler) parseClaims(c *fiber.Ctx) (jwt.MapClaims, error) {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		authHeader = c.Query("token")
	}
	if authHeader == "" {
		return nil, errors.New("token tidak ditemukan")
	}

	tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if h.Service != nil && h.Service.IsTokenRevoked(tokenStr) {
		return nil, errors.New("token telah dicabut (revoked), silakan login kembali")
	}
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("alg signing invalid")
		}
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

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Format data yang dikirim salah",
		})
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)

	if req.Username == "" || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Username dan password wajib diisi",
		})
	}

	clientIP := c.IP()
	if fwd := c.Get("X-Forwarded-For"); fwd != "" {
		clientIP = strings.TrimSpace(strings.Split(fwd, ",")[0])
	}

	// 1. Pemeriksaan Keamanan Brute Force (Pelindung Percobaan Berulang)
	guard := GetBruteForceGuard()
	guardKey := fmt.Sprintf("%s:%s", clientIP, req.Username)
	ipKey := fmt.Sprintf("ip:%s", clientIP)

	if blocked, waitSec := guard.IsBlocked(guardKey); blocked {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"error":               fmt.Sprintf("Terlalu banyak percobaan login gagal (5 kali). Demi keamanan, akun ini diblokir sementara selama %d detik lagi.", waitSec),
			"code":                "ACCOUNT_LOCKED",
			"retry_after_seconds": waitSec,
		})
	}
	if blocked, waitSec := guard.IsBlocked(ipKey); blocked {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"error":               fmt.Sprintf("Terlalu banyak percobaan login gagal dari IP Anda. Demi keamanan, silakan coba lagi dalam %d detik.", waitSec),
			"code":                "IP_LOCKED",
			"retry_after_seconds": waitSec,
		})
	}

	// 2. Verifikasi Autentikasi User (Token berdurasi 9 Jam)
	token, err := h.Service.Login(req.Username, req.Password)
	if err != nil {
		guard.RecordFailure(guardKey)
		guard.RecordFailure(ipKey)
		return c.Status(401).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Reset penghitung kegagalan brute force saat login berhasil
	guard.RecordSuccess(guardKey)
	guard.RecordSuccess(ipKey)

	// 3. Buat Session ID unik dan catat login ke database log aktivitas
	var sessionID string
	user, errUser := h.Service.GetUserByUsername(req.Username)
	if errUser == nil && user != nil && h.LogService != nil {
		sessionID = fmt.Sprintf("sess_%s_%d", user.ID, time.Now().UnixNano()%10000000)

		_, _ = h.LogService.LogLogin(domain.UserActivityLog{
			SessionID: sessionID,
			UserID:    user.ID,
			Username:  user.Username,
			Name:      user.Name,
			Role:      user.Role,
			Location:  user.Location,
			IPAddress: clientIP,
			UserAgent: c.Get("User-Agent"),
			LoginAt:   time.Now(),
		})
	}

	return c.JSON(fiber.Map{
		"message":    "Login berhasil",
		"token":      token,
		"session_id": sessionID,
		"user":       user,
		"expires_in": 32400, // 9 jam dalam detik
	})
}

// RefreshToken memperpanjang masa aktif token menjadi 9 jam baru saat telah mencapai usia 5 jam atau force
func (h *AuthHandler) RefreshToken(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		authHeader = c.Query("token")
	}
	if authHeader == "" {
		return c.Status(401).JSON(fiber.Map{"error": "Token otentikasi tidak ditemukan"})
	}

	force := c.Query("force") == "true" || c.Query("force") == "1"
	newToken, user, expiresIn, err := h.Service.RefreshToken(authHeader, force)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"status":     "success",
		"message":    "Token berhasil diperpanjang 9 jam",
		"token":      newToken,
		"expires_in": expiresIn,
		"user":       user,
	})
}

// VerifyToken memverifikasi kevalidan token dan memeriksa apakah telah memenuhi syarat perpanjangan (5 jam berjalan)
func (h *AuthHandler) VerifyToken(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		authHeader = c.Query("token")
	}
	if authHeader == "" {
		return c.Status(401).JSON(fiber.Map{"error": "Token otentikasi tidak ditemukan"})
	}

	claims, user, canRefresh, err := h.Service.VerifyToken(authHeader)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{
			"status":  "invalid",
			"error":   err.Error(),
			"expired": true,
		})
	}

	return c.JSON(fiber.Map{
		"status":      "valid",
		"can_refresh": canRefresh,
		"user":        user,
		"claims":      claims,
	})
}

// Logout mencatat kejadian logout secara presisi: apakah logout saja atau logout + menyelesaikan shift
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		authHeader = c.Query("token")
	}
	tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))

	claims, err := h.parseClaims(c)
	var userID string
	var location string
	if err == nil && claims != nil {
		userID = fmt.Sprintf("%v", claims["user_id"])
		location = fmt.Sprintf("%v", claims["location"])
	}

	// Blacklist token seketika (Anti-Replay Attack & Zombie Token Defense)
	if h.Service != nil && tokenStr != "" {
		_ = h.Service.RevokeToken(tokenStr, userID)
	}

	var req domain.LogoutRequest
	_ = c.BodyParser(&req)

	if req.Location == "" && location != "" {
		req.Location = location
	}

	// Fallback Session ID jika belum dikirim frontend
	if req.SessionID == "" && userID != "" {
		req.SessionID = fmt.Sprintf("sess_%s", userID)
	}

	var logEntry *domain.UserActivityLog
	if h.LogService != nil {
		logEntry, err = h.LogService.LogLogout(req, userID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Gagal mencatat log logout: " + err.Error(),
			})
		}
	}

	// Hapus sesi dari in-memory presence tracker seketika
	if h.PresenceService != nil && req.SessionID != "" {
		h.PresenceService.RemoveSession(req.SessionID)
	}

	return c.JSON(fiber.Map{
		"status":      "ok",
		"message":     "Logout berhasil dicatat",
		"logout_type": req.LogoutType,
		"shift":       req.Shift,
		"log":         logEntry,
	})
}

// syncPresenceWithLogs membersihkan sesi ACTIVE di database yang sudah tidak ada di presence tracker
func (h *AuthHandler) syncPresenceWithLogs() {
	if h.PresenceService != nil && h.LogService != nil {
		onlineUsers := h.PresenceService.GetOnlineUsers()
		onlineMap := make(map[string]bool, len(onlineUsers))
		for _, u := range onlineUsers {
			if u.SessionID != "" {
				onlineMap[u.SessionID] = true
			}
		}
		h.LogService.CleanupStaleActiveLogs(onlineMap)
	}
}

// GetUserLogs mengembalikan riwayat log aktivitas login dan logout untuk Super Admin & Admin
func (h *AuthHandler) GetUserLogs(c *fiber.Ctx) error {
	claims, err := h.parseClaims(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{
			"error": "Silakan login terlebih dahulu",
		})
	}

	role := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", claims["role"])))
	normalizedRole := strings.ReplaceAll(strings.ReplaceAll(role, "-", ""), "_", "")
	if normalizedRole != "superadmin" && normalizedRole != "admin" {
		return c.Status(403).JSON(fiber.Map{
			"error": "Akses Ditolak: Hanya Super Admin dan Admin yang dapat melihat log aktivitas",
		})
	}

	// Sinkronisasi status: tutup sesi database yang sudah tidak aktif / offline
	h.syncPresenceWithLogs()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	filter := domain.UserLogFilter{
		Page:       page,
		Limit:      limit,
		UserID:     c.Query("user_id"),
		Username:   c.Query("username"),
		Role:       c.Query("role"),
		Location:   c.Query("location"),
		LogoutType: c.Query("logout_type"),
		StartDate:  c.Query("start_date"),
		EndDate:    c.Query("end_date"),
		Search:     c.Query("search"),
	}

	logs, total, err := h.LogService.GetLogs(filter)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Gagal mengambil log aktivitas: " + err.Error(),
		})
	}

	summary, _ := h.LogService.GetSummary(filter.StartDate, filter.EndDate)

	return c.JSON(fiber.Map{
		"status":  "success",
		"total":   total,
		"page":    page,
		"limit":   limit,
		"data":    logs,
		"summary": summary,
	})
}

// GetUserLogSummary mengembalikan ringkasan statistik log aktivitas
func (h *AuthHandler) GetUserLogSummary(c *fiber.Ctx) error {
	if _, err := h.parseClaims(c); err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	// Sinkronisasi status: tutup sesi database yang sudah tidak aktif / offline
	h.syncPresenceWithLogs()

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	summary, err := h.LogService.GetSummary(startDate, endDate)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"summary": summary,
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
