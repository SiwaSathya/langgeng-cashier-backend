package http

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/valyala/fasthttp"

	"backend-cashier/domain"
	"backend-cashier/service"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

type OpnameHandler struct {
	Service *service.OpnameService
	Storage service.StorageService
}

func NewOpnameHandler(s *service.OpnameService, storage service.StorageService) *OpnameHandler {
	return &OpnameHandler{
		Service: s,
		Storage: storage,
	}
}

func (h *OpnameHandler) RegisterRoutes(app fiber.Router) {
	group := app.Group("api/stock-opnames")
	group.Post("/", h.CreateSession)
	group.Get("/", h.ListSessions)
	group.Get("/live", h.LiveGlobal)
	group.Get("/:id/live", h.LiveSession)
	group.Get("/:id", h.GetSession)
	group.Put("/:id/items/:product_id", h.UpdateCountItem)
	group.Post("/:id/upload-photo", h.UploadPhoto)
	group.Post("/:id/submit", h.SubmitForApproval)
	group.Post("/:id/approve", h.ApproveSession)
	group.Post("/:id/cancel", h.CancelSession)

	// Route tambahan untuk informasi multi-lokasi & kartu stok pada produk
	prodGroup := app.Group("api/products")
	prodGroup.Get("/:id/stocks", h.GetProductStockBreakdown)
	prodGroup.Get("/:id/stock-card", h.GetStockCard)
}

// resolveUserID mengekstrak User ID dari JWT token, header X-User-ID, atau default SYSTEM
func resolveUser(c *fiber.Ctx) (string, string) {
	userID := strings.TrimSpace(c.Get("X-User-ID"))
	userRole := strings.ToLower(strings.TrimSpace(c.Get("X-User-Role")))

	authHeader := c.Get("Authorization")
	if authHeader != "" {
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		tokenStr = strings.TrimSpace(tokenStr)
		if tokenStr != "" {
			token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
				return service.GetJWTSecret(), nil
			})
			if err == nil && token.Valid {
				if claims, ok := token.Claims.(jwt.MapClaims); ok {
					if id, ok := claims["user_id"].(string); ok && id != "" {
						userID = id
					} else if idFloat, ok := claims["user_id"].(float64); ok {
						userID = fmt.Sprintf("%.0f", idFloat)
					}
					if r, ok := claims["role"].(string); ok && r != "" {
						userRole = strings.ToLower(strings.TrimSpace(r))
					}
				}
			}
		}
	}

	if userID == "" {
		userID = "SYSTEM"
	}
	return userID, userRole
}

// POST /api/stock-opnames: Membuka sesi baru Stock Opname
func (h *OpnameHandler) CreateSession(c *fiber.Ctx) error {
	var req domain.CreateOpnameSessionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Format data request tidak valid: " + err.Error(),
		})
	}

	userID, _ := resolveUser(c)

	if req.CategoryID != nil && *req.CategoryID == 0 {
		req.CategoryID = nil
	}

	opname, err := h.Service.CreateSession(req.Location, req.CategoryID, userID, req.Notes)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Sesi Stock Opname berhasil dibuka",
		"data":    opname,
	})
}

// GET /api/stock-opnames: Menampilkan daftar sesi opname
func (h *OpnameHandler) ListSessions(c *fiber.Ctx) error {
	location := c.Query("location")
	status := c.Query("status")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	offset := (page - 1) * limit

	opnames, total, err := h.Service.ListSessions(location, status, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Gagal mengambil daftar opname: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"data":  opnames,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GET /api/stock-opnames/:id: Mengambil detail satu sesi opname
func (h *OpnameHandler) GetSession(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "ID sesi opname tidak valid",
		})
	}

	opname, err := h.Service.GetSession(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"data": opname,
	})
}

// PUT /api/stock-opnames/:id/items/:product_id: Input/update hasil hitung fisik barang
func (h *OpnameHandler) UpdateCountItem(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "ID sesi opname tidak valid",
		})
	}

	productID, err := strconv.Atoi(c.Params("product_id"))
	if err != nil || productID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "ID produk tidak valid",
		})
	}

	var req domain.UpdateOpnameItemRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Format request tidak valid: " + err.Error(),
		})
	}

	item, err := h.Service.UpdateCountItem(uint(id), uint(productID), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Hasil hitung berhasil disimpan",
		"data":    item,
	})
}

