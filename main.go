package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/uright008/go-openbmclapi-reborn/cluster"
	"github.com/uright008/go-openbmclapi-reborn/config"
	"github.com/uright008/go-openbmclapi-reborn/logger"
	"github.com/uright008/go-openbmclapi-reborn/server"
)

func main() {
	// 加载配置
	cfg, err := config.Load("config.toml")
	if err != nil {
		log.Printf("配置加载失败: %v", err)
		// 如果是配置文件刚创建的提示信息，则正常退出
		if err.Error() == "请修改配置文件后重新启动程序" {
			os.Exit(0)
		}
		log.Fatalf("无法加载配置: %v", err)
	}

	// 创建日志记录器
	appLogger := logger.New(cfg.Log.Level == "debug")

	appLogger.Info("OpenBMCLAPI 正在启动，集群ID: %s", cfg.Cluster.ID)

	// 检查必要配置是否已设置
	if cfg.Cluster.ID == "" || cfg.Cluster.Secret == "" {
		appLogger.Fatal("请在配置文件中设置集群ID和Secret")
	}

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
	httpServer := server.New(cfg, appLogger)
	err = httpServer.Start()
	if err != nil {
		appLogger.Fatal("无法启动HTTP服务器: %v", err)
	}

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 等待退出信号
	<-sigChan
	appLogger.Info("正在关闭 OpenBMCLAPI...")

	// 关闭HTTP服务器
	err = httpServer.Stop()
	if err != nil {
		appLogger.Error("关闭HTTP服务器时出错: %v", err)
	}

	// 关闭集群
	err = appCluster.Close()
	if err != nil {
		appLogger.Error("关闭集群时出错: %v", err)
	}

	appLogger.Info("OpenBMCLAPI 已关闭")
}
