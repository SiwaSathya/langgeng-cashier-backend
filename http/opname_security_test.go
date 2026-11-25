package http_test

import (
	"backend-cashier/db"
	localHttp "backend-cashier/http"
	"backend-cashier/service"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/joho/godotenv"
)

func makeTestJWT(userID, role string) string {
	claims := jwt.MapClaims{
		"user_id": userID,
		"role":    role,
		"exp":     time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString(service.GetJWTSecret())
	return signed
}

func TestOpnameSuperAdminAuthorizationHTTP(t *testing.T) {
	_ = godotenv.Load("../.env")
	db.New()
	if db.Postgres.DB == nil {
		t.Skip("Database not available")
	}
	db.RegisterTableToMigrate(db.Postgres.DB)

	storageSvc := service.NewStorageService()
	sseSvc := service.NewSSEService()
	opnameSvc := service.NewOpnameService(db.Postgres.DB, storageSvc, sseSvc)
	handler := localHttp.NewOpnameHandler(opnameSvc, storageSvc)

	app := fiber.New()
	handler.RegisterRoutes(app)

	blockedRoles := []string{"", "kasir", "super-kasir", "admin", "akuntan"}
	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"POST", "/api/stock-opnames", `{"location":"Toko Utama"}`},
		{"GET", "/api/stock-opnames", ""},
		{"GET", "/api/stock-opnames/sync-global-preview", ""},
		{"POST", "/api/stock-opnames/sync-global-stock", `{}`},
		{"GET", "/api/stock-opnames/1", ""},
		{"PUT", "/api/stock-opnames/1/items/1", `{"physical_qty":1}`},
		{"POST", "/api/stock-opnames/1/upload-photo", ""},
		{"POST", "/api/stock-opnames/1/submit", `{}`},
		{"POST", "/api/stock-opnames/1/approve", `{}`},
		{"POST", "/api/stock-opnames/1/cancel", `{}`},
	}

	for _, role := range blockedRoles {
		for _, ep := range endpoints {
			var reqBody *strings.Reader
			if ep.body != "" {
				reqBody = strings.NewReader(ep.body)
			} else {
				reqBody = strings.NewReader("")
			}
			req := httptest.NewRequest(ep.method, ep.path, reqBody)
			if ep.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			if role != "" {
				req.Header.Set("Authorization", "Bearer "+makeTestJWT("TEST-"+role, role))
			}

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("request %s %s failed: %v", ep.method, ep.path, err)
			}
			if resp.StatusCode != fiber.StatusForbidden {
				t.Errorf("role %q on %s %s expected HTTP 403 Forbidden, got %d", role, ep.method, ep.path, resp.StatusCode)
			}
		}
	}

	// Super Admin harus diperbolehkan mengakses endpoint opname (tidak mendapat 403)
	superToken := makeTestJWT("TEST-SUPER", "superadmin")
	reqList := httptest.NewRequest("GET", "/api/stock-opnames", nil)
	reqList.Header.Set("Authorization", "Bearer "+superToken)
	respList, err := app.Test(reqList)
	if err != nil {
		t.Fatalf("superadmin GET /api/stock-opnames failed: %v", err)
	}
	if respList.StatusCode != fiber.StatusOK {
		t.Errorf("superadmin GET /api/stock-opnames expected 200, got %d", respList.StatusCode)
	}

	// Super Admin harus diperbolehkan mengakses endpoint preview sinkronisasi stok global
	reqPreview := httptest.NewRequest("GET", "/api/stock-opnames/sync-global-preview", nil)
	reqPreview.Header.Set("Authorization", "Bearer "+superToken)
	respPreview, err := app.Test(reqPreview)
	if err != nil {
		t.Fatalf("superadmin GET /api/stock-opnames/sync-global-preview failed: %v", err)
	}
	if respPreview.StatusCode != fiber.StatusOK {
		t.Errorf("superadmin GET /api/stock-opnames/sync-global-preview expected 200, got %d", respPreview.StatusCode)
	}
	bodyBytes, _ := io.ReadAll(respPreview.Body)
	var previewResp struct {
		Data struct {
			LocationStatuses []struct {
				Location string `json:"location"`
			} `json:"location_statuses"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &previewResp); err != nil {
		t.Fatalf("failed to parse sync-global-preview response: %v (body=%s)", err, string(bodyBytes))
	}
	if len(previewResp.Data.LocationStatuses) != 5 {
		t.Errorf("expected 5 official locations in sync-global-preview, got %d", len(previewResp.Data.LocationStatuses))
	}
}
