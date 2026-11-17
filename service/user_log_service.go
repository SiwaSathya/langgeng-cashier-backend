package service

import (
	"backend-cashier/domain"
	"strings"
	"time"

	"gorm.io/gorm"
)

type UserLogService struct {
	DB *gorm.DB
}

func NewUserLogService(db *gorm.DB) *UserLogService {
	return &UserLogService{DB: db}
}

// LogLogin mencatat aktivitas login pengguna baru
func (s *UserLogService) LogLogin(logEntry domain.UserActivityLog) (*domain.UserActivityLog, error) {
	now := time.Now()
	if logEntry.LoginAt.IsZero() {
		logEntry.LoginAt = now
	}
	logEntry.LogoutType = domain.LogoutTypeActive
	if logEntry.Device == "" && logEntry.UserAgent != "" {
		logEntry.Device = parseDevice(logEntry.UserAgent)
	}

	// 1. Tutup otomatis sesi 'ACTIVE' sebelumnya milik user ini agar tidak menumpuk menjadi sesi hantu
	var oldActiveLogs []domain.UserActivityLog
	s.DB.Where("(user_id = ? OR username = ?) AND logout_type = ?", logEntry.UserID, logEntry.Username, domain.LogoutTypeActive).Find(&oldActiveLogs)
	for _, oldLog := range oldActiveLogs {
		duration := int64(now.Sub(oldLog.LoginAt).Seconds())
		if duration < 0 {
			duration = 0
		}
		notes := oldLog.Notes
		if notes != "" {
			notes += " | Sesi ditutup otomatis (Login baru)"
		} else {
			notes = "Sesi ditutup otomatis (Login baru)"
		}
		_ = s.DB.Model(&domain.UserActivityLog{}).Where("id = ?", oldLog.ID).Updates(map[string]interface{}{
			"logout_at":    now,
			"logout_type":  domain.LogoutTypeOnly,
			"duration_sec": duration,
			"notes":        notes,
		}).Error
	}

	// 2. Buat record baru
	if err := s.DB.Create(&logEntry).Error; err != nil {
		return nil, err
	}

	return &logEntry, nil
}

// CleanupStaleActiveLogs menutup sesi berstatus 'ACTIVE' yang tidak lagi aktif di presence / heartbeat (tab ditutup / offline)
func (s *UserLogService) CleanupStaleActiveLogs(onlineSessionIDs map[string]bool) int {
	now := time.Now()
	var activeLogs []domain.UserActivityLog
	// Ambil semua sesi yang masih bertipe ACTIVE dan sudah berjalan lebih dari 45 detik
	threshold := now.Add(-45 * time.Second)
	if err := s.DB.Where("logout_type = ? AND login_at <= ?", domain.LogoutTypeActive, threshold).Find(&activeLogs).Error; err != nil {
		return 0
	}

	closedCount := 0
	for _, logItem := range activeLogs {
		// Jika session_id ini tidak ada di daftar online presence, berarti sudah offline / tab ditutup
		if !onlineSessionIDs[logItem.SessionID] {
			logoutTime := now
			duration := int64(logoutTime.Sub(logItem.LoginAt).Seconds())
			if duration > 86400 { // Lebih dari 24 jam
				logoutTime = logItem.LoginAt.Add(30 * time.Minute)
				duration = 1800
			} else if duration < 0 {
				duration = 0
			}

			notes := logItem.Notes
			if notes != "" {
				notes += " | Sesi ditutup otomatis (Offline / Tab Ditutup)"
			} else {
				notes = "Sesi ditutup otomatis (Offline / Tab Ditutup)"
			}

			_ = s.DB.Model(&domain.UserActivityLog{}).Where("id = ?", logItem.ID).Updates(map[string]interface{}{
				"logout_at":    logoutTime,
				"logout_type":  domain.LogoutTypeOnly,
				"duration_sec": duration,
				"notes":        notes,
			}).Error
			closedCount++
		}
	}
	return closedCount
}

