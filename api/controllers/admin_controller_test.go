package controllers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/iips-oss/ispark/api/config"
	"github.com/iips-oss/ispark/api/models"
	"github.com/iips-oss/ispark/api/routes"
	"github.com/iips-oss/ispark/api/utils"
	"gorm.io/gorm"
)

func SetupAdminTestDB(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to SQLite: %v", err)
	}

	err = db.AutoMigrate(&models.Admin{})
	if err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	config.DB = db
}

func TestGetAdminProfile(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("test-jwt-", 4))
	t.Setenv("JWT_REFRESH_SECRET", strings.Repeat("test-refresh-jwt-", 4))

	SetupAdminTestDB(t)

	app := fiber.New()
	routes.SetupRoutes(app)

	// Seed an admin
	hashedPassword, _ := utils.HashPassword("Password123")
	testAdmin := models.Admin{
		AdminID:       "testadmin",
		Name:          "Test Admin",
		Email:         "test.admin@isparc.dev",
		Password:      hashedPassword,
		Role:          "admin",
		AssignedBatch: "IT2K20",
	}
	config.DB.Create(&testAdmin)

	// 1. Unauthenticated Request
	t.Run("GetAdminProfile_Unauthenticated", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/admin/profile", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Failed to execute request: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", resp.StatusCode)
		}
	})

	// 2. Authenticated Request
	t.Run("GetAdminProfile_Success", func(t *testing.T) {
		token, err := utils.GenerateAccessToken(testAdmin.AdminID, testAdmin.Email, testAdmin.Role)
		if err != nil {
			t.Fatalf("Failed to generate token: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/admin/profile", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Failed to execute request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var respBody map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		adminMap, ok := respBody["admin"].(map[string]interface{})
		if !ok {
			t.Fatalf("Admin data missing in response")
		}

		if adminMap["admin_id"] != testAdmin.AdminID {
			t.Errorf("Expected admin_id %s, got %v", testAdmin.AdminID, adminMap["admin_id"])
		}
		if adminMap["name"] != testAdmin.Name {
			t.Errorf("Expected name %s, got %v", testAdmin.Name, adminMap["name"])
		}
		if adminMap["email"] != testAdmin.Email {
			t.Errorf("Expected email %s, got %v", testAdmin.Email, adminMap["email"])
		}
		if adminMap["role"] != testAdmin.Role {
			t.Errorf("Expected role %s, got %v", testAdmin.Role, adminMap["role"])
		}
		if adminMap["assigned_batch"] != testAdmin.AssignedBatch {
			t.Errorf("Expected assigned_batch %s, got %v", testAdmin.AssignedBatch, adminMap["assigned_batch"])
		}
	})
}

func TestUpdateAdminProfile(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("test-jwt-", 4))
	t.Setenv("JWT_REFRESH_SECRET", strings.Repeat("test-refresh-jwt-", 4))

	SetupAdminTestDB(t)

	app := fiber.New()
	routes.SetupRoutes(app)

	// Seed admins for testing conflicts
	hashedPassword, _ := utils.HashPassword("Password123")
	testAdmin := models.Admin{
		AdminID:       "admin1",
		Name:          "Admin One",
		Email:         "admin1@isparc.dev",
		Password:      hashedPassword,
		Role:          "admin",
		AssignedBatch: "IT2K20",
	}
	config.DB.Create(&testAdmin)

	otherAdmin := models.Admin{
		AdminID:       "admin2",
		Name:          "Admin Two",
		Email:         "admin2@isparc.dev",
		Password:      hashedPassword,
		Role:          "admin",
		AssignedBatch: "IT2K20",
	}
	config.DB.Create(&otherAdmin)

	token, err := utils.GenerateAccessToken(testAdmin.AdminID, testAdmin.Email, testAdmin.Role)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// 1. Success case: Update own name and email
	t.Run("UpdateAdminProfile_Success", func(t *testing.T) {
		body := `{"name":"Admin One Updated","email":"new.admin1@isparc.dev"}`
		req := httptest.NewRequest("PUT", "/api/admin/profile", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Failed to execute request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		// Verify database state
		var dbAdmin models.Admin
		config.DB.Where("admin_id = ?", testAdmin.AdminID).First(&dbAdmin)
		if dbAdmin.Name != "Admin One Updated" {
			t.Errorf("Expected name 'Admin One Updated', got %s", dbAdmin.Name)
		}
		if dbAdmin.Email != "new.admin1@isparc.dev" {
			t.Errorf("Expected email 'new.admin1@isparc.dev', got %s", dbAdmin.Email)
		}
	})

	// 2. Blank name after trim -> 400
	t.Run("UpdateAdminProfile_BlankName", func(t *testing.T) {
		body := `{"name":"   ","email":"valid@isparc.dev"}`
		req := httptest.NewRequest("PUT", "/api/admin/profile", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Failed to execute request: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", resp.StatusCode)
		}
	})

	// 3. Invalid email format -> 400
	invalidEmails := []string{
		"not-an-email",
		"@@@",
		"admin@",
		"a@b",
		"  x  ",
	}
	for _, email := range invalidEmails {
		t.Run("UpdateAdminProfile_InvalidEmail_"+email, func(t *testing.T) {
			body := `{"name":"Valid Name","email":"` + email + `"}`
			req := httptest.NewRequest("PUT", "/api/admin/profile", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("Failed to execute request: %v", err)
			}
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("Expected 400 for email '%s', got %d", email, resp.StatusCode)
			}
		})
	}

	// 4. Duplicate email (exact case) -> 409
	t.Run("UpdateAdminProfile_DuplicateEmail_Exact", func(t *testing.T) {
		body := `{"name":"Valid Name","email":"admin2@isparc.dev"}`
		req := httptest.NewRequest("PUT", "/api/admin/profile", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Failed to execute request: %v", err)
		}
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("Expected 409, got %d", resp.StatusCode)
		}
	})

	// 5. Duplicate email (different case) -> 409
	t.Run("UpdateAdminProfile_DuplicateEmail_DifferentCase", func(t *testing.T) {
		body := `{"name":"Valid Name","email":"ADMIN2@isparc.dev"}`
		req := httptest.NewRequest("PUT", "/api/admin/profile", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Failed to execute request: %v", err)
		}
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("Expected 409, got %d", resp.StatusCode)
		}
	})
}
