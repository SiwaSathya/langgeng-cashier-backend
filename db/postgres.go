package db

import (
	"backend-cashier/domain"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/pandeptwidyaop/golog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PostgresStruct struct {
	DB *gorm.DB
}

var Postgres PostgresStruct

func New() {
	_ = godotenv.Load()

	host := os.Getenv("DB_HOST")
	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASSWORD")
	name := os.Getenv("DB_DATABASE")
	port := os.Getenv("DB_PORT")

	if host == "" || user == "" || pass == "" || name == "" || port == "" {
		log.Fatal("database env vars not set (DB_HOST/DB_USER/DB_PASSWORD/DB_DATABASE/DB_PORT). Ensure .env is loaded or env vars exported")
	}

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s TimeZone=Asia/Jakarta sslmode=disable", host, user, pass, name, port)

	if Postgres.DB == nil {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			log.Fatal(err)
		}

		Postgres = PostgresStruct{
			DB: db,
		}
		env := os.Getenv("ENV")
		if env != "test" {
			RegisterTableToMigrate(db)
		}
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
	)

	if e != nil {
		golog.Slack.Error("Failed to run migration", e)
		log.Fatal(e)
	}
}
