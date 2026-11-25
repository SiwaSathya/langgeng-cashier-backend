package cmd

import (
	"fmt"
	"log"
	"os"

	"backend-cashier/service"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	fmt.Println("==================================================")
	fmt.Println("    LANGGENG ELEKTRONIK - RESYNC DATA AKUNTANSI    ")
	fmt.Println("==================================================")

	_ = godotenv.Load(".env", "../.env", "../../.env")

	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_DATABASE")

	if dbHost == "" {
		dbHost = "127.0.0.1"
	}
	if dbPort == "" {
		dbPort = "5432"
	}

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s TimeZone=Asia/Jakarta sslmode=disable",
		dbHost, dbUser, dbPassword, dbName, dbPort)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("[FATAL] Gagal menghubungkan ke database: %v\n", err)
	}

	fmt.Printf("[OK] Terhubung ke database '%s' di %s:%s\n", dbName, dbHost, dbPort)
	fmt.Println("[INFO] Menghapus data akuntansi lama dan membuat ulang agar sinkron dengan data transaksi lain...")

	accountingSvc := service.NewAccountingService(db)

	summary, err := accountingSvc.SyncAllAccounting(db)
	if err != nil {
		log.Fatalf("[FATAL] Sinkronisasi akuntansi gagal: %v\n", err)
	}

	fmt.Println("--------------------------------------------------")
	fmt.Println("             RINGKASAN SINKRONISASI               ")
	fmt.Println("--------------------------------------------------")
	fmt.Printf("• Data Jurnal Lama Dibersihkan        : %d jurnal (%d item)\n", summary.DeletedJournals, summary.DeletedItems)
	fmt.Printf("• Data Piutang Lama Dibersihkan       : %d piutang\n", summary.DeletedPiutang)
	fmt.Printf("• Akun Standar Dibuat / Dipastikan   : %d akun\n", summary.AccountsCreated)
	fmt.Printf("• Jurnal Penjualan Baru (Sales)       : %d transaksi\n", summary.SalesSynced)
	fmt.Printf("• Jurnal Pembelian Baru (Purchases)   : %d nota\n", summary.PurchasesSynced)
	fmt.Printf("• Jurnal Biaya Operasional (Expenses) : %d pengeluaran\n", summary.ExpensesSynced)
	fmt.Printf("• Piutang Dagang Baru (Bon / DP)      : %d pelanggan\n", summary.PiutangSynced)
	fmt.Printf("• Total Entri Jurnal Baru Tercipta    : %d jurnal\n", summary.TotalNewJournals)
	fmt.Println("==================================================")
	fmt.Println("[SELESAI] Seluruh data akuntansi kini telah bersih & sinkron!")
	fmt.Println("==================================================")
}
