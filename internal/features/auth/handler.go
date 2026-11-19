package auth

import (
	"errors"
	"net/http"

	"app/internal/response"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	service AuthService
}

func NewAuthHandler(service AuthService) *AuthHandler {
	return &AuthHandler{service: service}
}

func currentUserID(c *gin.Context) (uint, bool) {
	val, exists := c.Get("userID")
	if !exists {
		response.Fail(c, http.StatusUnauthorized, "Unauthorized")
		return 0, false
	}
	return val.(uint), true
}

// writeError maps service errors to the standard response envelope.
func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrUserNotFound):
		response.Fail(c, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrInvalidCredentials):
		response.Fail(c, http.StatusUnauthorized, err.Error())
	case errors.Is(err, ErrUsernameTaken), errors.Is(err, ErrEmailTaken),
		errors.Is(err, ErrPasswordTooLong), errors.Is(err, ErrInvalidInput):
		response.Fail(c, http.StatusBadRequest, err.Error())
	default:
		response.Error(c, http.StatusInternalServerError, "Internal server error")
	}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	res, err := h.service.Register(req)
	if err != nil {
		writeError(c, err)
		return
	}

	response.Success(c, http.StatusCreated, "User registered successfully", res)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	res, err := h.service.Login(req)
	if err != nil {
		writeError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Login successful", res)
}

// Logout is stateless: the JWT is discarded client-side.
func (h *AuthHandler) Logout(c *gin.Context) {
	response.Success(c, http.StatusOK, "Logged out successfully")
}

func (h *AuthHandler) GetProfile(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	res, err := h.service.GetProfile(userID)
	if err != nil {
		writeError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Profile retrieved successfully", res)
}

func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	res, err := h.service.UpdateProfile(userID, req)
	if err != nil {
		writeError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Profile updated successfully", res)
}
