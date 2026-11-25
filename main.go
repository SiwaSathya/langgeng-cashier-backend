package main

import (
	"backend-cashier/db"
	_ "backend-cashier/docs"
	"backend-cashier/http"
	"backend-cashier/service"
	"log"
	"os"
	"strings"
	"time"

	swagger "github.com/arsmn/fiber-swagger/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
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
	accountingService := service.NewAccountingService(db.Postgres.DB)
	prodService := service.NewProductService(db.Postgres.DB)
	trxService := service.NewTransactionService(db.Postgres.DB, accountingService)
	purchaseService := service.NewPurchaseService(db.Postgres.DB, accountingService)
	masterService := service.NewMasterService(db.Postgres.DB)
	expenseService := service.NewExpenseService(db.Postgres.DB, accountingService)
	authService := service.NewAuthService(db.Postgres.DB)
	returService := service.NewReturService(db.Postgres.DB)
	attendanceService := service.NewAttendanceService(db.Postgres.DB)
	analyticsService := service.NewAnalyticsService(db.Postgres.DB)
	supplierService := service.NewSupplierService(db.Postgres.DB)
	storageService := service.NewStorageService()
	sseService := service.NewSSEService()
	service.SetGlobalSSE(sseService)
	opnameService := service.NewOpnameService(db.Postgres.DB, storageService, sseService)
	presenceService := service.NewPresenceService(sseService)
	userLogService := service.NewUserLogService(db.Postgres.DB)

	trxHandler := http.NewTransactionHandler(trxService, authService)
	masterHandler := http.NewMasterHandler(masterService)
	prodHandler := http.NewProductHandler(prodService, storageService)
	purchaseHandler := http.NewPurchaseHandler(purchaseService)
	authHandler := http.NewAuthHandler(authService, userLogService, presenceService)
	expenseHandler := http.NewExpenseHandler(expenseService, authService)
	returHandler := http.NewReturHandler(returService)
	attendanceHandler := http.NewAttendanceHandler(attendanceService)
	accountingHandler := http.NewAccountingHandler(accountingService)
	analyticsHandler := http.NewAnalyticsHandler(analyticsService)
	supplierHandler := http.NewSupplierHandler(supplierService)
	opnameHandler := http.NewOpnameHandler(opnameService, storageService)
	presenceHandler := http.NewPresenceHandler(presenceService, sseService)

	app := fiber.New(fiber.Config{
		AppName:   "Backend Cashier API",
		BodyLimit: 2 * 1024 * 1024, // Maksimal 2 MB BodyLimit untuk mencegah Memory Exhaustion (Payload Bombs / OOM Crash di Railway)
	})

	// 1. Kompresi HTTP Response (Gzip/Brotli) untuk Menghemat Bandwidth Keluar (Egress) hingga 70-85%
	app.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
	}))

	// 1. Logger Request HTTP
	app.Use(logger.New())

	// 2. CORS Middleware - Harus dipasang sebelum middleware lain agar preflight OPTIONS dijawab seketika
	app.Use(cors.New(cors.Config{
		AllowOriginsFunc: func(origin string) bool {
			// 1. Izinkan akses langsung via file browser (file:// mengirim Origin: null atau kosong)
			if origin == "" || origin == "null" {
				return true
			}
			cleanOrigin := strings.TrimRight(strings.ToLower(strings.TrimSpace(origin)), "/")
			// 2. Izinkan domain vercel resmi & preview deployment vercel
			if cleanOrigin == "https://langgeng-cashier.vercel.app" || strings.HasSuffix(cleanOrigin, ".vercel.app") || strings.Contains(cleanOrigin, "vercel.app") {
				return true
			}
			// 3. Izinkan localhost & IP lokal untuk testing & produksi lokal
			if strings.HasPrefix(cleanOrigin, "http://localhost:") ||
				strings.HasPrefix(cleanOrigin, "http://127.0.0.1:") ||
				strings.HasPrefix(cleanOrigin, "https://localhost:") ||
				strings.HasPrefix(cleanOrigin, "https://127.0.0.1:") ||
				cleanOrigin == "http://localhost" || cleanOrigin == "http://127.0.0.1" {
				return true
			}
			// 4. Periksa apakah ada konfigurasi tambahan via environment variable ALLOWED_ORIGINS
			if envOrigins := os.Getenv("ALLOWED_ORIGINS"); envOrigins != "" {
				for _, o := range strings.Split(envOrigins, ",") {
					if strings.TrimRight(strings.ToLower(strings.TrimSpace(o)), "/") == cleanOrigin {
						return true
					}
				}
			}
			// 5. Default izinkan origin yang sah
			return true
		},
		AllowCredentials: true,
		AllowMethods:     "GET,POST,PUT,DELETE,PATCH,OPTIONS,HEAD",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-Requested-With, X-User-Role, X-User-Location, X-User-ID, X-User-Name, Cache-Control, Pragma, If-Modified-Since, Range",
		ExposeHeaders:    "X-Renewed-Token, Content-Disposition, Content-Length, Content-Range",
		MaxAge:           86400,
	}))

	// 3. Enhanced Security Headers (X-Content-Type-Options, X-Frame-Options, dsb)
	app.Use(http.SecurityHeadersMiddleware())

	// 4. XSS Sanitizer Middleware (Mencegah Penyusupan Skrip Berbahaya / Input XSS)
	app.Use(http.XSSSanitizerMiddleware())

	// 5. SQL Injection Protection Firewall (Penyaring Otomatis Payload Injeksi Berbahaya)
	app.Use(http.SQLInjectionFirewallMiddleware())

	// 5. Rate Limiter Perlindungan API dari Serangan DoS / Brute-Force Massal
	app.Use("/api", limiter.New(limiter.Config{
		Max:        300, // 300 request per menit per IP
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "Batas frekuensi permintaan terlampaui (Rate Limit). Silakan tunggu beberapa saat.",
				"code":  "RATE_LIMIT_EXCEEDED",
			})
		},
	}))

	// 6. Rolling Session Middleware (Otomatis Perpanjang Token 9 Jam saat User Berinteraksi setelah 5 Jam)
	app.Use(http.RollingSessionMiddleware(authService))

	app.Get("/swagger/*", swagger.HandlerDefault)

	// Static route untuk file upload lokal (fallback jika MinIO belum diset)
	app.Static("/uploads", "./uploads")

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
	attendanceHandler.RegisterRoutes(app)
	accountingHandler.RegisterRoutes(app)
	analyticsHandler.RegisterRoutes(app)
	supplierHandler.RegisterRoutes(app)
	opnameHandler.RegisterRoutes(app)
	presenceHandler.RegisterRoutes(app)

	// Layani frontend static files langsung dari port backend
	app.Static("/", "../langgeng-frontend-main")

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
