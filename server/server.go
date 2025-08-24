package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/uright008/go-openbmclapi-reborn/cluster"
	"github.com/uright008/go-openbmclapi-reborn/utils"
)

// Server 定义HTTP服务器结构
type Server struct {
	cluster *cluster.Cluster
	server  *http.Server
}

// New 创建新的HTTP服务器实例
func NewServer(cluster *cluster.Cluster) *Server {
	return &Server{
		cluster: cluster,
	}
}

// Start 启动HTTP服务器
func (s *Server) SetupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// Download route
	mux.HandleFunc("/download/", s.handleDownload)

	// Health check route
	mux.HandleFunc("/health", s.handleHealth)

	return mux
}

func (s *Server) Start(addr string) error {
	mux := s.SetupRoutes()

	s.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	fmt.Printf("Starting server on %s\n", addr)
	return s.server.ListenAndServe()
}

// Stop stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// handleDownload handles file download requests
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Extract hash from URL
	hash := r.URL.Path[len("/download/"):]
	if hash == "" {
		http.Error(w, "Missing hash", http.StatusBadRequest)
		return
	}

	// Parse query parameters
	query := r.URL.Query()

	// Check signature
	// 注意: 这里我们假设cluster.Cluster有一个CheckSign方法，如果没有，我们需要实现它
	// 或者使用utils包中的签名验证函数
	if !utils.VerifySignature(s.cluster.Config.Cluster.Secret, hash, query.Get("sign")) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Get file from storage
	storage := s.cluster.Storage

	// Try to get the file from storage
	fileReader, err := storage.Get(hash)
	if err != nil {
		// File does not exist in storage
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer fileReader.Close()

	// Check if it's a WebDAV storage that returns a redirect
	if redirectReader, ok := fileReader.(interface{ GetRedirectURL() string }); ok {
		// For WebDAV storage, redirect to the actual file location
		redirectURL := redirectReader.GetRedirectURL()
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	// For regular file storage, serve the file content
	// Record hit for statistics
	// 注意: 这里我们假设cluster.Cluster有一个RecordHit方法，如果没有，我们需要实现它
	// s.cluster.RecordHit(0) // TODO: Get actual file size

	// Copy file content to response
	_, err = io.Copy(w, fileReader)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Log request
	duration := time.Since(startTime)
	fmt.Printf("[%s] %s %s %v\n", r.Method, r.URL.Path, "200", duration)
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
	return utils.VerifySignature(s.cluster.Config.Cluster.Secret, hash, signature)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
