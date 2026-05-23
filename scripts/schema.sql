-- 1. 用户表(Users)
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,            -- UUID v7 (应用层生成),按时间排序
    username VARCHAR(64) NOT NULL,  -- 用户名(用于登录、展示、@提及)
    email VARCHAR(255) NOT NULL,    -- 邮箱(用于登录、通知、找回密码)
    password_hash VARCHAR(255) NOT NULL,    -- bcrypt 哈希(永远不序列化到 JSON 响应中)
    display_name VARCHAR(100) NOT NULL DEFAULT '', -- 显示名称（可与 username 不同，支持中文）
    avatar_url TEXT NOT NULL DEFAULT '',        -- 头像 URL (可为空,前端显示默认头像)
    role VARCHAR(16) NOT NULL DEFAULT 'user',   -- 角色: user (普通用户) / admin (管理员)
    -- 唯一约束：用户名和邮箱全局唯一
    CONSTRAINT uq_users_username UNIQUE (username),
    CONSTRAINT uq_users_email unique (email)
);

-- 索引: 登录时按邮箱查询是最频繁的操作
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- 索引: 注册时检查用户名是否被使用
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);

-- 2. 视频表
create table IF not exists videos (
    id UUID PRIMARY KEY,    -- UUID v7
    user_id UUID NOT NULL,   -- 上传值 ID
    title VARCHAR(256) NOT NULL DEFAULT '', -- 视频标题(用户编辑)
    description TEXT NOT NULL DEFAULT '',   -- 视频描述(支持长文本)
    duration DOUBLE PRECISION NOT NULL DEFAULT 0, -- 视频时长(秒),ffprobe 提取后填充
    status VARCHAR(16) NOT NULL DEFAULT 'uploading', -- 状态：uploading/transcoding/ready/failed
    cover_url     TEXT         NOT NULL DEFAULT '',   -- 封面缩略图路径（转码时从第 5 秒截取）
    original_file TEXT         NOT NULL DEFAULT '',   -- 原始上传文件的存储路径
    original_size BIGINT       NOT NULL DEFAULT 0,    -- 原始文件大小（字节）
    view_count    BIGINT       NOT NULL DEFAULT 0,    -- 播放次数（用 UPDATE SET view_count = view_count + 1 原子递增）
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    -- 外键: 视频属于一个用户
    -- ON DELETE CASCADE: 用户被删除时，其所有视频也删除
    -- 业务逻辑: 视频文件占用存储空间，用户不存在了就没有人为此付费
    CONSTRAINT fk_videos_user FOREIGN KEY (user_id)
    REFERENCES users(id) ON DELETE CASCADE
);

-- 索引: 按用户查询视频（"我的视频"列表）—— 最常用的查询
CREATE INDEX IF NOT EXISTS idx_videos_user_id ON videos(user_id);

-- 索引: 按状态筛选（"正在转码的视频"、"已就绪的视频"）
CREATE INDEX IF NOT EXISTS idx_videos_status ON videos(status);

-- 索引：按时间倒序（首页展示最新视频）
CREATE INDEX IF NOT EXISTS idx_videos_created_at ON videos(created_at DESC);

-- 复合索引：按用户 + 状态（"我的正在转码中的视频"）
-- 字段顺序：user_id 放前面因为它是等值查询（=），status 放后面因为它是过滤条件
CREATE INDEX IF NOT EXISTS idx_videos_user_status ON videos(user_id, status);

-- 复合索引：按用户 + 时间（"我的视频"按时间排序）
-- 字段顺序：user_id 放前面（等值查询），created_at 放后面（排序字段）
-- PostgreSQL 可以用这个索引同时做 WHERE user_id=$1 过滤和 ORDER BY created_at DESC 排序
CREATE INDEX IF NOT EXISTS idx_videos_user_created ON videos(user_id, created_at DESC);

-- 3. 转码产物表 (video_transcodes)
CREATE TABLE IF NOT EXISTS video_transcodes (
    id            UUID PRIMARY KEY,              -- UUID v7
    video_id      UUID         NOT NULL,         -- 所属视频
    resolution    VARCHAR(10)  NOT NULL,         -- 分辨率标识：360p / 720p / 1080p
    playlist_url  TEXT         NOT NULL DEFAULT '', -- HLS m3u8 文件路径（相对于 data/videos/）
    file_size     BIGINT       NOT NULL DEFAULT 0,  -- 该分辨率转码产物的总大小（字节）
    video_bitrate INT          NOT NULL DEFAULT 0,  -- 视频码率（kbps），用于 ABR 带宽估算
    audio_bitrate INT          NOT NULL DEFAULT 0,  -- 音频码率（kbps）
    width         INT          NOT NULL DEFAULT 0,  -- 视频宽度（像素），如 1920
    height        INT          NOT NULL DEFAULT 0,  -- 视频高度（像素），如 1080
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    -- 外键：转码产物属于一个视频
    -- ON DELETE CASCADE：视频被删除时，其所有转码产物也应删除
    CONSTRAINT fk_transcodes_video FOREIGN KEY (video_id)
    REFERENCES videos(id) ON DELETE CASCADE,

    -- 唯一约束：同一个视频 + 同一个分辨率 只能有一条记录
    CONSTRAINT uq_transcodes_video_res UNIQUE (video_id, resolution)
);

-- 索引：按视频 ID 查询所有转码产物（播放页面展示所有可用分辨率）
CREATE INDEX IF NOT EXISTS idx_transcodes_video_id ON video_transcodes(video_id);

-- 4. 直播流表
CREATE TABLE IF NOT EXISTS live_streams (
    id            UUID PRIMARY KEY,              -- UUID v7
    user_id       UUID         NOT NULL,         -- 创建者（主播）ID
    title         VARCHAR(256) NOT NULL DEFAULT '', -- 直播标题
    description   TEXT         NOT NULL DEFAULT '', -- 直播简介
    stream_key    VARCHAR(128) NOT NULL,         -- 推流密钥（64 字符 hex），唯一
    status        VARCHAR(16)  NOT NULL DEFAULT 'waiting', -- 直播状态：waiting / live / ended
    started_at    TIMESTAMPTZ,                   -- 直播开始时间（NULL = 尚未开播）
    ended_at      TIMESTAMPTZ,                   -- 直播结束时间（NULL = 尚未结束）
    peak_viewers  INT          NOT NULL DEFAULT 0,   -- 峰值观众数（直播结束后汇总）
    total_views   BIGINT       NOT NULL DEFAULT 0,   -- 累计观看人次
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    -- 外键：直播间属于一个用户
    CONSTRAINT fk_live_user FOREIGN KEY (user_id)
    REFERENCES users(id) ON DELETE CASCADE,

    -- 唯一约束：stream_key 全局唯一
    CONSTRAINT uq_live_stream_key UNIQUE (stream_key)
);

-- 索引：查询"正在直播的所有直播间"（首页直播列表）
CREATE INDEX IF NOT EXISTS idx_live_status ON live_streams(status);

-- 索引：推流时通过 stream_key 定位直播间
-- 这是 RTMP publish 处理中最频繁的查询——每次推流连接都要查一次
CREATE INDEX IF NOT EXISTS idx_live_stream_key ON live_streams(stream_key);

-- 索引：创作者查看自己的直播间列表
CREATE INDEX IF NOT EXISTS idx_live_user_created ON live_streams(user_id, created_at DESC);