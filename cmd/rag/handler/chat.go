package handler

import (
	"net/http"

	"github.com/golllm/cmd/rag/model"
	"github.com/golllm/cmd/rag/service"

	"github.com/gin-gonic/gin"
)

// ChatHandler 聊天处理器
type ChatHandler struct {
	svc *service.RAGService
}

// NewChatHandler 创建聊天处理器
func NewChatHandler(svc *service.RAGService) *ChatHandler {
	return &ChatHandler{svc: svc}
}

// Ask 问答
// @Summary 知识库问答
// @Description 发送问题，系统会从知识库检索相关片段并生成回答
// @Accept json
// @Produce json
// @Param request body model.AskRequest true "问答请求"
// @Success 200 {object} model.AskResponse
// @Router /chat/ask [post]
func (h *ChatHandler) Ask(c *gin.Context) {
	var req model.AskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	if req.Question == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "问题不能为空",
		})
		return
	}

	resp, err := h.svc.Ask(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "问答处理失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
		"data": resp,
	})
}

// History 获取聊天历史
// @Summary 获取聊天历史
// @Description 根据会话ID获取聊天历史记录
// @Produce json
// @Param session_id path string true "会话ID"
// @Success 200 {object} model.ChatHistoryResponse
// @Router /chat/history/{session_id} [get]
func (h *ChatHandler) History(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "会话ID不能为空",
		})
		return
	}

	resp, err := h.svc.GetHistory(sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "会话不存在",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
		"data": resp,
	})
}