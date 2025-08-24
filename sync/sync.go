package sync

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/linkedin/goavro/v2"
	"github.com/uright008/go-openbmclapi-reborn/config"
	"github.com/uright008/go-openbmclapi-reborn/logger"
	"github.com/uright008/go-openbmclapi-reborn/storage"
	"github.com/uright008/go-openbmclapi-reborn/token"
)

const (
	// 版本号，用于User-Agent
	version = "1.0.0"
)

// File 表示一个需要同步的文件
type File struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	Hash  string `json:"hash"`
	MTime int64  `json:"mtime"`
}

// SyncManager 管理文件同步
type SyncManager struct {
	storage   storage.Storage
	tokenMgr  *token.TokenManager
	client    *http.Client
	serverURL string
	logger    *logger.Logger
	errorMgr  *ErrorRetryManager
	config    *config.SyncConfig
}

// NewSyncManager 创建新的同步管理器
func NewSyncManager(storage storage.Storage, tokenMgr *token.TokenManager, logger *logger.Logger, syncConfig *config.SyncConfig) *SyncManager {
	return &SyncManager{
		storage:   storage,
		tokenMgr:  tokenMgr,
		client:    &http.Client{Timeout: 30 * time.Second},
		serverURL: "https://openbmclapi.bangbang93.com",
		logger:    logger,
		errorMgr:  NewErrorRetryManager(5, logger),
		config:    syncConfig,
	}
}

