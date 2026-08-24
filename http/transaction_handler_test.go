package http_test

import (
	"backend-cashier/db"
	"backend-cashier/domain"
	localHttp "backend-cashier/http"
	"backend-cashier/service"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
)

func setupTestApp(t *testing.T) (*fiber.App, *service.TransactionService) {
	_ = godotenv.Load("../.env")
	db.New()
	if db.Postgres.DB == nil {
		t.Skip("Database not available, skipping integration test")
	}

	svc := service.NewTransactionService(db.Postgres.DB)
	handler := localHttp.NewTransactionHandler(svc)

	app := fiber.New()
	handler.RegisterRoutes(app)

	return app, svc
}

func TestHTTPCreateSalesAndPelunasan(t *testing.T) {
	app, _ := setupTestApp(t)
	database := db.Postgres.DB

	// Setup fixtures
	var user domain.User
	database.FirstOrCreate(&user, domain.User{ID: "kasir1", Name: "Kasir Test", Username: "kasir1"})

	var supplier domain.Supplier
	database.FirstOrCreate(&supplier, domain.Supplier{ID: "SUPP-TEST", Name: "Supplier Test"})

	var category domain.Category
	database.FirstOrCreate(&category, domain.Category{Name: "Category Test"})

	var brand domain.Brand
	database.FirstOrCreate(&brand, domain.Brand{Name: "Brand Test"})

	pCode := fmt.Sprintf("HTTP-PROD-%d", time.Now().UnixNano())
	prod := domain.Product{
		Kode:       pCode,
		Nama:       pCode,
		Satuan:     "PCS",
		Saldo:      20,
		HBeli:      50000,
		HJual:      100000,
		SupplierID: supplier.ID,
		CategoryID: category.ID,
		BrandID:    brand.ID,
	}
	database.Create(&prod)
	defer database.Unscoped().Delete(&prod)

	// 1. Test POST /api/sales with JSON object
	payload := domain.CreateTransactionRequest{
		MemberName:    "Pelanggan HTTP",
		UserID:        "kasir1",
		PaymentMethod: "Tunai",
		AmountPaid:    50000,
		IsDp:          true,
		Customer: domain.CustomerRequest{
			Name:        "Customer HTTP",
			PhoneNumber: fmt.Sprintf("0899%d", time.Now().UnixNano()%10000000),
		},
		Items: []domain.SalesItemRequest{
			{
				ProductSearch: pCode,
				Qty:           1,
				Price:         100000,
				Discount:      0,
			},
		},
	}

	bodyBytes, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/sales", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("POST /api/sales failed: %v", err)
	}

	if res.StatusCode != 201 {
		t.Fatalf("expected status 201, got %d", res.StatusCode)
	}

	var createResp domain.SalesResponse
	json.NewDecoder(res.Body).Decode(&createResp)
	if createResp.Invoice == "" {
		t.Fatalf("expected invoice in response")
	}

	// 2. Test PUT /api/sales/:id for pelunasan
	pelunasanPayload := domain.PelunasanRequest{
		PaymentMethod: "Transfer Mandiri",
		AmountPaid:    100000,
		IsDp:          false,
		Status:        "Lunas",
	}
	pelunasanBytes, _ := json.Marshal(pelunasanPayload)

	putReq := httptest.NewRequest("PUT", fmt.Sprintf("/api/sales/%s", createResp.Invoice), bytes.NewReader(pelunasanBytes))
	putReq.Header.Set("Content-Type", "application/json")

	putRes, err := app.Test(putReq)
	if err != nil {
		t.Fatalf("PUT /api/sales/:id failed: %v", err)
	}

	if putRes.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", putRes.StatusCode)
	}

	// 3. Test GET /api/sales returns grouped transactions
	getReq := httptest.NewRequest("GET", "/api/sales", nil)
	getRes, err := app.Test(getReq)
	if err != nil {
		t.Fatalf("GET /api/sales failed: %v", err)
	}
	if getRes.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", getRes.StatusCode)
	}

	var groups []domain.SalesTransactionGroup
	json.NewDecoder(getRes.Body).Decode(&groups)
	found := false
	for _, g := range groups {
		if g.Invoice == createResp.Invoice {
			found = true
			if len(g.Items) != 1 {
				t.Fatalf("expected 1 item, got %d", len(g.Items))
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected to find invoice %s in GET /api/sales", createResp.Invoice)
	}

	// Clean up
	database.Unscoped().Where("invoice = ?", createResp.Invoice).Delete(&domain.Sales{})
}
