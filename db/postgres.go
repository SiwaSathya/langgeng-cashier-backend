package db

import (
	"backend-cashier/domain"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/pandeptwidyaop/golog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PostgresDatabase struct {
	DB *gorm.DB
}

var Postgres PostgresDatabase

func New() {
	var e error
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Makassar",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_DATABASE"),
		os.Getenv("DB_PORT"),
	)

	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{})

	if e != nil {
		golog.Slack.Error("Failed to connect to database", e)
		log.Println(dsn)
		log.Fatal(e)
	}

	// Konfigurasi Connection Pool untuk Efisiensi Memori DB & Penghematan Biaya
	sqlDB, errSql := db.DB()
	if errSql == nil {
		sqlDB.SetMaxOpenConns(25)                  // Batas maksimum 25 koneksi terbuka (mencegah DB RAM meledak)
		sqlDB.SetMaxIdleConns(5)                   // 5 koneksi standby siap pakai
		sqlDB.SetConnMaxLifetime(15 * time.Minute) // Daur ulang koneksi berkala setiap 15 menit
		sqlDB.SetConnMaxIdleTime(5 * time.Minute)  // Tutup koneksi idle jika tidak dipakai selama 5 menit
	}

	Postgres = PostgresDatabase{
		DB: db,
	}

	_ = db.Exec("ALTER TABLE payment_methods ADD COLUMN IF NOT EXISTS shift bigint DEFAULT NULL;")

	envVal := strings.ToLower(strings.TrimSpace(os.Getenv("ENV")))
	autoMigrateVal := strings.ToLower(strings.TrimSpace(os.Getenv("AUTO_MIGRATE")))
	if envVal == "development" || envVal == "" || autoMigrateVal == "true" || autoMigrateVal == "1" {
		log.Println("[Database] Memulai GORM AutoMigrate tabel...")
		RegisterTableToMigrate(db)
		log.Println("[Database] GORM AutoMigrate selesai dengan sukses!")
	}
}

func RegisterTableToMigrate(db *gorm.DB) {
	e := db.AutoMigrate(
		&domain.User{},
		&domain.Brand{},
		&domain.Category{},
		&domain.Supplier{},
		&domain.Product{},
		&domain.Cashier{},
		&domain.PaymentType{},
		&domain.SalesRecap{},
		&domain.Purchase{},
		&domain.Expense{},
		&domain.Customer{},
		&domain.Sales{},
		&domain.Retur{},
		&domain.Attendance{},
		&domain.Account{},
		&domain.JournalEntry{},
		&domain.JournalItem{},
		&domain.PiutangDagang{},
		&domain.PaymentMethod{},
		&domain.ProductStock{},
		&domain.ProductImage{},
		&domain.StockMovement{},
		&domain.StockOpname{},
		&domain.StockOpnameItem{},
		&domain.UserActivityLog{},
		&domain.RevokedToken{},
	)

	if e != nil {
		golog.Slack.Error("Failed to run migration", e)
		log.Fatal(e)
	}
}
