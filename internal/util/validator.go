package util

import (
	"errors"
	"fmt"
	"io"
	"net/http"
)

// AllowedVideoMIMETypes 是允许上传的视频 MIME 白名单
// 支持的格式：
//
//	video/mp4       —— MP4 容器（H.264 + AAC），最通用的格式
//	video/quicktime —— MOV 容器（Apple 生态常用），底层与 MP4 同源（ISOBMFF）
//	video/x-matroska —— MKV 容器（开放格式），常见于录屏和 rip
//	video/x-flv     —— FLV 容器（Flash Video），直播场景常见
//	video/webm      —— WebM 容器（VP8/VP9），Web 原生支持
//	video/x-msvideo —— AVI 容器（老旧格式），兼容性考虑
var AllowedVideoMIMETypes = map[string]bool{
	"video/mp4":        true,
	"video/webm":       true,
	"video/quicktime":  true,
	"video/x-flv":      true,
	"video/x-msvideo":  true,
	"video/x-matroska": true,
}

// DetectContentType 通过读取文件头 Magic Bytes 检测真实内容类型
func DetectContentType(reader io.Reader) (string, error) {
	// 读取前 512 字节用于检测
	// 512 是 MIME sniffing 标准（RFC 2046 / WHATWG MIME Sniffing）规定的嗅探字节数
	buf := make([]byte, 512)
	n, err := io.ReadFull(reader, buf)

	// io.ReadFull 在 EOF 前读满 buf 时返回 io.ErrUnexpectedEOF
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && err != io.EOF {
		return "", fmt.Errorf("read content for detection: %w", err)
	}

	// 使用标准库的 MIME 类型检测
	contentType := http.DetectContentType(buf[:n])

	// 检查是否在视频白名单中
	if !AllowedVideoMIMETypes[contentType] {
		return "", fmt.Errorf("unsupported file type: %s", contentType)
	}
	return contentType, nil
}

// IsVideoContent 快速判断给定的 MIME 类型是否为允许的视频格式。
func IsVideoContent(contentType string) bool {
	return AllowedVideoMIMETypes[contentType]
}
