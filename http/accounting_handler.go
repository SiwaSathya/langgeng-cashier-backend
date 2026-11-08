package http

import (
	"fmt"
	"strconv"
	"time"

	"backend-cashier/domain"
	"backend-cashier/service"

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

	// Bagan Akun (COA)
	api.Get("/accounts", h.GetAllAccounts)
	api.Get("/accounts/:id", h.GetAccountByID)
	api.Post("/accounts", h.CreateAccount)
	api.Put("/accounts/:id", h.UpdateAccount)
	api.Delete("/accounts/:id", h.DeleteAccount)

	// Buku Harian (Journal Entries)
	api.Get("/export-excel", h.ExportJournalsExcel)
	api.Get("/journals", h.GetAllJournals)
	api.Get("/journals/:id", h.GetJournalByID)
	api.Post("/journals", h.CreateJournal)
	api.Put("/journals/:id", h.UpdateJournal)
	api.Delete("/journals/:id", h.DeleteJournal)
	api.Put("/journals/:id/toggle-check", h.ToggleCheckJournal)
	api.Patch("/journals/:id/check", h.ToggleCheckJournal)
	api.Post("/journals/sync-expenses", h.SyncExpenses)
	api.Post("/sync-all", h.SyncAll)

	// Rekapitulasi & Buku Besar
	api.Get("/recap", h.GetRekapitulasi)
	api.Get("/ledger", h.GetGeneralLedger)

	// Piutang Dagang (Accounts Receivable)
	api.Get("/piutang", h.GetAllPiutang)
	api.Post("/piutang", h.CreatePiutang)
	api.Put("/piutang/:id", h.UpdatePiutang)
	api.Delete("/piutang/:id", h.DeletePiutang)
}

// -------------------------------------------------------------
// BAGAN AKUN
// -------------------------------------------------------------

func (h *AccountingHandler) GetAllAccounts(c *fiber.Ctx) error {
	accounts, err := h.Service.GetAllAccounts()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(accounts)
}

func (h *AccountingHandler) GetAccountByID(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID akun tidak valid"})
	}
	acc, err := h.Service.GetAccountByID(uint(id))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(acc)
}

func (h *AccountingHandler) CreateAccount(c *fiber.Ctx) error {
	var req domain.AccountRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload data tidak valid"})
	}
	acc, err := h.Service.CreateAccount(req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(acc)
}

func (h *AccountingHandler) UpdateAccount(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID akun tidak valid"})
	}
	var req domain.AccountRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload data tidak valid"})
	}
	acc, err := h.Service.UpdateAccount(uint(id), req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(acc)
}

func (h *AccountingHandler) DeleteAccount(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID akun tidak valid"})
	}
	if err := h.Service.DeleteAccount(uint(id)); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Akun berhasil dihapus"})
}

// -------------------------------------------------------------
// BUKU HARIAN
// -------------------------------------------------------------

func (h *AccountingHandler) GetAllJournals(c *fiber.Ctx) error {
	var filter domain.JournalFilter
	if err := c.QueryParser(&filter); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Parameter query tidak valid"})
	}
	entries, total, err := h.Service.GetAllJournalEntries(filter)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"data":  entries,
		"total": total,
		"page":  filter.Page,
		"limit": filter.Limit,
	})
}

func (h *AccountingHandler) GetJournalByID(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID jurnal tidak valid"})
	}
	entry, err := h.Service.GetJournalEntryByID(uint(id))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(entry)
}

func (h *AccountingHandler) CreateJournal(c *fiber.Ctx) error {
	var req domain.CreateJournalRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload jurnal tidak valid"})
	}
	entry, err := h.Service.CreateJournalEntry(req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(entry)
}

func (h *AccountingHandler) UpdateJournal(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID jurnal tidak valid"})
	}
	var req domain.CreateJournalRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload jurnal tidak valid"})
	}
	entry, err := h.Service.UpdateJournalEntry(uint(id), req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(entry)
}

func (h *AccountingHandler) DeleteJournal(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID jurnal tidak valid"})
	}
	if err := h.Service.DeleteJournalEntry(uint(id)); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Jurnal transaksi berhasil dihapus"})
}

// -------------------------------------------------------------
// PIUTANG DAGANG
// -------------------------------------------------------------

func (h *AccountingHandler) GetAllPiutang(c *fiber.Ctx) error {
	var filter domain.PiutangFilter
	if err := c.QueryParser(&filter); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Parameter query tidak valid"})
	}
	records, total, err := h.Service.GetAllPiutang(filter)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"data":  records,
		"total": total,
		"page":  filter.Page,
		"limit": filter.Limit,
	})
}

func (h *AccountingHandler) CreatePiutang(c *fiber.Ctx) error {
	var req domain.PiutangDagangRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload piutang tidak valid"})
	}
	p, err := h.Service.CreatePiutang(req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(p)
}

func (h *AccountingHandler) UpdatePiutang(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID piutang tidak valid"})
	}
	var req domain.PiutangDagangRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload piutang tidak valid"})
	}
	p, err := h.Service.UpdatePiutang(uint(id), req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(p)
}

func (h *AccountingHandler) DeletePiutang(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID piutang tidak valid"})
	}
	if err := h.Service.DeletePiutang(uint(id)); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Data piutang berhasil dihapus"})
}

func (h *AccountingHandler) ToggleCheckJournal(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ID jurnal tidak valid"})
	}
	entry, err := h.Service.ToggleCheckJournal(uint(id))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(entry)
}

func (h *AccountingHandler) SyncExpenses(c *fiber.Ctx) error {
	count, err := h.Service.SyncMissingExpenseJournals()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("%d pengeluaran berhasil disinkronkan ke buku harian", count),
		"count":   count,
	})
}

// SyncAll menyinkronkan seluruh transaksi (sales, purchases, expenses, piutang) ke akuntansi
func (h *AccountingHandler) SyncAll(c *fiber.Ctx) error {
	summary, err := h.Service.SyncAllAccounting(nil)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"message": "Sinkronisasi seluruh data akuntansi berhasil dilakukan",
		"data":    summary,
	})
}

func (h *AccountingHandler) GetRekapitulasi(c *fiber.Ctx) error {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	location := c.Query("location")

	result, err := h.Service.GetRekapitulasi(startDate, endDate, location)
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
	location := c.Query("location")
	accountID, _ := strconv.Atoi(c.Query("account_id", "0"))

	cards, err := h.Service.GetGeneralLedger(uint(accountID), startDate, endDate, location)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(cards)
}

func (h *AccountingHandler) ExportJournalsExcel(c *fiber.Ctx) error {
	var filter domain.JournalFilter
	if err := c.QueryParser(&filter); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Parameter filter tidak valid"})
	}

	fileBytes, err := h.Service.ExportJournalsExcel(filter)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	dateSuffix := time.Now().Format("20060102")
	if filter.StartDate != "" {
		dateSuffix = filter.StartDate
		if filter.EndDate != "" && filter.EndDate != filter.StartDate {
			dateSuffix = fmt.Sprintf("%s_%s", filter.StartDate, filter.EndDate)
		}
	}

	filename := fmt.Sprintf("Laporan_Buku_Harian_%s.xlsx", dateSuffix)
	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

	return c.Send(fileBytes)
}
