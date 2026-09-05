package http

import (
	"backend-cashier/domain"
	"backend-cashier/service"

	"github.com/gofiber/fiber/v2"
)

type SupplierHandler struct {
	Service *service.SupplierService
}

func NewSupplierHandler(
	s *service.SupplierService,
) *SupplierHandler {

	return &SupplierHandler{
		Service: s,
	}

}

func (h *SupplierHandler) RegisterRoutes(
	app *fiber.App,
) {

	api := app.Group("/api")

	api.Post(
		"/suppliers",
		h.Create,
	)

	api.Get(
		"/suppliers",
		h.GetAll,
	)

	api.Get(
		"/suppliers/:id",
		h.GetByID,
	)

	api.Put(
		"/suppliers/:id",
		h.Update,
	)

	api.Delete(
		"/suppliers/:id",
		h.Delete,
	)

}

// CREATE

func (h *SupplierHandler) Create(
	c *fiber.Ctx,
) error {

	supplier := new(domain.Supplier)

	if err := c.BodyParser(supplier); err != nil {

		return c.Status(400).JSON(
			fiber.Map{
				"status":  "error",
				"message": "Format data tidak valid",
			},
		)

	}

	if err := h.Service.CreateSupplier(
		supplier,
	); err != nil {

		return c.Status(500).JSON(
			fiber.Map{
				"status":  "error",
				"message": err.Error(),
			},
		)

	}

	return c.Status(201).JSON(
		fiber.Map{

			"status": "success",

			"message": "Supplier berhasil dibuat",

			"data": supplier,
		},
	)

}

// GET ALL

func (h *SupplierHandler) GetAll(
	c *fiber.Ctx,
) error {

	data, err :=
		h.Service.GetAllSupplier()

	if err != nil {

		return c.Status(500).JSON(
			fiber.Map{
				"status":  "error",
				"message": err.Error(),
			},
		)

	}

	return c.JSON(
		fiber.Map{

			"status": "success",

			"data": data,
		},
	)

}

// GET BY ID

func (h *SupplierHandler) GetByID(
	c *fiber.Ctx,
) error {

	id := c.Params("id")

	supplier, err :=
		h.Service.GetSupplierByID(id)

	if err != nil {

		return c.Status(404).JSON(
			fiber.Map{

				"status": "error",

				"message": "Supplier tidak ditemukan",
			},
		)

	}

	return c.JSON(
		fiber.Map{

			"status": "success",

			"data": supplier,
		},
	)

}

// UPDATE

func (h *SupplierHandler) Update(
	c *fiber.Ctx,
) error {

	id := c.Params("id")

	supplier := new(domain.Supplier)

	if err := c.BodyParser(supplier); err != nil {

		return c.Status(400).JSON(
			fiber.Map{
				"message": "Format salah",
			},
		)

	}

	supplier.ID = id

	if err :=
		h.Service.UpdateSupplier(supplier); err != nil {

		return c.Status(500).JSON(
			fiber.Map{
				"message": err.Error(),
			},
		)

	}

	return c.JSON(
		fiber.Map{

			"status": "success",

			"message": "Supplier berhasil diperbarui",
		},
	)

}

// DELETE

func (h *SupplierHandler) Delete(
	c *fiber.Ctx,
) error {

	id := c.Params("id")

	if err :=
		h.Service.DeleteSupplier(id); err != nil {

		return c.Status(500).JSON(
			fiber.Map{

				"status": "error",

				"message": err.Error(),
			},
		)

	}

	return c.JSON(
		fiber.Map{

			"status": "success",

			"message": "Supplier berhasil dihapus",
		},
	)

}
