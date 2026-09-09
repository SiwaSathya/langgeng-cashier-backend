package service

import (
	"backend-cashier/domain"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	// Embed database timezone ke binary Go.
	// Ini penting agar Asia/Makassar tetap tersedia di Railway/container.
	_ "time/tzdata"
)

type AttendanceService struct {
	DB *gorm.DB
}

// ======================================================
// TIMEZONE
// ======================================================

// Bali menggunakan WITA / UTC+8.
var baliLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Makassar")
	if err != nil {
		return time.FixedZone("WITA", 8*60*60)
	}
	return loc
}()

func NewAttendanceService(db *gorm.DB) *AttendanceService {
	return &AttendanceService{
		DB: db,
	}
}

// ======================================================
// HELPER: PARSE TANGGAL
// ======================================================

func parseAttendanceDate(value string) time.Time {
	value = strings.TrimSpace(value)

	if value == "" {
		now := time.Now()
		return time.Date(
			now.Year(),
			now.Month(),
			now.Day(),
			0, 0, 0, 0,
			baliLocation,
		)
	}

	// yyyy-mm-dd
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return time.Date(
			parsed.Year(),
			parsed.Month(),
			parsed.Day(),
			0, 0, 0, 0,
			baliLocation,
		)
	}

	// RFC3339
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		// JANGAN .In(baliLocation)
		// Ambil tanggalnya secara literal
		return time.Date(
			parsed.Year(),
			parsed.Month(),
			parsed.Day(),
			0, 0, 0, 0,
			baliLocation,
		)
	}

	return time.Now()
}

// ======================================================
// HELPER: PARSE JAM
// ======================================================

func parseAttendanceTime(
	date time.Time,
	value string,
) *time.Time {

	value = strings.TrimSpace(value)

	if value == "" {
		return nil
	}

	// Input normal HH:mm
	if parsed, err := time.Parse("15:04", value); err == nil {

		fullTime := time.Date(
			date.Year(),
			date.Month(),
			date.Day(),
			parsed.Hour(),
			parsed.Minute(),
			0,
			0,
			baliLocation,
		)

		return &fullTime
	}

	// Jika frontend mengirim RFC3339
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {

		// PENTING:
		// Jangan gunakan parsed.In(baliLocation)
		//
		// Kita hanya ambil nilai jam yang dikirim secara literal.
		fullTime := time.Date(
			date.Year(),
			date.Month(),
			date.Day(),
			parsed.Hour(),
			parsed.Minute(),
			parsed.Second(),
			0,
			baliLocation,
		)

		return &fullTime
	}

	return nil
}

// ======================================================
// CREATE ATTENDANCE
// ======================================================

