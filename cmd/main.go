package main

import (
	"backend-cashier/domain"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/xuri/excelize/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type ProductJSON struct {
	Kode string `json:"kode"`

	Nama string `json:"nama"`

	Satuan string `json:"satuan"`

	Saldo float64 `json:"saldo"`

	HBeli float64 `json:"h_beli"`

	HJual float64 `json:"h_jual"`

	Supplier string `json:"supplier"`

	Category string `json:"category"`

	Brand string `json:"brand"`
}

type ProductJSONWrapper struct {
	Products []ProductJSON `json:"products"`
}

func getOrCreateCategory(
	db *gorm.DB,
	name string,
) domain.Category {

	var category domain.Category

	name = strings.TrimSpace(name)

	if name == "" {

		name = "Uncategorized"

	}

	err :=
		db.Where(
			"LOWER(name) LIKE ?",
			"%"+strings.ToLower(name)+"%",
		).
			First(&category).
			Error

	if err == gorm.ErrRecordNotFound {

		category =
			domain.Category{

				Name: name,
			}

		db.Create(
			&category,
		)

		fmt.Println(
			"[CREATE CATEGORY]",
			name,
		)

	}

	return category

}

func getOrCreateBrand(
	db *gorm.DB,
	name string,
) domain.Brand {

	var brand domain.Brand

	name =
		strings.TrimSpace(name)

	if name == "" {

		name = "No Brand"

	}

	err :=
		db.Where(
			"LOWER(name) LIKE ?",
			"%"+strings.ToLower(name)+"%",
		).
			First(&brand).
			Error

	if err == gorm.ErrRecordNotFound {

		brand =
			domain.Brand{

				Name: name,
			}

		db.Create(
			&brand,
		)

		fmt.Println(
			"[CREATE BRAND]",
			name,
		)

	}

	return brand

}

func getOrCreateSupplier(
	db *gorm.DB,
	name string,
) domain.Supplier {

	var supplier domain.Supplier

	name =
		strings.TrimSpace(name)

	if name == "" {

		name = "Tanpa Supplier"

	}

	err :=
		db.Where(
			"LOWER(name) LIKE ?",
			"%"+strings.ToLower(name)+"%",
		).
			First(&supplier).
			Error

	if err == gorm.ErrRecordNotFound {

		supplier =
			domain.Supplier{

				ID: uuid.New().String(),

				Name: name,
			}

		db.Create(
			&supplier,
		)

		fmt.Println(
			"[CREATE SUPPLIER]",
			name,
		)

	}

	return supplier

}

func getColumn(row []string, index int) string {
	if index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}

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
	importType := flag.String("type", "", "Tipe import: stock, rekap, pembelian, json")
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
	case "json":

		importProductJSON(
			db,
			"POLYTRON.json",
		)

	default:
		fmt.Println("Silakan gunakan flag -type untuk memilih:")
		fmt.Println("  go run main.go -type=stock")
		fmt.Println("  go run main.go -type=rekap")
	}
}

// Helper untuk mengambil kolom dengan aman tanpa takut index out of range
func getColumnSafe(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

// Helper untuk parsing float yang aman dari format string kosong atau spasial
func parseExcelFloat(val string) float64 {
	if val == "" {
		return 0
	}
	// Hilangkan koma jika ada format ribuan (misal: 2,060,160 atau 2.060.160 tergantung regional)
	// Kita bersihkan karakter non-numeric kecuali titik/koma desimal jika diperlukan
	val = strings.ReplaceAll(val, ",", "")

	res, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0
	}
	return res
}

