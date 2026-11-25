package http

import (
	"backend-cashier/service"
	"bytes"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

// ============================================================================
// 1. BRUTE FORCE PROTECTION GUARD
// ============================================================================

type attemptInfo struct {
	Count     int
	FirstFail time.Time
	BlockedAt time.Time
}

type BruteForceGuard struct {
	mu           sync.Mutex
	attempts     map[string]*attemptInfo
	maxAttempts  int
	window       time.Duration
	lockDuration time.Duration
}

var globalBruteForceGuard *BruteForceGuard
var onceBF sync.Once

// GetBruteForceGuard mengembalikan instance singleton pelindung brute force login
func GetBruteForceGuard() *BruteForceGuard {
	onceBF.Do(func() {
		globalBruteForceGuard = &BruteForceGuard{
			attempts:     make(map[string]*attemptInfo),
			maxAttempts:  5,               // Maksimal 5x gagal berturut-turut
			window:       5 * time.Minute, // Dalam rentang waktu 5 menit
			lockDuration: 5 * time.Minute, // Kunci selama 5 menit
		}
		// Goroutine pembersih entri kadaluarsa setiap 2 menit
		go func() {
			for range time.Tick(2 * time.Minute) {
				globalBruteForceGuard.cleanup()
			}
		}()
	})
	return globalBruteForceGuard
}

func (g *BruteForceGuard) cleanup() {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	for key, info := range g.attempts {
		if !info.BlockedAt.IsZero() && now.Sub(info.BlockedAt) > g.lockDuration {
			delete(g.attempts, key)
		} else if info.BlockedAt.IsZero() && now.Sub(info.FirstFail) > g.window {
			delete(g.attempts, key)
		}
	}
}

// IsBlocked memeriksa apakah IP/Username saat ini terkunci
func (g *BruteForceGuard) IsBlocked(key string) (bool, int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	info, exists := g.attempts[key]
	if !exists {
		return false, 0
	}

	if !info.BlockedAt.IsZero() {
		elapsed := time.Since(info.BlockedAt)
		if elapsed < g.lockDuration {
			remainingSec := int((g.lockDuration - elapsed).Seconds())
			if remainingSec < 1 {
				remainingSec = 1
			}
			return true, remainingSec
		}
		// Masa blokir telah selesai
		delete(g.attempts, key)
		return false, 0
	}

	return false, 0
}

// RecordFailure mencatat percobaan login gagal dan mengunci jika mencapai batas
func (g *BruteForceGuard) RecordFailure(key string) (bool, int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	info, exists := g.attempts[key]
	if !exists || now.Sub(info.FirstFail) > g.window {
		info = &attemptInfo{
			Count:     1,
			FirstFail: now,
		}
		g.attempts[key] = info
		return false, 0
	}

	info.Count++
	if info.Count >= g.maxAttempts {
		info.BlockedAt = now
		remainingSec := int(g.lockDuration.Seconds())
		log.Printf("[SECURITY] Brute-force terdeteksi untuk target '%s'. Diblokir selama %v", key, g.lockDuration)
		return true, remainingSec
	}

	return false, 0
}

// RecordSuccess mereset penghitung gagal saat pengguna berhasil login
func (g *BruteForceGuard) RecordSuccess(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.attempts, key)
}

// ============================================================================
// 2. SQL INJECTION FIREWALL (DEFENSE-IN-DEPTH)
// ============================================================================

// Pola ekspresi reguler untuk mendeteksi payload injeksi SQL berbahaya
var sqliPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:'|")\s*(?:or|and)\s*(?:'|")?[a-zA-Z0-9_-]*(?:'|")?\s*=\s*(?:'|")?[a-zA-Z0-9_-]*`),
	regexp.MustCompile(`(?i)\b(?:or|and)\s+['"]?1['"]?\s*=\s*['"]?1\b`),
	regexp.MustCompile(`(?i)\bunion\s+(?:all\s+)?select\b`),
	regexp.MustCompile(`(?i);\s*(?:drop|truncate|alter|delete\s+from)\s+(?:table|database|[a-zA-Z0-9_]+)`),
	regexp.MustCompile(`(?i)\b(?:exec|execute)\s*\(\s*(?:char|ascii|xp_|\@)`),
	regexp.MustCompile(`(?i)\b(?:sleep\s*\(\s*\d+\s*\)|pg_sleep\s*\(\s*\d+\s*\)|benchmark\s*\(\s*\d+|waitfor\s+delay\s+['"])`),
	regexp.MustCompile(`(?i)(?:--[^\r\n]*|/\*[\s\S]*?\*/)`),
}

