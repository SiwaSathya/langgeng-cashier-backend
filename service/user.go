package service

import (
	"backend-cashier/domain"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"gorm.io/gorm"
)

type AuthService struct {
	DB *gorm.DB
}

func NewAuthService(db *gorm.DB) *AuthService {
	return &AuthService{DB: db}
}

// Login memvalidasi user berdasarkan password mentah (tanpa hash)
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

	// 3. Generate JWT Token jika login sukses
	claims := jwt.MapClaims{
		"user_id":  user.ID,
		"name":     user.Name,
		"location": user.Location,
		"exp":      time.Now().Add(time.Hour * 24).Unix(), // Token aktif 1 hari
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Gunakan secret key bebas, sesuaikan dengan env aplikasi Anda
	secretKey := []byte("smartpos_secret_key")
	tokenString, err := token.SignedString(secretKey)
	if err != nil {
		return "", errors.New("gagal membuat token otentikasi")
	}

	return tokenString, nil
}
