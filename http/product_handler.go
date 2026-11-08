package http

import (
	"fmt"
	"strconv"
	"strings"

	"backend-cashier/domain"
	"backend-cashier/service"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

type ProductHandler struct {
	Service *service.ProductService
}

func NewProductHandler(s *service.ProductService) *ProductHandler {
	return &ProductHandler{Service: s}
}

func (h *ProductHandler) RegisterRoutes(app fiber.Router) {
	group := app.Group("api/products")
	group.Get("/historical-stock", h.GetHistoricalStock)
	group.Get("/export-stock", h.ExportHistoricalStock)
	group.Get("/", h.FetchProducts)
	group.Get("/:id", h.GetByID)
	group.Post("/", h.Create)
	group.Put("/:id", h.Update)
	group.Delete("/:id", h.Delete)
}

// isRestrictedStockRole memeriksa apakah role pengguna adalah kasir atau super-kasir
func isRestrictedStockRole(c *fiber.Ctx) bool {
	// 1. Cek query param
	roleQuery := strings.ToLower(strings.TrimSpace(c.Query("role")))
	if roleQuery == "kasir" || strings.Contains(roleQuery, "kasir") {
		return true
	}

	// 2. Cek Header X-User-Role
	roleHeader := strings.ToLower(strings.TrimSpace(c.Get("X-User-Role")))
	if roleHeader == "kasir" || strings.Contains(roleHeader, "kasir") {
		return true
	}

	// 3. Cek JWT Bearer token
	authHeader := c.Get("Authorization")
	if authHeader != "" {
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		tokenStr = strings.TrimSpace(tokenStr)
		if tokenStr != "" {
			token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
				return []byte("smartpos_secret_key"), nil
			})
			if err == nil && token != nil {
				if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
					if r, exists := claims["role"]; exists {
						userRole := strings.ToLower(fmt.Sprintf("%v", r))
						if userRole == "kasir" || strings.Contains(userRole, "kasir") {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// GetHistoricalStock mengambil laporan stok per tanggal tertentu beserta nilai nominal persediaan
func (h *ProductHandler) GetHistoricalStock(c *fiber.Ctx) error {
	if isRestrictedStockRole(c) {
		return c.Status(403).JSON(fiber.Map{
			"error": "Akses ditolak: Fitur nilai persediaan stok modal tidak dapat diakses oleh role kasir dan super-kasir",
		})
	}

	cutoffDate := c.Query("cutoff_date")
	location := c.Query("location")
	search := c.Query("search")

	report, err := h.Service.GetHistoricalStockReport(cutoffDate, location, search)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(report)
}

// ExportHistoricalStock mengunduh file Excel laporan stok per tanggal tertentu
func (h *ProductHandler) ExportHistoricalStock(c *fiber.Ctx) error {
	if isRestrictedStockRole(c) {
		return c.Status(403).JSON(fiber.Map{
			"error": "Akses ditolak: Fitur ekspor nilai persediaan stok modal tidak dapat diakses oleh role kasir dan super-kasir",
		})
	}

	cutoffDate := c.Query("cutoff_date")
	location := c.Query("location")
	search := c.Query("search")

	excelBytes, err := h.Service.ExportHistoricalStockExcel(cutoffDate, location, search)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	fileName := fmt.Sprintf("Laporan_Stok_%s.xlsx", cutoffDate)
	if cutoffDate == "" {
		fileName = fmt.Sprintf("Laporan_Stok_%s.xlsx", "terkini")
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	return c.Send(excelBytes)
}

// FetchProducts godoc
// @Summary      Get All Products
// @Description  Mengambil semua data produk dengan pagination dan pencarian nama/kode.
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        search  query     string  false  "Search by Name or Code"
// @Param        page    query     int     false  "Page number (default 1)"
// @Param        limit   query     int     false  "Items per page (default 10)"
// @Success      200     {object}  map[string]interface{}
// @Failure      500     {object}  map[string]string
// @Router       /produ2cts [get]
func (h *ProductHandler) FetchProducts(c *fiber.Ctx) error {
	search := c.Query("search")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	data, total, err := h.Service.GetAll(strings.ToUpper(search), page, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"message": err.Error()})
	}
	return c.Status(200).JSON(fiber.Map{"data": data, "total": total, "page": page, "limit": limit})
}

// GetByID godoc
// @Summary      Get Product by ID
// @Tags         Products
// @Produce      json
// @Param        id   path      int  true  "Product ID"
// @Success      200  {object}  domain.ProductRequest
// @Failure      404  {object}  map[string]string
// @Router       /products/{id} [get]
func (h *ProductHandler) GetByID(c *fiber.Ctx) error {
	id := c.Params("id")
	data, err := h.Service.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"message": "product not found"})
	}
	return c.Status(200).JSON(fiber.Map{"data": data})
}

// Create godoc
// @Summary      Create Product
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        product  body      domain.ProductRequest  true  "Product data"
// @Success      200      {object}  map[string]string
// @Failure      400      {object}  map[string]string
// @Router       /products [post]
func (h *ProductHandler) Create(c *fiber.Ctx) error {
	var input domain.Product
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"message": err.Error()})
	}

	if err := h.Service.Create(input); err != nil {
		return c.Status(400).JSON(fiber.Map{"message": err.Error()})
	}

	return c.Status(200).JSON(fiber.Map{"message": "product created successfully"})
}

// Update godoc
// @Summary      Update Product
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        id       path      int                    true  "Product ID"
// @Param        product  body      domain.ProductRequestUpdate  true  "Product data"
// @Success      200      {object}  map[string]string
// @Failure      400      {object}  map[string]string
// @Router       /products/{id} [put]
func (h *ProductHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var input domain.ProductRequestUpdate
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"message": err.Error()})
	}

	if err := h.Service.Update(id, input); err != nil {
		return c.Status(400).JSON(fiber.Map{"message": err.Error()})
	}

	return c.Status(200).JSON(fiber.Map{"message": "product updated successfully"})
}

// Delete godoc
// @Summary      Delete Product
// @Tags         Products
// @Produce      json
// @Param        id   path      int  true  "Product ID"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Router       /products/{id} [delete]
func (h *ProductHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.Service.Delete(id); err != nil {
		return c.Status(400).JSON(fiber.Map{"message": err.Error()})
	}

	return c.Status(200).JSON(fiber.Map{"message": "product deleted successfully"})
}
