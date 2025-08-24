package cluster

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/uright008/go-openbmclapi-reborn/config"
	"github.com/uright008/go-openbmclapi-reborn/logger"
	"github.com/uright008/go-openbmclapi-reborn/storage"
	"github.com/uright008/go-openbmclapi-reborn/sync"
	"github.com/uright008/go-openbmclapi-reborn/token"
)

const (
	// 版本号，用于User-Agent
	version = "1.0.0"
)

// Cluster 结构体定义
type Cluster struct {
	ID         string
	Secret     string
	IP         string
	Port       int
	PublicPort int
	BYOC       bool
	Storage    storage.Storage
	Config     *config.Config
	tokenMgr   *token.TokenManager
	syncMgr    *sync.SyncManager
	httpClient *http.Client
	errorMgr   *ErrorRetryManager
	logger     *logger.Logger
	serverURL  string
}

// NewCluster 创建一个新的集群实例
func NewCluster(cfg *config.Config, logger *logger.Logger) (*Cluster, error) {
	// 创建存储实例
	store, err := storage.NewStorage(cfg)
	if err != nil {
		return nil, fmt.Errorf("无法创建存储实例: %w", err)
	}

	// 创建HTTP客户端
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 创建令牌管理器
	serverURL := "https://openbmclapi.bangbang93.com" // 默认服务器URL
	tokenMgr := token.NewTokenManager(cfg.Cluster.ID, cfg.Cluster.Secret, serverURL)

	// 创建同步管理器
	syncMgr := sync.NewSyncManager(store, tokenMgr, logger)

	// 创建错误重试管理器
	errorMgr := NewErrorRetryManager(5, logger)

	cluster := &Cluster{
		ID:         cfg.Cluster.ID,
		Secret:     cfg.Cluster.Secret,
		IP:         cfg.Cluster.IP,
		Port:       cfg.Cluster.Port,
		PublicPort: cfg.Cluster.PublicPort,
		BYOC:       cfg.Cluster.BYOC,
		Storage:    store,
		Config:     cfg,
		tokenMgr:   tokenMgr,
		syncMgr:    syncMgr,
		httpClient: client,
		errorMgr:   errorMgr,
		logger:     logger,
		serverURL:  serverURL,
	}

	return cluster, nil
}

// doRequest 执行HTTP请求的统一方法
func (c *Cluster) doRequest(method, path string, params map[string]string) (*http.Response, error) {
	// 构建完整URL
	url := fmt.Sprintf("%s/%s", c.serverURL, path)

	// 创建请求
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("无法创建请求: %w", err)
	}

	// 获取认证令牌
	token, err := c.tokenMgr.GetToken()
	if err != nil {
		return nil, fmt.Errorf("无法获取认证令牌: %w", err)
	}

	// 设置请求头
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", fmt.Sprintf("openbmclapi-cluster/%s", version))

	// 添加查询参数
	if params != nil {
		q := req.URL.Query()
		for key, value := range params {
			q.Add(key, value)
		}
		req.URL.RawQuery = q.Encode()
	}

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("请求失败: %v", err)
		c.logger.Error("请求详情 - 方法: %s, URL: %s, Headers: %v", method, req.URL.String(), req.Header)
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	// 检查响应状态
	if resp.StatusCode >= 400 {
		// 读取响应体以便记录错误详情
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		c.logger.Error("请求返回错误状态码: %d", resp.StatusCode)
		c.logger.Error("请求详情 - 方法: %s, URL: %s, Headers: %v", method, req.URL.String(), req.Header)
		c.logger.Error("响应详情 - Body: %s", string(body))

		return nil, fmt.Errorf("请求返回错误状态码: %d, 响应内容: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

// Init 初始化集群
func (c *Cluster) Init() error {
	// 初始化存储
	err := c.Storage.Init()
	if err != nil {
		c.errorMgr.RecordError(fmt.Errorf("存储初始化失败: %w", err))
		return fmt.Errorf("存储初始化失败: %w", err)
	}

	// 检查存储是否可用
	ready, err := c.Storage.Check()
	if err != nil {
		c.errorMgr.RecordError(fmt.Errorf("存储检查失败: %w", err))
		return fmt.Errorf("存储检查失败: %w", err)
	}
	if !ready {
		err := fmt.Errorf("存储不可用")
		c.errorMgr.RecordError(err)
		return err
	}

	// 初始化成功，重置错误计数
	c.errorMgr.ResetErrors()
	return nil
}

// Connect 连接到中心服务器
func (c *Cluster) Connect() error {
	c.logger.Info("连接到中心服务器...")

	// 获取认证令牌
	_, err := c.tokenMgr.GetToken()
	if err != nil {
		c.errorMgr.RecordError(fmt.Errorf("无法获取认证令牌: %w", err))
		return fmt.Errorf("无法获取认证令牌: %w", err)
	}

	c.logger.Info("成功连接到中心服务器")
	// 连接成功，重置错误计数
	c.errorMgr.ResetErrors()
	return nil
}

// SyncFiles 同步文件
func (c *Cluster) SyncFiles() error {
	c.logger.Info("开始同步文件...")

	err := c.syncMgr.SyncFiles()
	if err != nil {
		c.errorMgr.RecordError(fmt.Errorf("文件同步失败: %w", err))
		return fmt.Errorf("文件同步失败: %w", err)
	}

	c.logger.Info("文件同步完成")
	// 同步成功，重置错误计数
	c.errorMgr.ResetErrors()
	return nil
}

// Close 关闭集群
func (c *Cluster) Close() error {
	c.logger.Info("关闭集群...")

	// 清理资源逻辑将在这里实现

	return nil
}

// GetFileList 从中心服务器获取文件列表
func (c *Cluster) GetFileList() error {
	// 设置查询参数
	params := map[string]string{}

	// 发送请求
	resp, err := c.doRequest("GET", "openbmclapi/files", params)
	if err != nil {
		c.errorMgr.RecordError(fmt.Errorf("无法获取文件列表: %w", err))
		return fmt.Errorf("无法获取文件列表: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("获取文件列表失败，状态码: %d", resp.StatusCode)
		c.errorMgr.RecordError(err)
		return err
	}

	// 处理响应将在后续实现
	c.logger.Info("成功获取文件列表")

	// 操作成功，重置错误计数
	c.errorMgr.ResetErrors()
	return nil
}