func (s *AttendanceService) CreateAttendance(
	req domain.AttendanceRequest,
) (*domain.Attendance, error) {

	// --------------------------------------------------
	// Validasi User
	// --------------------------------------------------

	if strings.TrimSpace(req.UserID) == "" {
		return nil, errors.New(
			"user ID / karyawan wajib dipilih",
		)
	}

	var user domain.User

	if err := s.DB.
		First(
			&user,
			"id = ?",
			req.UserID,
		).
		Error; err != nil {

		return nil, errors.New(
			"data karyawan tidak ditemukan",
		)
	}

	// --------------------------------------------------
	// Tanggal Absensi
	// --------------------------------------------------

	attDate := parseAttendanceDate(req.Date)

	// Normalisasi tanggal ke timezone WITA.
	attDate = time.Date(
		attDate.Year(),
		attDate.Month(),
		attDate.Day(),
		0,
		0,
		0,
		0,
		baliLocation,
	)

	// --------------------------------------------------
	// Status
	// --------------------------------------------------

	status := strings.TrimSpace(req.Status)

	if status == "" {
		status = "Pagi"
	}

	// --------------------------------------------------
	// Shift
	// --------------------------------------------------

	shift := req.Shift

	if shift == 0 {

		if strings.EqualFold(status, "Siang") {
			shift = 2
		} else {
			shift = 1
		}
	}

	// --------------------------------------------------
	// Check In
	// --------------------------------------------------

	checkInTime := parseAttendanceTime(
		attDate,
		req.CheckIn,
	)

	// Jika status hadir tetapi jam tidak dikirim,
	// gunakan jam sekarang dalam timezone WITA.
	if checkInTime == nil &&
		(strings.EqualFold(status, "Pagi") ||
			strings.EqualFold(status, "Siang") ||
			strings.EqualFold(status, "Lembur") ||
			strings.EqualFold(status, "Bantu") ||
			strings.EqualFold(status, "Hadir")) {

		now := time.Now().In(baliLocation)
		checkInTime = &now
	}

	// --------------------------------------------------
	// Check Out
	// --------------------------------------------------

	checkOutTime := parseAttendanceTime(
		attDate,
		req.CheckOut,
	)

	// --------------------------------------------------
	// Object Attendance
	// --------------------------------------------------

	attendance := domain.Attendance{
		UserID:   user.ID,
		Date:     attDate,
		Shift:    shift,
		Status:   status,
		CheckIn:  checkInTime,
		CheckOut: checkOutTime,
		Notes:    req.Notes,
	}

	// --------------------------------------------------
	// Insert DB
	// --------------------------------------------------

	if err := s.DB.
		Create(&attendance).
		Error; err != nil {

		return nil, err
	}

	// --------------------------------------------------
	// Preload User
	// --------------------------------------------------

	if err := s.DB.
		Preload("User").
		First(
			&attendance,
			attendance.ID,
		).
		Error; err != nil {

		return nil, err
	}

	return &attendance, nil
}

// ======================================================
// GET ALL ATTENDANCE
// ======================================================

