package util

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 是应用的根配置结构体,包括所有子系统的配置
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Storage  StorageConfig  `mapstructure:"storage"`
	FFmpeg   FFmpegConfig   `mapstructure:"ffmpeg"`
	JWT      JWTConfig      `mapstructure:"jwt"`
}

// ServerConfig HTTP 和 RTMP 服务器的监听端口和超时配置
type ServerConfig struct {
	HTTPPort     int           `mapstructure:"http_port"`     // HTTP API + 流媒体文件分发端口
	RTMPPort     int           `mapstructure:"rtmp_port"`     // RTMP 推流监听端口 (1935 = RTMP 标准端口)
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`  // HTTP 请求体读取超时，防止慢客户端攻击
	WriteTimeout time.Duration `mapstructure:"write_timeout"` // HTTP 响应写出超时，直播长连接和文件下载需要更长
}

// DatabaseConfig PostgreSQL 连接参数
// 生产环境的密码应通过环境变量注入,不写入 YAML 文件
type DatabaseConfig struct {
	Host         string `mapstructure:"host"`           // 数据库主机地址（IP 或域名）
	Port         int    `mapstructure:"port"`           // 数据库端口（默认 5432）
	User         string `mapstructure:"user"`           // 数据库用户名
	Password     string `mapstructure:"password"`       // 数据库密码——敏感信息，生产环境通过环境
	DBName       string `mapstructure:"dbname"`         // 数据库名称
	SSLMode      string `mapstructure:"sslmode"`        // SSL 模式：disable(开发)/require(生产)/verify-full(严格)
	MaxOpenConns int    `mapstructure:"max_open_conns"` // 连接池最大连接数
	MaxIdleConns int    `mapstructure:"max_idle_conns"` // 连接池最小空闲连接数
}

// DSN 返回 PostgreSQL 连接字符串（Data Source Name）
// 格式：postgres://user:password@host:port/dbname?sslmode=mode
// 这是 pgx 驱动所需的标准连接格式
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.SSLMode)
}

// RedisConfig Redis 连接参数
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`     // Redis 地址，格式 host:port，如 localhost:6379
	Password string `mapstructure:"password"` // Redis 认证密码（留空表示无密码）
	DB       int    `mapstructure:"db"`       // Redis 数据库编号（0-15）
}

// StorageConfig 文件存储配置
// data_dir 下的子目录结构:
// data/videos/	-- VOD 转码产物(m3u8 + TS 分片)
// data/live/	-- 直播 HLS 临时分片 (直播结束后自动清理)
// data/uploads/	-- 用户上传的原始视频文件
type StorageConfig struct {
	DataDir       string `mapstructure:"data_dir"`        // 数据存储根目录(相对路劲基于工作目录)
	MaxUploadSize uint64 `mapstructure:"max_upload_size"` //单个文件最大上传字节数
}

// FFmpegConfig FFmpeg 工具链配置
// MagicStream 通过 exec.Command 调佣外部 ffmpeg/ffprobe 进程
// 而不是通过 CGO 绑定 libav* 库
type FFmpegConfig struct {
	Path             string        `mapstructure:"path"`              // ffmpeg 可执行文件路径(如果在 PATH 中,写 "ffmpeg" 即可
	FFprobePath      string        `mapstructure:"ffprobe_path"`      // ffprobe 可执行文件路径(视频元数据探针)
	TranscodeTimeout time.Duration `mapstructure:"transcode_timeout"` // 单个视频转码的超时上限
}

