package storage

import (
	"fmt"
	"io"

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

	// ListFiles 列出所有已存在的文件
	ListFiles() ([]*FileInfo, error)

	// GC 垃圾回收
	GC(files []*FileInfo) error

	// GetLastModified 获取存储中所有文件的最新修改时间（Unix时间戳）
	GetLastModified() (int64, error)
}

func NewStorage(cfg *config.Config) (Storage, error) {
	switch cfg.Storage.Type {
	case "file":
		return NewFileStorage(cfg.Storage.Path), nil
	case "webdav":
		return NewWebDAVStorage(cfg.Storage.WebDAV), nil
	default:
		return nil, fmt.Errorf("不支持的存储类型: %s", cfg.Storage.Type)
	}
}
