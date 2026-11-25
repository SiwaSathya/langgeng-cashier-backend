package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"backend-cashier/db"
	"backend-cashier/domain"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load("../.env")
	_ = godotenv.Load(".env")

	db.New()
	database := db.Postgres.DB

	fmt.Println("==================================================================")
	fmt.Println("   LANGGENG ELEKTRONIK - FLUSH & RESET DATA STOCK OPNAME          ")
	fmt.Println("==================================================================")
	fmt.Println("Peringatan: Perintah ini akan menghapus:")
	fmt.Println("1. Seluruh riwayat sesi Stock Opname (tabel stock_opnames)")
	fmt.Println("2. Seluruh rincian item hitungan fisik (tabel stock_opname_items)")
	fmt.Println("3. Riwayat mutasi jenis OPNAME_ADJUST (tabel stock_movements)")
	fmt.Println("------------------------------------------------------------------")

	force := false
	for _, arg := range os.Args[1:] {
		if arg == "--force" || arg == "-f" || arg == "-y" {
			force = true
			break
		}
	}

	if !force {
		fmt.Print("Apakah Anda yakin ingin mengosongkan data Stock Opname? (ketik 'yes' untuk konfirmasi): ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "yes" && input != "y" {
			fmt.Println("Operasi dibatalkan.")
			return
		}
	}

	fmt.Println("Menghapus data stock opname...")

	tx := database.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1. Hapus rincian item opname
	resItems := tx.Exec("DELETE FROM stock_opname_items")
	if resItems.Error != nil {
		tx.Rollback()
		log.Fatalf("Gagal menghapus stock_opname_items: %v", resItems.Error)
	}

	// 2. Hapus header stock opname
	resHeader := tx.Exec("DELETE FROM stock_opnames")
	if resHeader.Error != nil {
		tx.Rollback()
		log.Fatalf("Gagal menghapus stock_opnames: %v", resHeader.Error)
	}

	// 3. Hapus mutasi stok jenis OPNAME_ADJUST
	resMovements := tx.Where("movement_type = ?", "OPNAME_ADJUST").Delete(&domain.StockMovement{})
	if resMovements.Error != nil {
		tx.Rollback()
		log.Fatalf("Gagal menghapus mutasi OPNAME_ADJUST: %v", resMovements.Error)
	}

	// Reset auto-increment sequence jika ada
	_ = tx.Exec("ALTER SEQUENCE IF EXISTS stock_opnames_id_seq RESTART WITH 1").Error
	_ = tx.Exec("ALTER SEQUENCE IF EXISTS stock_opname_items_id_seq RESTART WITH 1").Error

	if err := tx.Commit().Error; err != nil {
		log.Fatalf("Gagal commit transaksi flush: %v", err)
	}

	fmt.Println("SUKSES! Data Stock Opname berhasil di-flush dan siap dalam keadaan FRESH.")
	fmt.Printf("- Baris stock_opname_items dihapus: %d\n", resItems.RowsAffected)
	fmt.Printf("- Baris stock_opnames dihapus: %d\n", resHeader.RowsAffected)
	fmt.Printf("- Riwayat mutasi OPNAME_ADJUST dihapus: %d\n", resMovements.RowsAffected)
	fmt.Println("==================================================================")
}
