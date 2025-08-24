package config

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2" // 用于 TOML 格式支持
)

// ClusterConfig 集群配置
type ClusterConfig struct {
	ID         string `toml:"id"`
	Secret     string `toml:"secret"`
	IP         string `toml:"ip"`
	Port       int    `toml:"port"`
	PublicPort int    `toml:"public_port"`
	BYOC       bool   `toml:"byoc"`
}

// StorageConfig 存储配置
type StorageConfig struct {
	Type string `toml:"type"`
	Path string `toml:"path"`
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	SSLKey  string `toml:"ssl_key"`
	SSLCert string `toml:"ssl_cert"`
}

// FeaturesConfig 功能配置
type FeaturesConfig struct {
	EnableNginx      bool `toml:"enable_nginx"`
	DisableAccessLog bool `toml:"disable_access_log"`
	EnableUPNP       bool `toml:"enable_upnp"`
}

// SystemConfig 系统配置
type SystemConfig struct {
	Timezone string `toml:"timezone"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

// Config 主配置结构
type Config struct {
	Cluster  ClusterConfig  `toml:"cluster"`
	Storage  StorageConfig  `toml:"storage"`
	Security SecurityConfig `toml:"security"`
	Features FeaturesConfig `toml:"features"`
	System   SystemConfig   `toml:"system"`
	Log      LogConfig      `toml:"log"`
}

// Load 从文件加载配置，如果文件不存在则创建默认配置
func Load(filename string) (*Config, error) {
	// 检查配置文件是否存在
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		// 配置文件不存在，创建默认配置文件
		fmt.Printf("配置文件 %s 不存在，正在创建默认配置文件...\n", filename)
		err := createDefaultConfig(filename)
		if err != nil {
			return nil, fmt.Errorf("无法创建默认配置文件: %w", err)
		}
		fmt.Printf("已创建默认配置文件 %s，请修改配置后重新启动程序\n", filename)
		return nil, fmt.Errorf("请修改配置文件后重新启动程序")
	}

	// 读取配置文件
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("无法读取配置文件: %w", err)
	}

	var config Config
	err = toml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("无法解析配置文件: %w", err)
	}

	// 设置默认值
	setDefaults(&config)

	return &config, nil
}

// createDefaultConfig 创建默认配置文件
func createDefaultConfig(filename string) error {
	defaultConfig := &Config{
		Cluster: ClusterConfig{
			ID:         "",
			Secret:     "",
			IP:         "",
			Port:       4000,
			PublicPort: 0,
			BYOC:       false,
		},
		Storage: StorageConfig{
			Type: "file",
			Path: "./cache",
		},
		Security: SecurityConfig{
			SSLKey:  "",
			SSLCert: "",
		},
		Features: FeaturesConfig{
			EnableNginx:      false,
			DisableAccessLog: false,
			EnableUPNP:       false,
		},
		System: SystemConfig{
			Timezone: "Asia/Shanghai",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
	}

	// 将默认配置写入文件
	data, err := toml.Marshal(defaultConfig)
	if err != nil {
		return fmt.Errorf("无法序列化默认配置: %w", err)
	}

	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("无法写入配置文件: %w", err)
	}

	return nil
}

// setDefaults 设置配置默认值
func setDefaults(config *Config) {
	if config.Cluster.Port == 0 {
		config.Cluster.Port = 4000
	}

	if config.Cluster.PublicPort == 0 {
		config.Cluster.PublicPort = config.Cluster.Port
	}

	if config.Storage.Path == "" {
		config.Storage.Path = "./cache"
	}

	if config.System.Timezone == "" {
		config.System.Timezone = "Asia/Shanghai"
	}

	if config.Log.Level == "" {
		config.Log.Level = "info"
	}

	if config.Log.Format == "" {
		config.Log.Format = "text"
	}
}
