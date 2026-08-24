package domain

type User struct {
	ID       string `gorm:"primaryKey" json:"id"`
	Name     string `gorm:"size:100" json:"name"`
	Username string `gorm:"size:100;unique" json:"username"`
	Password string `gorm:"size:100" json:"password,omitempty"`
	Location string `gorm:"size:100" json:"location"`
	Role     string `gorm:"size:50" json:"role"` // admin, kasir, super-kasir, akuntan, superadmin
}

type UserRequest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
	Location string `json:"location"`
	Role     string `json:"role"`
}

type UserResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Location string `json:"location"`
	Role     string `json:"role"`
}
