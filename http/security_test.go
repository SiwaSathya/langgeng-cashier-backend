package http_test

import (
	"backend-cashier/http"
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestSQLInjectionFirewall(t *testing.T) {
	app := fiber.New()
	app.Use(http.SQLInjectionFirewallMiddleware())

	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// 1. Request normal yang sah (Legitimate Queries)
	legitReqs := []string{
		"/test?q=Televisi+Samsung+43+Inch",
		"/test?category=elektronik&page=1&limit=20",
		"/test?notes=Pembayaran+lunas+oleh+pelanggan",
	}

	for _, u := range legitReqs {
		req := httptest.NewRequest("GET", u, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Gagal menjalankan request: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("Request sah '%s' seharusnya diterima (200), tapi mendapat status %d", u, resp.StatusCode)
		}
	}

	// 2. Request berbahaya yang mengandung SQL Injection
	maliciousQueries := []string{
		"/test?q=" + url.QueryEscape("' UNION SELECT 1,2,3,4--"),
		"/test?user=" + url.QueryEscape("admin' OR '1'='1"),
		"/test?filter=" + url.QueryEscape("1; DROP TABLE users;--"),
		"/test?id=" + url.QueryEscape("1 AND 1=1"),
		"/test?code=" + url.QueryEscape("1' OR ''='"),
		"/test?q=" + url.QueryEscape("1; DROP DATABASE cashier"),
	}

	for _, u := range maliciousQueries {
		req := httptest.NewRequest("GET", u, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Gagal menjalankan request: %v", err)
		}
		if resp.StatusCode != fiber.StatusForbidden {
			t.Fatalf("SQL Injection '%s' seharusnya diblokir (403), tapi mendapat status %d", u, resp.StatusCode)
		}
	}

	// 3. Body berbahaya via POST
	badBody := `{"username": "admin' OR '1'='1", "password": "xyz"}`
	reqPost := httptest.NewRequest("POST", "/test", strings.NewReader(badBody))
	reqPost.Header.Set("Content-Type", "application/json")
	respPost, err := app.Test(reqPost)
	if err != nil {
		t.Fatalf("Request body test error: %v", err)
	}
	if respPost.StatusCode != fiber.StatusForbidden {
		body, _ := io.ReadAll(respPost.Body)
		t.Fatalf("SQL Injection pada body JSON seharusnya diblokir (403), got %d, body: %s", respPost.StatusCode, string(body))
	}
}
func TestXSSSanitizerMiddleware(t *testing.T) {
	app := fiber.New()
	app.Use(http.XSSSanitizerMiddleware())

	app.Post("/submit", func(c *fiber.Ctx) error {
		return c.SendString("SUCCESS")
	})

	// 1. Request normal tanpa skrip
	normalBody := `{"name": "Budi Santoso", "address": "Jl. Gatot Subroto No. 5"}`
	req1 := httptest.NewRequest("POST", "/submit", strings.NewReader(normalBody))
	req1.Header.Set("Content-Type", "application/json")
	res1, _ := app.Test(req1)
	if res1.StatusCode != fiber.StatusOK {
		t.Fatalf("Payload normal harus diizinkan (200), got %d", res1.StatusCode)
	}

	// 2. Payload XSS dengan tag <script>
	xssBody1 := `{"name": "<script>alert(document.cookie)</script>"}`
	req2 := httptest.NewRequest("POST", "/submit", strings.NewReader(xssBody1))
	req2.Header.Set("Content-Type", "application/json")
	res2, _ := app.Test(req2)
	if res2.StatusCode != fiber.StatusForbidden {
		t.Fatalf("Payload XSS <script> harus ditolak (403), got %d", res2.StatusCode)
	}

	// 3. Payload XSS dengan event handler inline onerror=
	xssBody2 := `{"avatar": "<img src=x onerror=stealJwt()>"}`
	req3 := httptest.NewRequest("POST", "/submit", strings.NewReader(xssBody2))
	req3.Header.Set("Content-Type", "application/json")
	res3, _ := app.Test(req3)
	if res3.StatusCode != fiber.StatusForbidden {
		t.Fatalf("Payload XSS inline event handler harus ditolak (403), got %d", res3.StatusCode)
	}
}

func TestClickjackingDefenseHeaders(t *testing.T) {
	app := fiber.New()
	app.Use(http.SecurityHeadersMiddleware())

	app.Get("/page", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	req := httptest.NewRequest("GET", "/page", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	xfo := res.Header.Get("X-Frame-Options")
	if xfo != "DENY" {
		t.Fatalf("X-Frame-Options harus DENY untuk mencegah Clickjacking, got: %s", xfo)
	}

	csp := res.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("Content-Security-Policy harus mengandung frame-ancestors 'none', got: %s", csp)
	}
}

func TestMultipartUploadAllowedThroughSecurityFirewall(t *testing.T) {
	app := fiber.New()
	app.Use(http.SQLInjectionFirewallMiddleware())
	app.Use(http.XSSSanitizerMiddleware())

	app.Post("/upload", func(c *fiber.Ctx) error {
		return c.SendString("UPLOADED")
	})

	// Boundary multipart standar mengandung '------' yang sebelumnya memicu SQLi regex '--'
	multipartBody := "------WebKitFormBoundary7MA4YWxkTrZu0gW\r\n" +
		"Content-Disposition: form-data; name=\"photo\"; filename=\"opname.png\"\r\n" +
		"Content-Type: image/png\r\n\r\n" +
		"fake-png-binary--bytes<script>not-html</script>\r\n" +
		"------WebKitFormBoundary7MA4YWxkTrZu0gW--\r\n"

	req := httptest.NewRequest("POST", "/upload", strings.NewReader(multipartBody))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Multipart test error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Multipart upload tidak boleh diblokir SQLi/XSS middleware (expected 200), got %d: %s", resp.StatusCode, string(body))
	}
}