// POST /api/stock-opnames/:id/upload-photo: Upload foto bukti fisik ke MinIO / local storage
func (h *OpnameHandler) UploadPhoto(c *fiber.Ctx) error {
	file, err := c.FormFile("photo")
	if err != nil {
		file, err = c.FormFile("file")
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "File foto wajib diunggah (form field: 'photo' atau 'file')",
			})
		}
	}

	src, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Gagal membaca file foto: " + err.Error(),
		})
	}
	defer src.Close()

	contentType := file.Header.Get("Content-Type")
	photoURL, err := h.Storage.UploadFile(c.Context(), "opname", file.Filename, src, file.Size, contentType)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Gagal menyimpan foto: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":   "Foto berhasil diunggah",
		"photo_url": photoURL,
	})
}

// POST /api/stock-opnames/:id/submit: Mengajukan sesi opname untuk approval supervisor
func (h *OpnameHandler) SubmitForApproval(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "ID sesi opname tidak valid",
		})
	}

	var req domain.SubmitOpnameRequest
	_ = c.BodyParser(&req)

	opname, err := h.Service.SubmitForApproval(uint(id), req.Notes)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Dokumen opname berhasil diajukan untuk approval",
		"data":    opname,
	})
}

// POST /api/stock-opnames/:id/approve: Menyetujui opname dan mengeksekusi penyesuaian stok cabang
func (h *OpnameHandler) ApproveSession(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "ID sesi opname tidak valid",
		})
	}

	approverID, role := resolveUser(c)

	// Validasi role: Kasir biasa tidak boleh approve
	if role == "kasir" || role == "supersubkasir" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Akses ditolak: Persetujuan Stock Opname hanya dapat dilakukan oleh Supervisor atau Admin",
		})
	}

	opname, err := h.Service.ApproveSession(uint(id), approverID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Dokumen opname berhasil disetujui dan penyesuaian stok telah diterapkan",
		"data":    opname,
	})
}

// POST /api/stock-opnames/:id/cancel: Membatalkan sesi opname
func (h *OpnameHandler) CancelSession(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "ID sesi opname tidak valid",
		})
	}

	userID, _ := resolveUser(c)

	opname, err := h.Service.CancelSession(uint(id), userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Dokumen opname berhasil dibatalkan",
		"data":    opname,
	})
}

// GET /api/products/:id/stocks: Rincian stok produk per cabang
func (h *OpnameHandler) GetProductStockBreakdown(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "ID produk tidak valid",
		})
	}

	stocks, err := h.Service.GetProductStockBreakdown(uint(id))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Gagal mengambil rincian stok cabang: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"product_id": id,
		"data":       stocks,
	})
}

// GET /api/products/:id/stock-card: Riwayat mutasi kartu stok produk
func (h *OpnameHandler) GetStockCard(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "ID produk tidak valid",
		})
	}

	location := c.Query("location")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 50
	}
	offset := (page - 1) * limit

	movements, total, err := h.Service.GetStockCard(uint(id), location, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Gagal mengambil kartu stok: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"product_id": id,
		"location":   location,
		"data":       movements,
		"total":      total,
		"page":       page,
		"limit":      limit,
	})
}


// LiveSession menyiarkan event live real-time (SSE) untuk sesi stock opname tertentu
func (h *OpnameHandler) LiveSession(c *fiber.Ctx) error {
	sessionIDStr := c.Params("id")
	sessionID, err := strconv.ParseUint(sessionIDStr, 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID sesi tidak valid"})
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	if h.Service.SSE == nil {
		return c.Status(503).SendString("SSE service unavailable")
	}

	topic := fmt.Sprintf("opname:%d", sessionID)
	ch, unsubscribe := h.Service.SSE.Subscribe(topic)

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer unsubscribe()

		fmt.Fprintf(w, "event: connected\ndata: {\"topic\":\"%s\",\"status\":\"connected\"}\n\n", topic)
		_ = w.Flush()

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case evt, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Event, evt.Data)
				if err := w.Flush(); err != nil {
					return
				}
			case <-ticker.C:
				// Heartbeat ping keep-alive
				fmt.Fprintf(w, ": heartbeat\n\n")
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	}))

	return nil
}

// LiveGlobal menyiarkan event global (seperti pembuatan sesi baru atau perubahan status dokumen)
func (h *OpnameHandler) LiveGlobal(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	if h.Service.SSE == nil {
		return c.Status(503).SendString("SSE service unavailable")
	}

	topic := "opname:all"
	ch, unsubscribe := h.Service.SSE.Subscribe(topic)

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer unsubscribe()

		fmt.Fprintf(w, "event: connected\ndata: {\"topic\":\"%s\",\"status\":\"connected\"}\n\n", topic)
		_ = w.Flush()

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case evt, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Event, evt.Data)
				if err := w.Flush(); err != nil {
					return
				}
			case <-ticker.C:
				fmt.Fprintf(w, ": heartbeat\n\n")
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	}))

	return nil
}