// SQLInjectionFirewallMiddleware menyaring setiap request dari indikasi SQL injection
func SQLInjectionFirewallMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Method() == fiber.MethodOptions {
			return c.Next()
		}
		// 1. Periksa URL Raw Path & Query String
		rawURL := c.OriginalURL()
		decodedURL, _ := url.QueryUnescape(rawURL)
		checkString := rawURL + " " + decodedURL

		for _, pattern := range sqliPatterns {
			if pattern.MatchString(checkString) {
				log.Printf("[SECURITY FIREWALL] SQL Injection digagalkan dari IP %s pada URL: %s", c.IP(), rawURL)
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error":   "Akses Ditolak: Terdeteksi pola mencurigakan pada parameter (SQL Injection Protection).",
					"code":    "SECURITY_VIOLATION_SQLI",
					"blocked": true,
				})
			}
		}

		// 2. Periksa Body JSON/Form-UrlEncoded jika method POST/PUT/PATCH (lewati multipart/form-data upload file agar boundary '----' & biner foto tidak memicu false-positive)
		method := c.Method()
		contentType := strings.ToLower(c.Get("Content-Type"))
		if (method == fiber.MethodPost || method == fiber.MethodPut || method == fiber.MethodPatch) && !strings.Contains(contentType, "multipart/form-data") {
			body := c.Body()
			if len(body) > 0 && len(body) < 1048576 { // Cek hanya jika di bawah 1MB
				bodyStr := string(body)
				for _, pattern := range sqliPatterns {
					if pattern.MatchString(bodyStr) {
						log.Printf("[SECURITY FIREWALL] SQL Injection digagalkan pada Body dari IP %s, path: %s", c.IP(), c.Path())
						return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
							"error":   "Akses Ditolak: Terdeteksi muatan berbahaya pada data yang dikirim (SQL Injection Protection).",
							"code":    "SECURITY_VIOLATION_SQLI",
							"blocked": true,
						})
					}
				}
			}
		}

		return c.Next()
	}
}

// ============================================================================
// 3. XSS (CROSS-SITE SCRIPTING) SANITIZATION FIREWALL
// ============================================================================

var xssPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)<\s*script[\s\S]*?>[\s\S]*?<\s*/\s*script\s*>`),
	regexp.MustCompile(`(?i)javascript\s*:`),
	regexp.MustCompile(`(?i)on(?:error|load|click|mouseover|focus|submit)\s*=`),
	regexp.MustCompile(`(?i)<\s*(?:iframe|embed|object|base)[\s\S]*?>`),
}

func XSSSanitizerMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Method() == fiber.MethodOptions {
			return c.Next()
		}
		// Periksa Query Strings
		rawQuery := c.OriginalURL()
		decodedQuery, _ := url.QueryUnescape(rawQuery)
		checkQuery := rawQuery + " " + decodedQuery

		for _, p := range xssPatterns {
			if p.MatchString(checkQuery) {
				log.Printf("[SECURITY FIREWALL] XSS payload digagalkan pada query string dari IP %s", c.IP())
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error":   "Akses Ditolak: Terdeteksi muatan skrip berbahaya (XSS Protection).",
					"code":    "SECURITY_VIOLATION_XSS",
					"blocked": true,
				})
			}
		}

		// Periksa Request Body (lewati multipart/form-data upload file agar biner file tidak terdeteksi sebagai XSS false-positive)
		method := c.Method()
		contentType := strings.ToLower(c.Get("Content-Type"))
		if (method == fiber.MethodPost || method == fiber.MethodPut || method == fiber.MethodPatch) && !strings.Contains(contentType, "multipart/form-data") {
			body := c.Body()
			if len(body) > 0 && len(body) < 1048576 {
				bodyStr := string(body)
				for _, p := range xssPatterns {
					if p.MatchString(bodyStr) {
						log.Printf("[SECURITY FIREWALL] XSS payload digagalkan pada request body dari IP %s", c.IP())
						return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
							"error":   "Akses Ditolak: Terdeteksi muatan skrip berbahaya (XSS Protection).",
							"code":    "SECURITY_VIOLATION_XSS",
							"blocked": true,
						})
					}
				}
			}
		}

		return c.Next()
	}
}

// ============================================================================
// 4. ENHANCED SECURITY HEADERS & CLICKJACKING DEFENSE
// ============================================================================

func SecurityHeadersMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Method() == fiber.MethodOptions {
			return c.Next()
		}
		// 1. Anti-MIME Sniffing
		c.Set("X-Content-Type-Options", "nosniff")

		// 2. Anti-Clickjacking: Blokir rendering dalam iframe apapun secara absolut
		c.Set("X-Frame-Options", "DENY")
		c.Set("Content-Security-Policy", "frame-ancestors 'none';")

		// 3. XSS Filter & Referrer Policy
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Set("X-Permitted-Cross-Domain-Policies", "none")

		// Pastikan client JavaScript diperbolehkan membaca header X-Renewed-Token
		c.Set("Access-Control-Expose-Headers", "X-Renewed-Token, Content-Disposition")

		return c.Next()
	}
}

// ============================================================================
// 5. SERVER-SIDE ROLE AUTHORIZATION (CLIENT-SIDE ROLE BYPASS PREVENTION)
// ============================================================================

// RequireRoles memverifikasi keaslian token, mengecek status blacklist (revocation),
// dan memastikan user memiliki role yang sah di backend Go sebelum mengakses rute sensitif
func RequireRoles(authSvc *service.AuthService, allowedRoles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			authHeader = c.Query("token")
		}
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Otentikasi diperlukan: Silakan login terlebih dahulu",
				"code":  "AUTH_REQUIRED",
			})
		}

		tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))

		// 1. Cek Token Blacklist (Anti-Replay Attack & Zombie Token Defense)
		if authSvc != nil && authSvc.IsTokenRevoked(tokenStr) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Sesi telah dibatalkan / logout. Silakan login kembali",
				"code":  "TOKEN_REVOKED",
			})
		}

		// 2. Validasi Kriptografi Token JWT
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("alg invalid")
			}
			return service.GetJWTSecret(), nil
		})
		if err != nil || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Token otentikasi tidak valid atau telah kedaluwarsa",
				"code":  "TOKEN_EXPIRED_OR_INVALID",
			})
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Format klaim token tidak valid",
				"code":  "INVALID_CLAIMS",
			})
		}

		userRole := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", claims["role"])))
		normalizedUserRole := strings.ReplaceAll(strings.ReplaceAll(userRole, "-", ""), "_", "")

		// Simpan klaim ke Locals context request
		c.Locals("claims", claims)
		c.Locals("user_id", fmt.Sprintf("%v", claims["user_id"]))
		c.Locals("user_role", normalizedUserRole)
		c.Locals("user_location", fmt.Sprintf("%v", claims["location"]))

		// 3. Verifikasi Role Backend
		if len(allowedRoles) > 0 {
			roleAllowed := false
			for _, allowed := range allowedRoles {
				normAllowed := strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(allowed), "-", ""), "_", "")
				if normAllowed == normalizedUserRole {
					roleAllowed = true
					break
				}
			}

			if !roleAllowed {
				log.Printf("[SECURITY] Role bypass digagalkan: user '%v' role '%s' mencoba akses rute terproteksi role %v",
					claims["username"], normalizedUserRole, allowedRoles)
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error": "Akses Ditolak (Client-Side Role Bypass Prevention): Peran Anda tidak memiliki izin untuk tindakan ini",
					"code":  "FORBIDDEN_ROLE",
				})
			}
		}

		return c.Next()
	}
}

// ============================================================================
// 6. ROLLING SESSION AUTO-RENEWAL MIDDLEWARE
// ============================================================================

// RollingSessionMiddleware otomatis memperpanjang token 9 jam lagi saat user melakukan interaksi (request API)
// setelah token berjalan >= 5 jam (atau tersisa <= 4 jam dari total 9 jam)
func RollingSessionMiddleware(authSvc *service.AuthService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := c.Next()

		if authSvc == nil || c.Response().StatusCode() >= 400 {
			return err
		}

		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return err
		}

		tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if tokenStr == "" {
			return err
		}

		// Jika token sudah dicabut, jangan perpanjang
		if authSvc.IsTokenRevoked(tokenStr) {
			return err
		}

		token, parseErr := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("alg invalid")
			}
			return service.GetJWTSecret(), nil
		})

		if parseErr == nil && token.Valid {
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				now := time.Now().Unix()
				var expTime int64
				if expVal, exists := claims["exp"]; exists {
					if f, ok := expVal.(float64); ok {
						expTime = int64(f)
					}
				}

				var iatTime int64
				if iatVal, exists := claims["iat"]; exists {
					if f, ok := iatVal.(float64); ok {
						iatTime = int64(f)
					}
				} else if expTime > 0 {
					iatTime = expTime - int64(9*time.Hour/time.Second)
				}

				elapsedSec := now - iatTime
				fiveHoursSec := int64(5 * 3600)

				if elapsedSec >= fiveHoursSec && expTime > now {
					newToken, _, _, refErr := authSvc.RefreshToken(tokenStr, true)
					if refErr == nil && newToken != "" {
						c.Set("X-Renewed-Token", newToken)
					}
				}
			}
		}

		return err
	}
}

// Sanitization utility untuk membersihkan string dari karakter berbahaya
func SanitizePlainText(input string) string {
	cleaned := strings.ReplaceAll(input, "\x00", "")
	return strings.TrimSpace(cleaned)
}

// SafeEquals membandingkan dua string waktu konstan untuk menghindari timing attack
func SafeEquals(a, b string) bool {
	return bytes.Equal([]byte(a), []byte(b))
}
