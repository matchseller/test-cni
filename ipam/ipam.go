package ipam

import (
	"fmt"
	"net"
	"os"
	"test-cni/utils"
)

const ipStorageBasePath = "/root/k8s_cni_ip_storage"
const IpStoragePath = ipStorageBasePath + "/ips"
const ContainerIdStoragePath = ipStorageBasePath + "/container_ids"

func GetUnusedIp(cidr string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil
	}
	firstIP := nextIP(ipNet.IP.Mask(ipNet.Mask))
	lastIP := getLastIP(ipNet)

	for ip := nextIP(firstIP); !ip.Equal(lastIP); ip = nextIP(ip) {
		if utils.FileIsExisted(fmt.Sprintf("%s/%s", IpStoragePath, ip.String())) {
			continue
		}
		return &net.IPNet{
			IP: ip, Mask: ipNet.Mask,
		}
	}
	return nil
}

func GetGateway(cidr string) *net.IPNet {
	ipNet := CidrToIpNet(cidr)
	if ipNet == nil {
		return nil
	}
	firstIP := ipNet.IP.Mask(ipNet.Mask)
	return &net.IPNet{
		IP:   nextIP(firstIP),
		Mask: ipNet.Mask,
	}
}

func CidrToIpNet(cidr string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil
	}
	return ipNet
}

func GetVxlanIp(cidr string) *net.IPNet {
	ipNet := CidrToIpNet(cidr)
	if ipNet == nil {
		return nil
	}
	return &net.IPNet{
		IP:   ipNet.IP.Mask(ipNet.Mask),
		Mask: ipNet.Mask,
	}
}

func ReleaseIp(containerId string) {
	containerIdFile := fmt.Sprintf("%s/%s", ContainerIdStoragePath, containerId)
	b, err := os.ReadFile(containerIdFile)
	if err != nil {
		return
	}
	_ = utils.DeleteFile(fmt.Sprintf("%s/%s", IpStoragePath, string(b)))
	_ = utils.DeleteFile(fmt.Sprintf("%s/%s", ContainerIdStoragePath, containerId))
}

func nextIP(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] > 0 {
			break
		}
	}
	return next
}

func getLastIP(ipNet *net.IPNet) net.IP {
	lastIP := make(net.IP, len(ipNet.IP))
	copy(lastIP, ipNet.IP.To4())
	for i := range lastIP {
		lastIP[i] |= ^ipNet.Mask[i]
	}
	return lastIP
}
