package http

import (
	"backend-cashier/domain"
	"backend-cashier/service"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type ProductHandler struct {
	Service *service.ProductService
}

func NewProductHandler(s *service.ProductService) *ProductHandler {
	return &ProductHandler{Service: s}
}

func (h *ProductHandler) RegisterRoutes(app fiber.Router) {
	group := app.Group("api/products")
	group.Get("/", h.FetchProducts)
	group.Get("/:id", h.GetByID)
	group.Post("/", h.Create)
	group.Put("/:id", h.Update)
	group.Delete("/:id", h.Delete)
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
	data, total, err := h.Service.GetAll(search, page, limit)
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
	p, err := h.Service.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Produk tidak ditemukan"})
	}
	return c.Status(200).JSON(p)
}

// Create godoc
// @Summary      Create New Product
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        product  body      domain.ProductRequest  true  "Product Body"
// @Success      201      {object}  map[string]string
// @Failure      400      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /products [post]
func (h *ProductHandler) Create(c *fiber.Ctx) error {
	var p domain.Product
	if err := c.BodyParser(&p); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if err := h.Service.Create(p); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{"message": "Produk berhasil dibuat"})
}

// Update godoc
// @Summary      Update Existing Product
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        id       path      int             true  "Product ID"
// @Param        product  body      domain.ProductRequest  true  "Update Data"
// @Success      200      {object}  map[string]string
// @Failure      400      {object}  map[string]string
// @Router       /products/{id} [put]
func (h *ProductHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var p domain.ProductRequestUpdate
	if err := c.BodyParser(&p); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if err := h.Service.Update(id, p); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(200).JSON(fiber.Map{"message": "Produk berhasil diperbarui"})
}

// Delete godoc
// @Summary      Delete Product
// @Tags         Products
// @Param        id   path      int  true  "Product ID"
// @Success      200  {object}  map[string]string
// @Router       /products/{id} [delete]
func (h *ProductHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.Service.Delete(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(200).JSON(fiber.Map{"message": "Produk berhasil dihapus"})
}
