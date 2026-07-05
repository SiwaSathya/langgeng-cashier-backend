package main

import (
	"backend-cashier/domain"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/xuri/excelize/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func getColumn(row []string, index int) string {
	if index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}

// Helper untuk membersihkan format angka ribuan (titik) agar bisa jadi float
func parseExcelNumber(val interface{}) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		if v == "" || v == "-" {
			return 0
		}
		// Bersihkan karakter non-angka kecuali minus dan koma/titik desimal
		cleanS := strings.ReplaceAll(v, ".", "")
		cleanS = strings.ReplaceAll(cleanS, ",", ".")

		res, err := strconv.ParseFloat(cleanS, 64)
		if err != nil {
			return 0
		}
		return res
	default:
		return 0
	}
}

func main() {
	importType := flag.String("type", "", "Tipe import: 'stock' atau 'rekap'")
	flag.Parse()

	_ = godotenv.Load()

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s TimeZone=Asia/Jakarta sslmode=disable",
		os.Getenv("DB_HOST"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"), os.Getenv("DB_DATABASE"), os.Getenv("DB_PORT"))

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	db.AutoMigrate(
		&domain.Supplier{}, &domain.Category{}, &domain.Brand{}, &domain.Product{},
		&domain.Cashier{}, &domain.PaymentType{}, &domain.SalesRecap{},
	)

	switch *importType {
	case "stock":
		importStock(db, "Data Stock.xlsx")
	case "rekap":
		importRekap(db, "REKAP PENJUALAN.xlsx")
	case "pembelian":
		importPurchaseAndExpense(db, "pbl.xlsx")
	default:
		fmt.Println("Silakan gunakan flag -type untuk memilih:")
		fmt.Println("  go run main.go -type=stock")
		fmt.Println("  go run main.go -type=rekap")
	}
}

func importStock(db *gorm.DB, fileName string) {
	fmt.Printf("Memulai import Stock dari %s...\n", fileName)
	f, err := excelize.OpenFile(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	rows, _ := f.GetRows("Sheet1")
	suppliersMap := make(map[string]bool)
	categoriesMap := make(map[string]uint)
	brandsMap := make(map[string]uint)

	for i, row := range rows {
		if i < 3 || len(row) < 5 {
			continue
		}

		kode := getColumn(row, 1)
		nama := getColumn(row, 3)
		satuan := getColumn(row, 4)
		saldo, _ := strconv.ParseFloat(getColumn(row, 5), 64)
		hBeli, _ := strconv.ParseFloat(getColumn(row, 6), 64)
		hJual, _ := strconv.ParseFloat(getColumn(row, 8), 64)
		supplierID := getColumn(row, 22)
		supplierNm := getColumn(row, 23)
		catName := getColumn(row, 25)
		brandName := getColumn(row, 26)

		if supplierID == "" {
			supplierID = "UNKNOWN"
			supplierNm = "Tanpa Supplier"
		}
		if !suppliersMap[supplierID] {
			s := domain.Supplier{ID: supplierID, Name: supplierNm}
			db.FirstOrCreate(&s)
			suppliersMap[supplierID] = true
		}

		if catName == "" {
			catName = "Uncategorized"
		}
		if _, ok := categoriesMap[catName]; !ok {
			c := domain.Category{Name: catName}
			db.Where(domain.Category{Name: catName}).FirstOrCreate(&c)
			categoriesMap[catName] = c.ID
		}

		if brandName == "" {
			brandName = "No Brand"
		}
		if _, ok := brandsMap[brandName]; !ok {
			b := domain.Brand{Name: brandName}
			db.Where(domain.Brand{Name: brandName}).FirstOrCreate(&b)
			brandsMap[brandName] = b.ID
		}

		product := domain.Product{
			Kode: kode, Nama: nama, Satuan: satuan, Saldo: saldo,
			HBeli: hBeli, HJual: hJual, SupplierID: supplierID,
			CategoryID: categoriesMap[catName], BrandID: brandsMap[brandName],
		}
		db.Where(domain.Product{Kode: kode}).FirstOrCreate(&product)
	}
	fmt.Println("Import Stock Selesai!")
}

func importRekap(db *gorm.DB, fileName string) {
	fmt.Printf("\n>>> Memulai Import Rekap Penjualan: %s\n", fileName)
	f, err := excelize.OpenFile(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	rows, _ := f.GetRows("Sheet1")
	for i, row := range rows {
		// Data dimulai dari Baris 5 (Indeks 4)
		if i < 4 || len(row) < 6 {
			continue
		}

		noStr := getColumn(row, 0)
		tglStr := getColumn(row, 1)
		jenisStr := getColumn(row, 2)
		kasirStr := getColumn(row, 6)

		// Validasi baris: Lewati jika kolom No dan Tanggal kosong (biasanya footer/spasi)
		if (noStr == "" && tglStr == "") || strings.Contains(strings.ToUpper(jenisStr), "TOTAL") {
			continue
		}

		// Handle Nama Kasir & Jenis (Lowercase agar tidak duplikat desak/DESAK)
		jenisClean := strings.ToLower(jenisStr)
		kasirClean := strings.ToLower(kasirStr)

		var pt domain.PaymentType
		db.Where("LOWER(name) = ?", jenisClean).FirstOrCreate(&pt, domain.PaymentType{Name: jenisStr})

		var cs domain.Cashier
		db.Where("LOWER(name) = ?", kasirClean).FirstOrCreate(&cs, domain.Cashier{Name: kasirStr})

		// Parsing Tanggal
		tgl, errTgl := time.Parse("2006-01-02", tglStr)
		if errTgl != nil {
			tgl, _ = time.Parse("02-01-2006", tglStr)
		}

		recap := domain.SalesRecap{
			Tanggal:       tgl,
			Jumlah:        parseExcelNumber(getColumn(row, 3)),
			Nilai:         parseExcelNumber(getColumn(row, 4)),
			SBGN:          parseExcelNumber(getColumn(row, 5)),
			PaymentTypeID: pt.ID,
			CashierID:     cs.ID,
		}

		if err := db.Create(&recap).Error; err != nil {
			fmt.Printf("[ERROR] Baris %d gagal simpan: %v\n", i+1, err)
		} else {
			fmt.Printf("[SUCCESS] Baris %d: Tgl=%s, Jenis=%s, Kasir=%s\n", i+1, tglStr, jenisStr, kasirStr)
		}
	}
	fmt.Println(">>> Import Rekap Penjualan Selesai!")
}

func importPurchaseAndExpense(db *gorm.DB, fileName string) {
	fmt.Printf("\n>>> Memproses Data Pengeluaran Bercampur...\n")
	f, err := excelize.OpenFile(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	rows, _ := f.GetRows(f.GetSheetList()[0])

	for i, row := range rows {
		if i == 0 || len(row) < 23 {
			continue
		}

		// Mapping dasar
		nota := getColumn(row, 0)                    // NOTA (A)
		kodeBarang := getColumn(row, 1)              // BARA (B)
		namaBarang := getColumn(row, 2)              // NAMA (C)
		total := parseExcelNumber(getColumn(row, 9)) // TOTAL (J)
		tglStr := getColumn(row, 14)                 // TGL (O)
		tgl, _ := time.Parse("1/2/2006", tglStr)

		// --- LOGIKA PEMISAHAN ---

		if kodeBarang != "" && kodeBarang != "-" {
			// A. MASUK SEBAGAI PENGELUARAN BARANG (STOCK)
			var prod domain.Product
			if err := db.Where("kode = ?", kodeBarang).First(&prod).Error; err == nil {
				// Catat ke Tabel Purchase
				qty := parseExcelNumber(getColumn(row, 3))
				purchase := domain.Purchase{
					Nota:      nota,
					Tanggal:   tgl,
					ProductID: prod.ID,
					Qty:       qty,
					HBeli:     parseExcelNumber(getColumn(row, 11)),
					Total:     total,
				}
				db.Create(&purchase)

				// Update Saldo Barang
				db.Model(&prod).Update("saldo", prod.Saldo+qty)
				fmt.Printf("[BARANG] %s berhasil masuk stok\n", namaBarang)
			}
		} else {
			// B. MASUK SEBAGAI BIAYA OPERASIONAL (NON-BARANG)
			expense := domain.Expense{
				Nota:      nota,
				Tanggal:   tgl,
				Deskripsi: namaBarang,
				Total:     total,
				Kategori:  getColumn(row, 13), // GOL (N)
			}

			if err := db.Create(&expense).Error; err == nil {
				fmt.Printf("[BIAYA] %s tercatat di pengeluaran operasional\n", namaBarang)
			}
		}
	}
	fmt.Println(">>> Proses Selesai: Data Barang & Non-Barang telah dipisahkan!")
}
