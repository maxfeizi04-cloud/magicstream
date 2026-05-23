// Package model 包含 MagicStream 的共享数据类型。
// errors.go 定义统一的业务错误类型和预定义错误工厂函数。
//
// 设计原则：
//
//  1. 错误分三层：
//     Code    —— HTTP 状态码，handler 用它设置响应状态码
//     Message —— 用户可读的错误描述，直接进入 HTTP response body
//     Err     —— 原始内部错误，只记录到日志，永不暴露给客户端
//
//  2. 安全底线——为什么 Err 不暴露在 Message 中：
//     PostgreSQL 的错误信息可能包含表名和约束名：
//     "duplicate key value violates unique constraint uq_users_email"
//     攻击者从中可以推断出：users 表有 email 字段，uq_users_email 约束名。
//     这些信息帮助攻击者构造更精准的 SQL 注入攻击。
//     所以客户端只应该看到："该邮箱已被注册"。
//
//  3. 预定义错误工厂函数 vs 全局变量：
//     工厂函数（func ErrNotFound(resource string)）每次返回新实例，
//     可以携带不同的上下文（如"用户 不存在" vs "视频 不存在"）。
//     全局变量（var ErrNotFound = ...）无法区分不同资源。
package model

import (
	"fmt"
	"net/http"
)

// AppError 是 MagicStream 中所有业务错误的统一类型
// 它实现了 error 接口，可以直接作为 Go error 返回值
// 它也实现了 Unwrap 接口，支持 errors.Is / errors.As 进行错误链判断
type AppError struct {
	Code    int    `json:"-"`       // HTTP 状态码（如 400/401/404/500），不序列化到响应 body
	Message string `json:"message"` // 用户可读的错误信息，直接出现在 JSON 响应的 "error" 字段中
	Err     error  `json:"-"`       // 原始内部错误（只记录到日志，不暴露给客户端）
}

// Error 实现 error 接口
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Unwrap 实现 errors.Unwrap 接口
func (e *AppError) Unwrap() error {
	return e.Err
}

// ErrNotFound 资源不存在（HTTP 404）
// resource 是资源名称，如 "用户"、"视频"、"直播间"
func ErrNotFound(resource string) *AppError {
	return &AppError{
		Code:    http.StatusNotFound,
		Message: fmt.Sprintf("%s 不存在", resource),
	}
}

// ErrUnauthorized 未认证或 Token 无效(HTTP 401)
// 当请求缺少 Authorization header、Token 过期、签名不匹配时使用
func ErrUnauthorized() *AppError {
	return &AppError{
		Code:    http.StatusUnauthorized,
		Message: "请先登录",
	}
}

// ErrForbidden 认证通过但权限不足(HTTP 403)
// 例如普通用户尝试删除其他用户的视频、非创作者尝试开播
func ErrForbidden() *AppError {
	return &AppError{
		Code:    http.StatusForbidden,
		Message: "没有权限执行此操作",
	}
}

// ErrValidation 请求参数校验失败（HTTP 422 Unprocessable Entity）
// 用 422 而不是 400，因为 400 的语义是"请求本身格式错误"（如 JSON 格式不对）
// 422 的语义是"格式正确但内容不合规"（如邮箱格式不对、密码太短）
// message 是具体的校验失败原因，由调用方传入
func ErrValidation(message string) *AppError {
	return &AppError{
		Code:    http.StatusUnprocessableEntity,
		Message: message,
	}
}

// ErrConflict 资源冲突（HTTP 409）。
// 例如注册时用户名已被使用、创建直播间时 stream_key 重复
func ErrConflict(message string) *AppError {
	return &AppError{
		Code:    http.StatusConflict,
		Message: message,
	}
}

// ErrInternal 服务器内部错误(HTTP 500)
// Message 永远固定为"服务器内部错误"，不暴露任何内部细节
// err 是原始错误，只记录到日志
func ErrInternal(err error) *AppError {
	return &AppError{
		Code:    http.StatusInternalServerError,
		Message: "服务器内部错误",
		Err:     err,
	}
}

// ErrTooManyRequests 请求频率超限(HTTP 429)
// 由速率限制中间件在触发限流时使用
func ErrTooManyRequests(message string) *AppError {
	return &AppError{
		Code:    http.StatusTooManyRequests,
		Message: message,
	}
}

// IsAppError 检查 error 链中是否存在指定 HTTP 状态码的 AppError
func IsAppError(err error, code int) bool {
	for err != nil {
		if appErr, ok := err.(*AppError); ok {
			return appErr.Code == code
		}
		// 沿 Unwrap 链向上查找
		type unwrapper interface{ Unwrap() error }
		if u, ok := err.(unwrapper); ok {
			err = u.Unwrap()
		} else {
			break
		}
	}
	return false
}
