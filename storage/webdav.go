package storage

import (
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"
	"github.com/uright008/go-openbmclapi-reborn/config"
)

// WebDAVStorage WebDAV存储实现
type WebDAVStorage struct {
	client   *gowebdav.Client
	endpoint string
	username string
	password string
	path     string
}

// NewWebDAVStorage 创建新的WebDAV存储实例
func NewWebDAVStorage(cfg config.WebDAVConfig) *WebDAVStorage {
	client := gowebdav.NewClient(cfg.Endpoint, cfg.Username, cfg.Password)

	// 确保路径以斜杠结尾
	path := cfg.Path
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	return &WebDAVStorage{
		client:   client,
		endpoint: cfg.Endpoint,
		username: cfg.Username,
		password: cfg.Password,
		path:     path,
	}
}

// Init 初始化WebDAV存储
func (w *WebDAVStorage) Init() error {
	// 检查连接是否正常
	err := w.retryOnLock(func() error {
		return w.client.Connect()
	})
	if err != nil {
		return fmt.Errorf("无法连接到WebDAV服务器: %w", err)
	}

	// 确保基础目录存在
	err = w.retryOnLock(func() error {
		return w.client.MkdirAll(w.path, 0755)
	})
	if err != nil {
		return fmt.Errorf("无法创建基础目录 %s: %w", w.path, err)
	}

	return nil
}

// Check 检查WebDAV存储是否可用
func (w *WebDAVStorage) Check() (bool, error) {
	// 尝试列出来基础目录内容
	_, err := w.client.ReadDir(w.path)
	if err != nil {
		return false, err
	}

	return true, nil
}

// Get 获取文件，返回重定向URL而不是实际文件内容
func (w *WebDAVStorage) Get(hash string) (io.ReadCloser, error) {
	// 构建文件在WebDAV服务器上的路径
	filePath := filepath.Join(w.path, hash[:2], hash)

	// 构建可访问的URL
	// 移除endpoint末尾的斜杠，添加文件路径
	endpoint := strings.TrimSuffix(w.endpoint, "/")

	// 确保filePath以斜杠开头
	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}

	// 构建完整URL
	fullURL := endpoint + filePath

	// URL编码
	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		return nil, fmt.Errorf("无法解析URL %s: %w", fullURL, err)
	}

	// 返回一个包含重定向URL的特殊ReadCloser
	return &redirectReadCloser{redirectURL: parsedURL.String()}, nil
}

// redirectReadCloser 一个特殊的ReadCloser，包含重定向URL
type redirectReadCloser struct {
	redirectURL string
}

// Read 实现io.Reader接口
func (r *redirectReadCloser) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("should redirect to %s", r.redirectURL)
}

// Close 实现io.Closer接口
func (r *redirectReadCloser) Close() error {
	return nil
}

// Put 存储文件
func (w *WebDAVStorage) Put(hash string, data io.Reader) error {
	// 创建目录
	dir := filepath.Join(w.path, hash[:2])
	err := w.retryOnLock(func() error {
		return w.client.MkdirAll(dir, 0755)
	})
	if err != nil {
		return fmt.Errorf("无法创建目录 %s: %w", dir, err)
	}

	// 读取数据
	fileData, err := io.ReadAll(data)
	if err != nil {
		return fmt.Errorf("无法读取文件数据: %w", err)
	}

	// 上传文件
	filePath := strings.ReplaceAll(filepath.Join(dir, hash), "\\", "/")
	err = w.retryOnLock(func() error {
		return w.client.Write(filePath, fileData, 0644)
	})
	if err != nil {
		return fmt.Errorf("无法写入文件 %s: %w", filePath, err)
	}

	return nil
}

// Delete 删除文件
func (w *WebDAVStorage) Delete(hash string) error {
	// 构建文件路径
	filePath := filepath.Join(w.path, hash[:2], hash)

	// 执行删除操作
	err := w.retryOnLock(func() error {
		return w.client.Remove(filePath)
	})

	// 如果是404错误（文件不存在），我们不返回错误
	if err != nil && (strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found")) {
		return nil
	}

	return err
}

