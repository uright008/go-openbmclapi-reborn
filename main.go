package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/uright008/go-openbmclapi-reborn/cluster"
	"github.com/uright008/go-openbmclapi-reborn/config"
	"github.com/uright008/go-openbmclapi-reborn/logger"
	"github.com/uright008/go-openbmclapi-reborn/server"
)

func main() {
	// 加载配置
	cfg, err := config.Load("config.toml")
	if err != nil {
		fmt.Printf("无法加载配置: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志记录器
	// 根据配置中的日志级别判断是否开启调试模式
	debugMode := cfg.Log.Level == "debug"
	appLogger := logger.New(debugMode)

	// 创建集群实例
	appCluster, err := cluster.NewCluster(cfg, appLogger)
	if err != nil {
		appLogger.Fatal("无法创建集群实例: %v", err)
	}

	// 初始化集群
	err = appCluster.Init()
	if err != nil {
		appLogger.Fatal("无法初始化集群: %v", err)
	}

	// 连接到中心服务器
	err = appCluster.Connect()
	if err != nil {
		appLogger.Fatal("无法连接到中心服务器: %v", err)
	}

	// 同步文件
	err = appCluster.SyncFiles()
	if err != nil {
		appLogger.Error("无法同步文件: %v", err)
		// 不中断启动过程，但记录错误
	}

	// 创建并启动HTTP服务器
	httpServer := server.NewServer(appCluster)
	err = httpServer.Start(fmt.Sprintf(":%d", cfg.Cluster.Port))
	if err != nil {
		appLogger.Fatal("无法启动HTTP服务器: %v", err)
	}

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 等待关闭信号
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-sigChan
		appLogger.Info("收到关闭信号，正在关闭服务器...")

		// 创建一个5秒的上下文用于关闭服务器
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// 关闭HTTP服务器
		if err := httpServer.Stop(ctx); err != nil {
			appLogger.Error("关闭HTTP服务器时出错: %v", err)
		}

		// 关闭集群
		if err := appCluster.Close(); err != nil {
			appLogger.Error("关闭集群时出错: %v", err)
		}
	}()

	appLogger.Info("服务器已启动，按 Ctrl+C 关闭")
	wg.Wait()
	appLogger.Info("服务器已关闭")
}
