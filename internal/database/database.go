package database

import (
	"errors"
	"fmt"
	"time"

	"app/internal/config"
	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	RoleAdmin       = "admin"
	RoleMember      = "member"
	DefaultCategory = "Umum"
)

// PostCategories is the single source of truth for the allowed article categories.
var PostCategories = []string{"Teknologi", "Pendidikan", "Gaya Hidup", "Bisnis", "Opini", "Umum"}

// User model for database package migrations
type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"uniqueIndex;not null" json:"username"`
	Email     string    `gorm:"uniqueIndex;not null" json:"email"`
	Password  string    `gorm:"not null" json:"-"`
	Role      string    `gorm:"not null;default:'member'" json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Post model. Content is stored as Markdown.
type Post struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	UserID         uint       `gorm:"not null;index" json:"user_id"`
	Title          string     `gorm:"not null" json:"title"`
	Slug           string     `gorm:"uniqueIndex;not null" json:"slug"`
	Category       string     `gorm:"not null;default:'Umum'" json:"category"`
	Excerpt        string     `json:"excerpt"`
	Content        string     `gorm:"type:text" json:"content"`
	CoverImagePath string     `json:"cover_image_path,omitempty"`
	IsPublished    bool       `gorm:"not null;default:false" json:"is_published"`
	PublishedAt    *time.Time `json:"published_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	User           *User      `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"user,omitempty"`
}

// Connect opens the runtime database: PostgreSQL by default, SQLite when
// DB_CONNECTION is "sqlite" or "sqlite3".
func Connect(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	if cfg.DBConnection == "sqlite" || cfg.DBConnection == "sqlite3" {
		dialector = sqlite.Open(cfg.DBDatabase)
	} else {
		dialector = postgres.Open(cfg.DSN())
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return db, nil
}

// ConnectTest returns a migrated, isolated SQLite in-memory database for unit tests.
// The pool is limited to one connection because every new connection to
// ":memory:" would otherwise be a separate, empty database.
func ConnectTest() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	_ = Migrate(db)
	return db, err
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&User{}, &Post{})
}

type seedAccount struct {
	username string
	email    string
	password string
	role     string
}

// Seed idempotently creates the default admin (from ADMIN_USERNAME / ADMIN_PASSWORD)
// and the default member account.
func Seed(db *gorm.DB, cfgs ...*config.Config) error {
	cfg := config.LoadConfig()
	if len(cfgs) > 0 && cfgs[0] != nil {
		cfg = cfgs[0]
	}

	accounts := []seedAccount{
		{cfg.AdminUsername, cfg.AdminUsername + "@delcom.org", cfg.AdminPassword, RoleAdmin},
		{"member", "member@delcom.org", "member", RoleMember},
	}

	for _, acc := range accounts {
		if err := seedUser(db, acc); err != nil {
			return err
		}
	}
	return nil
}

func seedUser(db *gorm.DB, acc seedAccount) error {
	var existing User
	err := db.Where("username = ?", acc.username).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(acc.password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return db.Create(&User{
		Username: acc.username,
		Email:    acc.email,
		Password: string(hashed),
		Role:     acc.role,
	}).Error
}
