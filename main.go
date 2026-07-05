package main

import (
	"backend-cashier/db"
	_ "backend-cashier/docs"
	"backend-cashier/http"
	"backend-cashier/service"
	"log"
	"os"

	swagger "github.com/arsmn/fiber-swagger/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/joho/godotenv"
)

// @title           Backend Cashier API
// @version         1.0
// @description     API Sistem Kasir dengan Manajemen Stok dan Biaya Operasional.
// @host            localhost:4000
// @BasePath        /
func main() {
	Init()
	prodService := service.NewProductService(db.Postgres.DB)
	trxService := service.NewTransactionService(db.Postgres.DB)
	purchaseService := service.NewPurchaseService(db.Postgres.DB)
	masterService := service.NewMasterService(db.Postgres.DB)
	expenseService := service.NewExpenseService(db.Postgres.DB)
	authService := service.NewAuthService(db.Postgres.DB)
	returService := service.NewReturService(db.Postgres.DB)

	trxHandler := http.NewTransactionHandler(trxService)
	masterHandler := http.NewMasterHandler(masterService)
	prodHandler := http.NewProductHandler(prodService)
	purchaseHandler := http.NewPurchaseHandler(purchaseService)
	authHandler := http.NewAuthHandler(authService)
	expenseHandler := http.NewExpenseHandler(expenseService)
	returHandler := http.NewReturHandler(returService)

	app := fiber.New(fiber.Config{
		AppName: "Backend Cashier API",
	})

	app.Use(logger.New())

	app.Get("/swagger/*", swagger.HandlerDefault)
	app.Use(cors.New())

	app.Get("/", func(c *fiber.Ctx) error {
		sqlDB, err := db.Postgres.DB.DB()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"status": "unhealthy",
				"reason": "database connection instance error",
			})
		}

		if err := sqlDB.Ping(); err != nil {
			return c.Status(500).JSON(fiber.Map{
				"status": "unhealthy",
				"reason": "database ping failed",
			})
		}

		return c.JSON(fiber.Map{
			"status":  "ok",
			"message": "system is running smoothly",
		})
	})

	// --- Registrasi Routes Bisnis ---
	trxHandler.RegisterRoutes(app)
	masterHandler.RegisterRoutes(app)
	prodHandler.RegisterRoutes(app)
	purchaseHandler.RegisterRoutes(app)
	expenseHandler.RegisterRoutes(app)
	authHandler.RegisterRoutes(app)
	returHandler.RegisterRoutes(app)

	port := os.Getenv("PORT")
	if port == "" {
		port = "4000"
	}

	log.Fatal(app.Listen(":" + port))
}

func Init() {
	initEnv()
	db.New()
}

func initEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Println(".env file not found, using system environment variables")
	}
}
