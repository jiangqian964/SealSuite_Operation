// Package scheduler 提供定时任务调度功能
// 基于 cron 库实现，支持添加、删除和管理定时任务
package scheduler

import (
	// 项目内部包
	"sealsuite-operation/internal/logger" // 日志系统
	"sync"
	"time"

	// 第三方库
	"github.com/robfig/cron/v3" // cron 定时任务库
	"go.uber.org/zap"           // 结构化日志
)

// JobFunc 定义任务函数类型
// 任务执行成功返回 nil，失败返回 error
type JobFunc func() error

// Scheduler 是定时任务调度器
// 封装了 cron 库，提供更便捷的任务管理功能
type Scheduler struct {
	cron *cron.Cron              // cron 调度器实例
	jobs map[string]cron.EntryID // 任务名称到任务 ID 的映射
	mu   sync.RWMutex            // 保护 jobs map 的读写锁
}

// New 创建一个新的调度器实例
// 返回:
//
//	*Scheduler - 新创建的调度器
//
// 注意：本项目统一使用 **秒级（6段）** Cron 表达式：`sec min hour dom mon dow`
// 例如：`0 */1 * * * *` 表示每分钟第 0 秒执行一次。
func New(location *time.Location) *Scheduler {
	if location == nil {
		location = time.Local
	}
	return &Scheduler{
		cron: cron.New(
			cron.WithSeconds(),
			cron.WithLocation(location),
		), // 创建秒级 cron 调度器（6段表达式）
		jobs: make(map[string]cron.EntryID), // 初始化任务映射
	}
}

// AddJob 添加一个定时任务
// 参数:
//
//	name - 任务名称，用于标识和管理任务
//	spec - cron 表达式（6段秒级），例如："0 */1 * * * *" 表示每分钟执行一次（第0秒触发）
//	job - 任务函数，返回 error 表示任务失败
//
// 返回:
//
//	error - 添加失败时的错误信息
func (s *Scheduler) AddJob(name string, spec string, job JobFunc) error {
	// 包装任务函数，添加日志记录和错误处理
	wrappedJob := func() {
		// 记录任务开始
		logger.Info("starting job", zap.String("job_name", name))

		// 执行任务
		if err := job(); err != nil {
			// 任务执行失败，记录错误日志
			logger.Error("job failed", zap.String("job_name", name), zap.Error(err))
		} else {
			// 任务执行成功，记录日志
			logger.Info("job completed successfully", zap.String("job_name", name))
		}
	}

	// 将包装后的任务添加到 cron 调度器
	id, err := s.cron.AddFunc(spec, wrappedJob)
	if err != nil {
		return err
	}

	// 保存任务名称和 ID 的映射关系
	s.mu.Lock()
	s.jobs[name] = id
	s.mu.Unlock()
	logger.Info("job added", zap.String("job_name", name), zap.String("cron_spec", spec))
	return nil
}

// Entries 返回 cron 的任务条目（用于查询 next/prev 等运行态）
func (s *Scheduler) Entries() []cron.Entry {
	return s.cron.Entries()
}

// EntryID 返回任务名称对应的 cron EntryID（用于查询 next/prev）。
func (s *Scheduler) EntryID(name string) (cron.EntryID, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.jobs[name]
	return id, ok
}

// RemoveJob 删除一个已添加的定时任务
// 参数:
//
//	name - 要删除的任务名称
func (s *Scheduler) RemoveJob(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 查找任务 ID
	if id, ok := s.jobs[name]; ok {
		// 从 cron 调度器中移除任务
		s.cron.Remove(id)
		// 从映射中删除
		delete(s.jobs, name)
		logger.Info("job removed", zap.String("job_name", name))
	}
}

// Start 启动调度器
// 启动后，已添加的任务会按照 cron 表达式定时执行
func (s *Scheduler) Start() {
	s.cron.Start()
	logger.Info("scheduler started")
}

// Stop 停止调度器
// 停止后，所有任务不会再被调度执行
// 正在执行的任务会继续执行直到完成
func (s *Scheduler) Stop() {
	s.cron.Stop()
	logger.Info("scheduler stopped")
}
