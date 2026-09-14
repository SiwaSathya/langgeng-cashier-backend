package service_test

import (
	"backend-cashier/db"
	"backend-cashier/service"
	"testing"

	"github.com/joho/godotenv"
)

func setupTestDB(t *testing.T) *service.TransactionService {
	_ = godotenv.Load("../.env")
	db.New()
	if db.Postgres.DB == nil {
		t.Skip("Database not available, skipping integration test")
	}
	db.RegisterTableToMigrate(db.Postgres.DB)
	accSvc := service.NewAccountingService(db.Postgres.DB)
	return service.NewTransactionService(db.Postgres.DB, accSvc)
}

// func TestCreateSalesAndPelunasanPerNota(t *testing.T) {
// 	svc := setupTestDB(t)
// 	database := db.Postgres.DB

// 	// 1. Setup sample user, supplier, category, brand, and products
// 	var user domain.User
// 	database.FirstOrCreate(&user, domain.User{ID: "kasir1", Name: "Kasir Test", Username: "kasir1"})

// 	var supplier domain.Supplier
// 	database.FirstOrCreate(&supplier, domain.Supplier{ID: "SUPP-TEST", Name: "Supplier Test"})

// 	var category domain.Category
// 	database.FirstOrCreate(&category, domain.Category{Name: "Category Test"})

// 	var brand domain.Brand
// 	database.FirstOrCreate(&brand, domain.Brand{Name: "Brand Test"})

// 	p1Code := fmt.Sprintf("TEST-PROD-1-%d", time.Now().UnixNano())
// 	p2Code := fmt.Sprintf("TEST-PROD-2-%d", time.Now().UnixNano())

// 	prod1 := domain.Product{
// 		Kode:       p1Code,
// 		Nama:       p1Code,
// 		Satuan:     "PCS",
// 		Saldo:      10,
// 		HBeli:      50000,
// 		HJual:      100000,
// 		SupplierID: supplier.ID,
// 		CategoryID: category.ID,
// 		BrandID:    brand.ID,
// 	}
// 	prod2 := domain.Product{
// 		Kode:       p2Code,
// 		Nama:       p2Code,
// 		Satuan:     "PCS",
// 		Saldo:      10,
// 		HBeli:      100000,
// 		HJual:      200000,
// 		SupplierID: supplier.ID,
// 		CategoryID: category.ID,
// 		BrandID:    brand.ID,
// 	}

// 	if err := database.Create(&prod1).Error; err != nil {
// 		t.Fatalf("failed to create test product 1: %v", err)
// 	}
// 	if err := database.Create(&prod2).Error; err != nil {
// 		t.Fatalf("failed to create test product 2: %v", err)
// 	}

// 	defer func() {
// 		database.Unscoped().Delete(&prod1)
// 		database.Unscoped().Delete(&prod2)
// 	}()

// 	// 2. Test Create Transaction with DP (Per Nota)
// 	custPhone := fmt.Sprintf("0812%d", time.Now().Unix()%10000000)
// 	txReq := domain.CreateTransactionRequest{
// 		MemberName:    "Pelanggan Setia",
// 		UserID:        "kasir1",
// 		PaymentMethod: "Tunai",
// 		AmountPaid:    100000, // DP 100.000 for total 300.000
// 		IsDp:          true,
// 		Customer: domain.CustomerRequest{
// 			Name:        "Budi Santoso",
// 			PhoneNumber: custPhone,
// 			Address:     "Jl. Sudirman No 1",
// 		},
// 		Items: []domain.SalesItemRequest{
// 			{
// 				ProductSearch: p1Code,
// 				Qty:           1,
// 				Price:         100000,
// 				Discount:      0,
// 			},
// 			{
// 				ProductSearch: p2Code,
// 				Qty:           1,
// 				Price:         200000,
// 				Discount:      0,
// 			},
// 		},
// 	}

// 	resp, err := svc.CreateSales(txReq)
// 	if err != nil {
// 		t.Fatalf("CreateSales failed: %v", err)
// 	}

// 	if resp.Invoice == "" {
// 		t.Fatalf("expected invoice not empty")
// 	}
// 	if resp.TotalNetto != 300000 {
// 		t.Fatalf("expected total netto 300000, got %f", resp.TotalNetto)
// 	}
// 	if resp.Status != "DP" {
// 		t.Fatalf("expected status DP, got %s", resp.Status)
// 	}

// 	// Verify only 1 customer was created for this transaction
// 	var customerCount int64
// 	database.Model(&domain.Customer{}).Where("phone_number = ?", custPhone).Count(&customerCount)
// 	if customerCount != 1 {
// 		t.Fatalf("expected exactly 1 customer created per nota, found %d", customerCount)
// 	}

