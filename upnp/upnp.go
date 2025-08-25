package upnp

import (
	"fmt"
	"time"

	"github.com/huin/goupnp"
	"github.com/huin/goupnp/dcps/internetgateway1"
	"github.com/uright008/go-openbmclapi-reborn/logger"
)

// SetupUPnP 设置UPnP端口映射
func SetupUPnP(port, publicPort int, logger *logger.Logger) (string, error) {
	logger.Info("正在设置UPnP端口映射...")

	// 发现IGD设备
	clients, externalIP, err := discoverIGD()
	if err != nil {
		return "", fmt.Errorf("无法发现IGD设备: %w", err)
	}

	if len(clients) == 0 {
		return "", fmt.Errorf("未找到IGD设备")
	}

	client := clients[0]

	// 获取外部IP地址
	if externalIP == "" {
		externalIP, err = client.GetExternalIPAddress()
		if err != nil {
			return "", fmt.Errorf("无法获取外部IP地址: %w", err)
		}
	}

	logger.Info("外部IP地址: %s", externalIP)

	// 执行端口映射
	err = doPortMap(client, port, publicPort, logger)
	if err != nil {
		return "", fmt.Errorf("端口映射失败: %w", err)
	}

	// 设置定期续期
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				err := doPortMap(client, port, publicPort, logger)
				if err != nil {
					logger.Error("UPnP端口映射续期失败: %v", err)
				} else {
					logger.Debug("UPnP端口映射续期成功")
				}
			}
		}
	}()

	logger.Info("UPnP端口映射设置成功")
	return externalIP, nil
}

// discoverIGD 发现IGD设备
func discoverIGD() ([]*internetgateway1.WANIPConnection1, string, error) {
	var clients []*internetgateway1.WANIPConnection1
	var externalIP string

	// 尝试WANIPConnection
	devs, err := goupnp.DiscoverDevices(internetgateway1.URN_WANIPConnection_1)
	if err != nil {
		return nil, "", err
	}

	for _, dev := range devs {
		if dev.Root == nil {
			continue
		}

		client, err := internetgateway1.NewWANIPConnection1ClientsByURL(dev.Location)
		if err != nil {
			continue
		}

		for _, c := range client {
			clients = append(clients, c)
			if externalIP == "" {
				ip, err := c.GetExternalIPAddress()
				if err == nil {
					externalIP = ip
				}
			}
		}
	}

	// 如果没有找到WANIPConnection，尝试WANPPPConnection
	if len(clients) == 0 {
		devs, err := goupnp.DiscoverDevices(internetgateway1.URN_WANPPPConnection_1)
		if err != nil {
			return nil, "", err
		}

		for _, dev := range devs {
			if dev.Root == nil {
				continue
			}

			client, err := internetgateway1.NewWANPPPConnection1ClientsByURL(dev.Location)
			if err != nil {
				continue
			}

			for _, c := range client {
				clients = append(clients, &internetgateway1.WANIPConnection1{ServiceClient: c.ServiceClient})
				if externalIP == "" {
					ip, err := c.GetExternalIPAddress()
					if err == nil {
						externalIP = ip
					}
				}
			}
		}
	}

	return clients, externalIP, nil
}

// doPortMap 执行端口映射
func doPortMap(client *internetgateway1.WANIPConnection1, port, publicPort int, logger *logger.Logger) error {
	logger.Debug("映射端口 %d 到 %d", port, publicPort)

	// 删除已有的端口映射
	_ = client.DeletePortMapping("", uint16(publicPort), "TCP")
	_ = client.DeletePortMapping("", uint16(publicPort), "UDP")

	// 创建新的TCP端口映射
	err := client.AddPortMapping("", uint16(publicPort), "TCP", uint16(port), "0.0.0.0", true, "openbmclapi", 3600)
	if err != nil {
		return fmt.Errorf("TCP端口映射失败: %w", err)
	}

	// 创建新的UDP端口映射
	err = client.AddPortMapping("", uint16(publicPort), "UDP", uint16(port), "0.0.0.0", true, "openbmclapi", 3600)
	if err != nil {
		return fmt.Errorf("UDP端口映射失败: %w", err)
	}

	return nil
}
