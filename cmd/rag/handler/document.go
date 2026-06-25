package handler

import (
	"net/http"
	"strconv"

	"github.com/golllm/cmd/rag/service"

	"github.com/gin-gonic/gin"
)

// DocumentHandler 文档处理器
type DocumentHandler struct {
	svc *service.DocumentService
}

// NewDocumentHandler 创建文档处理器
func NewDocumentHandler(svc *service.DocumentService) *DocumentHandler {
	return &DocumentHandler{svc: svc}
}

// Upload 上传文档
// @Summary 上传文档
// @Description 上传 TXT 或 Markdown 文件，系统会自动切片并向量化
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "文档文件"
// @Param title formData string false "文档标题"
// @Success 200 {object} model.UploadResponse
// @Router /documents/upload [post]
func (h *DocumentHandler) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请上传文件",
			"details": err.Error(),
		})
		return
	}

	// 限制文件大小 50MB
	if file.Size > 50*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "文件大小不能超过 50MB",
		})
		return
	}

	// 获取标题
	title := c.PostForm("title")

	resp, err := h.svc.Upload(file, title)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "上传失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "上传成功",
		"data": resp,
	})
}

// List 列出文档
// @Summary 列出文档
// @Description 获取文档列表
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} model.DocumentListResponse
// @Router /documents/list [get]
func (h *DocumentHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	resp, err := h.svc.List(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "查询失败",
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

// Get 获取文档详情
// @Summary 获取文档详情
// @Description 根据ID获取文档详情和切片列表
// @Produce json
// @Param id path int true "文档ID"
// @Success 200 {object} model.DocumentDetailResponse
// @Router /documents/{id} [get]
func (h *DocumentHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的文档ID",
		})
		return
	}

	resp, err := h.svc.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "文档不存在",
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

// Delete 删除文档
// @Summary 删除文档
// @Description 根据ID删除文档及其所有切片
// @Produce json
// @Param id path int true "文档ID"
// @Success 200 {object} map[string]interface{}
// @Router /documents/{id} [delete]
func (h *DocumentHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的文档ID",
		})
		return
	}

	if err := h.svc.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "删除失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "删除成功",
	})
}