package nettools

import (
	"crypto/rand"
	"fmt"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	"net"
	"os"
	"os/exec"
)

func GetBridge() (*netlink.Bridge, error) {
	brName := "testcni0"
	l, err := netlink.LinkByName(brName)
	if err != nil {
		return nil, err
	}

	br, ok := l.(*netlink.Bridge)
	if !ok {
		return nil, fmt.Errorf("found the device %s but it's not a bridge device", brName)
	}
	return br, nil
}

func SetVethMaster(veth *netlink.Veth, br *netlink.Bridge) error {
	err := netlink.LinkSetMaster(veth, br)
	if err != nil {
		return fmt.Errorf("add veth %s to master error: %s", veth.Attrs().Name, err.Error())
	}
	return nil
}

func CreateVethPair(ifName string, mtu int) (*netlink.Veth, *netlink.Veth, error) {
	var vethPairName string
	var err error
	for {
		vethPairName, err = RandomVethName()
		if err != nil {
			return nil, nil, fmt.Errorf("generate veth pair error:%s", err.Error())
		}

		_, err = netlink.LinkByName(vethPairName)
		if err != nil && !os.IsExist(err) {
			break
		}
	}

	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: ifName,
			MTU:  mtu,
		},
		PeerName: vethPairName,
	}

	//创建veth pair
	err = netlink.LinkAdd(veth)

	if err != nil {
		return nil, nil, fmt.Errorf("create veth pair error:%s", err.Error())
	}

	//尝试重新获取veth设备看是否能成功
	veth1, err := netlink.LinkByName(ifName)
	if err != nil {
		//如果获取失败就尝试删掉
		_ = netlink.LinkDel(veth1)
		return nil, nil, fmt.Errorf("reget veth by name:%s error:%s", ifName, err.Error())
	}

	//尝试重新获取veth设备看是否能成功
	veth2, err := netlink.LinkByName(vethPairName)
	if err != nil {
		//如果获取失败就尝试删掉
		_ = netlink.LinkDel(veth2)
		return nil, nil, fmt.Errorf("reget peer veth by name:%s error:%s", vethPairName, err.Error())
	}

	return veth1.(*netlink.Veth), veth2.(*netlink.Veth), nil
}

func RandomVethName() (string, error) {
	entropy := make([]byte, 4)
	_, err := rand.Read(entropy)
	if err != nil {
		return "", fmt.Errorf("failed to generate random veth name: %s", err.Error())
	}

	return fmt.Sprintf("veth%x", entropy), nil
}

func SetVethNsFd(veth *netlink.Veth, ns ns.NetNS) error {
	err := netlink.LinkSetNsFd(veth, int(ns.Fd()))
	if err != nil {
		return fmt.Errorf("failed to add the device %s to ns: %s", veth.Attrs().Name, err.Error())
	}
	return nil
}

func SetIpForVeth(name string, podIP string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return fmt.Errorf("failed to get device by name %s, error: %s", name, err.Error())
	}

	ipaddr, ipnet, err := net.ParseCIDR(podIP)
	if err != nil {
		return fmt.Errorf("failed to transform the ip %s, error : %s", podIP, err.Error())
	}
	ipnet.IP = ipaddr
	err = netlink.AddrAdd(link, &netlink.Addr{IPNet: ipnet})
	if err != nil {
		return fmt.Errorf("can not add the ip %s to device %s, error: %s", podIP, name, err.Error())
	}
	return nil
}

func SetUpVeth(veth ...*netlink.Veth) error {
	for _, v := range veth {
		err := netlink.LinkSetUp(v)
		if err != nil {
			return fmt.Errorf("set up veth:%s error:%s", v.Name, err.Error())
		}
	}
	return nil
}

func SetDefaultRouteToVeth(gwIP net.IP, veth netlink.Link) error {
	_, defNet, _ := net.ParseCIDR("0.0.0.0/0")
	return AddRoute(defNet, gwIP, veth, 0)
}

func AddRoute(ipn *net.IPNet, gw net.IP, dev netlink.Link, flag int, scope ...netlink.Scope) error {
	defaultScope := netlink.SCOPE_UNIVERSE
	if len(scope) > 0 {
		defaultScope = scope[0]
	}
	return netlink.RouteAdd(&netlink.Route{
		LinkIndex: dev.Attrs().Index,
		Scope:     defaultScope,
		Dst:       ipn,
		Gw:        gw,
		Flags:     flag,
	})
}

