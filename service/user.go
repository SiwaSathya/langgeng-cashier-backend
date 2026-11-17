package service

import (
	"backend-cashier/domain"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"gorm.io/gorm"
)

// In-memory cache untuk Token Blacklist agar verifikasi O(1) super cepat tanpa bottleneck database
var revokedTokensCache sync.Map

// GetJWTSecret mengembalikan secret key JWT dari environment variable atau default terpusat
func GetJWTSecret() []byte {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret != "" {
		return []byte(secret)
	}
	return []byte("smartpos_secret_key")
}

// HashToken menghitung nilai SHA-256 hex string dari token otentikasi
func HashToken(tokenStr string) string {
	cleanToken := strings.TrimSpace(strings.TrimPrefix(tokenStr, "Bearer "))
	h := sha256.Sum256([]byte(cleanToken))
	return hex.EncodeToString(h[:])
}

type AuthService struct {
	DB *gorm.DB
}

func NewAuthService(db *gorm.DB) *AuthService {
	svc := &AuthService{DB: db}
	svc.SeedDefaultUsers()

	// Background ticker pembersih token blacklist yang sudah kedaluwarsa secara alami setiap 1 jam
	go svc.startRevokedTokenCleanup(1 * time.Hour)

	return svc
}

func (s *AuthService) startRevokedTokenCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		// Hapus rekord token revoked dari database yang masa exp-nya sudah lewat
		_ = s.DB.Where("expires_at <= ?", now).Delete(&domain.RevokedToken{}).Error

		// Bersihkan in-memory cache
		revokedTokensCache.Range(func(key, value interface{}) bool {
			if expTime, ok := value.(time.Time); ok {
				if now.After(expTime) {
					revokedTokensCache.Delete(key)
				}
			}
			return true
		})
	}
}

// RevokeToken mendaftarkan token ke blacklist database dan in-memory cache seketika saat logout (Anti-Replay Attack)
func (s *AuthService) RevokeToken(tokenStr string, userID string) error {
	tokenStr = strings.TrimSpace(strings.TrimPrefix(tokenStr, "Bearer "))
	if tokenStr == "" {
		return nil
	}

	tokenHash := HashToken(tokenStr)
	now := time.Now()
	expiresAt := now.Add(9 * time.Hour)

	// Ambil waktu exp sebenarnya dari token
	token, _ := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return GetJWTSecret(), nil
	})
	if token != nil {
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if expVal, exists := claims["exp"]; exists {
				if f, ok := expVal.(float64); ok {
					expiresAt = time.Unix(int64(f), 0)
				}
			}
			if userID == "" {
				userID = fmt.Sprintf("%v", claims["user_id"])
			}
		}
	}

	// 1. Simpan ke in-memory cache seketika
	revokedTokensCache.Store(tokenHash, expiresAt)

	// 2. Simpan persistensi ke tabel PostgreSQL
	rec := domain.RevokedToken{
		TokenHash: tokenHash,
		UserID:    userID,
		ExpiresAt: expiresAt,
		RevokedAt: now,
	}
	return s.DB.Create(&rec).Error
}

// IsTokenRevoked memeriksa apakah token telah dicabut / logout (Blacklist Check)
func (s *AuthService) IsTokenRevoked(tokenStr string) bool {
	tokenStr = strings.TrimSpace(strings.TrimPrefix(tokenStr, "Bearer "))
	if tokenStr == "" {
		return true
	}

	tokenHash := HashToken(tokenStr)

	// 1. Cek in-memory cache (sangat cepat ~100 nanodetik)
	if expVal, ok := revokedTokensCache.Load(tokenHash); ok {
		if expTime, isTime := expVal.(time.Time); isTime {
			if time.Now().Before(expTime) {
				return true
			}
			revokedTokensCache.Delete(tokenHash)
		} else {
			return true
		}
	}

	// 2. Cek database jika belum ada di RAM (misal setelah server restart)
	var count int64
	if err := s.DB.Model(&domain.RevokedToken{}).Where("token_hash = ?", tokenHash).Count(&count).Error; err == nil && count > 0 {
		revokedTokensCache.Store(tokenHash, time.Now().Add(9*time.Hour))
		return true
	}

	return false
}

// SeedDefaultUsers memastikan user awal dengan role-role yang dibutuhkan selalu tersedia jika kosong
func (s *AuthService) SeedDefaultUsers() {
	var count int64
	s.DB.Model(&domain.User{}).Count(&count)
	if count == 0 {
		defaultUsers := []domain.User{
			{ID: "superadmin-1", Name: "Super Admin", Username: "superadmin", Password: "password123", Location: "Pusat", Role: "superadmin"},
			{ID: "admin-1", Name: "Admin Toko", Username: "admin", Password: "password123", Location: "Pusat", Role: "admin"},
			{ID: "kasir-1", Name: "Desak Kasir", Username: "kasir", Password: "password123", Location: "Pusat", Role: "kasir"},
			{ID: "superkasir-1", Name: "Super Kasir", Username: "superkasir", Password: "password123", Location: "Pusat", Role: "super-kasir"},
			{ID: "akuntan-1", Name: "Akuntan Keuangan", Username: "akuntan", Password: "password123", Location: "Pusat", Role: "akuntan"},
		}
		for _, u := range defaultUsers {
			s.DB.FirstOrCreate(&u, domain.User{Username: u.Username})
		}
	}
}

