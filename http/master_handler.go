package http

import (
	"backend-cashier/service"

	"github.com/gofiber/fiber/v2"
)

type MasterHandler struct {
	Service *service.MasterService
}

func NewMasterHandler(s *service.MasterService) *MasterHandler {
	return &MasterHandler{Service: s}
}

func (h *MasterHandler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/api")
	api.Get("/categories", h.GetCategories)
	api.Get("/products/cashier", h.GetProducts)
	api.Get("/master/resources", h.GetProductResources)
}

// GetCategories godoc
// @Summary      Get Daftar Kategori
// @Description  Mengambil semua data kategori barang yang tersedia
// @Tags         Master Data
// @Accept       json
// @Produce      json
// @Success      200  {array}   domain.Category
// @Failure      500  {object}  map[string]string
// @Router       /api/categories [get]
func (h *MasterHandler) GetCategories(c *fiber.Ctx) error {
	res, err := h.Service.GetAllCategories()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

// GetProducts godoc
// @Summary      Get Daftar Produk
// @Description  Mengambil data produk, bisa difilter berdasarkan nama atau kode barang
// @Tags         Master Data
// @Accept       json
// @Produce      json
// @Param        search  query     string  false  "Cari berdasarkan Nama atau Kode Barang"
// @Success      200  {array}   domain.ProductResponse
// @Failure      500  {object}  map[string]string
// @Router       /api/products [get]
func (h *MasterHandler) GetProducts(c *fiber.Ctx) error {
	res, err := h.Service.GetAllProducts(c.Query("search"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

// GetProductResources godoc
// @Summary      Get Dropdown Resources
// @Description  Mengambil daftar Category, Brand, dan Supplier untuk pilihan input produk.
// @Tags         Master Data
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  map[string]string
// @Router       /master/resources [get]
func (h *MasterHandler) GetProductResources(c *fiber.Ctx) error {
	data, err := h.Service.GetProductResources()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(200).JSON(data)
}