func (s *AttendanceService) GetAllAttendance(
	f domain.AttendanceFilter,
) ([]domain.Attendance, int64, error) {

	var records []domain.Attendance
	var total int64

	query := s.DB.
		Model(&domain.Attendance{}).
		Preload("User")

	// --------------------------------------------------
	// Filter tanggal tertentu
	// --------------------------------------------------

	if strings.TrimSpace(f.Date) != "" {

		date, err := time.ParseInLocation(
			"2006-01-02",
			f.Date,
			baliLocation,
		)

		if err == nil {

			start := time.Date(
				date.Year(),
				date.Month(),
				date.Day(),
				0,
				0,
				0,
				0,
				baliLocation,
			)

			end := start.Add(24 * time.Hour)

			query = query.Where(
				"date >= ? AND date < ?",
				start,
				end,
			)
		}
	}

	// --------------------------------------------------
	// Filter range tanggal
	// --------------------------------------------------

	if strings.TrimSpace(f.StartDate) != "" &&
		strings.TrimSpace(f.EndDate) != "" {

		startDate, startErr := time.ParseInLocation(
			"2006-01-02",
			f.StartDate,
			baliLocation,
		)

		endDate, endErr := time.ParseInLocation(
			"2006-01-02",
			f.EndDate,
			baliLocation,
		)

		if startErr == nil && endErr == nil {

			start := time.Date(
				startDate.Year(),
				startDate.Month(),
				startDate.Day(),
				0,
				0,
				0,
				0,
				baliLocation,
			)

			end := time.Date(
				endDate.Year(),
				endDate.Month(),
				endDate.Day(),
				0,
				0,
				0,
				0,
				baliLocation,
			).Add(24 * time.Hour)

			query = query.Where(
				"date >= ? AND date < ?",
				start,
				end,
			)
		}
	}

	// --------------------------------------------------
	// User
	// --------------------------------------------------

	if strings.TrimSpace(f.UserID) != "" {
		query = query.Where(
			"user_id = ?",
			f.UserID,
		)
	}

	// --------------------------------------------------
	// Shift
	// --------------------------------------------------

	if f.Shift > 0 {
		query = query.Where(
			"shift = ?",
			f.Shift,
		)
	}

	// --------------------------------------------------
	// Status
	// --------------------------------------------------

	if strings.TrimSpace(f.Status) != "" {
		query = query.Where(
			"LOWER(status) = ?",
			strings.ToLower(f.Status),
		)
	}

	// --------------------------------------------------
	// Search
	// --------------------------------------------------

	if strings.TrimSpace(f.Search) != "" {

		searchPattern :=
			"%" +
				strings.ToLower(
					strings.TrimSpace(f.Search),
				) +
				"%"

		query = query.
			Joins(
				"LEFT JOIN users ON users.id = attendances.user_id",
			).
			Where(
				`LOWER(users.name) LIKE ?
				OR LOWER(attendances.notes) LIKE ?`,
				searchPattern,
				searchPattern,
			)
	}

	// --------------------------------------------------
	// Count
	// --------------------------------------------------

	if err := query.
		Count(&total).
		Error; err != nil {

		return nil, 0, err
	}

	// --------------------------------------------------
	// Pagination
	// --------------------------------------------------

	page := f.Page

	if page < 1 {
		page = 1
	}

	limit := f.Limit

	if limit < 1 {
		limit = 10
	}

	offset := (page - 1) * limit

	// --------------------------------------------------
	// Find
	// --------------------------------------------------

	err := query.
		Order("date DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&records).
		Error

	return records, total, err
}

// ======================================================
// GET ATTENDANCE BY ID
// ======================================================

func (s *AttendanceService) GetAttendanceByID(
	id uint,
) (*domain.Attendance, error) {

	var record domain.Attendance

	if err := s.DB.
		Preload("User").
		First(
			&record,
			id,
		).
		Error; err != nil {

		return nil, errors.New(
			"data absensi tidak ditemukan",
		)
	}

	return &record, nil
}

// ======================================================
// UPDATE ATTENDANCE
// ======================================================

func (s *AttendanceService) UpdateAttendance(
	id uint,
	req domain.AttendanceRequest,
) (*domain.Attendance, error) {

	var record domain.Attendance

	if err := s.DB.
		First(
			&record,
			id,
		).
		Error; err != nil {

		return nil, errors.New(
			"data absensi tidak ditemukan",
		)
	}

	updates := map[string]interface{}{}

	// --------------------------------------------------
	// User
	// --------------------------------------------------

	if strings.TrimSpace(req.UserID) != "" {

		var user domain.User

		if err := s.DB.
			First(
				&user,
				"id = ?",
				req.UserID,
			).
			Error; err != nil {

			return nil, errors.New(
				"data karyawan tidak ditemukan",
			)
		}

		updates["user_id"] = req.UserID
	}

	// --------------------------------------------------
	// Ambil tanggal dasar
	// --------------------------------------------------

	recordDate := record.Date.In(baliLocation)

	// --------------------------------------------------
	// Date
	// --------------------------------------------------

	if strings.TrimSpace(req.Date) != "" {

		if d, err := time.ParseInLocation(
			"2006-01-02",
			req.Date,
			baliLocation,
		); err == nil {

			recordDate = d

			updates["date"] = time.Date(
				d.Year(),
				d.Month(),
				d.Day(),
				0,
				0,
				0,
				0,
				baliLocation,
			)
		}
	}

	// --------------------------------------------------
	// Shift
	// --------------------------------------------------

	if req.Shift > 0 {
		updates["shift"] = req.Shift
	}

	// --------------------------------------------------
	// Status
	// --------------------------------------------------

	if strings.TrimSpace(req.Status) != "" {
		updates["status"] = req.Status
	}

	// --------------------------------------------------
	// Check In
	// --------------------------------------------------

	if strings.TrimSpace(req.CheckIn) != "" {

		checkIn := parseAttendanceTime(
			recordDate,
			req.CheckIn,
		)

		if checkIn != nil {
			updates["check_in"] = checkIn
		}
	}

	// --------------------------------------------------
	// Check Out
	// --------------------------------------------------

	if strings.TrimSpace(req.CheckOut) != "" {

		checkOut := parseAttendanceTime(
			recordDate,
			req.CheckOut,
		)

		if checkOut != nil {
			updates["check_out"] = checkOut
		}
	}

	// --------------------------------------------------
	// Notes
	// --------------------------------------------------

	updates["notes"] = req.Notes

	// --------------------------------------------------
	// Update
	// --------------------------------------------------

	if err := s.DB.
		Model(&record).
		Updates(updates).
		Error; err != nil {

		return nil, err
	}

	// --------------------------------------------------
	// Reload
	// --------------------------------------------------

	if err := s.DB.
		Preload("User").
		First(
			&record,
			id,
		).
		Error; err != nil {

		return nil, err
	}

	return &record, nil
}

// ======================================================
// DELETE ATTENDANCE
// ======================================================

func (s *AttendanceService) DeleteAttendance(
	id uint,
) error {

	var record domain.Attendance

	if err := s.DB.
		First(
			&record,
			id,
		).
		Error; err != nil {

		return errors.New(
			"data absensi tidak ditemukan",
		)
	}

	return s.DB.
		Delete(&record).
		Error
}

// ======================================================
// GET SUMMARY
// ======================================================

func (s *AttendanceService) GetSummary(
	dateStr string,
) (*domain.AttendanceSummary, error) {

	if strings.TrimSpace(dateStr) == "" {
		dateStr = time.Now().
			In(baliLocation).
			Format("2006-01-02")
	}

	date, err := time.ParseInLocation(
		"2006-01-02",
		dateStr,
		baliLocation,
	)

	if err != nil {
		return nil, errors.New(
			"format tanggal tidak valid",
		)
	}

	start := time.Date(
		date.Year(),
		date.Month(),
		date.Day(),
		0,
		0,
		0,
		0,
		baliLocation,
	)

	end := start.Add(24 * time.Hour)

	var summary domain.AttendanceSummary

	baseQuery := func() *gorm.DB {
		return s.DB.
			Model(&domain.Attendance{}).
			Where(
				"date >= ? AND date < ?",
				start,
				end,
			)
	}

	// Pagi
	baseQuery().
		Where(
			"status IN ?",
			[]string{
				"Pagi",
				"Hadir Pagi",
				"Hadir",
			},
		).
		Count(&summary.Pagi)

	// Siang
	baseQuery().
		Where(
			"status IN ?",
			[]string{
				"Siang",
				"Hadir Siang",
			},
		).
		Count(&summary.Siang)

	// Libur
	baseQuery().
		Where(
			"status IN ?",
			[]string{
				"Libur",
				"Off",
			},
		).
		Count(&summary.Libur)

	// Lembur
	baseQuery().
		Where(
			"status IN ?",
			[]string{
				"Lembur",
				"Full Lembur",
			},
		).
		Count(&summary.Lembur)

	// Bantu
	baseQuery().
		Where(
			"status IN ?",
			[]string{
				"Bantu",
				"Setengah Lembur",
				"Lembur Setengah",
			},
		).
		Count(&summary.Bantu)

	// Sakit
	baseQuery().
		Where(
			"status = ?",
			"Sakit",
		).
		Count(&summary.Sakit)

	// Dispensasi / Izin
	baseQuery().
		Where(
			"status IN ?",
			[]string{
				"Dispensasi",
				"Izin",
			},
		).
		Count(&summary.Dispensasi)

	summary.Total =
		summary.Pagi +
			summary.Siang +
			summary.Libur +
			summary.Lembur +
			summary.Bantu +
			summary.Sakit +
			summary.Dispensasi

	return &summary, nil
}