func CreateArpEntry(ip, mac, dev string) error {
	processInfo := exec.Command(
		"/bin/bash", "-c",
		fmt.Sprintf("arp -s %s %s -i %s", ip, mac, dev),
	)
	_, err := processInfo.Output()
	return err
}

func CreateFdbEntry(mac, ip, dev string) error {
	processInfo := exec.Command(
		"/bin/bash", "-c",
		fmt.Sprintf("bridge fdb append %s dev %s dst %s", mac, dev, ip),
	)
	_, err := processInfo.Output()
	return err
}

func AddSNat(podCidr, hostIp, dev string) error {
	processInfo := exec.Command(
		"/bin/bash", "-c",
		fmt.Sprintf("iptables -t nat -A POSTROUTING -s %s -o %s -j SNAT --to %s", podCidr, dev, hostIp),
	)
	_, err := processInfo.Output()
	return err
}

func GetNetNs(namespace string) (*ns.NetNS, error) {
	netNs, err := ns.GetNS(namespace)
	if err != nil {
		return nil, fmt.Errorf("get ns:%s error:%s", namespace, err.Error())
	}
	return &netNs, nil
}

func CreateVxlanAndUp(name string, mtu int, addr *net.IPNet) (*netlink.Vxlan, error) {
	l, _ := netlink.LinkByName(name)

	vxlan, ok := l.(*netlink.Vxlan)
	if ok && vxlan != nil {
		return vxlan, nil
	}
	vxlan = &netlink.Vxlan{
		VxlanId: 1,
		LinkAttrs: netlink.LinkAttrs{
			Name: name,
			MTU:  mtu,
		},
		Port: 8472,
	}
	err := netlink.LinkAdd(vxlan)
	if err != nil {
		return nil, fmt.Errorf("create vxlan:%s error:%s", name, err.Error())
	}

	l, err = netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("get vxlan by name:%s error:%s", name, err.Error())
	}

	vxlan, ok = l.(*netlink.Vxlan)
	if !ok {
		return nil, fmt.Errorf("found the device %s but it's not a vxlan", name)
	}
	addr.Mask = net.IPv4Mask(255, 255, 255, 255)
	if err = netlink.AddrAdd(vxlan, &netlink.Addr{IPNet: addr}); err != nil {
		return nil, fmt.Errorf("can not add the ip %v to vxlan %s, err: %s", addr, name, err.Error())
	}
	if err = netlink.LinkSetUp(vxlan); err != nil {
		return nil, fmt.Errorf("setup vxlan %s error, err: %v", name, err)
	}
	return vxlan, nil
}

func CreateBridge(brName string, gw *net.IPNet, mtu int) (*netlink.Bridge, error) {
	l, err := netlink.LinkByName(brName)
	if err != nil && err.Error() != "Link not found" {
		return nil, fmt.Errorf("found bridge first by name:%s error:%s", brName, err.Error())
	}

	br, ok := l.(*netlink.Bridge)
	if ok && br != nil {
		return br, nil
	}

	br = &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name:   brName,
			MTU:    mtu,
			TxQLen: -1,
		},
	}

	err = netlink.LinkAdd(br)
	if err != nil {
		return nil, fmt.Errorf("can not create bridge:%s, err:%s", brName, err.Error())
	}

	//这里需要通过netlink重新获取网桥，否则光创建的话无法从上头拿到其他属性
	l, err = netlink.LinkByName(brName)
	if err != nil && err.Error() != "Link not found" {
		return nil, fmt.Errorf("found bridge second by name:%s error:%s", brName, err.Error())
	}

	br, ok = l.(*netlink.Bridge)
	if !ok {
		return nil, fmt.Errorf("found the device %s but it's not a bridge device", brName)
	}

	addr := &netlink.Addr{IPNet: gw}
	if err = netlink.AddrAdd(br, addr); err != nil {
		return nil, fmt.Errorf("can not add the gw %v to bridge %s, err: %s", addr, brName, err.Error())
	}

	if err = netlink.LinkSetUp(br); err != nil {
		return nil, fmt.Errorf("set up bridge %s error, err: %s", brName, err.Error())
	}
	return br, nil
}

func GetHostInterfacesIps() (map[string]string, []string, error) {
	ipToInterfaceName := make(map[string]string)
	var ips []string
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, err
	}
	for _, i := range interfaces {
		addrs, err := i.Addrs()
		if err != nil {
			return nil, nil, err
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				ipToInterfaceName[ipnet.IP.String()] = i.Name
				ips = append(ips, ipnet.IP.String())
			}
		}
	}
	return ipToInterfaceName, ips, nil
}
