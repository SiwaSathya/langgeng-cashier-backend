package domain

import (
	"time"

	"gorm.io/gorm"
)

type Attendance struct {
	gorm.Model
	UserID   string     `gorm:"size:100;index" json:"user_id"`
	User     User       `gorm:"foreignKey:UserID" json:"user"`
	Date     time.Time  `gorm:"index" json:"date"`
	Shift    uint       `json:"shift"` // 1, 2, 3
	Status   string     `gorm:"size:50" json:"status"` // Pagi, Siang, Libur, Lembur, Bantu, Sakit, Dispensasi
	CheckIn  *time.Time `json:"check_in"`
	CheckOut *time.Time `json:"check_out"`
	Notes    string     `gorm:"type:text" json:"notes"`
}

type AttendanceRequest struct {
	UserID   string `json:"user_id"`
	Date     string `json:"date"` // YYYY-MM-DD
	Shift    uint   `json:"shift"`
	Status   string `json:"status"` // Pagi, Siang, Libur, Lembur, Bantu, Sakit, Dispensasi
	CheckIn  string `json:"check_in"`
	CheckOut string `json:"check_out"`
	Notes    string `json:"notes"`
}

type AttendanceFilter struct {
	Date      string `query:"date"`
	StartDate string `query:"start_date"`
	EndDate   string `query:"end_date"`
	UserID    string `query:"user_id"`
	Shift     uint   `query:"shift"`
	Status    string `query:"status"`
	Search    string `query:"search"`
	Page      int    `query:"page"`
	Limit     int    `query:"limit"`
}

type AttendanceSummary struct {
	Pagi       int64 `json:"pagi"`
	Siang      int64 `json:"siang"`
	Libur      int64 `json:"libur"`
	Lembur     int64 `json:"lembur"`
	Bantu      int64 `json:"bantu"`
	Sakit      int64 `json:"sakit"`
	Dispensasi int64 `json:"dispensasi"`
	Total      int64 `json:"total"`
}
