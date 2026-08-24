package http

import (
	"backend-cashier/domain"
	"backend-cashier/service"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type AccountingHandler struct {
	Service *service.AccountingService
}

func NewAccountingHandler(s *service.AccountingService) *AccountingHandler {
	return &AccountingHandler{Service: s}
}

func (h *AccountingHandler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/api/accounting")

	// Akun / Chart of Accounts
	api.Get("/accounts", h.GetAllAccounts)
	api.Post("/accounts", h.CreateAccount)
	api.Get("/accounts/:id", h.GetAccountByID)
	api.Put("/accounts/:id", h.UpdateAccount)
	api.Delete("/accounts/:id", h.DeleteAccount)

	// Buku Harian (Journal Entries)
	api.Get("/journals", h.GetAllJournalEntries)
	api.Post("/journals", h.CreateJournalEntry)
	api.Get("/journals/:id", h.GetJournalEntryByID)
	api.Put("/journals/:id", h.UpdateJournalEntry)
	api.Delete("/journals/:id", h.DeleteJournalEntry)

	// Rekapitulasi & Buku Besar
	api.Get("/recap", h.GetRekapitulasi)
	api.Get("/ledger", h.GetGeneralLedger)

	// Piutang Dagang
	api.Get("/piutang", h.GetAllPiutang)
	api.Post("/piutang", h.CreatePiutang)
	api.Get("/piutang/:id", h.GetPiutangByID)
	api.Put("/piutang/:id", h.UpdatePiutang)
	api.Delete("/piutang/:id", h.DeletePiutang)
}

func (h *AccountingHandler) GetAllPiutang(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	filter := domain.PiutangFilter{
		StartDate: c.Query("start_date"),
		EndDate:   c.Query("end_date"),
		Search:    c.Query("search"),
		Status:    c.Query("status"),
		Page:      page,
		Limit:     limit,
	}

	records, total, err := h.Service.GetAllPiutang(filter)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"data":  records,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (h *AccountingHandler) CreatePiutang(c *fiber.Ctx) error {
	var req domain.PiutangDagangRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Format request tidak valid",
		})
	}

	result, err := h.Service.CreatePiutang(req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "Data piutang dagang berhasil disimpan",
		"data":    result,
	})
}

func (h *AccountingHandler) GetPiutangByID(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "ID piutang tidak valid",
		})
	}

	result, err := h.Service.GetPiutangByID(uint(id))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(result)
}

func (h *AccountingHandler) UpdatePiutang(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "ID piutang tidak valid",
		})
	}

	var req domain.PiutangDagangRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Format request tidak valid",
		})
	}

	result, err := h.Service.UpdatePiutang(uint(id), req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Data piutang dagang berhasil diperbarui",
		"data":    result,
	})
}

func (h *AccountingHandler) DeletePiutang(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "ID piutang tidak valid",
		})
	}

	if err := h.Service.DeletePiutang(uint(id)); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Data piutang dagang berhasil dihapus",
	})
}

func (h *AccountingHandler) GetAllAccounts(c *fiber.Ctx) error {
	results, err := h.Service.GetAllAccounts()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(results)
}

func (h *AccountingHandler) CreateAccount(c *fiber.Ctx) error {
	var req domain.Account
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Format request tidak valid",
		})
	}

	result, err := h.Service.CreateAccount(req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "Akun berhasil dibuat",
		"data":    result,
	})
}

func (h *AccountingHandler) GetAccountByID(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "ID akun tidak valid",
		})
	}

	result, err := h.Service.GetAccountByID(uint(id))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(result)
}

func (h *AccountingHandler) UpdateAccount(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "ID akun tidak valid",
		})
	}

	var req domain.Account
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Format request tidak valid",
		})
	}

	result, err := h.Service.UpdateAccount(uint(id), req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Akun berhasil diperbarui",
		"data":    result,
	})
}

func (h *AccountingHandler) DeleteAccount(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "ID akun tidak valid",
		})
	}

	if err := h.Service.DeleteAccount(uint(id)); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Akun berhasil dihapus",
	})
}

func (h *AccountingHandler) GetAllJournalEntries(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	accountID, _ := strconv.Atoi(c.Query("account_id", "0"))

	filter := domain.JournalFilter{
		StartDate: c.Query("start_date"),
		EndDate:   c.Query("end_date"),
		Search:    c.Query("search"),
		AccountID: uint(accountID),
		Page:      page,
		Limit:     limit,
	}

	entries, total, err := h.Service.GetAllJournalEntries(filter)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"data":  entries,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (h *AccountingHandler) CreateJournalEntry(c *fiber.Ctx) error {
	var req domain.CreateJournalRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Format request tidak valid: " + err.Error(),
		})
	}

	result, err := h.Service.CreateJournalEntry(req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "Transaksi jurnal buku harian berhasil dicatat",
		"data":    result,
	})
}

func (h *AccountingHandler) GetJournalEntryByID(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "ID jurnal tidak valid",
		})
	}

	result, err := h.Service.GetJournalEntryByID(uint(id))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(result)
}

func (h *AccountingHandler) UpdateJournalEntry(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "ID jurnal tidak valid",
		})
	}

	var req domain.CreateJournalRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Format request tidak valid: " + err.Error(),
		})
	}

	result, err := h.Service.UpdateJournalEntry(uint(id), req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Transaksi jurnal berhasil diperbarui",
		"data":    result,
	})
}

func (h *AccountingHandler) DeleteJournalEntry(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "ID jurnal tidak valid",
		})
	}

	if err := h.Service.DeleteJournalEntry(uint(id)); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Jurnal transaksi berhasil dihapus",
	})
}

func (h *AccountingHandler) GetRekapitulasi(c *fiber.Ctx) error {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	result, err := h.Service.GetRekapitulasi(startDate, endDate)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(result)
}

func (h *AccountingHandler) GetGeneralLedger(c *fiber.Ctx) error {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	accountID, _ := strconv.Atoi(c.Query("account_id", "0"))

	cards, err := h.Service.GetGeneralLedger(startDate, endDate, uint(accountID))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(cards)
}
