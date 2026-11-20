package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"app/internal/database"
	"app/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const testSecret = "test-jwt-secret"

func init() {
	gin.SetMode(gin.TestMode)
}

type envelope struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type stubRepo struct {
	UserRepository
	createErr error
	updateErr error
}

func (s *stubRepo) Create(u *database.User) error {
	if s.createErr != nil {
		return s.createErr
	}
	return s.UserRepository.Create(u)
}

func (s *stubRepo) Update(u *database.User) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	return s.UserRepository.Update(u)
}

func setup(t *testing.T) (*gin.Engine, *gorm.DB, UserRepository, AuthService) {
	t.Helper()
	db, err := database.ConnectTest()
	require.NoError(t, err)

	repo := NewUserRepository(db)
	service := NewAuthService(repo, testSecret)
	handler := NewAuthHandler(service)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		if v := c.GetHeader("X-Test-User"); v != "" {
			id, _ := strconv.ParseUint(v, 10, 32)
			c.Set("userID", uint(id))
		}
		c.Next()
	})
	r.POST("/register", handler.Register)
	r.POST("/login", handler.Login)
	r.POST("/logout", handler.Logout)
	r.GET("/profile", handler.GetProfile)
	r.PUT("/profile", handler.UpdateProfile)
	return r, db, repo, service
}

func do(r http.Handler, method, path, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func parse(t *testing.T, w *httptest.ResponseRecorder) envelope {
	t.Helper()
	var e envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &e))
	return e
}

func registerUser(t *testing.T, r http.Handler, username, email string) AuthResponse {
	t.Helper()
	w := do(r, http.MethodPost, "/register", `{"username":"`+username+`","email":"`+email+`","password":"password123"}`)
	require.Equal(t, http.StatusCreated, w.Code)
	var res AuthResponse
	require.NoError(t, json.Unmarshal(parse(t, w).Data, &res))
	return res
}

func TestAuth_RegisterAndLogin_Success(t *testing.T) {
	r, _, repo, _ := setup(t)

	// Register with role "admin"
	w := do(r, http.MethodPost, "/register",
		`{"username":"adminuser","email":"admin@example.com","password":"password123","role":"admin"}`)
	assert.Equal(t, http.StatusCreated, w.Code)
	var regAdmin AuthResponse
	require.NoError(t, json.Unmarshal(parse(t, w).Data, &regAdmin))
	assert.Equal(t, database.RoleAdmin, regAdmin.User.Role)

	// Register with default role or invalid role: becomes "member"
	w = do(r, http.MethodPost, "/register",
		`{"username":"johndoe","email":"John@Example.com","password":"password123","role":"superman"}`)
	assert.Equal(t, http.StatusCreated, w.Code)
	env := parse(t, w)
	assert.Equal(t, "success", env.Status)

	var reg AuthResponse
	require.NoError(t, json.Unmarshal(env.Data, &reg))
	assert.Equal(t, "johndoe", reg.User.Username)
	assert.Equal(t, "john@example.com", reg.User.Email)
	assert.Equal(t, database.RoleMember, reg.User.Role)
	assert.NotEmpty(t, reg.Token)

	// Token carries the right claims and is signed with the secret
	claims := &middleware.Claims{}
	tok, err := jwt.ParseWithClaims(reg.Token, claims, func(*jwt.Token) (interface{}, error) {
		return []byte(testSecret), nil
	})
	require.NoError(t, err)
	assert.True(t, tok.Valid)
	assert.Equal(t, reg.User.ID, claims.UserID)
	assert.Equal(t, database.RoleMember, claims.Role)

	// Stored in DB with a bcrypt hash
	user, err := repo.FindByUsernameOrEmail("johndoe")
	require.NoError(t, err)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(user.Password), []byte("password123")))

	// Login by username
	wLogin := do(r, http.MethodPost, "/login", `{"username":"johndoe","password":"password123"}`)
	assert.Equal(t, http.StatusOK, wLogin.Code)
	var login AuthResponse
	require.NoError(t, json.Unmarshal(parse(t, wLogin).Data, &login))
	assert.NotEmpty(t, login.Token)

	// Login by e-mail (case-insensitive)
	wEmail := do(r, http.MethodPost, "/login", `{"username":"JOHN@example.COM","password":"password123"}`)
	assert.Equal(t, http.StatusOK, wEmail.Code)

	// Logout
	wLogout := do(r, http.MethodPost, "/logout", "")
	assert.Equal(t, http.StatusOK, wLogout.Code)
	logout := parse(t, wLogout)
	assert.Equal(t, "success", logout.Status)
	assert.Empty(t, logout.Data)
}

