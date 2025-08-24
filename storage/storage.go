package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/uright008/go-openbmclapi-reborn/config"
)

// FileInfo 文件信息
type FileInfo struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
	Path string `json:"path"`
}

// Storage 定义存储接口
type Storage interface {
	// Init 初始化存储
	Init() error

	// Check 检查存储是否可用
	Check() (bool, error)

	// Get 获取文件
	Get(hash string) (io.ReadCloser, error)

	// Put 存储文件
	Put(hash string, data io.Reader) error

	// Delete 删除文件
	Delete(hash string) error

	// Exists 检查文件是否存在
	Exists(hash string) (bool, error)

	// WriteFile 写入文件
	WriteFile(path string, content []byte, fileInfo *FileInfo) error

	// GetMissingFiles 获取缺失的文件列表
	GetMissingFiles(files []*FileInfo) ([]*FileInfo, error)

	// GC 垃圾回收
	GC(files []*FileInfo) error

	// GetLastModified 获取存储中所有文件的最新修改时间（Unix时间戳）
	GetLastModified() (int64, error)
}

// FileStorage 文件存储实现
type FileStorage struct {
	path string
}

// NewStorage 创建新的存储实例
func NewStorage(cfg *config.Config) (Storage, error) {
	switch cfg.Storage.Type {
	case "file":
		return &FileStorage{
			path: cfg.Storage.Path,
		}, nil
	default:
		return nil, fmt.Errorf("不支持的存储类型: %s", cfg.Storage.Type)
	}
}

// Init 初始化文件存储
func (fs *FileStorage) Init() error {
	// 创建存储目录
	err := os.MkdirAll(fs.path, 0755)
	if err != nil {
		return fmt.Errorf("无法创建存储目录 %s: %w", fs.path, err)
	}
	return nil
}

// Check 检查文件存储是否可用
func (fs *FileStorage) Check() (bool, error) {
	// 检查目录是否存在且可写
	_, err := os.Stat(fs.path)
	if os.IsNotExist(err) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	// 尝试创建测试文件
	testFile := filepath.Join(fs.path, ".check")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		return false, err
	}

	// 删除测试文件
	_ = os.Remove(testFile)

	return true, nil
}

// Get 获取文件
func (fs *FileStorage) Get(hash string) (io.ReadCloser, error) {
	path := filepath.Join(fs.path, hash[:2], hash)
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// Put 存储文件
func (fs *FileStorage) Put(hash string, data io.Reader) error {
	// 创建目录
	dir := filepath.Join(fs.path, hash[:2])
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("无法创建目录 %s: %w", dir, err)
	}

	// 创建文件
	path := filepath.Join(dir, hash)
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("无法创建文件 %s: %w", path, err)
	}
	defer file.Close()

	// 写入数据
	_, err = io.Copy(file, data)
	if err != nil {
		return fmt.Errorf("无法写入文件 %s: %w", path, err)
	}

	return nil
}

// Delete 删除文件
func (fs *FileStorage) Delete(hash string) error {
	path := filepath.Join(fs.path, hash[:2], hash)
	err := os.Remove(path)
	if err != nil {
		return err
	}
	return nil
}

// Exists 检查文件是否存在
func (fs *FileStorage) Exists(hash string) (bool, error) {
	path := filepath.Join(fs.path, hash[:2], hash)
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// WriteFile 写入文件
func (fs *FileStorage) WriteFile(path string, content []byte, fileInfo *FileInfo) error {
	fullPath := filepath.Join(fs.path, path)

	// 确保目录存在
	dir := filepath.Dir(fullPath)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("无法创建目录: %w", err)
	}

	// 写入文件
	err = os.WriteFile(fullPath, content, 0644)
	if err != nil {
		return fmt.Errorf("无法写入文件: %w", err)
	}

	return nil
}

// GetMissingFiles 获取缺失的文件列表
func (fs *FileStorage) GetMissingFiles(files []*FileInfo) ([]*FileInfo, error) {
	var missing []*FileInfo

	for _, file := range files {
		exists, err := fs.Exists(file.Hash)
		if err != nil {
			return nil, err
		}

		if !exists {
			missing = append(missing, file)
		}
	}

	return missing, nil
}

// GC 垃圾回收
func (fs *FileStorage) GC(files []*FileInfo) error {
	// 创建有效文件的映射
	validFiles := make(map[string]bool)
	for _, file := range files {
		validFiles[file.Hash] = true
	}

	// 遍历缓存目录，删除无效文件
	err := filepath.Walk(fs.path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// 获取相对路径作为哈希值
		relPath, err := filepath.Rel(fs.path, path)
		if err != nil {
			return err
		}

		// 如果文件不在有效文件列表中，则删除
		if !validFiles[relPath] {
			err = os.Remove(path)
			if err != nil {
				fmt.Printf("无法删除文件 %s: %v\n", path, err)
			} else {
				fmt.Printf("已删除无效文件: %s\n", path)
			}
		}

		return nil
	})

	return err
}

// GetLastModified 获取存储中所有文件的最新修改时间（Unix时间戳）
func (fs *FileStorage) GetLastModified() (int64, error) {
	var lastModified int64 = 0

	err := filepath.Walk(fs.path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			// 获取文件的修改时间
			modTime := info.ModTime().Unix()
			if modTime > lastModified {
				lastModified = modTime
			}
		}

		return nil
	})

	if err != nil {
		return 0, err
	}

	return lastModified, nil
}
