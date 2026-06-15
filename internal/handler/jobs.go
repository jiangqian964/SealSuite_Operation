// Package handler 提供业务任务处理函数
// 包含从 SealSuite 获取数据、处理数据等具体业务逻辑
package handler

import (
	"encoding/json" // JSON 编码
	"fmt"           // 格式化输出

	// 项目内部包
	"sealsuite-operation/internal/logger"    // 日志系统
	"sealsuite-operation/internal/sealsuite" // SealSuite API 客户端

	// 第三方库
	"go.uber.org/zap" // 结构化日志
)

// JobHandler 是任务处理器
// 封装了 SealSuite 客户端，提供各种业务任务的处理方法
type JobHandler struct {
	sealSuiteClient *sealsuite.Client // SealSuite API 客户端
}

// NewJobHandler 创建一个新的任务处理器
// 参数:
//
//	client - SealSuite API 客户端实例
//
// 返回:
//
//	*JobHandler - 新创建的任务处理器
func NewJobHandler(client *sealsuite.Client) *JobHandler {
	return &JobHandler{
		sealSuiteClient: client, // 保存 API 客户端引用
	}
}

// ExampleSyncJob 是示例同步任务
// 调用示例 API 并记录响应数据
// 返回:
//
//	error - 任务执行失败时的错误信息
func (h *JobHandler) ExampleSyncJob() error {
	logger.Info("📋 执行示例同步任务")

	// 调用 SealSuite 示例 API
	resp, err := h.sealSuiteClient.Get("/api/v1/example/endpoint")
	if err != nil {
		logger.Error("❌ 调用SealSuite API失败", zap.Error(err))
		return err
	}

	// 将响应数据格式化为缩进的 JSON，方便日志查看
	dataJson, _ := json.MarshalIndent(resp.Data, "", "  ")
	logger.Info("✅ 示例任务完成", zap.String("data", string(dataJson)))
	return nil
}

// SyncUsersJob 是用户数据同步任务
// 从 SealSuite 获取用户列表数据
// 返回:
//
//	error - 任务执行失败时的错误信息
func (h *JobHandler) SyncUsersJob() error {
	logger.Info("👥 执行用户数据同步任务")

	// 调用用户列表 API
	resp, err := h.sealSuiteClient.Get("/api/v1/users")
	if err != nil {
		logger.Error("❌ 同步用户数据失败", zap.Error(err))
		return err
	}

	// 格式化并记录响应数据
	dataJson, _ := json.MarshalIndent(resp.Data, "", "  ")
	logger.Info("✅ 用户数据同步完成", zap.String("data", string(dataJson)))

	// 解析并记录用户总数
	if dataMap, ok := resp.Data.(map[string]interface{}); ok {
		if total, ok := dataMap["total"].(float64); ok {
			logger.Info(fmt.Sprintf("📊 共同步 %d 条用户记录", int(total)))
		}
	}

	return nil
}

// SyncDevicesJob 是设备数据同步任务
// 从 SealSuite 获取设备列表数据
// 返回:
//
//	error - 任务执行失败时的错误信息
func (h *JobHandler) SyncDevicesJob() error {
	logger.Info("💻 执行设备数据同步任务")

	// 调用设备列表 API
	resp, err := h.sealSuiteClient.Get("/api/v1/devices")
	if err != nil {
		logger.Error("❌ 同步设备数据失败", zap.Error(err))
		return err
	}

	// 格式化并记录响应数据
	dataJson, _ := json.MarshalIndent(resp.Data, "", "  ")
	logger.Info("✅ 设备数据同步完成", zap.String("data", string(dataJson)))

	// 解析并记录设备总数
	if dataMap, ok := resp.Data.(map[string]interface{}); ok {
		if total, ok := dataMap["total"].(float64); ok {
			logger.Info(fmt.Sprintf("📊 共同步 %d 条设备记录", int(total)))
		}
	}

	return nil
}