// 	// Verify both sales records have status DP and same invoice and customer ID
// 	var salesList []domain.Sales
// 	database.Where("invoice = ?", resp.Invoice).Find(&salesList)
// 	if len(salesList) != 2 {
// 		t.Fatalf("expected 2 sales rows, got %d", len(salesList))
// 	}

// 	for _, s := range salesList {
// 		if s.Status != "DP" {
// 			t.Fatalf("expected sales item status DP, got %s", s.Status)
// 		}
// 		if !s.IsDp {
// 			t.Fatalf("expected sales item is_dp true")
// 		}
// 		if s.CustomerID == nil {
// 			t.Fatalf("expected sales item customer_id not nil")
// 		}
// 	}

// 	// Verify product stock decremented
// 	var checkP1, checkP2 domain.Product
// 	database.First(&checkP1, prod1.ID)
// 	database.First(&checkP2, prod2.ID)
// 	if checkP1.Saldo != 9 {
// 		t.Fatalf("expected prod1 saldo 9, got %f", checkP1.Saldo)
// 	}
// 	if checkP2.Saldo != 9 {
// 		t.Fatalf("expected prod2 saldo 9, got %f", checkP2.Saldo)
// 	}

// 	// 3. Test Pelunasan Per-Nota
// 	pelunasanReq := domain.PelunasanRequest{
// 		PaymentMethod: "Transfer BCA",
// 		AmountPaid:    300000,
// 		IsDp:          false,
// 		Status:        "Lunas",
// 	}

// 	pelunasanResp, err := svc.PelunasanSales(resp.Invoice, pelunasanReq)
// 	if err != nil {
// 		t.Fatalf("PelunasanSales failed: %v", err)
// 	}

// 	if pelunasanResp.Status != "Lunas" {
// 		t.Fatalf("expected status Lunas after pelunasan, got %s", pelunasanResp.Status)
// 	}
// 	if pelunasanResp.IsDp {
// 		t.Fatalf("expected is_dp false after pelunasan")
// 	}

// 	// Verify all sales items under that invoice are updated to Lunas
// 	var updatedSales []domain.Sales
// 	database.Where("invoice = ?", resp.Invoice).Find(&updatedSales)
// 	for _, s := range updatedSales {
// 		if s.Status != "Lunas" {
// 			t.Fatalf("expected updated sales item status Lunas, got %s", s.Status)
// 		}
// 		if s.IsDp {
// 			t.Fatalf("expected updated sales item is_dp false")
// 		}
// 		if s.PaymentMethod != "Transfer BCA" {
// 			t.Fatalf("expected payment method 'Transfer BCA', got %s", s.PaymentMethod)
// 		}
// 	}

// 	// 4. Test GetAllSales returns grouped by invoice
// 	groupedSales, err := svc.GetAllSales(domain.SalesFilter{})
// 	if err != nil {
// 		t.Fatalf("GetAllSales failed: %v", err)
// 	}

// 	foundGroup := false
// 	for _, grp := range groupedSales {
// 		if grp.Invoice == resp.Invoice {
// 			foundGroup = true
// 			if len(grp.Items) != 2 {
// 				t.Fatalf("expected 2 items in grouped invoice, got %d", len(grp.Items))
// 			}
// 			if grp.TotalNetto != 300000 {
// 				t.Fatalf("expected total netto 300000, got %f", grp.TotalNetto)
// 			}
// 			if grp.TotalQty != 2 {
// 				t.Fatalf("expected total qty 2, got %f", grp.TotalQty)
// 			}
// 			if grp.Status != "Lunas" {
// 				t.Fatalf("expected grouped status Lunas, got %s", grp.Status)
// 			}
// 			break
// 		}
// 	}
// 	if !foundGroup {
// 		t.Fatalf("expected invoice %s to be found in GetAllSales result", resp.Invoice)
// 	}

// 	// 5. Test duplicate Pelunasan fails
// 	_, err = svc.PelunasanSales(resp.Invoice, pelunasanReq)
// 	if err == nil {
// 		t.Fatalf("expected error when trying to settle already settled invoice")
// 	}

// 	// Clean up test data
// 	database.Unscoped().Where("sales_invoice = ?", resp.Invoice).Delete(&domain.PiutangDagang{})
// 	database.Unscoped().Where("invoice = ?", resp.Invoice).Delete(&domain.Sales{})
// 	if salesList[0].CustomerID != nil {
// 		database.Unscoped().Delete(&domain.Customer{}, *salesList[0].CustomerID)
// 	}
// }
