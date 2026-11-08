package domain

import (
	"time"

	"gorm.io/gorm"
)

// Account (Chart of Accounts / Bagan Akun Standar)
type Account struct {
	gorm.Model
	Code           string  `gorm:"uniqueIndex;size:50" json:"code"`
	Name           string  `gorm:"size:150" json:"name"`
	Type           string  `gorm:"size:50" json:"type"`            // ASSET, LIABILITY, EQUITY, REVENUE, EXPENSE
	NormalPosition string  `gorm:"size:20" json:"normal_position"` // DEBET, KREDIT
	InitialBalance float64 `gorm:"default:0" json:"initial_balance"`
	Description    string  `gorm:"size:255" json:"description"`
}

type AccountRequest struct {
	Code           string  `json:"code"`
	Name           string  `json:"name"`
	Type           string  `json:"type"`
	NormalPosition string  `json:"normal_position"`
	InitialBalance float64 `json:"initial_balance"`
	Description    string  `json:"description"`
}

// JournalEntry (Buku Harian Jurnal Umum)
type JournalEntry struct {
	gorm.Model
	EntryNumber string        `gorm:"uniqueIndex;size:100" json:"entry_number"`
	Date        time.Time     `gorm:"index" json:"date"`
	Description string        `gorm:"size:255" json:"description"`
	Reference   string        `gorm:"size:100;index" json:"reference"` // No Nota, No Invoice, No Voucher
	Location    string        `gorm:"size:100;index" json:"location"`  // Toko Utama, Toko Sudirman, Toko Paye
	IsChecked   bool          `gorm:"default:false;index" json:"is_checked"`
	Items       []JournalItem `gorm:"foreignKey:JournalEntryID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"items"`
}

// JournalItem (Detail Garis Debet / Kredit Jurnal)
type JournalItem struct {
	gorm.Model
	JournalEntryID uint    `gorm:"index" json:"journal_entry_id"`
	AccountID      uint    `gorm:"index" json:"account_id"`
	Account        Account `gorm:"foreignKey:AccountID" json:"account"`
	Description    string  `gorm:"size:255" json:"description"`
	Debet          float64 `gorm:"default:0" json:"debet"`
	Kredit         float64 `gorm:"default:0" json:"kredit"`
}

type JournalItemRequest struct {
	AccountID   uint    `json:"account_id"`
	AccountName string  `json:"account_name,omitempty"`
	Description string  `json:"description"`
	Debet       float64 `json:"debet"`
	Kredit      float64 `json:"kredit"`
}

// Alias backward compatibility untuk CreateJournalItemRequest
type CreateJournalItemRequest = JournalItemRequest

type CreateJournalRequest struct {
	Date        string               `json:"date"` // Format: YYYY-MM-DD
	Description string               `json:"description"`
	Reference   string               `json:"reference"`
	Location    string               `json:"location"`
	Items       []JournalItemRequest `json:"items"`
}

type JournalFilter struct {
	StartDate string `query:"start_date"`
	EndDate   string `query:"end_date"`
	Month     int    `query:"month"`
	Year      int    `query:"year"`
	Search    string `query:"search"`
	Location  string `query:"location"`
	Status    string `query:"status"` // "checked", "unchecked", or empty for all
	Page      int    `query:"page"`
	Limit     int    `query:"limit"`
}

// DTO untuk Rekapitulasi Jurnal
type AccountingRecapItem struct {
	AccountID   uint    `json:"account_id"`
	AccountCode string  `json:"account_code"`
	AccountName string  `json:"account_name"`
	AccountType string  `json:"account_type"`
	TotalDebet  float64 `json:"total_debet"`
	TotalKredit float64 `json:"total_kredit"`
}

type AccountingRecapResponse struct {
	StartDate   string                `json:"start_date"`
	EndDate     string                `json:"end_date"`
	Items       []AccountingRecapItem `json:"items"`
	TotalDebet  float64               `json:"total_debet"`
	TotalKredit float64               `json:"total_kredit"`
	Difference  float64               `json:"difference"`
	IsBalanced  bool                  `json:"is_balanced"`
}

