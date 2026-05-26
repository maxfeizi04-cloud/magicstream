package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/maxfeizi04-cloud/magicstream/internal/model"
	"github.com/maxfeizi04-cloud/magicstream/internal/service"
)

// VideoHandler 处理视频 CRUD 的 HTTP 请求。
type VideoHandler struct {
	videoService *service.VideoService
}

// NewVideoHandler 创建 VideoHandler 实例。
func NewVideoHandler(videoService *service.VideoService) *VideoHandler {
	return &VideoHandler{
		videoService: videoService,
	}
}

// HandleCreate 创建视频记录。
//
// POST /api/v1/videos
// Body: {"title": "...", "description": "..."}
//
// 注意：此接口只创建数据库记录，不上传文件。
// 文件上传通过 POST /api/v1/videos/:id/upload 单独完成。
func (h *VideoHandler) HandleCreate(c *gin.Context) {
	// ---- 第 1 步：解析请求体 ----
	// Gin 的 ShouldBindJSON 内部用 json.NewDecoder 解析 JSON，
	// 同时根据 struct tag binding:"required" 等规则自动校验。
	var req model.CreateVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid request: " + err.Error(),
		})
		return
	}

	// ---- 第 2 步：获取当前用户 ID ----
	userID, err := getUserIDFromContext(c)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "authentication required",
		})
		return
	}

	// ---- 第 3 步：调用服务层创建视频 ----
	video, err := h.videoService.CreateVideo(c.Request.Context(), userID, req)
	if err != nil {
		writeAppError(c, err, "failed to create video")
		return
	}

	// ---- 第 4 步：返回响应 ----
	// 使用 ToResponse() 转换，确保不泄露内部字段（如 FileName）
	c.JSON(http.StatusCreated, video.ToResponse())
}

// HandleGet 获取视频详情。
//
// GET /api/v1/videos/:id
//
// Gin 的 :id 参数通过 c.Param("id") 获取。
// 注意：Param 返回值是字符串，需要手动解析为 UUID。
func (h *VideoHandler) HandleGet(c *gin.Context) {
	// ---- 第 1 步：解析路径参数 ----
	videoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid video ID format",
		})
		return
	}

	// ---- 第 2 步：调用服务层 ----
	video, err := h.videoService.GetVideo(c.Request.Context(), videoID)
	if err != nil {
		writeAppError(c, err, "video not found")
		return
	}

	// ---- 第 3 步：返回响应 ----
	c.JSON(http.StatusOK, video.ToResponse())
}

// HandleList 分页获取视频列表。
//
// GET /api/v1/videos?user_id=<uuid>&status=<status>&page=1&size=20
//
// 查询参数说明：
//
//	user_id - 可选，过滤特定用户的视频
//	status  - 可选，过滤特定状态的视频
//	page    - 页码，从 1 开始（默认 1）
//	size    - 每页条数（默认 20，最大 100）
func (h *VideoHandler) HandleList(c *gin.Context) {
	// ---- 第 1 步：解析查询参数 ----
	params := model.ListVideoParams{}

	// 可选的用户过滤
	if userIDStr := c.Query("user_id"); userIDStr != "" {
		uid, err := uuid.Parse(userIDStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "invalid user_id format",
			})
			return
		}
		params.UserID = &uid
	}

	// 可选的状态过滤
	if statusStr := c.Query("status"); statusStr != "" {
		params.Status = model.VideoStatus(statusStr)
	}

	// 分页参数（带默认值）
	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	size := 20
	if sizeStr := c.Query("size"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 {
			size = s
		}
	}

	params.Offset = (page - 1) * size
	params.Limit = size
	params.Validate()

	// ---- 第 2 步：调用服务层 ----
	videos, total, err := h.videoService.ListVideos(c.Request.Context(), params)
	if err != nil {
		writeAppError(c, err, "failed to list videos")
		return
	}

	// ---- 第 3 步：转换响应 ----
	responses := make([]model.VideoResponse, 0, len(videos))
	for i := range videos {
		responses = append(responses, videos[i].ToResponse())
	}

	c.JSON(http.StatusOK, model.VideoListResponse{
		Videos: responses,
		Total:  total,
	})
}

// HandleUpdate 更新视频元数据。
//
// PUT /api/v1/videos/:id
// Body: {"title": "...", "description": "..."}
//
// 支持部分更新：只传需要修改的字段。
// 使用指针类型区分"不传"和"传空字符串"。
func (h *VideoHandler) HandleUpdate(c *gin.Context) {
	// ---- 第 1 步：解析路径参数 ----
	videoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid video ID format",
		})
		return
	}

	// ---- 第 2 步：解析请求体 ----
	var req model.UpdateVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid request: " + err.Error(),
		})
		return
	}

	// ---- 第 3 步：获取当前用户 ID ----
	userID, err := getUserIDFromContext(c)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "authentication required",
		})
		return
	}

	// ---- 第 4 步：调用服务层 ----
	video, err := h.videoService.UpdateVideo(c.Request.Context(), userID, videoID, req)
	if err != nil {
		writeAppError(c, err, "failed to update video")
		return
	}

	c.JSON(http.StatusOK, video.ToResponse())
}

// HandleDelete 删除视频及其关联文件。
//
// DELETE /api/v1/videos/:id
//
// 前置条件：
//  1. 视频存在
//  2. 操作者是视频的上传者
func (h *VideoHandler) HandleDelete(c *gin.Context) {
	// ---- 第 1 步：解析路径参数 ----
	videoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid video ID format",
		})
		return
	}

	// ---- 第 2 步：获取当前用户 ID ----
	userID, err := getUserIDFromContext(c)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "authentication required",
		})
		return
	}

	// ---- 第 3 步：调用服务层 ----
	if err := h.videoService.DeleteVideo(c.Request.Context(), userID, videoID); err != nil {
		writeAppError(c, err, "failed to delete video")
		return
	}

	// ---- 第 4 步：返回 204 No Content ----
	// RESTful 惯例：DELETE 成功返回 204（无响应体）
	c.Status(http.StatusNoContent)
}

// writeAppError 从错误链中提取 AppError 并返回对应的 HTTP 状态码和消息。
// 如果错误链中不存在 AppError，则返回 500。
func writeAppError(c *gin.Context, err error, fallbackMsg string) {
	var appErr *model.AppError
	if errors.As(err, &appErr) {
		c.AbortWithStatusJSON(appErr.Code, gin.H{"error": appErr.Message})
		return
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": fallbackMsg})
}

type ctxKey string

const ctxUserID ctxKey = "userID"

// getUserIDFromContext 从 Gin context 中提取由 AuthMiddleware 注入的用户 ID。
//
// AuthMiddleware（阶段 2）将 userID 存入 c.Request.Context()：
//
//	ctx = context.WithValue(ctx, "userID", userID)
//
// Gin 的 c.Request.Context() 返回的是 *http.Request 的 context，
// 与标准 Go context 完全兼容，可以在 handler/service/store 层间传递。
func getUserIDFromContext(c *gin.Context) (uuid.UUID, error) {
	// AuthMiddleware 将 userID 注入到 Request 的 context 中
	// 使用 context.Value 模式而非 Gin 的 c.Set/c.Get，
	// 因为 Request.Context() 可以在 handler → service → store 全链路使用
	userIDRaw := c.Request.Context().Value(ctxUserID)
	if userIDRaw == nil {
		return uuid.Nil, fmt.Errorf("userID not found in context")
	}

	userID, ok := userIDRaw.(uuid.UUID)
	if !ok {
		return uuid.Nil, fmt.Errorf("userID has unexpected type: %T", userIDRaw)
	}

	return userID, nil
}
