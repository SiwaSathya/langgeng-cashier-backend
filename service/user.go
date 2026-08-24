package service

import (
	"backend-cashier/domain"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"gorm.io/gorm"
)

type AuthService struct {
	DB *gorm.DB
}

func NewAuthService(db *gorm.DB) *AuthService {
	svc := &AuthService{DB: db}
	svc.SeedDefaultUsers()
	return svc
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

// Login memvalidasi user berdasarkan password mentah dan menyertakan role dalam token
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

	// 3. Generate JWT Token jika login sukses
	claims := jwt.MapClaims{
		"user_id":  user.ID,
		"name":     user.Name,
		"username": user.Username,
		"location": user.Location,
		"role":     role,
		"exp":      time.Now().Add(time.Hour * 24).Unix(), // Token aktif 1 hari
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	secretKey := []byte("smartpos_secret_key")
	tokenString, err := token.SignedString(secretKey)
	if err != nil {
		return "", errors.New("gagal membuat token otentikasi")
	}

	return tokenString, nil
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
		// Validasi jika username diubah
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
