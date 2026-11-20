package auth

import (
	"errors"
	"strings"
	"time"

	"app/internal/database"
	"app/internal/middleware"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUsernameTaken      = errors.New("username already registered")
	ErrEmailTaken         = errors.New("email already registered")
	ErrPasswordTooLong    = errors.New("password must not be longer than 72 bytes")
	ErrInvalidInput       = errors.New("username must not be blank")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
)

type AuthService interface {
	Register(req RegisterRequest) (*AuthResponse, error)
	Login(req LoginRequest) (*AuthResponse, error)
	GetProfile(userID uint) (*UserResponse, error)
	UpdateProfile(userID uint, req UpdateProfileRequest) (*UserResponse, error)
}

type authService struct {
	repo      UserRepository
	jwtSecret string
}

func NewAuthService(repo UserRepository, jwtSecret string) AuthService {
	return &authService{repo: repo, jwtSecret: jwtSecret}
}

func mapNotFound(err, notFound error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound
	}
	return err
}

// ensureFree returns `taken` when the identifier (username or email) is already
// in use, nil when it is free, and the raw error for any unexpected DB failure.
func (s *authService) ensureFree(identifier string, taken error) error {
	_, err := s.repo.FindByUsernameOrEmail(identifier)
	if err == nil {
		return taken
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func hashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		// bcrypt only fails here when the password exceeds 72 bytes.
		return "", ErrPasswordTooLong
	}
	return string(hashed), nil
}

func toUserResponse(user *database.User) UserResponse {
	return UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

func (s *authService) buildAuthResponse(user *database.User) *AuthResponse {
	return &AuthResponse{
		Token: s.generateToken(user),
		User:  toUserResponse(user),
	}
}

func (s *authService) Register(req RegisterRequest) (*AuthResponse, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" {
		return nil, ErrInvalidInput
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))

	if err := s.ensureFree(username, ErrUsernameTaken); err != nil {
		return nil, err
	}
	if err := s.ensureFree(email, ErrEmailTaken); err != nil {
		return nil, err
	}

	hashed, err := hashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	role := database.RoleMember
	if strings.EqualFold(strings.TrimSpace(req.Role), database.RoleAdmin) {
		role = database.RoleAdmin
	}

	user := &database.User{
		Username: username,
		Email:    email,
		Password: hashed,
		Role:     role,
	}
	if err := s.repo.Create(user); err != nil {
		return nil, err
	}

	return s.buildAuthResponse(user), nil
}

func (s *authService) Login(req LoginRequest) (*AuthResponse, error) {
	user, err := s.repo.FindByUsernameOrEmail(strings.TrimSpace(req.Username))
	if err != nil {
		return nil, mapNotFound(err, ErrInvalidCredentials)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.buildAuthResponse(user), nil
}

func (s *authService) GetProfile(userID uint) (*UserResponse, error) {
	user, err := s.repo.FindByID(userID)
	if err != nil {
		return nil, mapNotFound(err, ErrUserNotFound)
	}
	res := toUserResponse(user)
	return &res, nil
}

func (s *authService) UpdateProfile(userID uint, req UpdateProfileRequest) (*UserResponse, error) {
	user, err := s.repo.FindByID(userID)
	if err != nil {
		return nil, mapNotFound(err, ErrUserNotFound)
	}

	if req.Email != "" {
		email := strings.ToLower(strings.TrimSpace(req.Email))
		if email != user.Email {
			if err := s.ensureFree(email, ErrEmailTaken); err != nil {
				return nil, err
			}
			user.Email = email
		}
	}

	if req.Password != "" {
		hashed, err := hashPassword(req.Password)
		if err != nil {
			return nil, err
		}
		user.Password = hashed
	}

	if err := s.repo.Update(user); err != nil {
		return nil, err
	}

	res := toUserResponse(user)
	return &res, nil
}

// generateToken signs a 24h HS256 JWT. Signing with a []byte key cannot fail,
// so the error is intentionally discarded.
func (s *authService) generateToken(user *database.User) string {
	claims := &middleware.Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte(s.jwtSecret))
	return signed
}
