package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FileStorage 文件存储实现
type FileStorage struct {
	path string
}

// NewFileStorage 创建新的文件存储实例
func NewFileStorage(path string) *FileStorage {
	return &FileStorage{
		path: path,
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
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("文件不存在: %s", hash)
		}
		return nil, fmt.Errorf("无法打开文件 %s: %w", path, err)
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
func (fs *FileStorage) WriteFile(filePath string, content []byte, fileInfo *FileInfo) error {
	fullPath := filepath.Join(fs.path, filePath)

	// 确保目录存在
	dir := filepath.Dir(fullPath)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("无法创建目录: %w", err)
	}

	// 写入文件
	err = os.WriteFile(fullPath, content, 0644)
	if err != nil {
		return fmt.Errorf("无法写入文件 %s: %w", filePath, err)
	}

	return nil
}

// ListFiles 列出所有已存在的文件
func (fs *FileStorage) ListFiles() ([]*FileInfo, error) {
	var files []*FileInfo

	// 遍历存储目录，获取所有已存在的文件
	err := filepath.Walk(fs.path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// 忽略无法访问的目录或文件
			return nil
		}

		// 只处理文件，忽略目录
		if info.IsDir() {
			return nil
		}

		// 获取相对于存储路径的相对路径
		relPath, err := filepath.Rel(fs.path, path)
		if err != nil {
			return nil
		}

		// 验证是否符合我们的存储结构（两级目录结构）
		if len(relPath) >= 3 && relPath[2] == filepath.Separator {
			// 提取文件名（hash）
			hash := relPath[0:2] + relPath[3:]
			fileInfo := &FileInfo{
				Hash: hash,
				Size: info.Size(),
				Path: path,
			}
			files = append(files, fileInfo)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("遍历目录失败: %w", err)
	}

	return files, nil
}

// calculateFileChecksum 计算文件的SHA256校验和
func (fs *FileStorage) calculateFileChecksum(path string) string {
	file, err := os.Open(path)
	if err != nil {
		fmt.Printf("警告：无法打开文件 %s: %v\n", path, err)
		return ""
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		fmt.Printf("警告：无法计算文件 %s 的校验和: %v\n", path, err)
		return ""
	}

	return hex.EncodeToString(hash.Sum(nil))
}

// GetMissingFiles 获取缺失的文件列表
func (fs *FileStorage) GetMissingFiles(files []*FileInfo) ([]*FileInfo, error) {
	// 获取所有已存在的文件
	existingFiles, err := fs.ListFiles()
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
func (fs *FileStorage) GC(files []*FileInfo) error {
	// 获取所有已存在的文件
	existingFiles, err := fs.ListFiles()
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
			err := fs.Delete(file.Hash)
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
func (fs *FileStorage) GetLastModified() (int64, error) {
	var lastModified int64

	err := filepath.Walk(fs.path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if !info.IsDir() {
			modTime := info.ModTime().Unix()
			if modTime > lastModified {
				lastModified = modTime
			}
		}

		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("遍历目录失败: %w", err)
	}

	return lastModified, nil
}
