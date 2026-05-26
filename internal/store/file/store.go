package file

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileStore 封装文件系统操作,统一处理
type FileStore struct {
	// baseDir 是所有文件上传的根目录
	baseDir string

	// maxSize 是单次上传的最大字节数
	maxSize int64
}

// NewFileStore 创建 FileStore 实例
func NewFileStore(baseDir string, maxSize int64) *FileStore {
	return &FileStore{baseDir: baseDir, maxSize: maxSize}
}

// SaveUpload 将上传的 reader 内容保存到磁盘
func (s *FileStore) SaveUpload(ctx context.Context, reader io.Reader, dir, filename string) (string, int64, error) {
	// 1. 大小限制
	var limitedReader io.Reader
	if s.maxSize > 0 {
		limitedReader = io.LimitReader(reader, s.maxSize)
	} else {
		limitedReader = reader
	}

	// 2. 路径穿越防护
	cleanDir := filepath.Clean(dir)
	cleanFilename := filepath.Clean(filename)

	// 禁止空文件名或只包含路径分隔符的文件名
	if cleanFilename == "." || cleanFilename == "" || strings.Contains(cleanFilename, string(filepath.Separator)) {
		return "", 0, fmt.Errorf("无效文件名: %s", cleanFilename)
	}

	// 拼接完整路径
	fullPath := filepath.Join(s.baseDir, cleanDir, cleanFilename)

	// 再次 Clean 确保拼接后的路径也是规范的
	fullPath = filepath.Clean(fullPath)

	// 获取 baseDir 的绝对路径以做精确前缀比较
	absBaseDir, err := filepath.Abs(fullPath)
	if err != nil {
		return "", 0, fmt.Errorf("resolve base dir: %w", err)
	}

	// 核心安全检查: fullPath 必须以 absBaseDir 开头
	// Windows 上路径分隔符不同，用 os.IsPathSeparator 做跨平台检查
	if !strings.HasPrefix(fullPath, absBaseDir) {
		return "", 0, fmt.Errorf("path traversal detected: %q is outside: %q", fullPath, absBaseDir)
	}

	// 额外防御: 确保 fullPath 的下一个字符是路径分隔符
	// 防止 /data/videos 被 /data/videos_evil 绕过
	if len(fullPath) > len(absBaseDir) {
		sep := fullPath[len(absBaseDir)]
		if !os.IsPathSeparator(sep) {
			return "", 0, fmt.Errorf("path traversal detected: %q escapes base dir", fullPath)
		}
	}

	// 3. 创建目录并写入文件
	destDir := filepath.Dir(fullPath)
	if err := os.MkdirAll(destDir, 0750); err != nil {
		return "", 0, fmt.Errorf("create directory %q: %w", destDir, err)
	}

	// 创建目标文件
	file, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return "", 0, fmt.Errorf("create file %q: %w", fullPath, err)
	}
	defer file.Close()

	// 将上传内容拷贝到文件
	written, err := io.Copy(file, limitedReader)
	if err != nil {
		// 写入失败时删除已创建的空/部分文件，避免残留
		os.Remove(fullPath)
		return "", 0, fmt.Errorf("write file %q: %w", fullPath, err)
	}

	// 检查是否被 LImitReader 截断
	if s.maxSize > 0 && written > s.maxSize {
		// 再读 1 字节确认是否真的超过限制
		oneMore := make([]byte, 1)
		n, readErr := reader.Read(oneMore)
		if n > 0 || readErr != nil {
			os.Remove(fullPath)
			return "", 0, fmt.Errorf("file exceeds maximum size of %d bytes: %v", s.maxSize, readErr)
		}
	}
	return fullPath, written, nil

}

// DeleteDir 递归删除一个目录及其所有内容
func (s *FileStore) DeleteDir(dir string) error {
	// 路径穿越防护
	cleanDir := filepath.Clean(dir)
	fullPath := filepath.Join(s.baseDir, cleanDir)
	fullPath = filepath.Clean(fullPath)

	absBaseDir, err := filepath.Abs(fullPath)
	if err != nil {
		return fmt.Errorf("resolve base dir: %w", err)
	}
	if !strings.HasPrefix(fullPath, absBaseDir) {
		return fmt.Errorf("path traversal detected: %q is outside: %q", fullPath, cleanDir)
	}

	// 不允许删除 baseDir 本身
	if fullPath == s.baseDir {
		return fmt.Errorf("refusing to delete base directory: %q", absBaseDir)
	}
	if err := os.RemoveAll(fullPath); err != nil {
		return fmt.Errorf("remove directory %q: %w", fullPath, err)
	}
	return nil
}

// GetFilePath 拼接 baseDir 和相对路径,返回完整文件路径
func (s *FileStore) GetFilePath(parts ...string) string {
	allParts := append([]string{s.baseDir}, parts...)

	return filepath.Join(allParts...)
}