// Login memvalidasi user berdasarkan password mentah dan menyertakan role dalam token berdurasi 9 jam
func (s *AuthService) Login(username, password string) (string, error) {
	var user domain.User

	// 1. Cari user berdasarkan username
	err := s.DB.Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("username atau password salah")
		}
		return "", err
	}

	// 2. Bandingkan password langsung (Plain Text)
	if user.Password != password {
		return "", errors.New("username atau password salah")
	}

	role := user.Role
	if role == "" {
		role = "kasir"
	}

	// 3. Generate JWT Token jika login sukses dengan masa aktif 9 jam dan timestamp penerbitan (iat)
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id":  user.ID,
		"name":     user.Name,
		"username": user.Username,
		"location": user.Location,
		"role":     role,
		"iat":      now.Unix(),
		"nbf":      now.Unix(),
		"exp":      now.Add(time.Hour * 9).Unix(), // Token aktif 9 jam
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(GetJWTSecret())
	if err != nil {
		return "", errors.New("gagal membuat token otentikasi")
	}

	return tokenString, nil
}

// RefreshToken memperpanjang masa aktif token menjadi 9 jam lagi jika token masih valid dan telah berjalan >= 5 jam (atau dipaksa via force=true)
func (s *AuthService) RefreshToken(tokenStr string, force bool) (string, *domain.User, int64, error) {
	tokenStr = strings.TrimSpace(strings.TrimPrefix(tokenStr, "Bearer "))
	if tokenStr == "" {
		return "", nil, 0, errors.New("token otentikasi tidak ditemukan")
	}

	// Cegah Zombie Token / Replay Attack: Tolak jika token ada di daftar blacklist
	if s.IsTokenRevoked(tokenStr) {
		return "", nil, 0, errors.New("token telah dibatalkan / logout. Silakan login kembali")
	}

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("metode signing token tidak valid: %v", t.Header["alg"])
		}
		return GetJWTSecret(), nil
	})
	if err != nil || !token.Valid {
		return "", nil, 0, errors.New("token otentikasi tidak valid atau telah kedaluwarsa")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", nil, 0, errors.New("format klaim token tidak valid")
	}

	userID := fmt.Sprintf("%v", claims["user_id"])
	if userID == "" || userID == "<nil>" {
		return "", nil, 0, errors.New("identitas user tidak valid dalam token")
	}

	// Cek apakah user masih ada dan aktif di database
	var user domain.User
	if err := s.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		return "", nil, 0, errors.New("user tidak ditemukan atau telah dinonaktifkan")
	}
	user.Password = "" // Cegah information leakage

	now := time.Now()
	var expTime int64
	if expVal, exists := claims["exp"]; exists {
		if f, ok := expVal.(float64); ok {
			expTime = int64(f)
		}
	}

	// Hitung waktu terbit (iat)
	var iatTime int64
	if iatVal, exists := claims["iat"]; exists {
		if f, ok := iatVal.(float64); ok {
			iatTime = int64(f)
		}
	} else if expTime > 0 {
		iatTime = expTime - int64(9*time.Hour/time.Second)
	} else {
		iatTime = now.Add(-6 * time.Hour).Unix()
	}

	elapsedSec := now.Unix() - iatTime
	remainingSec := expTime - now.Unix()
	fiveHoursSec := int64(5 * 3600) // 5 jam dalam detik = 18.000 detik

	// Jika belum mencapai 5 jam berjalan dan tidak ada paksaan (force), kembalikan token lama yang masih berlaku
	if !force && elapsedSec < fiveHoursSec && remainingSec > 0 {
		return tokenStr, &user, remainingSec, nil
	}

	// Terbitkan token baru dengan masa aktif 9 jam penuh
	role := user.Role
	if role == "" {
		role = "kasir"
	}

	newClaims := jwt.MapClaims{
		"user_id":  user.ID,
		"name":     user.Name,
		"username": user.Username,
		"location": user.Location,
		"role":     role,
		"iat":      now.Unix(),
		"nbf":      now.Unix(),
		"exp":      now.Add(time.Hour * 9).Unix(), // Perpanjangan 9 jam
	}

	newToken := jwt.NewWithClaims(jwt.SigningMethodHS256, newClaims)
	newTokenString, err := newToken.SignedString(GetJWTSecret())
	if err != nil {
		return "", nil, 0, errors.New("gagal menerbitkan perpanjangan token")
	}

	return newTokenString, &user, int64(9 * 3600), nil
}