// doRequest 执行HTTP请求的统一方法
func (sm *SyncManager) doRequest(method, path string, params map[string]string) (*http.Response, error) {
	// 构建完整URL
	url := fmt.Sprintf("%s/%s", sm.serverURL, path)

	// 创建请求
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("无法创建请求: %w", err)
	}

	// 获取认证令牌
	token, err := sm.tokenMgr.GetToken()
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
	resp, err := sm.client.Do(req)
	if err != nil {
		sm.logger.Error("请求失败: %v", err)
		// 对Authorization头进行脱敏处理
		headers := req.Header.Clone()
		if headers.Get("Authorization") != "" {
			headers.Set("Authorization", "Bearer ***")
		}
		sm.logger.Error("请求详情 - 方法: %s, URL: %s, Headers: %v", method, req.URL.String(), headers)
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	// 检查响应状态
	if resp.StatusCode >= 400 {
		// 读取响应体以便记录错误详情
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		sm.logger.Error("请求返回错误状态码: %d", resp.StatusCode)
		// 对Authorization头进行脱敏处理
		headers := req.Header.Clone()
		if headers.Get("Authorization") != "" {
			headers.Set("Authorization", "Bearer ***")
		}
		sm.logger.Error("请求详情 - 方法: %s, URL: %s, Headers: %v", method, req.URL.String(), headers)
		sm.logger.Error("响应详情 - Body: %s", string(body))

		return nil, fmt.Errorf("请求返回错误状态码: %d, 响应内容: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

// decompress 使用zstd解压缩数据
func decompress(data []byte) ([]byte, error) {
	reader, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("创建zstd解压器失败: %w", err)
	}
	defer reader.Close()

	decompressed, err := reader.DecodeAll(data, nil)
	if err != nil {
		return nil, fmt.Errorf("解压数据失败: %w", err)
	}

	return decompressed, nil
}

// GetFileList 从中心服务器获取文件列表
func (sm *SyncManager) GetFileList() ([]*File, error) {
	// 获取最后修改时间
	lastModified, err := sm.storage.GetLastModified()
	if err != nil {
		sm.logger.Warn("无法获取最后修改时间: %v", err)
		lastModified = 0 // 如果无法获取最后修改时间，则获取所有文件
	}

	// 设置查询参数
	params := map[string]string{
		"lastModified": fmt.Sprintf("%d", lastModified),
	}

	// 发送请求
	resp, err := sm.doRequest("GET", "openbmclapi/files", params)
	if err != nil {
		sm.errorMgr.RecordError(fmt.Errorf("无法获取文件列表: %w", err))
		return nil, fmt.Errorf("无法获取文件列表: %w", err)
	}
	defer resp.Body.Close()

	// 处理NO_CONTENT状态码 (204) - 表示没有文件需要同步
	if resp.StatusCode == http.StatusNoContent {
		sm.logger.Info("服务器返回无内容状态 (204) - 没有文件需要同步")
		// 返回空的文件列表
		return []*File{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("获取文件列表失败，状态码: %d", resp.StatusCode)
		sm.errorMgr.RecordError(err)
		return nil, err
	}

	// 以二进制方式读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		sm.errorMgr.RecordError(fmt.Errorf("无法读取响应: %w", err))
		return nil, fmt.Errorf("无法读取响应: %w", err)
	}

	// 使用zstd解压缩整个响应体
	decompressed, err := decompress(body)
	if err != nil {
		sm.errorMgr.RecordError(fmt.Errorf("解压响应数据失败: %w", err))
		return nil, fmt.Errorf("解压响应数据失败: %w", err)
	}

	// 将解压后的数据转换为文件列表
	files, err := convertBytesToFiles(decompressed)
	if err != nil {
		sm.errorMgr.RecordError(fmt.Errorf("解析文件列表失败: %w", err))
		return nil, fmt.Errorf("解析文件列表失败: %w", err)
	}

	// 操作成功，重置错误计数
	sm.errorMgr.ResetErrors()
	return files, nil
}

// convertBytesToFiles 将解压后的字节数据转换为文件列表
func convertBytesToFiles(data []byte) ([]*File, error) {
	// 定义与Node.js版本对应的Avro Schema
	schema := `{
		"type": "array",
		"items": {
		  "name": "FileListEntry",
		  "type": "record",
		  "fields": [
			{"name": "path", "type": "string"},
			{"name": "hash", "type": "string"},
			{"name": "size", "type": "long"},
			{"name": "mtime", "type": "long"}
		  ]
		}
	  }`

	// 创建Avro编解码器
	codec, err := goavro.NewCodec(schema)
	if err != nil {
		return nil, fmt.Errorf("创建Avro编解码器失败: %w", err)
	}

	// 解码Avro数据
	native, _, err := codec.NativeFromBinary(data)
	if err != nil {
		return nil, fmt.Errorf("从二进制数据解码Avro失败: %w", err)
	}

	// 类型断言为切片
	records, ok := native.([]interface{})
	if !ok {
		return nil, fmt.Errorf("解码的数据不是预期的数组类型")
	}

	// 转换为文件列表
	var files []*File
	for _, record := range records {
		// 类型断言为map
		recordMap, ok := record.(map[string]interface{})
		if !ok {
			continue
		}

		file := &File{}

		if path, ok := recordMap["path"].(string); ok {
			file.Path = path
		}

		if hash, ok := recordMap["hash"].(string); ok {
			file.Hash = hash
		}

		if size, ok := recordMap["size"].(int64); ok {
			file.Size = size
		} else if size, ok := recordMap["size"].(int32); ok {
			file.Size = int64(size)
		}

		if mtime, ok := recordMap["mtime"].(int64); ok {
			file.MTime = mtime
		} else if mtime, ok := recordMap["mtime"].(int32); ok {
			file.MTime = int64(mtime)
		}

		files = append(files, file)
	}

	return files, nil
}

// SyncFiles 同步文件
func (sm *SyncManager) SyncFiles() error {
	var lastErr error

	// 最多重试5次
	for i := 0; i < 5; i++ {
		// 获取文件列表
		files, err := sm.GetFileList()
		if err != nil {
			lastErr = fmt.Errorf("无法获取文件列表: %w", err)
			sm.logger.Error("获取文件列表失败 (%d/5): %v", i+1, err)

			// 等待一段时间再重试
			if i < 4 { // 最后一次不需要等待
				time.Sleep(time.Duration(i+1) * time.Second)
			}
			continue
		}

		// 检查是否没有文件需要同步
		if len(files) == 0 {
			sm.logger.Info("没有文件需要同步")
			sm.errorMgr.ResetErrors()
			return nil
		}

		// 转换文件格式
		storageFiles := convertFiles(files)

		// 检查缺失的文件
		missingFiles, err := sm.storage.GetMissingFiles(storageFiles)
		if err != nil {
			lastErr = fmt.Errorf("无法检查缺失的文件: %w", err)
			sm.logger.Error("检查缺失文件失败 (%d/5): %v", i+1, err)

			// 等待一段时间再重试
			if i < 4 { // 最后一次不需要等待
				time.Sleep(time.Duration(i+1) * time.Second)
			}
			continue
		}

		// 下载缺失的文件 (限制并发数和启动间隔)
		failedCount := sm.downloadMissingFiles(missingFiles)

		if failedCount > 0 {
			lastErr = fmt.Errorf("有 %d 个文件下载失败", failedCount)
			sm.logger.Error("文件下载失败 (%d/5): %v", i+1, lastErr)

			// 等待一段时间再重试
			if i < 4 { // 最后一次不需要等待
				time.Sleep(time.Duration(i+1) * time.Second)
			}
			continue
		}

		// 同步成功，重置错误计数
		sm.errorMgr.ResetErrors()
		sm.logger.Info("文件同步完成，共处理 %d 个文件", len(files))
		return nil
	}

	// 所有重试都失败了
	sm.errorMgr.RecordError(lastErr)
	return fmt.Errorf("文件同步失败，已重试5次: %w", lastErr)
}

// downloadMissingFiles 下载缺失的文件，限制并发数和启动间隔
func (sm *SyncManager) downloadMissingFiles(missingFiles []*storage.FileInfo) int {
	maxConcurrent := sm.config.MaxConcurrency
	startInterval := sm.config.StartIntervalMs

	// 如果最大并发数设置为0或负数，则使用默认值64
	if maxConcurrent <= 0 {
		maxConcurrent = 64
	}

	// 如果启动间隔设置为负数，则使用默认值100ms
	if startInterval < 0 {
		startInterval = 100
	}

	// 创建信号量控制并发数
	semaphore := make(chan struct{}, maxConcurrent)

	// 创建错误通道收集下载错误
	errChan := make(chan error, len(missingFiles))

	// 创建等待组等待所有下载完成
	var wg sync.WaitGroup

	// 创建进度计数器
	var downloadedCount int64
	totalFiles := len(missingFiles)

	// 显示初始进度信息
	sm.logger.Info("文件总数: 0/%d", totalFiles)

	// 下载每个缺失的文件
	for i, file := range missingFiles {
		// 控制启动间隔
		if i > 0 {
			time.Sleep(time.Duration(startInterval) * time.Millisecond)
		}

		// 增加等待组计数
		wg.Add(1)

		// 启动下载协程
		go func(f *storage.FileInfo) {
			// 释放信号量和等待组
			defer func() {
				// 增加已完成计数
				current := atomic.AddInt64(&downloadedCount, 1)

				// 更新总进度（使用\r实现原地更新）
				sm.logger.Info("\r文件总数: %d/%d", current, totalFiles)

				<-semaphore
				wg.Done()
			}()

			// 获取信号量
			semaphore <- struct{}{}

			// 下载文件
			if err := sm.downloadFile(f); err != nil {
				errChan <- err
			}
		}(file)
	}

	// 等待所有下载完成
	wg.Wait()

	// 确保进度显示最终状态
	sm.logger.Info("\r文件总数: %d/%d", totalFiles, totalFiles)

	// 关闭错误通道
	close(errChan)

	// 统计失败数量
	failedCount := 0
	for range errChan {
		failedCount++
	}

	// 显示最终结果
	sm.logger.Info("\n文件下载完成: 成功 %d, 失败 %d, 总计 %d",
		totalFiles-failedCount, failedCount, totalFiles)

	return failedCount
}

// downloadFile 下载单个文件
func (sm *SyncManager) downloadFile(file *storage.FileInfo) error {
	// 创建请求路径
	path := fmt.Sprintf("openbmclapi/download/%s", file.Hash)

	// 发送请求
	resp, err := sm.doRequest("GET", path, nil)
	if err != nil {
		sm.errorMgr.RecordError(fmt.Errorf("无法下载文件: %w", err))
		return fmt.Errorf("无法下载文件: %w", err)
	}
	defer resp.Body.Close()

	// 保存文件
	if err := sm.storage.Put(file.Hash, resp.Body); err != nil {
		sm.errorMgr.RecordError(fmt.Errorf("无法保存文件: %w", err))
		return fmt.Errorf("无法保存文件: %w", err)
	}

	// 操作成功，重置错误计数
	sm.errorMgr.ResetErrors()
	return nil
}

// convertFiles 转换文件格式
func convertFiles(files []*File) []*storage.FileInfo {
	var result []*storage.FileInfo
	for _, file := range files {
		result = append(result, &storage.FileInfo{
			Hash: file.Hash,
			Size: file.Size,
			Path: file.Path,
		})
	}
	return result
}

// ErrorRetryManager 错误重试管理器
type ErrorRetryManager struct {
	maxRetries    int
	errorCount    int
	lastErrorTime time.Time
	logger        *logger.Logger
}

// NewErrorRetryManager 创建新的错误重试管理器
func NewErrorRetryManager(maxRetries int, logger *logger.Logger) *ErrorRetryManager {
	return &ErrorRetryManager{
		maxRetries: maxRetries,
		logger:     logger,
	}
}

// RecordError 记录错误，如果错误次数超过最大重试次数则关闭进程
func (erm *ErrorRetryManager) RecordError(err error) {
	erm.errorCount++
	erm.lastErrorTime = time.Now()

	erm.logger.Error("发生错误 (%d/%d): %v", erm.errorCount, erm.maxRetries, err)

	if erm.errorCount > erm.maxRetries {
		erm.logger.Fatal("错误次数超过最大重试次数 (%d)，正在关闭进程", erm.maxRetries)
	}
}

// ResetErrors 重置错误计数
func (erm *ErrorRetryManager) ResetErrors() {
	if erm.errorCount > 0 {
		erm.logger.Info("重置错误计数: %d -> 0", erm.errorCount)
		erm.errorCount = 0
	}
}

// GetErrorCount 获取当前错误计数
func (erm *ErrorRetryManager) GetErrorCount() int {
	return erm.errorCount
}
