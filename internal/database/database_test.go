package database

import (
	"strings"
	"testing"

	"app/internal/config"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

func TestConnectTest_And_Migrate_And_Seed(t *testing.T) {
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "admin")

	db, err := ConnectTest()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	assert.True(t, db.Migrator().HasTable(&User{}))
	assert.True(t, db.Migrator().HasTable(&Post{}))

	// Seeding with explicit config
	cfg := &config.Config{
		AdminUsername: "admin",
		AdminPassword: "admin",
	}
	assert.NoError(t, Seed(db, cfg))

	var admin User
	assert.NoError(t, db.Where("username = ?", "admin").First(&admin).Error)
	assert.Equal(t, RoleAdmin, admin.Role)
	assert.Equal(t, "admin@delcom.org", admin.Email)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte("admin")))

	var member User
	assert.NoError(t, db.Where("username = ?", "member").First(&member).Error)
	assert.Equal(t, RoleMember, member.Role)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(member.Password), []byte("member")))

	// Seeding again (default config from environment, then a nil config) is idempotent
	assert.NoError(t, Seed(db))
	assert.NoError(t, Seed(db, nil))

	var userCount int64
	db.Model(&User{}).Count(&userCount)
	assert.Equal(t, int64(2), userCount)
}

func TestSeed_CustomAdminCredentials(t *testing.T) {
	db, err := ConnectTest()
	assert.NoError(t, err)

	assert.NoError(t, Seed(db, &config.Config{AdminUsername: "boss", AdminPassword: "s3cret"}))

	var boss User
	assert.NoError(t, db.Where("username = ?", "boss").First(&boss).Error)
	assert.Equal(t, RoleAdmin, boss.Role)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(boss.Password), []byte("s3cret")))
}

func TestSeed_Errors(t *testing.T) {
	// bcrypt refuses passwords longer than 72 bytes
	db, err := ConnectTest()
	assert.NoError(t, err)
	err = Seed(db, &config.Config{AdminUsername: "admin", AdminPassword: strings.Repeat("x", 73)})
	assert.Error(t, err)

	// Missing users table -> lookup error that is not "record not found"
	db2, err := ConnectTest()
	assert.NoError(t, err)
	assert.NoError(t, db2.Migrator().DropTable(&Post{}, &User{}))
	err = Seed(db2, &config.Config{AdminUsername: "admin", AdminPassword: "admin"})
	assert.Error(t, err)
}

func TestPostModel_PersistsWithAuthor(t *testing.T) {
	db, err := ConnectTest()
	assert.NoError(t, err)

	u := User{Username: "writer", Email: "writer@example.com", Password: "x", Role: RoleMember}
	assert.NoError(t, db.Create(&u).Error)

	p := Post{UserID: u.ID, Title: "Halo", Slug: "halo", Category: DefaultCategory, Content: "# Halo"}
	assert.NoError(t, db.Create(&p).Error)
	assert.False(t, p.IsPublished)
	assert.Nil(t, p.PublishedAt)

	// slug is unique
	dup := Post{UserID: u.ID, Title: "Halo 2", Slug: "halo", Category: DefaultCategory}
	assert.Error(t, db.Create(&dup).Error)

	assert.Contains(t, PostCategories, DefaultCategory)
	assert.Len(t, PostCategories, 6)
}

func TestConnect_Sqlite(t *testing.T) {
	cfg := &config.Config{
		DBConnection: "sqlite",
		DBDatabase:   ":memory:",
	}
	db, err := Connect(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, db)

	assert.NoError(t, Migrate(db))
}

func TestConnect_Postgres_Invalid(t *testing.T) {
	// Connecting to a non-existent postgres port must return an error
	cfg := &config.Config{
		DBConnection: "pgsql",
		DBHost:       "127.0.0.1",
		DBPort:       "54399",
		DBDatabase:   "invalid_db",
		DBUsername:   "invalid_user",
		DBPassword:   "invalid_pass",
		DBSSLMode:    "disable",
	}
	db, err := Connect(cfg)
	assert.Error(t, err)
	assert.Nil(t, db)
}
