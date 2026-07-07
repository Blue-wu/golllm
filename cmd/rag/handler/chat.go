package handler

import (
	"net/http"
	"strconv"

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

// ListSessions 获取会话列表
// @Summary 获取会话列表
// @Description 根据用户ID获取会话列表
// @Produce json
// @Param user_id query string true "用户ID"
// @Param page query int false "页码"
// @Param page_size query int false "每页大小"
// @Success 200 {object} model.SessionListResponse
// @Router /chat/sessions [get]
func (h *ChatHandler) ListSessions(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		userID = "default"
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	resp, err := h.svc.ListSessions(userID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取会话列表失败",
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

// DeleteSession 删除会话
// @Summary 删除会话
// @Description 根据会话ID删除会话
// @Produce json
// @Param session_id path string true "会话ID"
// @Success 200 {object} gin.H
// @Router /chat/sessions/{session_id} [delete]
func (h *ChatHandler) DeleteSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "会话ID不能为空",
		})
		return
	}

	err := h.svc.DeleteSession(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "删除会话失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
	})
}

// ResetSession 重置会话
// @Summary 重置会话
// @Description 重置会话状态，清空上下文
// @Produce json
// @Param session_id path string true "会话ID"
// @Success 200 {object} gin.H
// @Router /chat/sessions/{session_id}/reset [post]
func (h *ChatHandler) ResetSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "会话ID不能为空",
		})
		return
	}

	err := h.svc.ResetSession(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "重置会话失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
	})
}

// StartTask 启动任务式对话
// @Summary 启动任务式对话
// @Description 启动一个多轮任务式对话，如订单查询、退款申请等
// @Produce json
// @Param session_id path string true "会话ID"
// @Param task_type query string true "任务类型: order_query, refund_request"
// @Success 200 {object} gin.H
// @Router /chat/sessions/{session_id}/task/start [post]
func (h *ChatHandler) StartTask(c *gin.Context) {
	sessionID := c.Param("session_id")
	taskType := c.Query("task_type")

	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "会话ID不能为空",
		})
		return
	}

	if taskType == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "任务类型不能为空",
		})
		return
	}

	response, err := h.svc.StartTask(sessionID, taskType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "启动任务失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
		"data": gin.H{
			"session_id": sessionID,
			"task_type":  taskType,
			"response":   response,
		},
	})
}

// CancelTask 取消任务式对话
// @Summary 取消任务式对话
// @Description 取消当前进行中的任务式对话
// @Produce json
// @Param session_id path string true "会话ID"
// @Success 200 {object} gin.H
// @Router /chat/sessions/{session_id}/task/cancel [post]
func (h *ChatHandler) CancelTask(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "会话ID不能为空",
		})
		return
	}

	err := h.svc.CancelTask(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "取消任务失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
	})
}
