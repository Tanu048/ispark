package models

import (
	"time"

	"gorm.io/gorm"
)

// Canonical course names. These are the programmes IIPS offers and the single
// source of truth for course values — used for seeding, the report course filter,
// registration and anywhere a canonical course list is needed, so course names
// never drift between features.
const (
	CourseMBAMS5Yr = "MBA(MS) 5yrs Integrated"
	CourseMCA5Yr   = "MCA 5yrs Integrated"
	CourseMTechCS  = "MTech(CS) 5yrs Integrated"
	CourseMTechIT  = "MTech(IT) 5yrs Integrated"
	CourseBComHons = "B.Com (Hons) UG"
	CourseMBAMS2Yr = "MBA(MS) 2yrs PG"
	CourseMBAAPR   = "MBA(APR) 2yrs PG"
	CourseMBAT     = "MBA(T) 2yrs PG"
)

// CanonicalCourses is the ordered list of programmes offered by IIPS.
var CanonicalCourses = []string{
	CourseMBAMS5Yr, CourseMCA5Yr, CourseMTechCS, CourseMTechIT,
	CourseBComHons, CourseMBAMS2Yr, CourseMBAAPR, CourseMBAT,
}

type Student struct {
	RollNo       string         `gorm:"primaryKey;type:varchar(50)" json:"roll_no"`
	Name         string         `gorm:"type:varchar(100);not null" json:"name"`
	CourseName   string         `gorm:"type:varchar(100);not null" json:"course_name"`
	Semester     int            `gorm:"not null" json:"semester"`
	ContactNo    string         `gorm:"type:varchar(20);not null" json:"contact_no"`
	EmailID      string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"email_id"`
	DOB          string         `gorm:"type:varchar(50)" json:"dob"`
	Gender       string         `gorm:"type:varchar(20)" json:"gender"`
	EnrollmentNo string         `gorm:"type:varchar(50);uniqueIndex;not null" json:"enrollment_no"`
	Password     string         `gorm:"type:varchar(255);not null" json:"-"` // "-" hides password from json marshalling
	IsVerified   bool           `gorm:"default:false" json:"is_verified"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	Certificates []Certificate  `gorm:"foreignKey:StudentRollNo" json:"certificates"`
	Enrollments  []Enrollment   `gorm:"foreignKey:StudentRollNo" json:"enrollments"`

	Batch               string `gorm:"-" json:"batch"`
	CreditsEarned       int    `gorm:"-" json:"credits_earned"`
	ActivityCount       int    `gorm:"-" json:"activity_count"`
	PendingCertificates int    `gorm:"-" json:"pending_certificates"`
	EngagementStatus    string `gorm:"-" json:"engagement_status"`
}
