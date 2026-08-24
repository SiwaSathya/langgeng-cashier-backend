package domain

import (
	"time"

	"gorm.io/gorm"
)

// Account (Bagan Akun / Chart of Accounts)
type Account struct {
	gorm.Model
	Code           string  `gorm:"size:50;uniqueIndex" json:"code"`
	Name           string  `gorm:"size:150;index" json:"name"`
	Type           string  `gorm:"size:50" json:"type"` // ASSET, LIABILITY, EQUITY, REVENUE, EXPENSE
	NormalPosition string  `gorm:"size:10" json:"normal_position"` // DEBET, KREDIT
	InitialBalance float64 `gorm:"default:0" json:"initial_balance"`
	CurrentBalance float64 `gorm:"-" json:"current_balance"`
}

// JournalEntry (Buku Harian Header)
type JournalEntry struct {
	gorm.Model
	EntryNumber string        `gorm:"size:100;uniqueIndex" json:"entry_number"`
	Date        time.Time     `gorm:"index" json:"date"`
	Description string        `gorm:"size:255" json:"description"`
	Reference   string        `gorm:"size:100" json:"reference"`
	Items       []JournalItem `gorm:"foreignKey:JournalEntryID;constraint:OnDelete:CASCADE;" json:"items"`
}

// JournalItem (Buku Harian Detail)
type JournalItem struct {
	gorm.Model
	JournalEntryID uint    `gorm:"index" json:"journal_entry_id"`
	AccountID      uint    `gorm:"index" json:"account_id"`
	Account        Account `gorm:"foreignKey:AccountID" json:"account"`
	Description    string  `gorm:"size:255" json:"description"`
	Debet          float64 `gorm:"default:0" json:"debet"`
	Kredit         float64 `gorm:"default:0" json:"kredit"`
}

type CreateJournalRequest struct {
	Date        string                     `json:"date"` // YYYY-MM-DD
	Description string                     `json:"description"`
	Reference   string                     `json:"reference"`
	Items       []CreateJournalItemRequest `json:"items"`
}

type CreateJournalItemRequest struct {
	AccountID   uint    `json:"account_id"`
	AccountName string  `json:"account_name,omitempty"`
	Description string  `json:"description"`
	Debet       float64 `json:"debet"`
	Kredit      float64 `json:"kredit"`
}

type JournalFilter struct {
	StartDate string `query:"start_date"`
	EndDate   string `query:"end_date"`
	Search    string `query:"search"`
	AccountID uint   `query:"account_id"`
	Page      int    `query:"page"`
	Limit     int    `query:"limit"`
}

// Rekapitulasi Baris (Akumulasi Debet & Kredit per Akun)
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

// Buku Besar (Ledger)
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
	AccountID      uint                    `json:"account_id"`
	AccountCode    string                  `json:"account_code"`
	AccountName    string                  `json:"account_name"`
	AccountType    string                  `json:"account_type"`
	NormalPosition string                  `json:"normal_position"`
	InitialBalance float64                 `json:"initial_balance"`
	TotalDebet     float64                 `json:"total_debet"`
	TotalKredit    float64                 `json:"total_kredit"`
	EndingBalance  float64                 `json:"ending_balance"`
	Transactions   []LedgerTransactionItem `json:"transactions"`
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
	Page      int    `query:"page"`
	Limit     int    `query:"limit"`
}
