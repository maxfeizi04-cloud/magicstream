package handler

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/maxfeizi04-cloud/magicstream/internal/model"
	"github.com/maxfeizi04-cloud/magicstream/internal/store/file"
	"github.com/maxfeizi04-cloud/magicstream/internal/store/pg"
	"github.com/maxfeizi04-cloud/magicstream/internal/util"
)

// UploadHandler 处理视频文件上传的 HTTP 请求。
type UploadHandler struct {
	videoStore pg.VideoStore
	fileStore  *file.FileStore
}

// NewUploadHandler 创建 UploadHandler 实例。
func NewUploadHandler(videoStore pg.VideoStore, fileStore *file.FileStore) *UploadHandler {
	return &UploadHandler{
		videoStore: videoStore,
		fileStore:  fileStore,
	}
}

// HandleUpload 处理视频文件上传请求
// 路由: POST /api/v1/videos/:id/upload
// 需要: multipart/form-data，字段名为 "file"
// 需要: Authorization header（由 AuthMiddleware 保证）
func (h *UploadHandler) HandleUpload(c *gin.Context) {
	// 1. 获取 video_id
	videoIDStr := c.Param("id")
	videoID, err := uuid.Parse(videoIDStr)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "无效视频 ID 格式",
		})
		return
	}

	// 2. 获取当前用户 ID
	userIDRaw := c.Request.Context().Value("userid")
	if userIDRaw == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "请先完成认证",
		})
		return
	}
	userID, ok := userIDRaw.(uuid.UUID)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "无效认证上下文",
		})
		return
	}

	// 3. 查询视频记录,验证所有权
	// 只有视频的上传者才能上传文件到此视频记录
	video, err := h.videoStore.GetByID(c.Request.Context(), videoID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": "视频不存在",
		})
		return
	}

	if video.UserID != userID {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": "只能编辑本人的视频内容",
		})
		return
	}

	// 只有 uploading 状态的视频才能接收新文件上传
	if video.Status != model.VideoStatusUploading {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{
			"error": fmt.Errorf("视频正在: %s, 不能生成", video.Status),
		})
		return
	}

	// 4. 获取生成文件
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "表格数据中缺少文件字段“file",
		})
		return
	}

	// 5. 打开文件流病检测类型
	uploadedFile, err := fileHeader.Open()
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "未能打开上传的文件",
		})
		return
	}
	defer uploadedFile.Close()

	// 6. Magic Bytes 类型检测
	contentType, err := util.DetectContentType(uploadedFile)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("invalid file type: %v", err),
		})
		return
	}
	// 重新打开文件（因为 DetectContentType 消费了前 512 字节）
	// multipart.FileHeader.Open() 每次返回从头开始的 reader
	uploadedFile.Close()
	uploadedFile, err = fileHeader.Open()
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "failed to re-open uploaded file",
		})
		return
	}
	defer uploadedFile.Close()

	// 7. 安全保存文件到磁盘
	ext := contentTypeToExtension(contentType)

	// 保存目录：data/uploads/<video_id>/
	// 文件名: original.<ext>
	dir := filepath.Join("uploads", videoIDStr)
	fileName := "original" + ext

	savePath, writtenBytes, err := h.fileStore.SaveUpload(
		c.Request.Context(),
		uploadedFile,
		dir,
		fileName,
	)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Errorf("文件保存失败: %v", err),
		})
		return
	}

	// 8. 更新数据库状态
	video.Status = model.VideoStatusUploading
	video.FileName = stripPath(fileHeader.Filename)
	video.FileSize = writtenBytes

	// 更新数据库记录
	if err := h.videoStore.Update(c.Request.Context(), video); err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "文件保存但未能更新数据库",
		})
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"message":      "文件上传成功",
		"video_id":     videoID,
		"file_name":    video.FileName,
		"file_size":    video.FileSize,
		"content_type": contentType,
		"save_path":    savePath,
	})
}

// contentTypeToExtension 根据 MIME 类型返回文件扩展名（包含前导点）
// 注意: 这里不用客户端上传的文件名扩展名，而是根据 Magic Bytes 检测出的真实类型决定
// 杜绝文件名后缀伪造攻击
func contentTypeToExtension(contentType string) string {
	switch contentType {
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	case "video/x-flv":
		return ".flv"
	case "video/x-matroska":
		return ".mkv"
	case "video/x-msvideo":
		return ".mav"
	default:
		return ".bin"
	}
}

// stripPath 移除文件名中的路径部分，只保留纯文件名。
// 浏览器上传的文件名可能带路径（如 C:\Users\...\video.mp4 或 /home/user/video.mp4），
// 需要提取纯文件名。
func stripPath(filename string) string {
	// 处理 Windows 和 Unix 两种路径分隔符
	filename = filepath.Base(filename)
	// 替换 Windows 反斜杠
	filename = strings.ReplaceAll(filename, "\\", "/")
	return filepath.Base(filename)
}
