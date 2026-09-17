// Package logger 提供结构化日志功能
// 基于 zap 库实现，支持日志分级、多输出和结构化字段
package logger

import (
	"fmt"
	"os" // 操作系统接口，用于程序退出

	"go.uber.org/zap"         // Uber 开源的高性能日志库
	"go.uber.org/zap/zapcore" // zap 的核心配置包
)

// globalLogger 是全局日志单例
// 通过 Init 函数初始化后，可以通过便捷函数直接使用
var globalLogger *zap.Logger

// Init 初始化日志系统
// 参数:
//
//	level - 日志级别，可选值：debug, info, warn, error
//	filename - 日志文件路径，如果为空则只输出到控制台
//
// 返回:
//
//	error - 初始化失败时的错误信息
func Init(level string, filename string) error {
	// 使用 zap 生产环境配置作为基础
	config := zap.NewProductionConfig()
	// 设置时间字段名为 "timestamp"
	config.EncoderConfig.TimeKey = "timestamp"
	// 使用 ISO8601 格式编码时间
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// 根据传入的参数设置日志级别
	if level == "debug" {
		// Debug 级别：输出所有日志
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	} else if level == "info" {
		// Info 级别：输出 info, warn, error
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	} else if level == "warn" {
		// Warn 级别：输出 warn, error
		config.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	} else if level == "error" {
		// Error 级别：只输出 error
		config.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	}

	// 如果指定了日志文件，则同时输出到控制台和文件
	if filename != "" {
		// 普通日志输出位置
		config.OutputPaths = []string{"stdout", filename}
		// 错误日志输出位置
		config.ErrorOutputPaths = []string{"stderr", filename}
	}

	// 根据配置构建 logger 实例
	logger, err := config.Build()
	if err != nil {
		return err
	}

	// 保存到全局变量
	globalLogger = logger
	return nil
}

// Get 获取全局 logger 实例
// 如果 logger 未初始化，则创建一个默认的生产环境 logger
// 返回:
//
//	*zap.Logger - 全局 logger 实例
func Get() *zap.Logger {
	if globalLogger == nil {
		// 如果未初始化，创建一个默认的生产环境 logger
		globalLogger, _ = zap.NewProduction()
	}
	return globalLogger
}

// Sync 刷入日志缓冲区内容到输出
// 应该在程序退出前调用，确保所有日志都已写入
func Sync() {
	if globalLogger != nil {
		// 忽略错误，因为在程序退出时刷入可能会失败
		_ = globalLogger.Sync()
	}
}

// Debug 输出 Debug 级别日志
// 参数:
//
//	msg - 日志消息内容
//	fields - 结构化字段，可变参数，例如：zap.String("key", "value")
func Debug(msg string, fields ...zap.Field) {
	Get().Debug(msg, fields...)
}

// Info 输出 Info 级别日志
// 用于记录程序正常运行时的重要信息
// 参数:
//
//	msg - 日志消息内容
//	fields - 结构化字段，可变参数
func Info(msg string, fields ...zap.Field) {
	Get().Info(msg, fields...)
}

// Warn 输出 Warn 级别日志
// 用于记录警告信息，表示可能存在问题但不影响程序继续运行
// 参数:
//
//	msg - 日志消息内容
//	fields - 结构化字段，可变参数
func Warn(msg string, fields ...zap.Field) {
	Get().Warn(msg, fields...)
}

// Error 输出 Error 级别日志
// 用于记录错误信息，表示程序出现了问题
// 参数:
//
//	msg - 日志消息内容
//	fields - 结构化字段，可变参数
func Error(msg string, fields ...zap.Field) {
	Get().Error(msg, fields...)
}

// Fatal 输出 Fatal 级别日志并退出程序
// 用于记录严重错误，程序无法继续运行
// 参数:
//
//	msg - 日志消息内容
//	fields - 结构化字段，可变参数
func Fatal(msg string, fields ...zap.Field) {
	Get().Fatal(msg, fields...)
	// 日志输出后立即退出程序，状态码为 1
	os.Exit(1)
}

// Print 输出普通日志，在 logger 初始化前使用 fmt 输出，初始化后使用 Info 级别
// 用于启动阶段的进度输出
// 参数:
//
//	msg - 日志消息内容
//	fields - 结构化字段，可变参数
func Print(msg string, fields ...zap.Field) {
	if globalLogger == nil {
		// logger 未初始化时，使用 fmt 输出到控制台
		fmt.Println(msg)
	} else {
		// logger 初始化后，使用 Info 级别
		Get().Info(msg, fields...)
	}
}
