package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/maxfeizi04-cloud/magicstream/internal/model"
	"github.com/maxfeizi04-cloud/magicstream/internal/store/file"
	"github.com/maxfeizi04-cloud/magicstream/internal/store/pg"
)

// VideoService 封装视频相关的所有业务逻辑
type VideoService struct {
	videoStore pg.VideoStore
	fileStore  *file.FileStore
}

// NewVideoService 创建 VideoService 实例
func NewVideoService(videoStore pg.VideoStore, fileStore *file.FileStore) *VideoService {
	return &VideoService{
		videoStore: videoStore,
		fileStore:  fileStore,
	}
}

// CreateVideo 创建一条新的视频记录
// 前置条件: userID 已通过认证中间件验证
// 后置条件: 数据库中存在一条 status=uploading 的视频记录
func (s *VideoService) CreateVideo(ctx context.Context, userID uuid.UUID, req model.CreateVideoRequest) (*model.Video, error) {
	video := &model.Video{
		ID:          uuid.New(),
		UserID:      userID,
		Title:       req.Title,
		Description: req.Description,
		Status:      model.VideoStatusUploading,
	}
	if err := s.videoStore.Create(ctx, video); err != nil {
		return nil, fmt.Errorf("创建视频失败: %w", err)
	}
	return video, nil
}

// GetVideo 获取单条视频详情
func (s *VideoService) GetVideo(ctx context.Context, videoID uuid.UUID) (*model.Video, error) {
	video, err := s.videoStore.GetByID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	return video, nil
}

// ListVideos 分页获取视频列表
//
// 支持可选过滤条件：
//   - userID != nil → 只返回该用户的视频（"我的视频"）
//   - userID == nil → 返回全部视频（首页）
//   - status != "" → 按状态过滤（管理面板）
func (s *VideoService) ListVideos(ctx context.Context, params model.ListVideoParams) ([]*model.Video, int, error) {
	params.Validate()
	videos, total, err := s.videoStore.List(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("list videos: %w", err)
	}
	return videos, total, nil
}

// UpdateVideo 更新视频的元数据（标题、描述）
func (s *VideoService) UpdateVideo(ctx context.Context, userID, videoID uuid.UUID, req model.UpdateVideoRequest) (*model.Video, error) {
	// 获取现有记录
	video, err := s.videoStore.GetByID(ctx, videoID)
	if err != nil {
		return nil, err
	}

	// 所有权校验
	if video.UserID != userID {
		return nil, fmt.Errorf("user %s does not own video %s: %w", userID, videoID, model.ErrForbidden())
	}
	// 部分更新: 只修改提供了值的字段
	if req.Title != nil {
		video.Title = *req.Title
	}
	if req.Description != nil {
		video.Description = *req.Description
	}

	if err := s.videoStore.Update(ctx, video); err != nil {
		return nil, fmt.Errorf("update video: %w", err)
	}

	return video, nil
}
func (s *VideoService) DeleteVideo(ctx context.Context, userID, videoID uuid.UUID) error {
	// 第 1 步：获取视频记录（确认存在 + 所有权校验）
	video, err := s.videoStore.GetByID(ctx, videoID)
	if err != nil {
		return err
	}

	// 所有权校验：只有上传者可以删除视频
	if video.UserID != userID {
		return fmt.Errorf("user %s does not own video %s: %w", userID, videoID, model.ErrForbidden())
	}

	// 第 2 步：从数据库删除
	if err := s.videoStore.Delete(ctx, videoID); err != nil {
		return fmt.Errorf("delete video from database: %w", err)
	}

	// 第 3 步：清理磁盘文件（尽力而为，失败不影响 API 返回）
	// 目录路径基于 video_id，而非用户输入，所以不存在路径穿越风险
	videoDir := fmt.Sprintf("uploads/%s", videoID.String())
	if err := s.fileStore.DeleteDir(videoDir); err != nil {
		// 文件删除失败只记录日志，不向客户端返回错误
		// 原因：从用户角度看视频已经删除成功（数据库无此记录）
		// 遗留文件可以后续通过清理脚本处理
		// 注意：生产环境此处应使用结构化日志记录
		_ = err // 显式忽略错误（生产环境替换为 logger.Warn()）
	}

	return nil
}

// IncrementViewCount 递增视频播放计数。
//
// 在用户开始播放视频时调用（不是播放完毕时）。
// 使用原子 UPDATE 而非 SELECT + UPDATE，避免并发竞态。
func (s *VideoService) IncrementViewCount(ctx context.Context, videoID uuid.UUID) error {
	return s.videoStore.IncrementViewCount(ctx, videoID)
}
