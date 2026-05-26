package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maxfeizi04-cloud/magicstream/internal/model"
)

// VideoStore defines all operations for video persistence.
type VideoStore interface {
	// Create 创建一条视频记录,返回完整的 Video (含数据库生成的 id,时间戳)
	Create(ctx context.Context, video *model.Video) error

	// GetByID 按主键查询单条视频
	GetByID(ctx context.Context, id uuid.UUID) (*model.Video, error)

	// List 分页查询视频列表,同时返回总数
	List(ctx context.Context, params model.ListVideoParams) ([]*model.Video, int, error)

	// Update 更新视频的可编辑字段
	Update(ctx context.Context, video *model.Video) error

	// Delete 删除一条视频记录
	Delete(ctx context.Context, id uuid.UUID) error

	// IncrementViewCount 原子递增播放计数
	IncrementViewCount(ctx context.Context, id uuid.UUID) error

	// UpdateStatus 更新视频状态
	UpdateStatus(ctx context.Context, id uuid.UUID, status model.VideoStatus) error

	// UpdateMeta 更新视频元数据
	UpdateMeta(ctx context.Context, id uuid.UUID, duration float64, width, height int) error
}

// videoStore 是 VideoStore 的 pgx 实现
type videoStore struct {
	pool *pgxpool.Pool
}

// NewVideoStore 创建 VideoStore 实例
func NewVideoStore(pool *pgxpool.Pool) VideoStore {
	return &videoStore{pool: pool}
}

// Create 插入一条视频记录
// 使用 INSERT ... RETURNING 语法，一次往返拿到数据库生成的 id、created_at、updated_at
// 无需插入后再查询
func (s *videoStore) Create(ctx context.Context, video *model.Video) error {
	// 如果调用方未提供 ID,由服务端生成
	// 这样做的原因是：服务端生成 UUID 可以保证幂等性——客户端可以带 idempotency key 重试
	if video.ID == uuid.Nil {
		video.ID = uuid.New()
	}

	query := `
		INSERT INTO videos (id, user_id, title, description, file_name, file_size, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, title, description, file_name, file_size, 
		duration, width, height, status, view_count, created_at, updated_at`

	row := s.pool.QueryRow(ctx, query,
		video.ID,
		video.UserID,
		video.Title,
		video.Description,
		video.FileName,
		video.FileSize,
		video.Status)

	return row.Scan(
		&video.ID,
		&video.UserID,
		&video.Title,
		&video.Description,
		&video.FileName,
		&video.FileSize,
		&video.Duration,
		&video.Width,
		&video.Height,
		&video.Status,
		&video.ViewCount,
		&video.CreatedAt,
		&video.UpdatedAt)
}

// GetByID 安主键查询单条视频
func (s *videoStore) GetByID(ctx context.Context, id uuid.UUID) (*model.Video, error) {
	query := `
		SELECT id, user_id, title, description, file_name, file_size,
		       duration, width, height, status, view_count, created_at, updated_at
		FROM videos
		WHERE id = $1`

	var v model.Video
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&v.ID,
		&v.UserID,
		&v.Title,
		&v.Description,
		&v.FileName,
		&v.FileSize,
		&v.Duration,
		&v.Width,
		&v.Height,
		&v.Status,
		&v.ViewCount,
		&v.CreatedAt,
		&v.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("video %s: %w", id, model.ErrNotFound("video"))
		}
		return nil, fmt.Errorf("query video by id: %w", err)
	}
	return &v, nil
}

