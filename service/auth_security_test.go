package service_test

import (
	"backend-cashier/db"
	"backend-cashier/domain"
	"backend-cashier/http"
	"backend-cashier/service"
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

func TestAuthNineHourTokenAndRollingRefresh(t *testing.T) {
	setupFeatureTestDB(t)
	database := db.Postgres.DB
	authSvc := service.NewAuthService(database)

	// Buat user pengujian
	uname := fmt.Sprintf("sec_user_%d", time.Now().UnixNano()%1000000)
	pwd := "SecP@ssw0rd123"
	_, err := authSvc.CreateUser(domain.UserRequest{
		Name:     "Security Tester",
		Username: uname,
		Password: pwd,
		Role:     "kasir",
		Location: "Toko Utama",
	})
	if err != nil {
		t.Fatalf("Gagal membuat user uji: %v", err)
	}

	// 1. Uji Login dan periksa masa aktif token 9 jam
	tokenStr, err := authSvc.Login(uname, pwd)
	if err != nil {
		t.Fatalf("Login gagal: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("Token login kosong")
	}

	// Parse token dan verifikasi klaim exp dan iat
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return service.GetJWTSecret(), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("Token tidak valid: %v", err)
	}

	claims := token.Claims.(jwt.MapClaims)
	expFloat := claims["exp"].(float64)
	iatFloat := claims["iat"].(float64)

	diffSec := int64(expFloat - iatFloat)
	expectedSec := int64(9 * 3600) // 9 jam = 32400 detik
	if diffSec != expectedSec {
		t.Fatalf("Masa aktif token tidak 9 jam: diharapkan %d detik, didapatkan %d detik", expectedSec, diffSec)
	}

	// 2. Uji RefreshToken saat token baru terbit (< 5 jam) tanpa force -> harus mengembalikan token yang sama
	refreshed, _, remaining, err := authSvc.RefreshToken(tokenStr, false)
	if err != nil {
		t.Fatalf("RefreshToken gagal: %v", err)
	}
	if refreshed != tokenStr {
		t.Fatalf("Token seharusnya tidak diubah jika belum mencapai 5 jam berjalan: got %s, want %s", refreshed, tokenStr)
	}
	if remaining <= 0 {
		t.Fatalf("Remaining seconds harus positif, got %d", remaining)
	}

	// 3. Uji RefreshToken dengan force -> harus menerbitkan token baru berdurasi 9 jam
	time.Sleep(10 * time.Millisecond) // Pastikan timestamp berbeda
	_newTokenStr, _, newRemaining, err := authSvc.RefreshToken(tokenStr, true)
	if err != nil {
		t.Fatalf("Force RefreshToken gagal: %v", err)
	}
	if _newTokenStr == "" || newRemaining != expectedSec {
		t.Fatalf("Perpanjangan token harus 9 jam penuh (%d detik), got %d detik", expectedSec, newRemaining)
	}

	// 4. Uji simulasi token yang sudah berjalan 5 jam (iat dimundurkan 5.5 jam)
	fiveHoursAgo := time.Now().Add(-5*time.Hour - 30*time.Minute)
	simClaims := jwt.MapClaims{
		"user_id":  claims["user_id"],
		"name":     claims["name"],
		"username": claims["username"],
		"role":     claims["role"],
		"iat":      fiveHoursAgo.Unix(),
		"exp":      fiveHoursAgo.Add(9 * time.Hour).Unix(),
	}
	simToken := jwt.NewWithClaims(jwt.SigningMethodHS256, simClaims)
	simTokenStr, _ := simToken.SignedString(service.GetJWTSecret())

	autoRenewed, _, autoRemain, err := authSvc.RefreshToken(simTokenStr, false)
	if err != nil {
		t.Fatalf("Auto RefreshToken setelah 5 jam gagal: %v", err)
	}
	if autoRenewed == simTokenStr {
		t.Fatal("Token yang sudah berjalan >= 5 jam seharusnya otomatis diperpanjang saat user berinteraksi")
	}
	if autoRemain != expectedSec {
		t.Fatalf("Token baru hasil perpanjangan harus 9 jam (%d detik), got %d detik", expectedSec, autoRemain)
	}
}

func TestBruteForceProtectionGuard(t *testing.T) {
	guard := http.GetBruteForceGuard()
	testTarget := fmt.Sprintf("test_ip_%d:user", time.Now().UnixNano())

	// Awal: target belum terblokir
	blocked, _ := guard.IsBlocked(testTarget)
	if blocked {
		t.Fatal("Target awal seharusnya tidak terblokir")
	}

	// Catat 4 kali kegagalan berturut-turut (belum mencapai ambang 5)
	for i := 1; i <= 4; i++ {
		isBlocked, _ := guard.RecordFailure(testTarget)
		if isBlocked {
			t.Fatalf("Percobaan gagal ke-%d seharusnya belum memblokir target", i)
		}
	}

	// Percobaan gagal ke-5 -> harus terblokir
	isBlocked, waitSec := guard.RecordFailure(testTarget)
	if !isBlocked {
		t.Fatal("Percobaan gagal ke-5 harus memblokir target")
	}
	if waitSec <= 0 {
		t.Fatalf("Sisa waktu blokir harus lebih dari 0 detik, got %d", waitSec)
	}

	// Periksa status pemblokiran aktif
	checkBlocked, remaining := guard.IsBlocked(testTarget)
	if !checkBlocked || remaining <= 0 {
		t.Fatalf("Target seharusnya masih dalam kondisi terblokir: blocked=%v, remaining=%d", checkBlocked, remaining)
	}

	// Reset via RecordSuccess
	guard.RecordSuccess(testTarget)
	afterSuccess, _ := guard.IsBlocked(testTarget)
	if afterSuccess {
		t.Fatal("Setelah login berhasil (RecordSuccess), pemblokiran harus direset")
	}
}