// VerifyToken memvalidasi keabsahan token dan memeriksa apakah sudah memenuhi syarat perpanjangan (berjalan >= 5 jam)
func (s *AuthService) VerifyToken(tokenStr string) (jwt.MapClaims, *domain.User, bool, error) {
	tokenStr = strings.TrimSpace(strings.TrimPrefix(tokenStr, "Bearer "))
	if tokenStr == "" {
		return nil, nil, false, errors.New("token tidak ditemukan")
	}

	// Cegah Zombie Token / Replay Attack
	if s.IsTokenRevoked(tokenStr) {
		return nil, nil, false, errors.New("token telah dibatalkan / logout")
	}

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("metode signing token tidak valid: %v", t.Header["alg"])
		}
		return GetJWTSecret(), nil
	})
	if err != nil || !token.Valid {
		return nil, nil, false, errors.New("token otentikasi tidak valid atau telah kedaluwarsa")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, nil, false, errors.New("klaim token tidak valid")
	}

	userID := fmt.Sprintf("%v", claims["user_id"])
	var user domain.User
	if err := s.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, nil, false, errors.New("user tidak ditemukan")
	}
	user.Password = "" // Cegah information leakage

	now := time.Now()
	var iatTime int64
	if iatVal, exists := claims["iat"]; exists {
		if f, ok := iatVal.(float64); ok {
			iatTime = int64(f)
		}
	} else if expVal, exists := claims["exp"]; exists {
		if f, ok := expVal.(float64); ok {
			iatTime = int64(f) - int64(9*time.Hour/time.Second)
		}
	}

	canRefresh := (now.Unix() - iatTime) >= int64(5*3600)
	return claims, &user, canRefresh, nil
}

func (s *AuthService) CreateUser(req domain.UserRequest) (*domain.UserResponse, error) {
	if req.Username == "" || req.Password == "" || req.Name == "" {
		return nil, errors.New("nama, username, dan password wajib diisi")
	}

	// Cek apakah username sudah ada
	var existing domain.User
	if err := s.DB.Where("username = ?", req.Username).First(&existing).Error; err == nil {
		return nil, errors.New("username sudah digunakan, silakan pilih username lain")
	}

	id := req.ID
	if id == "" {
		id = fmt.Sprintf("USER-%d", time.Now().UnixNano()%100000000)
	}

	role := req.Role
	if role == "" {
		role = "kasir"
	}

	user := domain.User{
		ID:       id,
		Name:     req.Name,
		Username: req.Username,
		Password: req.Password,
		Location: req.Location,
		Role:     role,
	}

	if err := s.DB.Create(&user).Error; err != nil {
		return nil, err
	}

	return &domain.UserResponse{
		ID:       user.ID,
		Name:     user.Name,
		Username: user.Username,
		Location: user.Location,
		Role:     user.Role,
	}, nil
}

func (s *AuthService) GetAllUsers() ([]domain.UserResponse, error) {
	var users []domain.User
	if err := s.DB.Order("name asc").Find(&users).Error; err != nil {
		return nil, err
	}

	var results []domain.UserResponse
	for _, u := range users {
		role := u.Role
		if role == "" {
			role = "kasir"
		}
		results = append(results, domain.UserResponse{
			ID:       u.ID,
			Name:     u.Name,
			Username: u.Username,
			Location: u.Location,
			Role:     role,
		})
	}
	return results, nil
}

func (s *AuthService) GetUserByID(id string) (*domain.UserResponse, error) {
	var user domain.User
	if err := s.DB.First(&user, "id = ?", id).Error; err != nil {
		return nil, errors.New("user tidak ditemukan")
	}

	return &domain.UserResponse{
		ID:       user.ID,
		Name:     user.Name,
		Username: user.Username,
		Location: user.Location,
		Role:     user.Role,
	}, nil
}

func (s *AuthService) UpdateUser(id string, req domain.UserRequest) (*domain.UserResponse, error) {
	var user domain.User
	if err := s.DB.First(&user, "id = ?", id).Error; err != nil {
		return nil, errors.New("user tidak ditemukan")
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Username != "" {
		if req.Username != user.Username {
			var existing domain.User
			if err := s.DB.Where("username = ? AND id != ?", req.Username, id).First(&existing).Error; err == nil {
				return nil, errors.New("username sudah digunakan oleh akun lain")
			}
		}
		updates["username"] = req.Username
	}
	if req.Password != "" {
		updates["password"] = req.Password
	}
	if req.Location != "" {
		updates["location"] = req.Location
	}
	if req.Role != "" {
		updates["role"] = req.Role
	}

	if err := s.DB.Model(&user).Updates(updates).Error; err != nil {
		return nil, err
	}

	s.DB.First(&user, "id = ?", id)
	return &domain.UserResponse{
		ID:       user.ID,
		Name:     user.Name,
		Username: user.Username,
		Location: user.Location,
		Role:     user.Role,
	}, nil
}

func (s *AuthService) DeleteUser(id string) error {
	var user domain.User
	if err := s.DB.First(&user, "id = ?", id).Error; err != nil {
		return errors.New("user tidak ditemukan")
	}
	return s.DB.Delete(&user).Error
}

func (s *AuthService) GetUserByUsername(username string) (*domain.User, error) {
	var user domain.User
	if err := s.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	user.Password = "" // Sanitasi agar password hash tidak bocor ke tab Network / DevTools
	return &user, nil
}