// JWTConfig JWT 令牌的签名密钥和有效期配置
// 双密钥设计（Access 和 Refresh 使用不同密钥）的原因:
//
//	Access Token 的密钥可能在日志、URL、前端代码中被意外暴露。
//	如果 Access 和 Refresh 共用密钥，攻击者拿到 Access 密钥后
//	就能伪造 Refresh Token 无限续期，彻底绕开认证
//	使用不同密钥后，即使 Access 密钥泄露，攻击者也无法生成有效的 Refresh Token
//	Access Token 15 分钟过期后攻击自然失效
type JWTConfig struct {
	AccessSecret  string        `mapstructure:"access_secret"`  // Access Token 签名密钥(短时效,15分钟)
	RefreshSecret string        `mapstructure:"refresh_secret"` // Refresh Token 签名密钥(长时效,7天)
	AccessTTL     time.Duration `mapstructure:"access_ttl"`     // Access Token 有效期
	RefreshTTL    time.Duration `mapstructure:"refresh_ttl"`    // Refresh Token 有效期
}

// Load 从给定的 YAML 文件加载配置,并应用环境变量覆盖
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// 1. 设置配置文件
	// SetConfigFile 指定具体的文件路径(不是搜索路径)
	v.SetConfigFile(configPath)
	// SetConfigType 明确指定格式--避免 viper 根据扩展猜错
	v.SetConfigType("yaml")

	// 2. 配置环境变量覆盖
	// SetEnvPrefix 设置环境变量的前缀为 "MAGICSTREAM"
	// 这意味着 viper 只会识别 MAGICSTREAM_ 开头的环境变量
	v.SetEnvPrefix("MAGICSTREAM")
	// SetEnvKeyReplacer 把配置 key 中的点号(.)替换为下划线(_)
	// 原因：环境变量不支持点号，但 viper 的 key 使用点号作为分层分隔符
	// 例如 database.host → viper 查找环境变量时变成 DATABASE_HOST
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	// AutomaticEnv 开启自动环境变量绑定
	// 当代码中 v.GetString("database.host") 时，
	// viper 自动检查环境变量 MAGICSTREAM_DATABASE_HOST 是否存在，
	// 存在则使用环境变量的值，不存在则使用配置文件的值
	v.AutomaticEnv()

	// 3. 读取配置文件
	// ReadInConfig 在 SetConfigFile 指定的路径读取文件并解析
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件 %s 失败: %w", configPath, err)
	}

	// 4. 反序列化到结构体
	// Unmarshal 将 viper 内部的所有配置值映射到 Config 结构体
	// 此时环境变量的覆盖已经生效，所以结构体会得到最终值
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	// 5. 验证配置完整性
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("配置校验失败: %w", err)
	}
	return &cfg, nil
}

// 配置验证

