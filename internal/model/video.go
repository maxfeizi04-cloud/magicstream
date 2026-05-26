package model

import (
	"time"

	"github.com/google/uuid"
)

// VideoStatus 表示视频在转码管线的生命周期状态
// 使用 string 类型的别名而非裸 string,编译器可检查常量拼写,防止散落魔法字符串
type VideoStatus string

const (
	// VideoStatusUploading -- 视频记录已创建但文件尚未上传完毕
	VideoStatusUploading VideoStatus = "uploading"

	// VideoStatusTranscoding -- 文件上传完毕, ffmpeg 正在执行转码
	VideoStatusTranscoding VideoStatus = "transcoding"

	// VideoStatusReady —— 所有分辨率转码产物就绪，可供播放
	VideoStatusReady VideoStatus = "ready"

	// VideoStatusFailed —— 上传或转码过程发生不可恢复错误
	VideoStatusFailed VideoStatus = "failed"
)

// IsPlayable 判断视频当前状态是否允许播放
func (s VideoStatus) IsPlayable() bool {
	return s == VideoStatusReady
}

// Video 是视频资源的领域模型,对应数据库 videos 表
type Video struct {
	ID          uuid.UUID   `json:"id"`          // [创建时] 数据库自动生成
	UserID      uuid.UUID   `json:"user_id"`     // [创建时] 上传者 ID，来自 JWT token
	Title       string      `json:"title"`       // [创建时] 用户填写的视频标题
	Description string      `json:"description"` // [创建时] 用户填写的视频简介
	FileName    string      `json:"file_name"`   // [上传后] 原始上传文件名（不含路径）
	FileSize    int64       `json:"file_size"`   // [上传后] 文件大小（字节）
	Duration    float64     `json:"duration"`    // [转码后] 视频时长（秒），ffprobe 提取
	Width       int         `json:"width"`       // [转码后] 视频宽度（像素）
	Height      int         `json:"height"`      // [转码后] 视频高度（像素）
	Status      VideoStatus `json:"status"`      // [创建时] 初始为 uploading，随管线推进更新
	ViewCount   int64       `json:"view_count"`  // [运行时] 累计播放次数，原子递增
	CreatedAt   time.Time   `json:"created_at"`  // [创建时] 数据库自动设置
	UpdatedAt   time.Time   `json:"updated_at"`  // [自动] 每次 UPDATE 时数据库自动更新
}

// CreateVideoRequest 是 POST /api/v1/videos 的请求体
type CreateVideoRequest struct {
	Title       string `json:"title" binding:"required,min=1,max=200"`
	Description string `json:"description" binding:"max=5000"`
}

// UpdateVideoRequest 是 PUT /api/v1/videos/:id 的请求体
type UpdateVideoRequest struct {
	Title       *string `json:"title" binding:"required,min=1,max=200"`
	Description *string `json:"description" binding:"max=5000"`
}

// VideoResponse 是对外暴露的视频信息结构
type VideoResponse struct {
	ID          uuid.UUID   `json:"id"`
	UserID      uuid.UUID   `json:"user_id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Duration    float64     `json:"duration"`
	Width       int         `json:"width"`
	Height      int         `json:"height"`
	Status      VideoStatus `json:"status"`
	ViewCount   int64       `json:"view_count"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// VideoListResponse 是视频列表接口的响应结构
// Total 字段用于前端分页器显示总页数
type VideoListResponse struct {
	Videos []Video `json:"videos"`
	Total  int     `json:"total"`
}

// ToResponse 将领域模型转换对外响应结构
func (v *Video) ToResponse() VideoResponse {
	return VideoResponse{
		ID:          v.ID,
		UserID:      v.UserID,
		Title:       v.Title,
		Description: v.Description,
		Duration:    v.Duration,
		Width:       v.Width,
		Height:      v.Height,
		Status:      v.Status,
		ViewCount:   v.ViewCount,
		CreatedAt:   v.CreatedAt,
		UpdatedAt:   v.UpdatedAt,
	}
}

// ListVideoParams 封装视频列表查询的所有过滤与分页参数
type ListVideoParams struct {
	UserID *uuid.UUID  // nil 表示不过滤用户,返回全部视频
	Status VideoStatus // 空字符串（VideoStatus("")）表示不过滤状态，返回所有状态的视频
	Offset int         // 分页偏移量（从 0 开始）
	Limit  int         // 每页条数（建议 20，最大 100）
}

// Validate 对分页参数做合法性校验,防止恶意请求
func (p *ListVideoParams) Validate() {
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 20
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
}

// IsEmpty 判断 Status 是否为空（不过滤状态）
func (s VideoStatus) IsEmpty() bool {
	return string(s) == ""
}