// LogLogout memperbarui catatan log saat user logout (baik dengan menyelesaikan shift maupun logout biasa)
func (s *UserLogService) LogLogout(req domain.LogoutRequest, userID string) (*domain.UserActivityLog, error) {
	now := time.Now()
	logoutType := strings.TrimSpace(req.LogoutType)
	if logoutType == "" {
		logoutType = domain.LogoutTypeOnly
	}

	var logEntry domain.UserActivityLog
	query := s.DB.Where("logout_type = ?", domain.LogoutTypeActive)

	if req.SessionID != "" {
		query = query.Where("session_id = ?", req.SessionID)
	} else if userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	err := query.Order("id DESC").First(&logEntry).Error
	if err != nil {
		// Jika sesi aktif spesifik tidak ditemukan, buat record log ringkas untuk mencatat kejadian logout
		logEntry = domain.UserActivityLog{
			SessionID:   req.SessionID,
			UserID:      userID,
			Location:    req.Location,
			LoginAt:     now,
			LogoutAt:    &now,
			LogoutType:  logoutType,
			ShiftNumber: req.Shift,
			Notes:       req.Notes,
		}
		if s.DB.Create(&logEntry).Error != nil {
			return nil, err
		}
		return &logEntry, nil
	}

	// Update data logout
	logEntry.LogoutAt = &now
	logEntry.LogoutType = logoutType
	logEntry.DurationSec = int64(now.Sub(logEntry.LoginAt).Seconds())

	if req.Shift != nil && *req.Shift > 0 {
		logEntry.ShiftNumber = req.Shift
	}
	if req.Location != "" {
		logEntry.Location = req.Location
	}
	if req.Notes != "" {
		if logEntry.Notes != "" {
			logEntry.Notes += " | " + req.Notes
		} else {
			logEntry.Notes = req.Notes
		}
	}

	if logoutType == domain.LogoutTypeWithShift && logEntry.ShiftNumber != nil {
		shiftNote := "Selesai Shift " + string(rune('0'+*logEntry.ShiftNumber))
		if logEntry.Notes != "" && !strings.Contains(logEntry.Notes, shiftNote) {
			logEntry.Notes = shiftNote + " - " + logEntry.Notes
		} else if logEntry.Notes == "" {
			logEntry.Notes = shiftNote
		}
	}

	if err := s.DB.Save(&logEntry).Error; err != nil {
		return nil, err
	}

	return &logEntry, nil
}

// GetLogs mengambil daftar log aktivitas login-logout dengan filter dan pagination
func (s *UserLogService) GetLogs(filter domain.UserLogFilter) ([]domain.UserActivityLog, int64, error) {
	var logs []domain.UserActivityLog
	var total int64

	db := s.DB.Model(&domain.UserActivityLog{})

	if filter.UserID != "" {
		db = db.Where("user_id = ?", filter.UserID)
	}
	if filter.Username != "" {
		db = db.Where("username ILIKE ?", "%"+filter.Username+"%")
	}
	if filter.Role != "" && filter.Role != "ALL" {
		db = db.Where("role ILIKE ?", "%"+filter.Role+"%")
	}
	if filter.Location != "" && filter.Location != "ALL" {
		db = db.Where("location = ?", filter.Location)
	}
	if filter.LogoutType != "" && filter.LogoutType != "ALL" {
		db = db.Where("logout_type = ?", filter.LogoutType)
	}

	// Filter rentang tanggal berdasarkan login_at
	if filter.StartDate != "" {
		db = db.Where("login_at >= ?", filter.StartDate+" 00:00:00")
	}
	if filter.EndDate != "" {
		db = db.Where("login_at <= ?", filter.EndDate+" 23:59:59")
	}

	// Pencarian teks umum (nama, username, location, notes, device)
	if filter.Search != "" {
		sPattern := "%" + filter.Search + "%"
		db = db.Where("name ILIKE ? OR username ILIKE ? OR location ILIKE ? OR notes ILIKE ? OR device ILIKE ? OR ip_address ILIKE ?",
			sPattern, sPattern, sPattern, sPattern, sPattern, sPattern)
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	err := db.Order("login_at DESC, id DESC").Limit(limit).Offset(offset).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// GetSummary menghitung statistik aktivitas login dan logout per jenis
func (s *UserLogService) GetSummary(startDate, endDate string) (map[string]interface{}, error) {
	db := s.DB.Model(&domain.UserActivityLog{})
	if startDate != "" {
		db = db.Where("login_at >= ?", startDate+" 00:00:00")
	}
	if endDate != "" {
		db = db.Where("login_at <= ?", endDate+" 23:59:59")
	}

	var totalSessions int64
	var totalLogoutWithShift int64
	var totalLogoutOnly int64
	var totalActive int64

	db.Count(&totalSessions)
	s.DB.Model(&domain.UserActivityLog{}).Where("logout_type = ?", domain.LogoutTypeWithShift).Count(&totalLogoutWithShift)
	s.DB.Model(&domain.UserActivityLog{}).Where("logout_type = ?", domain.LogoutTypeOnly).Count(&totalLogoutOnly)
	s.DB.Model(&domain.UserActivityLog{}).Where("logout_type = ?", domain.LogoutTypeActive).Count(&totalActive)

	return map[string]interface{}{
		"total_sessions":          totalSessions,
		"total_logout_with_shift": totalLogoutWithShift,
		"total_logout_only":       totalLogoutOnly,
		"total_active":            totalActive,
	}, nil
}
