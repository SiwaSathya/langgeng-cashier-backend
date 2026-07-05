package domain

type User struct {
	ID       string `gorm:"primaryKey"`
	Name     string `gorm:"size:100"`
	Username string `gorm:"size:100;unique"`
	Password string `gorm:"size:100"`
	Location string `gorm:"size:100"`
}
