package service

import (
	"backend-cashier/domain"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

type AttendanceService struct {
	DB *gorm.DB
}

func NewAttendanceService(db *gorm.DB) *AttendanceService {
	return &AttendanceService{DB: db}
}

func (s *AttendanceService) CreateAttendance(req domain.AttendanceRequest) (*domain.Attendance, error) {
	if req.UserID == "" {
		return nil, errors.New("user ID / karyawan wajib dipilih")
	}

	var user domain.User
	if err := s.DB.First(&user, "id = ?", req.UserID).Error; err != nil {
		return nil, errors.New("data karyawan tidak ditemukan")
	}

	var attDate time.Time
	var err error
	if req.Date != "" {
		attDate, err = time.Parse("2006-01-02", req.Date)
		if err != nil {
			attDate, err = time.Parse(time.RFC3339, req.Date)
			if err != nil {
				attDate = time.Now()
			}
		}
	} else {
		attDate = time.Now()
	}

	status := req.Status
	if status == "" {
		status = "Pagi"
	}

	shift := req.Shift
	if shift == 0 {
		if strings.EqualFold(status, "Siang") {
			shift = 2
		} else {
			shift = 1
		}
	}

	var checkInTime *time.Time
	if req.CheckIn != "" {
		t, err := time.Parse("15:04", req.CheckIn)
		if err == nil {
			fullTime := time.Date(attDate.Year(), attDate.Month(), attDate.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
			checkInTime = &fullTime
		} else {
			t2, err2 := time.Parse(time.RFC3339, req.CheckIn)
			if err2 == nil {
				checkInTime = &t2
			}
		}
	}
	if checkInTime == nil && (status == "Pagi" || status == "Siang" || status == "Lembur" || status == "Bantu" || status == "Hadir") {
		now := time.Now()
		checkInTime = &now
	}

	var checkOutTime *time.Time
	if req.CheckOut != "" {
		t, err := time.Parse("15:04", req.CheckOut)
		if err == nil {
			fullTime := time.Date(attDate.Year(), attDate.Month(), attDate.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
			checkOutTime = &fullTime
		} else {
			t2, err2 := time.Parse(time.RFC3339, req.CheckOut)
			if err2 == nil {
				checkOutTime = &t2
			}
		}
	}

	attendance := domain.Attendance{
		UserID:   user.ID,
		Date:     attDate,
		Shift:    shift,
		Status:   status,
		CheckIn:  checkInTime,
		CheckOut: checkOutTime,
		Notes:    req.Notes,
	}

	if err := s.DB.Create(&attendance).Error; err != nil {
		return nil, err
	}

	s.DB.Preload("User").First(&attendance, attendance.ID)
	return &attendance, nil
}

func (s *AttendanceService) GetAllAttendance(f domain.AttendanceFilter) ([]domain.Attendance, int64, error) {
	var records []domain.Attendance
	var total int64

	query := s.DB.Model(&domain.Attendance{}).Preload("User")

	if f.Date != "" {
		query = query.Where("DATE(date) = ?", f.Date)
	}
	if f.StartDate != "" && f.EndDate != "" {
		query = query.Where("date BETWEEN ? AND ?", f.StartDate+" 00:00:00", f.EndDate+" 23:59:59")
	}
	if f.UserID != "" {
		query = query.Where("user_id = ?", f.UserID)
	}
	if f.Shift > 0 {
		query = query.Where("shift = ?", f.Shift)
	}
	if f.Status != "" {
		query = query.Where("LOWER(status) = ?", strings.ToLower(f.Status))
	}
	if f.Search != "" {
		searchPattern := "%" + strings.ToLower(f.Search) + "%"
		query = query.Joins("LEFT JOIN users ON users.id = attendances.user_id").
			Where("LOWER(users.name) LIKE ? OR LOWER(attendances.notes) LIKE ?", searchPattern, searchPattern)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := f.Page
	if page < 1 {
		page = 1
	}
	limit := f.Limit
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	err := query.Order("date desc, id desc").Limit(limit).Offset(offset).Find(&records).Error
	return records, total, err
}

func (s *AttendanceService) GetAttendanceByID(id uint) (*domain.Attendance, error) {
	var record domain.Attendance
	if err := s.DB.Preload("User").First(&record, id).Error; err != nil {
		return nil, errors.New("data absensi tidak ditemukan")
	}
	return &record, nil
}

func (s *AttendanceService) UpdateAttendance(id uint, req domain.AttendanceRequest) (*domain.Attendance, error) {
	var record domain.Attendance
	if err := s.DB.First(&record, id).Error; err != nil {
		return nil, errors.New("data absensi tidak ditemukan")
	}

	updates := map[string]interface{}{}
	if req.UserID != "" {
		updates["user_id"] = req.UserID
	}
	if req.Date != "" {
		d, err := time.Parse("2006-01-02", req.Date)
		if err == nil {
			updates["date"] = d
		}
	}
	if req.Shift > 0 {
		updates["shift"] = req.Shift
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if req.CheckIn != "" {
		t, err := time.Parse("15:04", req.CheckIn)
		if err == nil {
			fullTime := time.Date(record.Date.Year(), record.Date.Month(), record.Date.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
			updates["check_in"] = &fullTime
		}
	}
	if req.CheckOut != "" {
		t, err := time.Parse("15:04", req.CheckOut)
		if err == nil {
			fullTime := time.Date(record.Date.Year(), record.Date.Month(), record.Date.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
			updates["check_out"] = &fullTime
		}
	}
	updates["notes"] = req.Notes

	if err := s.DB.Model(&record).Updates(updates).Error; err != nil {
		return nil, err
	}

	s.DB.Preload("User").First(&record, id)
	return &record, nil
}

func (s *AttendanceService) DeleteAttendance(id uint) error {
	var record domain.Attendance
	if err := s.DB.First(&record, id).Error; err != nil {
		return errors.New("data absensi tidak ditemukan")
	}
	return s.DB.Delete(&record).Error
}

func (s *AttendanceService) GetSummary(dateStr string) (*domain.AttendanceSummary, error) {
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	var summary domain.AttendanceSummary
	query := s.DB.Model(&domain.Attendance{}).Where("DATE(date) = ?", dateStr)

	query.Where("status = ? OR status = ? OR status = ?", "Pagi", "Hadir Pagi", "Hadir").Count(&summary.Pagi)
	query.Where("status = ? OR status = ?", "Siang", "Hadir Siang").Count(&summary.Siang)
	query.Where("status = ? OR status = ?", "Libur", "Off").Count(&summary.Libur)
	query.Where("status = ? OR status = ?", "Lembur", "Full Lembur").Count(&summary.Lembur)
	query.Where("status = ? OR status = ? OR status = ?", "Bantu", "Setengah Lembur", "Lembur Setengah").Count(&summary.Bantu)
	query.Where("status = ?", "Sakit").Count(&summary.Sakit)
	query.Where("status = ? OR status = ?", "Dispensasi", "Izin").Count(&summary.Dispensasi)

	summary.Total = summary.Pagi + summary.Siang + summary.Libur + summary.Lembur + summary.Bantu + summary.Sakit + summary.Dispensasi

	return &summary, nil
}
