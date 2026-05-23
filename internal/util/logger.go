// Package util 包含 MagicStream  的基础工具函数
// logger.go 负责结构化日志(zerolog)的初始化和配置
package util

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger 是全局的 zerolog Logger 实例
var Logger zerolog.Logger

// InitLogger 初始化全局 Logger
func InitLogger(isDev bool) {
	if isDev {
		// 开发环境: ConsoleWriter 输出
		output := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
			NoColor:    !isDev,
		}
		Logger = zerolog.New(output).With().Timestamp().Caller().Logger()

		// 开发环境日志级别设置为 Debug -- 看到所有日志
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		// 生成环境: JSON 输出
		Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()

		// 生产环境日志级别设为 Info --忽略 Debug 日志（减少 IO 和存储压力）
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	Logger.Info().Bool("development", isDev).Msg("日志系统初始化完成")
}

// WithRequestID 为当前请求创建一个带 request_id 的子 Logger
func WithRequestID(requestID string) zerolog.Logger {
	return Logger.With().Str("request_id", requestID).Logger()
}
