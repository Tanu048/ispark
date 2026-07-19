package controllers_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/iips-oss/ispark/api/config"
	"github.com/iips-oss/ispark/api/controllers"
	"github.com/iips-oss/ispark/api/models"
	"gorm.io/gorm"
)

// setupReportsApp spins up an in-memory database seeded with a small, known data
// set and a Fiber app wired straight to the report handlers (no auth middleware,
// which is exercised separately). Generated files land under ./uploads and are
// cleaned up when the test finishes.
func setupReportsApp(t *testing.T) *fiber.App {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Student{}, &models.Certificate{}, &models.Enrollment{}, &models.Activity{},
		&models.Admin{}, &models.GeneratedReport{}, &models.ScheduledReport{}, &models.ReportAuditLog{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	config.DB = db
	seedReportsData(t, db)

	t.Cleanup(func() { _ = os.RemoveAll("./uploads") })

	app := fiber.New()
	g := app.Group("/api/admin/platform")
	g.Get("/reports/summary", controllers.GetReportsSummary)
	g.Get("/reports/filters", controllers.GetReportFilters)
	g.Get("/reports/export/counts", controllers.GetExportCounts)
	g.Get("/reports/export", controllers.ExportData)
	g.Post("/reports/generate", controllers.GenerateReport)
	g.Get("/reports", controllers.GetGeneratedReports)
	g.Get("/reports/:id", controllers.GetReportDetail)
	g.Get("/reports/:id/download", controllers.DownloadReport)
	return app
}

// seedReportsData inserts two students in different courses/semesters, each with
// an approved certificate (dated "now") and an enrollment, plus one pending
// certificate. That gives: 2 students, 3 certificates (2 approved), 2 enrollments.
func seedReportsData(t *testing.T, db *gorm.DB) {
	t.Helper()

	now := time.Now()
	students := []models.Student{
		{RollNo: "S1", Name: "Alice", CourseName: models.CourseMTechCS, Semester: 6, EmailID: "a@x.dev", EnrollmentNo: "EN-S1", Password: "x", IsVerified: true},
		{RollNo: "S2", Name: "Bob", CourseName: models.CourseMCA5Yr, Semester: 2, EmailID: "b@x.dev", EnrollmentNo: "EN-S2", Password: "x", IsVerified: false},
	}
	if err := db.Create(&students).Error; err != nil {
		t.Fatalf("seed students: %v", err)
	}

	certs := []models.Certificate{
		{StudentRollNo: "S1", ActivityName: "Hackathon", ActivityCategory: "TECHNICAL", ActivityDate: now, Status: "Approved", Credits: 10},
		{StudentRollNo: "S1", ActivityName: "Seminar", ActivityCategory: "TECHNICAL", ActivityDate: now, Status: "Pending", Credits: 5},
		{StudentRollNo: "S2", ActivityName: "Workshop", ActivityCategory: "TECHNICAL", ActivityDate: now, Status: "Approved", Credits: 5},
	}
	if err := db.Create(&certs).Error; err != nil {
		t.Fatalf("seed certificates: %v", err)
	}

	enrollments := []models.Enrollment{
		{StudentRollNo: "S1", ActivityID: 1, Status: "Enrolled"},
		{StudentRollNo: "S2", ActivityID: 1, Status: "Completed"},
	}
	if err := db.Create(&enrollments).Error; err != nil {
		t.Fatalf("seed enrollments: %v", err)
	}
}

// doReq sends a JSON request through the Fiber app and returns the response and body.
func doReq(t *testing.T, app *fiber.App, method, path string, body any) (*http.Response, []byte) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

// TestGenerateReportFormatValidation confirms only CSV is accepted; PDF and Excel
// are rejected so the download can never be a mislabelled file.
func TestGenerateReportFormatValidation(t *testing.T) {
	app := setupReportsApp(t)

	for _, format := range []string{"PDF", "Excel"} {
		resp, _ := doReq(t, app, http.MethodPost, "/api/admin/platform/reports/generate",
			map[string]string{"type": "Student Performance", "format": format})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("format %q: expected 400, got %d", format, resp.StatusCode)
		}
	}

	resp, body := doReq(t, app, http.MethodPost, "/api/admin/platform/reports/generate",
		map[string]string{"type": "Student Performance", "format": "CSV"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("CSV: expected 201, got %d (%s)", resp.StatusCode, body)
	}

	var out struct {
		Report models.GeneratedReport `json:"report"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Report.Status != "Ready" {
		t.Errorf("expected Ready, got %q", out.Report.Status)
	}
	if !strings.HasSuffix(out.Report.FileName, ".csv") {
		t.Errorf("expected a .csv file name, got %q", out.Report.FileName)
	}
}

// TestExportCountsCreditsMatchStudents confirms the credits export count equals
// the student total (the credits export is one row per student), not the number
// of approved certificates.
func TestExportCountsCreditsMatchStudents(t *testing.T) {
	app := setupReportsApp(t)

	_, body := doReq(t, app, http.MethodGet, "/api/admin/platform/reports/export/counts", nil)
	var counts struct {
		Students     int64 `json:"students"`
		Credits      int64 `json:"credits"`
		Certificates int64 `json:"certificates"`
	}
	if err := json.Unmarshal(body, &counts); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if counts.Students != 2 {
		t.Errorf("expected 2 students, got %d", counts.Students)
	}
	if counts.Credits != counts.Students {
		t.Errorf("credits count (%d) should equal students count (%d)", counts.Credits, counts.Students)
	}

	// And the credits export really does produce one row per student.
	_, csvBody := doReq(t, app, http.MethodGet, "/api/admin/platform/reports/export?type=credits", nil)
	if got := dataRowCount(string(csvBody)); int64(got) != counts.Credits {
		t.Errorf("credits export has %d rows, count says %d", got, counts.Credits)
	}
}

// TestReportFiltersAreCanonical confirms the filter options come from the actual
// student records rather than a hardcoded list.
func TestReportFiltersAreCanonical(t *testing.T) {
	app := setupReportsApp(t)

	_, body := doReq(t, app, http.MethodGet, "/api/admin/platform/reports/filters", nil)
	var filters struct {
		Courses   []string `json:"courses"`
		Semesters []int    `json:"semesters"`
	}
	if err := json.Unmarshal(body, &filters); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Courses are the fixed canonical program list, not distinct row values.
	for _, want := range models.CanonicalCourses {
		if !contains(filters.Courses, want) {
			t.Errorf("expected canonical course %q, got %v", want, filters.Courses)
		}
	}
	if len(filters.Courses) != len(models.CanonicalCourses) {
		t.Errorf("expected exactly %d courses, got %v", len(models.CanonicalCourses), filters.Courses)
	}
	// Semesters are the full 1..10 domain, not just the populated ones.
	for _, want := range []int{1, 6, 10} {
		if !containsInt(filters.Semesters, want) {
			t.Errorf("expected semester %d to be present, got %v", want, filters.Semesters)
		}
	}
}

// TestStudentReportDateRangeSelectsStudents confirms the selected date range
// determines which students appear: a future window (no activity) yields only
// the header, while a window covering the seeded data includes the students.
func TestStudentReportDateRangeSelectsStudents(t *testing.T) {
	app := setupReportsApp(t)

	// Future window: no student has activity in it, so no data rows are produced.
	future := generateStudentReportCSV(t, app, map[string]string{
		"type":      "Student Performance",
		"format":    "CSV",
		"date_from": "2099-01-01",
		"date_to":   "2099-12-31",
	})
	if got := dataRowCount(future); got != 0 {
		t.Errorf("future range should return no student rows, got %d:\n%s", got, future)
	}

	// Window covering today: the seeded students (whose activity is dated now)
	// reappear, so the range genuinely selects reportable students.
	now := time.Now()
	current := generateStudentReportCSV(t, app, map[string]string{
		"type":      "Student Performance",
		"format":    "CSV",
		"date_from": now.AddDate(0, 0, -1).Format("2006-01-02"),
		"date_to":   now.AddDate(0, 0, 1).Format("2006-01-02"),
	})
	if got := dataRowCount(current); got != 2 {
		t.Errorf("range covering the seeded data should return both students, got %d:\n%s", got, current)
	}
}

// generateStudentReportCSV generates a report from the given payload and returns
// its downloaded CSV body.
func generateStudentReportCSV(t *testing.T, app *fiber.App, payload map[string]string) string {
	t.Helper()

	resp, body := doReq(t, app, http.MethodPost, "/api/admin/platform/reports/generate", payload)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("generate: expected 201, got %d (%s)", resp.StatusCode, body)
	}
	var out struct {
		Report struct {
			ID uint `json:"id"`
		} `json:"report"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	_, csvBody := doReq(t, app, http.MethodGet,
		"/api/admin/platform/reports/"+itoa(out.Report.ID)+"/download", nil)
	return string(csvBody)
}

// TestDownloadReportReturnsCSV confirms a generated report downloads as real CSV
// content with the expected header and data.
func TestDownloadReportReturnsCSV(t *testing.T) {
	app := setupReportsApp(t)

	_, body := doReq(t, app, http.MethodPost, "/api/admin/platform/reports/generate",
		map[string]string{"type": "Student Performance", "format": "CSV"})
	var out struct {
		Report struct {
			ID uint `json:"id"`
		} `json:"report"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	resp, csvBody := doReq(t, app, http.MethodGet,
		"/api/admin/platform/reports/"+itoa(out.Report.ID)+"/download", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download: expected 200, got %d", resp.StatusCode)
	}

	text := string(csvBody)
	if !strings.HasPrefix(text, "Roll No,Name,Course") {
		t.Errorf("unexpected CSV header: %q", firstLine(text))
	}
	if !strings.Contains(text, "Alice") || !strings.Contains(text, models.CourseMTechCS) {
		t.Errorf("download missing seeded student data:\n%s", text)
	}
}

// --- small test helpers ----------------------------------------------------

func parseCSV(s string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		rows = append(rows, strings.Split(line, ","))
	}
	return rows
}

func dataRowCount(s string) int {
	rows := parseCSV(s)
	if len(rows) == 0 {
		return 0
	}
	return len(rows) - 1 // exclude header
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func containsInt(xs []int, want int) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func itoa(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
