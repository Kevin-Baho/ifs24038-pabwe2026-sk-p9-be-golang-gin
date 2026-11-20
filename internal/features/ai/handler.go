package ai

import (
	"errors"
	"net/http"
	"strings"

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
		response.Fail(c, http.StatusBadRequest, "topic is required")
		return
	}

	topic := strings.TrimSpace(req.Topic)
	if topic == "" {
		topic = strings.TrimSpace(req.Prompt)
	}

	if topic == "" {
		response.Fail(c, http.StatusBadRequest, "topic is required")
		return
	}

	req.Topic = topic
	req.Prompt = topic

	res, err := h.service.GeneratePost(req)
	if err != nil {
		if errors.Is(err, ErrPromptRequired) || errors.Is(err, ErrTopicRequired) {
			response.Fail(c, http.StatusBadRequest, "topic is required")
		} else {
			response.Error(c, http.StatusInternalServerError, err.Error())
		}
		return
	}

	response.Success(c, http.StatusOK, "Blog post generated successfully", res)
}