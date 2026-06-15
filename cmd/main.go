// Package main 是 SealSuite 自动运营服务的程序入口
// 该服务负责定时从 SealSuite 系统获取数据，并执行自动运营任务
package main

import (
	"context" // 上下文管理
	"net/http"
	"os"        // 操作系统接口
	"os/signal" // 信号处理
	"syscall"   // 系统调用
	"time"

	// 项目内部包
	"sealsuite-operation/internal/bootstrap"
	"sealsuite-operation/internal/config" // 配置管理
	"sealsuite-operation/internal/logger" // 日志系统
	"sealsuite-operation/internal/runner"
	"sealsuite-operation/internal/web"

	// 第三方库
	"go.uber.org/zap" // 结构化日志
)

// main 是程序的主入口函数
// 执行流程：
// 1. 加载配置文件
// 2. 初始化日志系统
// 3. 初始化 SealSuite/飞连 API 客户端
// 4. 初始化基于 SQLite 业务存储的 Runner 并启动调度
// 5. 启动本地 Web 控制台（静态前端 + REST API）
// 6. 等待终止信号并优雅关闭
func main() {
	// ---------------------------
	// 1. 加载配置文件
	// ---------------------------
	// 从 config.yaml 加载配置，如果失败则 panic
	cfg, err := config.Load("config.yaml")
	if err != nil {
		panic(err)
	}

	// ---------------------------
	// 2. 初始化日志系统
	// ---------------------------
	// 使用配置文件中的日志级别和日志文件路径初始化
	if err := logger.Init(cfg.Log.Level, cfg.Log.Filename); err != nil {
		panic(err)
	}
	// 程序退出时确保日志刷入文件
	defer logger.Sync()

	// 打印启动横幅
	logger.Info("========================================")
	logger.Info("SealSuite Operation Service 启动")
	logger.Info("========================================")

	// ---------------------------
	// 3. 装配 SQLite、Service 与 API 客户端
	// ---------------------------
	app, err := bootstrap.Build(cfg)
	if err != nil {
		panic(err)
	}
	defer app.DB.Close()

	// ---------------------------
	// 4. 初始化 Runner；如存在旧版 jobs.yaml，会在首次启动时迁入 SQLite，再按 SQLite 运行态数据装载
	// ---------------------------
	r := runner.New(cfg, app.Client)
	if err := r.Reload(); err != nil {
		logger.Error("runner reload failed", zap.Error(err))
	}

	// ---------------------------
	// 5. 启动 Web 控制台
	// ---------------------------
	srv, err := web.NewServerWithServices(cfg, r, app.Services)
	if err != nil {
		logger.Fatal("failed to init web server", zap.Error(err))
	}
	go func() {
		logger.Info("web console listening", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("web server error", zap.Error(err))
		}
	}()

	// ---------------------------
	// 6. 等待终止信号并优雅关闭
	// ---------------------------
	logger.Info("========================================")
	logger.Info("服务运行中... 按 Ctrl+C 停止")
	logger.Info("========================================")

	// 创建可取消的上下文（虽然这里未直接使用，但保留扩展性）
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建信号通道，监听系统信号
	sigChan := make(chan os.Signal, 1)
	// 监听中断信号（Ctrl+C）和终止信号
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 阻塞等待信号到来
	<-sigChan
	logger.Info("========================================")
	logger.Info("正在关闭服务...")

	// 先关闭 HTTP，再关闭 runner
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	r.Stop()

	logger.Info("✅ 服务已停止")
	logger.Info("========================================")
}
