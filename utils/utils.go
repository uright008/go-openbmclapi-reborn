package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// SignRequest 使用HMAC-SHA256签名请求
func SignRequest(secret, message string) string {
	key := []byte(secret)
	h := hmac.New(sha256.New, key)
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifySignature 验证HMAC-SHA256签名
func VerifySignature(secret, message, signature string) bool {
	expected := SignRequest(secret, message)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// GetPublicIP 获取公网IP地址
func GetPublicIP() (string, error) {
	// 这里可以实现获取公网IP的逻辑
	// 暂时返回一个默认值
	return "", fmt.Errorf("未实现获取公网IP功能")
}

// IsPrivateIP 检查IP是否为私有IP
func IsPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// 私有IP地址范围
	_, private24BitBlock, _ := net.ParseCIDR("10.0.0.0/8")
	_, private20BitBlock, _ := net.ParseCIDR("172.16.0.0/12")
	_, private16BitBlock, _ := net.ParseCIDR("192.168.0.0/16")

	return private24BitBlock.Contains(ip) ||
		private20BitBlock.Contains(ip) ||
		private16BitBlock.Contains(ip)
}

// ExtractHashFromPath 从路径中提取哈希值
func ExtractHashFromPath(path string) string {
	// 移除前缀 /download/
	path = strings.TrimPrefix(path, "/download/")

	// 获取第一个路径段作为哈希值
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[0]
	}

	return ""
}

// ParseQuery 解析查询参数
func ParseQuery(query string) url.Values {
	values, err := url.ParseQuery(query)
	if err != nil {
		return make(url.Values)
	}
	return values
}

// FormatBytes 格式化字节数
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