// DTO untuk Buku Besar (General Ledger)
type LedgerTransactionItem struct {
	ID          uint      `json:"id"`
	Date        time.Time `json:"date"`
	EntryNumber string    `json:"entry_number"`
	Description string    `json:"description"`
	Reference   string    `json:"reference"`
	Debet       float64   `json:"debet"`
	Kredit      float64   `json:"kredit"`
	Balance     float64   `json:"balance"`
}

type LedgerAccountCard struct {
	AccountID       uint                    `json:"account_id"`
	AccountCode     string                  `json:"account_code"`
	AccountName     string                  `json:"account_name"`
	AccountType     string                  `json:"account_type"`
	NormalPosition  string                  `json:"normal_position"`
	StartingBalance float64                 `json:"starting_balance"`
	Transactions    []LedgerTransactionItem `json:"transactions"`
	TotalDebet      float64                 `json:"total_debet"`
	TotalKredit     float64                 `json:"total_kredit"`
	EndingBalance   float64                 `json:"ending_balance"`
}

// PiutangDagang (Buku Pembantu Piutang / Accounts Receivable)
type PiutangDagang struct {
	gorm.Model
	CustomerID    *uint     `json:"customer_id"`
	Customer      Customer  `gorm:"foreignKey:CustomerID" json:"customer"`
	CustomerName  string    `gorm:"size:150" json:"customer_name"`
	CustomerPhone string    `gorm:"size:50" json:"customer_phone"`
	SalesInvoice  string    `gorm:"size:100;index" json:"sales_invoice"`
	Tanggal       time.Time `gorm:"index" json:"tanggal"`
	SaldoAwal     float64   `gorm:"default:0" json:"saldo_awal"`
	Debet         float64   `gorm:"default:0" json:"debet"`  // Penambahan piutang (Total Penjualan)
	Kredit        float64   `gorm:"default:0" json:"kredit"` // Pembayaran / Pelunasan
	SaldoAkhir    float64   `gorm:"default:0" json:"saldo_akhir"`
	Keterangan    string    `gorm:"size:255" json:"keterangan"`
	Status        string    `gorm:"size:50" json:"status"` // Belum Lunas, Lunas
}

type PiutangDagangRequest struct {
	CustomerID    *uint   `json:"customer_id"`
	CustomerName  string  `json:"customer_name"`
	CustomerPhone string  `json:"customer_phone"`
	SalesInvoice  string  `json:"sales_invoice"`
	Tanggal       string  `json:"tanggal"` // YYYY-MM-DD
	SaldoAwal     float64 `json:"saldo_awal"`
	Debet         float64 `json:"debet"`
	Kredit        float64 `json:"kredit"`
	Keterangan    string  `json:"keterangan"`
	Status        string  `json:"status"`
}

type PiutangFilter struct {
	StartDate string `query:"start_date"`
	EndDate   string `query:"end_date"`
	Search    string `query:"search"`
	Status    string `query:"status"`
	GroupBy   string `query:"group_by"` // "phone" atau empty
	Page      int    `query:"page"`
	Limit     int    `query:"limit"`
}

// DTO untuk Ringkasan Sinkronisasi Akuntansi
type SyncAccountingSummary struct {
	DeletedJournals  int `json:"deleted_journals"`
	DeletedItems     int `json:"deleted_items"`
	DeletedPiutang   int `json:"deleted_piutang"`
	AccountsCreated  int `json:"accounts_created"`
	SalesSynced      int `json:"sales_synced"`
	PurchasesSynced  int `json:"purchases_synced"`
	ExpensesSynced   int `json:"expenses_synced"`
	PiutangSynced    int `json:"piutang_synced"`
	TotalNewJournals int `json:"total_new_journals"`
}