// List 分页查询视频列表
func (s *videoStore) List(ctx context.Context, params model.ListVideoParams) ([]*model.Video, int, error) {
	params.Validate()

	query := `
		SELECT id, user_id, title, description, file_name, file_size,
		duration, width, height, status, view_count, created_at, updated_at,
		COUNT(*) OVER() AS total_count
		FROM videos
		WHERE ($1::uuid IS NULL OR user_id = $1)
			AND ($2::text = '' OR status = $2::text)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`

	rows, err := s.pool.Query(ctx, query,
		params.UserID,
		string(params.Status),
		params.Limit,
		params.Offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query videos list: %w", err)
	}
	defer rows.Close()

	var videos []*model.Video
	var totalCount int
	var hasRows bool

	for rows.Next() {
		hasRows = true
		var v model.Video
		err := rows.Scan(
			&v.ID,
			&v.UserID,
			&v.Title,
			&v.Description,
			&v.FileName,
			&v.FileSize,
			&v.Duration,
			&v.Width,
			&v.Height,
			&v.Status,
			&v.ViewCount,
			&v.CreatedAt,
			&v.UpdatedAt,
			&totalCount,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan video row: %w", err)
		}
		videos = append(videos, &v)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("query videos rows: %w", err)
	}

	// 如果结果集为空，totalCount 保持为 0
	// 注意：当 WHERE 条件无匹配时，窗口函数不会执行，需要显式处理
	if !hasRows {
		totalCount = 0
	}
	return videos, totalCount, nil
}

// Update 更新视频的可编辑字段。
// 只允许更新 title 和 description，状态变更通过专门方法（如 FinishUpload）完成，
// 避免业务代码绕过状态机逻辑直接修改 status。
func (s *videoStore) Update(ctx context.Context, video *model.Video) error {
	query := `
		UPDATE videos
		SET title = $1, description = $2, updated_at = NOW()
		WHERE id = $3
		RETURNING id, user_id, title, description, file_name, file_size,
		          duration, width, height, status, view_count, created_at, updated_at`

	row := s.pool.QueryRow(ctx, query,
		video.Title,
		video.Description,
		video.ID,
	)

	return row.Scan(
		&video.ID,
		&video.UserID,
		&video.Title,
		&video.Description,
		&video.FileName,
		&video.FileSize,
		&video.Duration,
		&video.Width,
		&video.Height,
		&video.Status,
		&video.ViewCount,
		&video.CreatedAt,
		&video.UpdatedAt,
	)
}

// UpdateStatus 更新视频状态。
// 独立于 Update 方法，因为状态变更由转码管线触发而非用户编辑触发，
// 且状态变更通常伴随其他字段更新（如 duration, width, height）。
func (s *videoStore) UpdateStatus(ctx context.Context, id uuid.UUID, status model.VideoStatus) error {
	query := `UPDATE videos SET status = $1, updated_at = NOW() WHERE id = $2`
	tag, err := s.pool.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("update video status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("video %s: %w", id, model.ErrNotFound("video"))
	}
	return nil
}

// UpdateMeta 更新 ffprobe 提取的元数据（时长、分辨率）。
// 此方法在上传完成、probe 执行后调用。
func (s *videoStore) UpdateMeta(ctx context.Context, id uuid.UUID, duration float64, width, height int) error {
	query := `
		UPDATE videos
		SET duration = $1, width = $2, height = $3, updated_at = NOW()
		WHERE id = $4`
	tag, err := s.pool.Exec(ctx, query, duration, width, height, id)
	if err != nil {
		return fmt.Errorf("update video meta: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("video %s: %w", id, model.ErrNotFound("video"))
	}
	return nil
}

// Delete 删除视频记录（仅数据库，不涉及文件系统）
// 注意: 调用方应先在需要时通过 GetByID 获取视频信息，再调用此方法删除记录
// 磁盘文件的清理需要由调用方（服务层）负责
func (s *videoStore) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM videos WHERE id = $1`
	tag, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete video: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("video %s: %w", id, model.ErrNotFound("video"))
	}
	return nil
}

// IncrementViewCount 原子递增播放计数
func (s *videoStore) IncrementViewCount(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE videos SET view_count = view_count + 1 WHERE id = $1`
	tag, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("increment video view count: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("video %s: %w", id, model.ErrNotFound("video"))
	}
	return nil
}

// 确保编译期检查 videoStore 实现了 VideoStore 接口。
// 如果接口新增方法而这里没实现，编译就会报错，而不是运行时才发现。
var _ VideoStore = (*videoStore)(nil)
