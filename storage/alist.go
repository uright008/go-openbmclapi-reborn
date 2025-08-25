package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/uright008/go-openbmclapi-reborn/config"
)

// AListStorage AList存储实现
type AListStorage struct {
	client   *http.Client
	endpoint string
	username string
	password string
	path     string
	token    string
}

// AListLoginRequest AList登录请求
type AListLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AListLoginResponse AList登录响应
type AListLoginResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Token string `json:"token"`
	} `json:"data"`
}

// AListMakeDirRequest AList创建目录请求
type AListMakeDirRequest struct {
	Path string `json:"path"`
}

// AListPutRequest AList上传文件请求
type AListPutRequest struct {
	Path        string `json:"path"`
	File        string `json:"file"`
	Content     []byte `json:"content"`
	ContentType string `json:"content_type"`
}

// AListDeleteRequest AList删除文件请求
type AListDeleteRequest struct {
	Path string `json:"path"`
}

// AListFileInfo AList文件信息
type AListFileInfo struct {
	Name     string      `json:"name"`
	Size     int64       `json:"size"`
	IsDir    bool        `json:"is_dir"`
	Modified interface{} `json:"modified"` // 使用interface{}来处理不同的数据类型
}

// AListListResponse AList列表响应
type AListListResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Content []struct {
			Name     string      `json:"name"`
			Size     int64       `json:"size"`
			IsDir    bool        `json:"is_dir"`
			Modified interface{} `json:"modified"`
		} `json:"content"`
	} `json:"data"`
}

// AListFsInfoResponse AList文件系统信息响应
type AListFsInfoResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Files []AListFileInfo `json:"files"`
	} `json:"data"`
}

// NewAListStorage 创建新的AList存储实例
func NewAListStorage(cfg config.AListConfig) *AListStorage {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 确保路径以斜杠开头，不以斜杠结尾
	path := cfg.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if strings.HasSuffix(path, "/") && path != "/" {
		path = strings.TrimSuffix(path, "/")
	}

	return &AListStorage{
		client:   client,
		endpoint: strings.TrimSuffix(cfg.Endpoint, "/"),
		username: cfg.Username,
		password: cfg.Password,
		path:     path,
		token:    cfg.Token,
	}
}

// Init 初始化AList存储
func (a *AListStorage) Init() error {
	// 如果没有提供token，则尝试登录获取token
	if a.token == "" {
		err := a.login()
		if err != nil {
			return fmt.Errorf("AList登录失败: %w", err)
		}
	}

	// 确保基础目录存在
	err := a.makeDir(a.path)
	if err != nil {
		return fmt.Errorf("无法创建基础目录 %s: %w", a.path, err)
	}

	return nil
}