func TestAuth_Register_Duplicate_And_Invalid(t *testing.T) {
	r, _, _, _ := setup(t)

	registerUser(t, r, "alice", "alice@example.com")

	// Duplicate username
	w2 := do(r, http.MethodPost, "/register", `{"username":"alice","email":"other@example.com","password":"password123"}`)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
	assert.Equal(t, "username already registered", parse(t, w2).Message)

	// Duplicate e-mail with a different username
	w3 := do(r, http.MethodPost, "/register", `{"username":"alice2","email":"ALICE@example.com","password":"password123"}`)
	assert.Equal(t, http.StatusBadRequest, w3.Code)
	assert.Equal(t, "email already registered", parse(t, w3).Message)

	// Blank username passes binding but is rejected by the service
	w4 := do(r, http.MethodPost, "/register", `{"username":"   ","email":"blank@example.com","password":"password123"}`)
	assert.Equal(t, http.StatusBadRequest, w4.Code)

	// Invalid JSON, invalid e-mail, short and too-long passwords
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, "/register", "{invalid-json").Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, "/register", `{"username":"x","email":"not-an-email","password":"password123"}`).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, "/register", `{"username":"x","email":"x@example.com","password":"123"}`).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, "/register",
		`{"username":"x","email":"x@example.com","password":"`+strings.Repeat("a", 73)+`"}`).Code)
}

func TestAuth_Login_Failures(t *testing.T) {
	r, _, _, _ := setup(t)

	// Unknown user
	w1 := do(r, http.MethodPost, "/login", `{"username":"nonexistent","password":"password123"}`)
	assert.Equal(t, http.StatusUnauthorized, w1.Code)
	assert.Equal(t, "invalid credentials", parse(t, w1).Message)

	// Wrong password
	registerUser(t, r, "bob", "bob@example.com")
	w2 := do(r, http.MethodPost, "/login", `{"username":"bob","password":"wrongpassword"}`)
	assert.Equal(t, http.StatusUnauthorized, w2.Code)

	// Invalid JSON
	w3 := do(r, http.MethodPost, "/login", "{invalid")
	assert.Equal(t, http.StatusBadRequest, w3.Code)
}

func TestAuth_Profile(t *testing.T) {
	r, _, _, _ := setup(t)
	user := registerUser(t, r, "charlie", "charlie@example.com").User
	other := registerUser(t, r, "dave", "dave@example.com").User
	id := strconv.Itoa(int(user.ID))

	// GetProfile
	w1 := do(r, http.MethodGet, "/profile", "", "X-Test-User", id)
	assert.Equal(t, http.StatusOK, w1.Code)
	var profile UserResponse
	require.NoError(t, json.Unmarshal(parse(t, w1).Data, &profile))
	assert.Equal(t, "charlie", profile.Username)

	// Missing user in context
	assert.Equal(t, http.StatusUnauthorized, do(r, http.MethodGet, "/profile", "").Code)
	assert.Equal(t, http.StatusUnauthorized, do(r, http.MethodPut, "/profile", "{}").Code)

	// Unknown user id
	assert.Equal(t, http.StatusNotFound, do(r, http.MethodGet, "/profile", "", "X-Test-User", "99999").Code)
	assert.Equal(t, http.StatusNotFound, do(r, http.MethodPut, "/profile", `{"email":"x@example.com"}`, "X-Test-User", "88888").Code)

	// UpdateProfile invalid JSON
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPut, "/profile", "{bad-json", "X-Test-User", id).Code)

	// UpdateProfile email + password
	w6 := do(r, http.MethodPut, "/profile", `{"email":"Charlie_Updated@example.com","password":"newpassword123"}`, "X-Test-User", id)
	assert.Equal(t, http.StatusOK, w6.Code)
	var updated UserResponse
	require.NoError(t, json.Unmarshal(parse(t, w6).Data, &updated))
	assert.Equal(t, "charlie_updated@example.com", updated.Email)

	// New password works, old one does not
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, "/login", `{"username":"charlie","password":"newpassword123"}`).Code)
	assert.Equal(t, http.StatusUnauthorized, do(r, http.MethodPost, "/login", `{"username":"charlie","password":"password123"}`).Code)

	// Only password, only e-mail, same e-mail (no-op), empty body
	assert.Equal(t, http.StatusOK, do(r, http.MethodPut, "/profile", `{"password":"another-pass-456"}`, "X-Test-User", id).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPut, "/profile", `{"email":"only_email@example.com"}`, "X-Test-User", id).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPut, "/profile", `{"email":"only_email@example.com"}`, "X-Test-User", id).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPut, "/profile", `{}`, "X-Test-User", id).Code)

	// E-mail already used by another author
	wTaken := do(r, http.MethodPut, "/profile", `{"email":"dave@example.com"}`, "X-Test-User", id)
	assert.Equal(t, http.StatusBadRequest, wTaken.Code)
	assert.Equal(t, "email already registered", parse(t, wTaken).Message)
	assert.NotEqual(t, user.ID, other.ID)

	// Validation: bad e-mail / short password
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPut, "/profile", `{"email":"nope"}`, "X-Test-User", id).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPut, "/profile", `{"password":"123"}`, "X-Test-User", id).Code)
}

