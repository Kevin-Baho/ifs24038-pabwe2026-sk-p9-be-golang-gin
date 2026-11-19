package ai

import (
	"errors"
	"net/http"

	"app/internal/response"
	"github.com/gin-gonic/gin"
)

type AiHandler struct {
	service AiService
}

func NewAiHandler(service AiService) *AiHandler {
	return &AiHandler{service: service}
}

func (h *AiHandler) GeneratePost(c *gin.Context) {
	var req GeneratePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	res, err := h.service.GeneratePost(req)
	if err != nil {
		if errors.Is(err, ErrPromptRequired) {
			response.Fail(c, http.StatusBadRequest, err.Error())
		} else {
			response.Error(c, http.StatusInternalServerError, err.Error())
		}
		return
	}

	response.Success(c, http.StatusOK, "Blog post generated successfully", res)
}