// login 登录AList获取token
func (a *AListStorage) login() error {
	loginReq := AListLoginRequest{
		Username: a.username,
		Password: a.password,
	}

	body, err := json.Marshal(loginReq)
	if err != nil {
		return fmt.Errorf("无法序列化登录请求: %w", err)
	}

	req, err := http.NewRequest("POST", a.endpoint+"/api/auth/login", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("无法创建登录请求: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("登录请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("登录请求返回状态码: %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("无法读取登录响应: %w", err)
	}

	var loginResp AListLoginResponse
	err = json.Unmarshal(respBody, &loginResp)
	if err != nil {
		return fmt.Errorf("无法解析登录响应: %w", err)
	}

	if loginResp.Code != 200 {
		return fmt.Errorf("登录失败: %s", loginResp.Message)
	}

	a.token = loginResp.Data.Token
	return nil
}

// makeDir 创建目录
func (a *AListStorage) makeDir(path string) error {
	makeDirReq := AListMakeDirRequest{
		Path: path,
	}

	body, err := json.Marshal(makeDirReq)
	if err != nil {
		return fmt.Errorf("无法序列化创建目录请求: %w", err)
	}

	req, err := http.NewRequest("POST", a.endpoint+"/api/fs/mkdir", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("无法创建目录请求: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", a.token)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("创建目录请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("无法读取创建目录响应: %w", err)
	}

	// 200表示成功，409表示目录已存在
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("创建目录失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Check 检查AList存储是否可用
func (a *AListStorage) Check() (bool, error) {
	// 尝试列出基础目录内容
	_, err := a.listDir(a.path)
	if err != nil {
		return false, err
	}

	return true, nil
}

// Get 获取文件，返回重定向URL而不是实际文件内容
func (a *AListStorage) Get(hash string) (io.ReadCloser, error) {
	// 构建文件在AList服务器上的路径
	filePath := filepath.Join(a.path, hash[:2], hash)

	// 构建可访问的URL
	fullURL := a.endpoint + "/d" + filePath

	// 返回一个包含重定向URL的特殊ReadCloser
	return &redirectReadCloser{redirectURL: fullURL}, nil
}

// Put 存储文件
func (a *AListStorage) Put(hash string, data io.Reader) error {
	// 创建目录
	dir := filepath.Join(a.path, hash[:2])
	err := a.makeDir(dir)
	if err != nil {
		return fmt.Errorf("无法创建目录 %s: %w", dir, err)
	}

	// 读取数据
	fileData, err := io.ReadAll(data)
	if err != nil {
		return fmt.Errorf("无法读取文件数据: %w", err)
	}

	// 构建文件路径
	filePath := filepath.Join(dir, hash)

	// 上传文件
	err = a.uploadFile(filePath, fileData)
	if err != nil {
		return fmt.Errorf("无法上传文件 %s: %w", filePath, err)
	}

	return nil
}

// uploadFile 上传文件到AList
func (a *AListStorage) uploadFile(path string, data []byte) error {
	// AList的上传API需要使用multipart/form-data格式
	// 这里我们使用简单的PUT方法上传文件

	// 构建完整URL
	url := a.endpoint + "/api/fs/put"

	// 创建请求
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("无法创建上传请求: %w", err)
	}

	// 设置头部
	req.Header.Set("Authorization", a.token)
	req.Header.Set("File-Path", path)
	req.Header.Set("Content-Type", "application/octet-stream")

	// 发送请求
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("上传文件失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("无法读取上传响应: %w", err)
	}

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("上传文件失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Delete 删除文件
func (a *AListStorage) Delete(hash string) error {
	// 构建文件路径
	filePath := filepath.Join(a.path, hash[:2], hash)

	// 删除文件
	err := a.deleteFile(filePath)
	if err != nil {
		// 如果是文件不存在错误，我们不返回错误
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("无法删除文件 %s: %w", filePath, err)
	}

	return nil
}

// deleteFile 从AList删除文件
func (a *AListStorage) deleteFile(path string) error {
	deleteReq := AListDeleteRequest{
		Path: path,
	}

	body, err := json.Marshal(deleteReq)
	if err != nil {
		return fmt.Errorf("无法序列化删除请求: %w", err)
	}

	req, err := http.NewRequest("POST", a.endpoint+"/api/fs/remove", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("无法创建删除请求: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", a.token)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("删除文件请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("无法读取删除响应: %w", err)
	}

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("删除文件失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Exists 检查文件是否存在
func (a *AListStorage) Exists(hash string) (bool, error) {
	// 构建文件路径
	filePath := filepath.Join(a.path, hash[:2], hash)

	// 检查文件是否存在
	exists, err := a.fileExists(filePath)
	if err != nil {
		return false, fmt.Errorf("检查文件存在性失败 %s: %w", filePath, err)
	}

	return exists, nil
}

// fileExists 检查AList中的文件是否存在
func (a *AListStorage) fileExists(path string) (bool, error) {
	// 使用list接口检查文件是否存在
	dir := filepath.Dir(path)
	filename := filepath.Base(path)

	files, err := a.listDir(dir)
	if err != nil {
		return false, err
	}

	for _, file := range files {
		if file.Name == filename && !file.IsDir {
			return true, nil
		}
	}

	return false, nil
}

// listDir 列出AList目录中的文件
func (a *AListStorage) listDir(path string) ([]AListFileInfo, error) {
	// 构建请求URL
	url := fmt.Sprintf("%s/api/fs/list?path=%s", a.endpoint, path)

	// 创建请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("无法创建列表请求: %w", err)
	}

	// 设置头部
	req.Header.Set("Authorization", a.token)

	// 发送请求
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("列表请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("无法读取列表响应: %w", err)
	}

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("列表请求失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 解析响应
	decoder := json.NewDecoder(strings.NewReader(string(respBody)))
	decoder.UseNumber()

	var listResp AListListResponse
	err = decoder.Decode(&listResp)
	if err != nil {
		return nil, fmt.Errorf("无法解析列表响应: %w", err)
	}

	if listResp.Code != 200 {
		return nil, fmt.Errorf("列表请求失败: %s", listResp.Message)
	}

	// 转换为统一的AListFileInfo结构
	var result []AListFileInfo
	for _, item := range listResp.Data.Content {
		info := AListFileInfo{
			Name:     item.Name,
			Size:     item.Size,
			IsDir:    item.IsDir,
			Modified: item.Modified,
		}
		result = append(result, info)
	}

	return result, nil
}

// WriteFile 写入文件
func (a *AListStorage) WriteFile(filePath string, content []byte, fileInfo *FileInfo) error {
	// 构建完整路径
	fullPath := filepath.Join(a.path, filePath)

	// 确保目录存在
	dir := filepath.Dir(fullPath)
	err := a.makeDir(dir)
	if err != nil {
		return fmt.Errorf("无法创建目录 %s: %w", dir, err)
	}

	// 上传文件
	err = a.uploadFile(fullPath, content)
	if err != nil {
		return fmt.Errorf("无法写入文件 %s: %w", fullPath, err)
	}

	return nil
}

// ListFiles 列出所有已存在的文件
func (a *AListStorage) ListFiles() ([]*FileInfo, error) {
	var files []*FileInfo

	// 遍历存储目录，获取所有已存在的文件
	err := a.walkDir(a.path, "", &files)
	if err != nil {
		return nil, fmt.Errorf("遍历目录失败: %w", err)
	}

	return files, nil
}

// walkDir 递归遍历目录
func (a *AListStorage) walkDir(basePath, relPath string, files *[]*FileInfo) error {
	currentPath := filepath.Join(basePath, relPath)
	entries, err := a.listDir(currentPath)
	if err != nil {
		// 忽略无法访问的目录
		// 但记录警告信息以便调试
		fmt.Printf("[WARN] 无法访问目录 %s: %v\n", currentPath, err)
		return nil
	}

	for _, entry := range entries {
		entryRelPath := filepath.Join(relPath, entry.Name)

		if entry.IsDir {
			// 递归处理子目录
			err := a.walkDir(basePath, entryRelPath, files)
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
						Size: entry.Size,
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
func (a *AListStorage) GetMissingFiles(files []*FileInfo) ([]*FileInfo, error) {
	// 获取所有已存在的文件
	existingFiles, err := a.ListFiles()
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
func (a *AListStorage) GC(files []*FileInfo) error {
	// 获取所有已存在的文件
	existingFiles, err := a.ListFiles()
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
			err := a.Delete(file.Hash)
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
func (a *AListStorage) GetLastModified() (int64, error) {
	var lastModified int64

	// 遍历所有文件查找最新修改时间
	err := a.walkDirModified(a.path, "", &lastModified)
	if err != nil {
		return 0, fmt.Errorf("遍历目录失败: %w", err)
	}

	return lastModified, nil
}

// walkDirModified 递归遍历目录查找最新修改时间
func (a *AListStorage) walkDirModified(basePath, relPath string, lastModified *int64) error {
	currentPath := filepath.Join(basePath, relPath)
	entries, err := a.listDir(currentPath)
	if err != nil {
		// 忽略无法访问的目录
		return nil
	}

	for _, entry := range entries {
		entryRelPath := filepath.Join(relPath, entry.Name)

		if entry.IsDir {
			// 递归处理子目录
			err := a.walkDirModified(basePath, entryRelPath, lastModified)
			if err != nil {
				return err
			}
		} else {
			// 检查文件修改时间
			modifiedTime := a.parseModifiedTime(entry.Modified)
			if modifiedTime > *lastModified {
				*lastModified = modifiedTime
			}
		}
	}

	return nil
}

// parseModifiedTime 解析修改时间，处理不同类型的modified字段
func (a *AListStorage) parseModifiedTime(modified interface{}) int64 {
	switch v := modified.(type) {
	case string:
		// 尝试解析时间戳字符串
		var t int64
		if _, err := fmt.Sscanf(v, "%d", &t); err == nil {
			return t
		}
		return 0
	case int64:
		return v
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		return 0
	default:
		return 0
	}
}
