package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/uright008/go-openbmclapi-reborn/config"
	"github.com/uright008/go-openbmclapi-reborn/logger"
	"github.com/uright008/go-openbmclapi-reborn/utils"
)

// Server 定义HTTP服务器结构
type Server struct {
	httpServer *http.Server
	config     *config.Config
	logger     *logger.Logger
}

// New 创建新的HTTP服务器实例
func New(cfg *config.Config, log *logger.Logger) *Server {
	return &Server{
		config: cfg,
		logger: log,
	}
}

// Start 启动HTTP服务器
func (s *Server) Start() error {
	// 创建HTTP服务器
	addr := fmt.Sprintf("%s:%d", s.config.Cluster.IP, s.config.Cluster.Port)
	s.httpServer = &http.Server{
		Addr: addr,
	}

	// 注册路由处理函数
	s.registerRoutes()

	// 在goroutine中启动服务器以避免阻塞
	go func() {
		s.logger.Info("HTTP服务器正在启动，监听地址: %s", addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP服务器启动失败: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)
	return nil
}

// Stop 停止HTTP服务器
func (s *Server) Stop() error {
	if s.httpServer == nil {
		return nil
	}

	s.logger.Info("正在关闭HTTP服务器...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("HTTP服务器关闭失败: %w", err)
	}

	s.logger.Info("HTTP服务器已关闭")
	return nil
}

// registerRoutes 注册路由处理函数
func (s *Server) registerRoutes() {
	// 偽裝成nginx的路由
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 根据原始openbmclapi的行为，这里应该返回nginx相关信息
		// 但目前我们只是简单地返回一个信息
		w.Header().Set("Server", "nginx")
		fmt.Fprintf(w, "Welcome to OpenBMCLAPI - a mirror cluster for BMCLAPI")
	})

	// 健康检查路由
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	// 下载路由
	http.HandleFunc("/download/", s.handleDownload)

	// 认证路由
	http.HandleFunc("/auth", s.handleAuth)

	// 测速路由
	http.HandleFunc("/measure", func(w http.ResponseWriter, r *http.Request) {
		// 这里应该实现测速逻辑
		// 目前只是占位符
		fmt.Fprintf(w, "Measure endpoint")
	})
}

// handleDownload 处理文件下载请求
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// 从URL中提取哈希值
	hash := utils.ExtractHashFromPath(r.URL.Path)
	if hash == "" {
		http.Error(w, "Invalid hash", http.StatusBadRequest)
		return
	}

	// 验证请求签名
	if !s.verifyRequest(r, hash) {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	// 检查文件是否存在
	// 这里需要访问存储系统来检查文件，暂时留空，后续实现
	// exists, err := storage.Exists(hash)
	// if err != nil {
	// 	http.Error(w, "Internal server error", http.StatusInternalServerError)
	// 	return
	// }
	//
	// if !exists {
	// 	http.NotFound(w, r)
	// 	return
	// }

	// 返回文件内容
	// 这里需要实现文件读取和传输逻辑，暂时返回404
	http.NotFound(w, r)

	// 记录请求日志
	duration := time.Since(startTime)
	s.logger.LogRequest(r.Method, r.URL.Path, duration, http.StatusOK)
}

// handleAuth 处理认证请求
func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	// 从X-Original-URI头中提取原始URI
	originalURI := r.Header.Get("X-Original-URI")
	if originalURI == "" {
		originalURI = r.URL.Path
	}

	// 从URI中提取哈希值
	hash := utils.ExtractHashFromPath(originalURI)
	if hash == "" {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// 验证请求签名
	if !s.verifyRequest(r, hash) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// 验证通过
	w.WriteHeader(http.StatusNoContent)
}

// verifyRequest 验证请求签名
func (s *Server) verifyRequest(r *http.Request, hash string) bool {
	// 获取查询参数
	query := r.URL.Query()

	// 获取签名
	signature := query.Get("sign")
	if signature == "" {
		return false
	}

	// 验证签名
	return utils.VerifySignature(s.config.Cluster.Secret, hash, signature)
}