// Validate 检查配置中所有必填字段和值的合法性
func (c *Config) Validate() error {
	// --- 服务器端口号验证 ---
	// 端口号范围 1- 65535
	if c.Server.HTTPPort <= 0 || c.Server.HTTPPort > 65535 {
		return fmt.Errorf("server.http_port 必须在 1-65535 之间,当前值: %d", c.Server.HTTPPort)
	}
	if c.Server.RTMPPort <= 0 || c.Server.RTMPPort > 65535 {
		return fmt.Errorf("server.rtmp_port 必须在 1-65535 之间,当前值: %d", c.Server.RTMPPort)
	}

	// HTTP 和 RTMP 不能占用同一端口
	if c.Server.HTTPPort == c.Server.RTMPPort {
		return fmt.Errorf("HTTP 端口(%d) 和 RTMP 端口(%d) 不能相同", c.Server.HTTPPort, c.Server.RTMPPort)
	}

	// --- 数据库配置验证 ---
	if c.Database.Host == "" {
		return fmt.Errorf("database.host 不能为空--必须指定 postgreSQL 服务器的地址")
	}
	if c.Database.Port <= 0 || c.Database.Port > 65535 {
		return fmt.Errorf("database.port 必须在 1-65535 之间,当前值: %d", c.Database.Port)
	}
	if c.Database.User == "" {
		return fmt.Errorf("database.user 不能为空--必须指定用于连接数据库的用户名")
	}
	if c.Database.DBName == "" {
		return fmt.Errorf("database,dbname 不能为空--必须指定要连接的目标数据库名")
	}
	// MaxOpenConns 至少需要 1 (至少有一个连接可用才能提供数据库服务)
	if c.Database.MaxOpenConns < 1 || c.Database.MaxOpenConns > 1000 {
		return fmt.Errorf("database.max_open_conns 应在 1-1000 之间，当前值 %d", c.Database.MaxOpenConns)
	}
	// MaxIdleConns 可以为 0（表示不保持空闲连接），但不能超过 MaxOpenConns
	if c.Database.MaxIdleConns < 0 {
		return fmt.Errorf("database.max_idle_conns 不能为负数，当前值 %d", c.Database.MaxIdleConns)
	}
	if c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		return fmt.Errorf("database.max_idle_conns(%d) 不能大于 max_open_conns(%d)",
			c.Database.MaxIdleConns, c.Database.MaxOpenConns)
	}

	// --- Redis 配置验证 ---
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis.addr 不能为空——必须指定 Redis 服务器的地址（如 localhost:6379")
	}
	// Redis 数据库编号范围 0-15，这是 Redis 的硬限制
	if c.Redis.DB < 0 || c.Redis.DB > 15 {
		return fmt.Errorf("redis.db 必须在 0-15 之间，当前值 %d", c.Redis.DB)
	}

	// --- 存储配置验证 ---
	if c.Storage.DataDir == "" {
		return fmt.Errorf("storage.data_dir 不能为空——必须指定视频和转码产物的存储目录")
	}
	// max_upload_size 必须为正数。如果设为 0，所有上传都会失败（等同于禁止上传）
	if c.Storage.MaxUploadSize <= 0 {
		return fmt.Errorf("storage.max_upload_size 必须大于 0，当前值 %d", c.Storage.MaxUploadSize)
	}

	// --- FFmpeg 配置验证
	if c.FFmpeg.Path == "" {
		return fmt.Errorf("ffmpeg.path 不能为空——必须指定 ffmpeg 可执行文件的路径或名称")
	}
	if c.FFmpeg.FFprobePath == "" {
		return fmt.Errorf("ffmpeg.ffprobe_path 不能为空——必须指定 ffprobe 可执行文件的路径或名称")
	}
	if c.FFmpeg.TranscodeTimeout <= 0 {
		return fmt.Errorf("ffmpeg.transcode_timeout 必须大于 0，当前值 %v", c.FFmpeg.TranscodeTimeout)
	}

	// --- JWT 配置验证 ---
	// 安全底线：密钥不能为空，也不能使用模板中的默认值（change-me / change-me-too）
	// 生产环境必须替换为随机生成的长字符串
	if c.JWT.AccessSecret == "" || c.JWT.AccessSecret == "change-me" {
		return fmt.Errorf("jwt.access_secret 必须设置为随机字符串，不能使用默认值 'change-me'")
	}
	if c.JWT.RefreshSecret == "" || c.JWT.RefreshSecret == "change-me-too" {
		return fmt.Errorf("jwt.refresh_secret 必须设置为随机字符串，不能使用默认值 'change-me-too")
	}
	// 强制双密钥不同——这不是"建议"，是安全基线
	if c.JWT.AccessSecret == c.JWT.RefreshSecret {
		return fmt.Errorf("jwt.access_secret 和 jwt.refresh_secret 不能相同——两个密钥必须不同以隔离风险")
	}
	// TTL 必须为正--过期的 Token 无意义, TTL <= 0 的 Token 签发时就已经过期
	if c.JWT.AccessTTL <= 0 {
		return fmt.Errorf("jwt.access_ttl 必须大于 0，当前值 %v", c.JWT.AccessTTL)
	}
	if c.JWT.RefreshTTL <= 0 {
		return fmt.Errorf("jwt.refresh_ttl 必须大于 0，当前值 %v", c.JWT.RefreshTTL)
	}
	return nil
}
