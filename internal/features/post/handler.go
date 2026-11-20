package post

import (
	"errors"
	"net/http"
	"strconv"

	"app/internal/response"
	"github.com/gin-gonic/gin"
)

type PostHandler struct {
	service PostService
}

func NewPostHandler(service PostService) *PostHandler {
	return &PostHandler{service: service}
}

func currentUserID(c *gin.Context) (uint, bool) {
	val, exists := c.Get("userID")
	if !exists {
		response.Fail(c, http.StatusUnauthorized, "Unauthorized")
		return 0, false
	}
	return val.(uint), true
}

func parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "Invalid ID parameter")
		return 0, false
	}
	return uint(id), true
}

// writeError maps service errors to the standard response envelope.
func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		response.Fail(c, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrUserNotFound):
		response.Fail(c, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrInvalidCategory), errors.Is(err, ErrInvalidInput):
		response.Fail(c, http.StatusBadRequest, err.Error())
	default:
		response.Error(c, http.StatusInternalServerError, "Internal server error")
	}
}

// ---------- Public ----------

func (h *PostHandler) GetCategories(c *gin.Context) {
	response.Success(c, http.StatusOK, "Categories retrieved successfully", h.service.GetCategories())
}

func (h *PostHandler) GetPublicPosts(c *gin.Context) {
	posts, err := h.service.GetPublicPosts(c.Query("q"), c.Query("category"))
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Posts retrieved successfully", posts)
}

func (h *PostHandler) GetPublicPostBySlug(c *gin.Context) {
	post, err := h.service.GetPublicPostBySlug(c.Param("slug"))
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Post retrieved successfully", post)
}

// ---------- Author ----------

func (h *PostHandler) GetMyPosts(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	posts, err := h.service.GetMyPosts(userID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Posts retrieved successfully", posts)
}

func (h *PostHandler) CreatePost(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var req CreatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	post, err := h.service.CreatePost(userID, req)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Post created successfully", post)
}

func (h *PostHandler) GetStats(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	stats, err := h.service.GetStats(userID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Stats retrieved successfully", stats)
}

func (h *PostHandler) GetPostByID(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}

	post, err := h.service.GetMyPost(userID, id)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Post retrieved successfully", post)
}

func (h *PostHandler) UpdatePost(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}

	var req UpdatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	post, err := h.service.UpdatePost(userID, id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Post updated successfully", post)
}

func (h *PostHandler) TogglePublish(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}

	post, err := h.service.TogglePublish(userID, id)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Post publication status updated successfully", post)
}

func (h *PostHandler) DeletePost(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}

	if err := h.service.DeletePost(userID, id); err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Post deleted successfully")
}

// ---------- Admin ----------

func (h *PostHandler) AdminGetUsers(c *gin.Context) {
	users, err := h.service.GetAllUsers()
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Users retrieved successfully", users)
}

func (h *PostHandler) AdminGetPosts(c *gin.Context) {
	posts, err := h.service.GetAllPosts()
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Posts retrieved successfully", posts)
}

func (h *PostHandler) AdminGetUserPosts(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	posts, err := h.service.GetUserPosts(id)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "User posts retrieved successfully", posts)
}
