package main

import (
	"context"
	"fmt"
	"github.com/vishvananda/netlink"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"strings"
	"test-cni/ipam"
	"test-cni/nettools"
	"test-cni/utils"
	"time"
)

func main() {
	defer func() {
		select {}
	}()
	err := utils.CopyFile("/root/test-cni", "/opt/cni/bin/")
	if err != nil {
		fmt.Println("copy file error:", err.Error())
		return
	}
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	nodes, err := clientSet.CoreV1().Nodes().List(context.TODO(), v1.ListOptions{})
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	ipToInterfaceName, ips, err := nettools.GetHostInterfacesIps()
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	var currentNode *corev1.Node
	var currentInternalIp string
	for _, n := range nodes.Items {
		var isCurrentNode bool
		for _, a := range n.Status.Addresses {
			if a.Type == "InternalIP" && utils.StringsIn(ips, a.Address) {
				currentInternalIp = a.Address
				isCurrentNode = true
				break
			}
		}
		if isCurrentNode {
			currentNode = &n
			break
		}
	}
	if currentNode == nil {
		fmt.Println("currentNode is nil")
		return
	}
	if currentNode.Spec.PodCIDR == "" {
		fmt.Println("pod cidr is empty!")
		return
	}
	var currentInternalIpInterface string
	var ok bool
	if currentInternalIpInterface, ok = ipToInterfaceName[currentInternalIp]; !ok {
		fmt.Println("can not found the internalIp interface")
		return
	}
	//创建bridge设备
	currentGw := ipam.GetGateway(currentNode.Spec.PodCIDR)
	if currentGw == nil {
		fmt.Println("currentGw can not be nil")
		return
	}
	_, err = nettools.CreateBridge("testcni0", currentGw, 1450)
	if err != nil {
		fmt.Println("CreateBridge error:", err.Error())
		return
	}

	//创建vxlan设备
	vxlanIp := ipam.GetVxlanIp(currentNode.Spec.PodCIDR)
	if vxlanIp == nil {
		fmt.Println("vxlanIp can not be empty")
		return
	}
	vxlan, err := nettools.CreateVxlanAndUp("testcni.1", 1500, vxlanIp)
	if err != nil {
		fmt.Println("CreateVxlanAndUp error:", err.Error())
		return
	}

	//更新currentNode
	currentNode.Annotations["vxlan_ip_to_vxlan_mac"] = fmt.Sprintf("%s|%s", vxlanIp.IP.String(), vxlan.HardwareAddr)
	currentNode.Annotations["vxlan_mac_to_host_ip"] = fmt.Sprintf("%s|%s", vxlan.HardwareAddr, currentInternalIp)
	_, err = clientSet.CoreV1().Nodes().Update(context.TODO(), currentNode, v1.UpdateOptions{})
	if err != nil {
		fmt.Println("update node info error:", err.Error())
		return
	}

	//将网络插件配置写入相应文件
	cniConfig := fmt.Sprintf(`{
        "cniVersion": "0.4.0",
        "name": "test-cni",
        "type": "test-cni",
        "subnet": "%s"
}`, currentNode.Spec.PodCIDR)
	err = utils.CreateFile("/etc/cni/net.d/10-testcni.conf", []byte(cniConfig), 0766)
	if err != nil {
		fmt.Println("CreateCniConfig error:", err.Error())
		return
	}

	//添加snat
	err = nettools.AddSNat(currentNode.Spec.PodCIDR, currentInternalIp, currentInternalIpInterface)
	if err != nil {
		fmt.Println("add snat error:", err.Error())
		return
	}
	time.Sleep(5 * time.Second)

	var otherNodes []corev1.Node
	nodes, err = clientSet.CoreV1().Nodes().List(context.TODO(), v1.ListOptions{})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, n := range nodes.Items {
		if n.Name != currentNode.Name {
			otherNodes = append(otherNodes, n)
		}
	}

	//写入fdb、arp、路由表
	for _, n := range otherNodes {
		ipToMac := n.Annotations["vxlan_ip_to_vxlan_mac"]
		ipToMacArr := strings.Split(ipToMac, "|")
		if len(ipToMacArr) != 2 {
			fmt.Println(fmt.Sprintf("vxlan_ip_to_vxlan_mac:%s incorrect", ipToMac))
			return
		}

		macToIp := n.Annotations["vxlan_mac_to_host_ip"]
		macToIpArr := strings.Split(macToIp, "|")
		if len(macToIpArr) != 2 {
			fmt.Println(fmt.Sprintf("vxlan_mac_to_host_ip:%s incorrect", macToIp))
			return
		}

		err = nettools.CreateFdbEntry(macToIpArr[0], macToIpArr[1], vxlan.Name)
		if err != nil {
			fmt.Println("CreateFdbEntry error:", err.Error())
			return
		}

		err = nettools.CreateArpEntry(ipToMacArr[0], ipToMacArr[1], vxlan.Name)
		if err != nil {
			fmt.Println("CreateArpEntry error:", err.Error())
			return
		}

		otherGw := ipam.GetVxlanIp(n.Spec.PodCIDR)
		if otherGw == nil {
			fmt.Println("otherGw can not be nil")
			return
		}
		ipNet := ipam.CidrToIpNet(n.Spec.PodCIDR)
		if ipNet == nil {
			fmt.Println("ipNet can not be nil")
			return
		}
		err = nettools.AddRoute(ipNet, otherGw.IP, vxlan, int(netlink.FLAG_ONLINK))
		if err != nil {
			fmt.Println(fmt.Sprintf("AddRoute second %s,%s error:%s", ipNet, otherGw.IP, err.Error()))
			return
		}
	}
	fmt.Println("plugin init ok!")
}