// Exists 检查文件是否存在
func (w *WebDAVStorage) Exists(hash string) (bool, error) {
	filePath := filepath.Join(w.path, hash[:2], hash)
	err := w.retryOnLock(func() error {
		_, err := w.client.Stat(filePath)
		return err
	})

	if err != nil {
		// 检查是否是文件不存在错误
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// retryOnLock 在遇到423锁定错误时重试操作
func (w *WebDAVStorage) retryOnLock(operation func() error) error {
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		err := operation()
		if err != nil {
			// 检查是否是423锁定错误
			if strings.Contains(err.Error(), "423") || strings.Contains(err.Error(), "Locked") {
				// 如果不是最后一次重试，则等待1分钟后重试
				if i < maxRetries-1 {
					fmt.Printf("[INFO] 遇到423锁定错误，等待1分钟后重试 (%d/%d)\n", i+1, maxRetries-1)
					time.Sleep(1 * time.Minute)
					continue
				}
			}
			// 如果不是423错误或已达到最大重试次数，则返回错误
			return err
		}
		// 操作成功，返回nil
		return nil
	}
	return nil
}

// WriteFile 写入文件
func (w *WebDAVStorage) WriteFile(filePath string, content []byte, fileInfo *FileInfo) error {
	fullPath := filepath.Join(w.path, filePath)
	// 确保目录存在
	dir := filepath.Dir(fullPath)
	err := w.retryOnLock(func() error {
		return w.client.MkdirAll(dir, 0755)
	})
	if err != nil {
		return fmt.Errorf("无法创建目录: %w", err)
	}

	// 写入文件
	err = w.retryOnLock(func() error {
		return w.client.Write(fullPath, content, 0644)
	})
	if err != nil {
		return fmt.Errorf("无法写入文件 %s: %w", fullPath, err)
	}
	return nil
}

// ListFiles 列出所有已存在的文件
func (w *WebDAVStorage) ListFiles() ([]*FileInfo, error) {
	var files []*FileInfo

	// 遍历存储目录，获取所有已存在的文件
	err := w.walkDir(w.path, "", &files)
	if err != nil {
		return nil, fmt.Errorf("遍历目录失败: %w", err)
	}

	return files, nil
}

// walkDir 递归遍历目录
func (w *WebDAVStorage) walkDir(basePath, relPath string, files *[]*FileInfo) error {
	currentPath := filepath.Join(basePath, relPath)
	entries, err := w.client.ReadDir(currentPath)
	if err != nil {
		// 忽略无法访问的目录
		return nil
	}

	for _, entry := range entries {
		entryRelPath := filepath.Join(relPath, entry.Name())

		if entry.IsDir() {
			// 递归处理子目录
			err := w.walkDir(basePath, entryRelPath, files)
			if err != nil {
				return err
			}
		} else {
			// 处理文件
			// 验证是否符合我们的存储结构（两级目录结构）
			// 检查路径是否至少有3个字符（两级目录）并且第三个字符是路径分隔符
			if len(entryRelPath) >= 3 {
				// 检查是否符合两级目录结构（例如 "ab/cdefghijk..."）
				parts := strings.Split(filepath.ToSlash(entryRelPath), "/")
				if len(parts) >= 2 && len(parts[0]) == 2 {
					// 提取文件名（hash）
					hash := strings.ReplaceAll(entryRelPath, string(filepath.Separator), "")[2:]
					fileInfo := &FileInfo{
						Hash: hash,
						Size: entry.Size(),
						Path: filepath.Join(basePath, entryRelPath),
					}
					*files = append(*files, fileInfo)
				}
			}
		}
	}

	return nil
}

// GetMissingFiles 获取缺失的文件列表
func (w *WebDAVStorage) GetMissingFiles(files []*FileInfo) ([]*FileInfo, error) {
	// 获取所有已存在的文件
	existingFiles, err := w.ListFiles()
	if err != nil {
		return nil, fmt.Errorf("无法列出已存在的文件: %w", err)
	}

	// 创建一个map来存储本地已存在的文件
	existingMap := make(map[string]bool)
	for _, file := range existingFiles {
		existingMap[file.Hash] = true
	}

	// 找出缺失的文件
	var missing []*FileInfo
	for _, file := range files {
		if !existingMap[file.Hash] {
			missing = append(missing, file)
		}
	}

	return missing, nil
}

// GC 垃圾回收
func (w *WebDAVStorage) GC(files []*FileInfo) error {
	// 获取所有已存在的文件
	existingFiles, err := w.ListFiles()
	if err != nil {
		return fmt.Errorf("无法列出已存在的文件: %w", err)
	}

	// 创建一个map来存储需要保留的文件
	keepMap := make(map[string]bool)
	for _, file := range files {
		keepMap[file.Hash] = true
	}

	// 删除不需要的文件
	var deletedCount int
	for _, file := range existingFiles {
		if !keepMap[file.Hash] {
			err := w.Delete(file.Hash)
			if err != nil {
				// 记录错误但继续删除其他文件
				fmt.Printf("无法删除文件 %s: %v\n", file.Hash, err)
				continue
			}
			deletedCount++
		}
	}

	fmt.Printf("垃圾回收完成，删除了 %d 个文件\n", deletedCount)
	return nil
}

// GetLastModified 获取存储中所有文件的最新修改时间（Unix时间戳）
func (w *WebDAVStorage) GetLastModified() (int64, error) {
	var lastModified int64

	// 遍历所有文件查找最新修改时间
	err := w.walkDirModified(w.path, "", &lastModified)
	if err != nil {
		return 0, fmt.Errorf("遍历目录失败: %w", err)
	}

	return lastModified, nil
}

// walkDirModified 递归遍历目录查找最新修改时间
func (w *WebDAVStorage) walkDirModified(basePath, relPath string, lastModified *int64) error {
	currentPath := filepath.Join(basePath, relPath)
	entries, err := w.client.ReadDir(currentPath)
	if err != nil {
		// 忽略无法访问的目录
		return nil
	}

	for _, entry := range entries {
		entryRelPath := filepath.Join(relPath, entry.Name())

		if entry.IsDir() {
			// 递归处理子目录
			err := w.walkDirModified(basePath, entryRelPath, lastModified)
			if err != nil {
				return err
			}
		} else {
			// 检查文件修改时间
			modTime := entry.ModTime().Unix()
			if modTime > *lastModified {
				*lastModified = modTime
			}
		}
	}

	return nil
}
