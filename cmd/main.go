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
	startTime := time.Now()

	// ---------------------------
	// 1. 加载配置文件
	// ---------------------------
	logger.Print("========================================")
	logger.Print("[启动] 1/6 - 加载配置文件...")
	cfg, err := config.Load("config.yaml")
	if err != nil {
		logger.Fatal("[启动] 配置加载失败", zap.Error(err))
	}
	logger.Print("[启动] 配置文件加载成功", zap.String("config_path", "config.yaml"))

	// ---------------------------
	// 2. 初始化日志系统
	// ---------------------------
	logger.Print("[启动] 2/6 - 初始化日志系统...")
	if err := logger.Init(cfg.Log.Level, cfg.Log.Filename); err != nil {
		logger.Fatal("[启动] 日志系统初始化失败", zap.Error(err))
	}
	defer logger.Sync()

	logger.Info("========================================")
	logger.Info("  SealSuite Operation Service 启动")
	logger.Info("========================================")
	logger.Info("[启动] 日志系统初始化完成",
		zap.String("level", cfg.Log.Level),
		zap.String("log_file", cfg.Log.Filename),
	)

	// ---------------------------
	// 3. 装配 SQLite、Service 与 API 客户端
	// ---------------------------
	logger.Info("[启动] 3/6 - 初始化数据库和服务...")
	app, err := bootstrap.Build(cfg)
	if err != nil {
		logger.Fatal("[启动] 数据库和服务初始化失败", zap.Error(err))
	}
	defer app.DB.Close()
	logger.Info("[启动] 数据库和服务初始化完成",
		zap.String("db_path", cfg.Database.Path),
	)

	// ---------------------------
	// 4. 初始化 Runner；如存在旧版 jobs.yaml，会在首次启动时迁入 SQLite，再按 SQLite 运行态数据装载
	// ---------------------------
	logger.Info("[启动] 4/6 - 初始化任务运行器...")
	r := runner.New(cfg, app.Client)
	if err := r.InitDB(app.DB); err != nil {
		logger.Error("[启动] 任务运行器数据库初始化失败", zap.Error(err))
	} else {
		logger.Info("[启动] 任务运行器数据库初始化完成")
	}
	if err := r.Reload(); err != nil {
		logger.Error("[启动] 任务运行器重载失败", zap.Error(err))
	} else {
		logger.Info("[启动] 任务运行器初始化完成")
	}

	// ---------------------------
	// 5. 启动 Web 控制台
	// ---------------------------
	logger.Info("[启动] 5/6 - 启动 Web 控制台...")
	srv, err := web.NewServerWithServices(cfg, r, app.Services)
	if err != nil {
		logger.Fatal("[启动] Web 控制台初始化失败", zap.Error(err))
	}
	go func() {
		logger.Info("[启动] Web 控制台已启动",
			zap.String("addr", srv.Addr),
			zap.String("url", "http://"+srv.Addr),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("[运行] Web 服务错误", zap.Error(err))
		}
	}()

	// ---------------------------
	// 6. 等待终止信号并优雅关闭
	// ---------------------------
	logger.Info("[启动] 6/6 - 服务启动完成")
	logger.Info("========================================")
	logger.Info("  服务运行中... 按 Ctrl+C 停止")
	logger.Info("========================================")
	logger.Info("[启动] 启动耗时", zap.Duration("duration", time.Since(startTime)))
	printStartupSummary(cfg, r)

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
	logger.Info("[关闭] 收到终止信号，正在关闭服务...")
	shutdownStart := time.Now()

	// 先关闭 HTTP，再关闭 runner
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logger.Info("[关闭] 停止 Web 服务...")
	_ = srv.Shutdown(ctx)
	logger.Info("[关闭] Web 服务已停止")

	logger.Info("[关闭] 停止任务调度器...")
	r.Stop()
	logger.Info("[关闭] 任务调度器已停止")

	logger.Info("[关闭] 关闭耗时", zap.Duration("duration", time.Since(shutdownStart)))
	logger.Info("✅ 服务已安全停止")
	logger.Info("========================================")
}

// printStartupSummary 打印启动摘要信息，方便排错
func printStartupSummary(cfg *config.Config, r *runner.Runner) {
	logger.Info("--- 启动摘要 ---")
	logger.Info("[配置] SealSuite API",
		zap.String("host", cfg.SealSuite.Host),
		zap.String("scheme", cfg.SealSuite.Scheme),
		zap.Int("port", cfg.SealSuite.Port),
		zap.Bool("mock_mode", cfg.SealSuite.MockMode),
		zap.Int("timeout_sec", cfg.SealSuite.Timeout),
		zap.Int("retry_times", cfg.SealSuite.RetryTimes),
	)
	logger.Info("[配置] 调度器",
		zap.Bool("enabled", cfg.Scheduler.Enabled),
		zap.String("timezone", cfg.Scheduler.Timezone),
	)
	logger.Info("[配置] Web 服务",
		zap.String("bind", cfg.Server.Bind),
		zap.Int("port", cfg.Server.Port),
		zap.String("mode", cfg.Server.Mode),
	)
	logger.Info("[配置] 数据库",
		zap.String("path", cfg.Database.Path),
	)
	logger.Info("[配置] LLM",
		zap.Bool("planner_enabled", cfg.LLM.Planner.Enabled),
		zap.String("planner_provider", cfg.LLM.Planner.Provider),
		zap.Bool("formatter_enabled", cfg.LLM.Formatter.Enabled),
		zap.String("formatter_provider", cfg.LLM.Formatter.Provider),
	)
	logger.Info("--- 摘要结束 ---")
}