func importStock(db *gorm.DB, fileName string) {
	fmt.Printf("Memulai import Stock dari %s...\n", fileName)
	f, err := excelize.OpenFile(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// Ganti ke sheet yang sesuai jika diperlukan (misal Sheet1)
	rows, err := f.GetRows("Sheet1")
	if err != nil {
		log.Fatalf("Gagal membaca sheet: %v", err)
	}

	suppliersMap := make(map[string]bool)
	categoriesMap := make(map[string]uint)
	brandsMap := make(map[string]uint)

	for i, row := range rows {
		// Header ada di baris ke-4 (indeks 3), data mulai dari indeks 4
		if i < 4 {
			continue
		}

		// Pastikan baris memiliki data minimal (misal kolom Kode dan Nama tidak kosong)
		kode := getColumnSafe(row, 1)
		if kode == "" || kode == "-" {
			continue
		}

		nama := getColumnSafe(row, 3)
		satuan := getColumnSafe(row, 4)

		// Gunakan parser float yang lebih aman
		saldo := parseExcelFloat(getColumnSafe(row, 5))
		hBeli := parseExcelFloat(getColumnSafe(row, 6))
		hJual := parseExcelFloat(getColumnSafe(row, 8)) // Kolom HJUAL (Indeks 8)

		supplierID := getColumnSafe(row, 22)
		supplierNm := getColumnSafe(row, 23)
		catName := getColumnSafe(row, 25)
		brandName := getColumnSafe(row, 26)

		if supplierID == "" || supplierID == "-" {
			supplierID = "UNKNOWN"
			supplierNm = "Tanpa Supplier"
		}
		if !suppliersMap[supplierID] {
			s := domain.Supplier{ID: supplierID, Name: supplierNm}
			db.Where(domain.Supplier{ID: supplierID}).FirstOrCreate(&s)
			suppliersMap[supplierID] = true
		}

		if catName == "" || catName == "-" {
			catName = "Uncategorized"
		}
		if _, ok := categoriesMap[catName]; !ok {
			c := domain.Category{Name: catName}
			db.Where(domain.Category{Name: catName}).FirstOrCreate(&c)
			categoriesMap[catName] = c.ID
		}

		if brandName == "" || brandName == "-" {
			brandName = "No Brand"
		}
		if _, ok := brandsMap[brandName]; !ok {
			b := domain.Brand{Name: brandName}
			db.Where(domain.Brand{Name: brandName}).FirstOrCreate(&b)
			brandsMap[brandName] = b.ID
		}

		var existingProduct domain.Product
		err := db.Where("kode = ?", kode).First(&existingProduct).Error

		if err == gorm.ErrRecordNotFound {
			newProduct := domain.Product{
				Kode:       kode,
				Nama:       nama,
				Satuan:     satuan,
				Saldo:      saldo,
				HBeli:      hBeli,
				HJual:      hJual,
				SupplierID: supplierID,
				CategoryID: categoriesMap[catName],
				BrandID:    brandsMap[brandName],
			}
			db.Create(&newProduct)
		} else if err == nil {
			existingProduct.Nama = nama
			existingProduct.Satuan = satuan
			existingProduct.Saldo = saldo
			existingProduct.HBeli = hBeli
			existingProduct.HJual = hJual
			existingProduct.SupplierID = supplierID
			existingProduct.CategoryID = categoriesMap[catName]
			existingProduct.BrandID = brandsMap[brandName]

			db.Save(&existingProduct)
		}
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

func importProductJSON(
	db *gorm.DB,
	fileName string,
) {

	fmt.Println(
		"Start Import Product JSON",
	)

	file, err :=
		os.ReadFile(fileName)

	if err != nil {

		log.Fatal(err)

	}

	var data ProductJSONWrapper

	err =
		json.Unmarshal(
			file,
			&data,
		)

	if err != nil {

		log.Fatal(err)

	}

	for _, item := range data.Products {

		fmt.Println(
			"Processing:",
			item.Kode,
		)

		// =====================
		// MASTER DATA
		// =====================

		category :=
			getOrCreateCategory(
				db,
				item.Category,
			)

		brand :=
			getOrCreateBrand(
				db,
				item.Brand,
			)

		supplier :=
			getOrCreateSupplier(
				db,
				item.Supplier,
			)

		// =====================
		// PRODUCT
		// =====================

		var product domain.Product

		err :=
			db.Where(
				"kode = ?",
				item.Kode,
			).
				First(
					&product,
				).
				Error

		if err == gorm.ErrRecordNotFound {

			product =
				domain.Product{

					Kode: item.Kode,

					Nama: item.Nama,

					Satuan: item.Satuan,

					Saldo: item.Saldo,

					HBeli: item.HBeli,

					HPokok: item.HBeli,

					HJual: item.HJual,

					SupplierID: supplier.ID,

					CategoryID: category.ID,

					BrandID: brand.ID,
				}

			err =
				db.Create(
					&product,
				).
					Error

			if err != nil {

				fmt.Println(
					"[ERROR CREATE PRODUCT]",
					err,
				)

			} else {

				fmt.Println(
					"[INSERT]",
					item.Nama,
				)

			}

		} else {

			product.Nama =
				item.Nama

			product.Satuan =
				item.Satuan

			product.Saldo =
				item.Saldo

			product.HBeli =
				item.HBeli

			product.HPokok =
				item.HBeli

			product.HJual =
				item.HJual

			product.SupplierID =
				supplier.ID

			product.CategoryID =
				category.ID

			product.BrandID =
				brand.ID

			db.Save(
				&product,
			)

			fmt.Println(
				"[UPDATE]",
				item.Nama,
			)

		}

	}

	fmt.Println(
		"Import Product JSON selesai",
	)

}