func TestAuth_Service_PasswordTooLong(t *testing.T) {
	_, _, _, service := setup(t)
	long := strings.Repeat("p", 73)

	_, err := service.Register(RegisterRequest{Username: "longpass", Email: "long@example.com", Password: long})
	assert.ErrorIs(t, err, ErrPasswordTooLong)

	res, err := service.Register(RegisterRequest{Username: "okpass", Email: "ok@example.com", Password: "password123"})
	require.NoError(t, err)

	_, err = service.UpdateProfile(res.User.ID, UpdateProfileRequest{Password: long})
	assert.ErrorIs(t, err, ErrPasswordTooLong)
}

func TestAuth_Handler_PasswordTooLong_ReturnsBadRequest(t *testing.T) {
	// The handler maps ErrPasswordTooLong to 400 even if it slips past binding.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	writeError(c, ErrPasswordTooLong)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuth_Service_RepositoryFailures(t *testing.T) {
	_, _, repo, _ := setup(t)

	// Create failure
	failCreate := NewAuthService(&stubRepo{UserRepository: repo, createErr: errors.New("insert failed")}, testSecret)
	_, err := failCreate.Register(RegisterRequest{Username: "u1", Email: "u1@example.com", Password: "password123"})
	assert.EqualError(t, err, "insert failed")

	// Update failure
	okSvc := NewAuthService(repo, testSecret)
	res, err := okSvc.Register(RegisterRequest{Username: "u2", Email: "u2@example.com", Password: "password123"})
	require.NoError(t, err)

	failUpdate := NewAuthService(&stubRepo{UserRepository: repo, updateErr: errors.New("update failed")}, testSecret)
	_, err = failUpdate.UpdateProfile(res.User.ID, UpdateProfileRequest{Email: "u2b@example.com"})
	assert.EqualError(t, err, "update failed")

	// The stub delegates to the real repository when no error is configured
	delegating := &stubRepo{UserRepository: repo}
	u := &database.User{Username: "u3", Email: "u3@example.com", Password: "x", Role: database.RoleMember}
	require.NoError(t, delegating.Create(u))
	u.Email = "u3b@example.com"
	require.NoError(t, delegating.Update(u))
}

func TestAuth_DatabaseFailure_ReturnsInternalError(t *testing.T) {
	r, db, _, _ := setup(t)
	user := registerUser(t, r, "erin", "erin@example.com").User
	id := strconv.Itoa(int(user.ID))

	require.NoError(t, db.Migrator().DropTable(&database.Post{}, &database.User{}))

	w1 := do(r, http.MethodPost, "/register", `{"username":"newbie","email":"newbie@example.com","password":"password123"}`)
	assert.Equal(t, http.StatusInternalServerError, w1.Code)
	assert.Equal(t, "error", parse(t, w1).Status)

	w2 := do(r, http.MethodPost, "/login", `{"username":"erin","password":"password123"}`)
	assert.Equal(t, http.StatusInternalServerError, w2.Code)

	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, "/profile", "", "X-Test-User", id).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPut, "/profile", `{"email":"x@example.com"}`, "X-Test-User", id).Code)
}
