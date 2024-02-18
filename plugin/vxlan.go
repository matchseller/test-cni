package plugin

import (
	"encoding/json"
	"fmt"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	types "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	"net"
	"test-cni/ipam"
	"test-cni/nettools"
	"test-cni/skel"
	"test-cni/utils"
	"time"
)

type PConf struct {
	cniTypes.NetConf
	RuntimeConfig *struct {
		TestConfig map[string]interface{} `json:"testConfig"`
	} `json:"runtimeConfig"`

	Subnet string `json:"subnet"`
}

func GetConfigs(args *skel.CmdArgs) *PConf {
	pluginConfig := &PConf{}
	if err := json.Unmarshal(args.StdinData, pluginConfig); err != nil {
		return nil
	}
	return pluginConfig
}

func Bootstrap(args *skel.CmdArgs, pluginConfig *PConf, containerId string) (*types.Result, error) {
	defer utils.ReleaseLock()
	var podIP *net.IPNet
	for {
		ok, err := utils.AcquireLock()
		if err != nil {
			return nil, fmt.Errorf("AcquireLock error:%s", err.Error())
		}
		if !ok {
			time.Sleep(1 * time.Second)
			continue
		}
		podIP = ipam.GetUnusedIp(pluginConfig.Subnet)
		if podIP == nil {
			return nil, fmt.Errorf("can not allocation ip address from subnet:%s", pluginConfig.Subnet)
		}
		break
	}

	netNs, err := nettools.GetNetNs(args.Netns)
	if err != nil {
		return nil, err
	}

	gw := ipam.GetGateway(pluginConfig.Subnet)
	if gw == nil {
		return nil, fmt.Errorf("can not get gw from subnet:%s", pluginConfig.Subnet)
	}

	br, err := nettools.GetBridge()
	if err != nil {
		return nil, fmt.Errorf("get bridge error:%s", err.Error())
	}

	err = (*netNs).Do(func(hostNs ns.NetNS) error {
		//创建一对veth设备
		containerVeth, hostVeth, err := nettools.CreateVethPair(args.IfName, 1450)
		if err != nil {
			return fmt.Errorf("create veth error:%s", err.Error())
		}

		//把随机起名的veth那头放在宿主机的namespace
		err = nettools.SetVethNsFd(hostVeth, hostNs)
		if err != nil {
			return fmt.Errorf("set veth to hostNs error:%s", err.Error())
		}

		//把要被放到pod中的那头veth塞上podIP
		err = nettools.SetIpForVeth(containerVeth.Name, podIP.String())
		if err != nil {
			return fmt.Errorf("set ip to veth error:%s", err.Error())
		}

		err = nettools.SetUpVeth(containerVeth)
		if err != nil {
			return fmt.Errorf("set up containerVeth error:%s", err.Error())
		}

		//创建默认路由
		err = nettools.SetDefaultRouteToVeth(gw.IP, containerVeth)
		if err != nil {
			return fmt.Errorf("SetDefaultRouteToVeth error:%s", err.Error())
		}

		_ = hostNs.Do(func(_ ns.NetNS) error {
			//重新获取一次host上的veth，因为hostVeth发生了改变
			_hostVeth, err := netlink.LinkByName(hostVeth.Attrs().Name)
			if err != nil {
				return fmt.Errorf("get hostVeth error:%s", err.Error())
			}
			var ok bool
			if hostVeth, ok = _hostVeth.(*netlink.Veth); !ok {
				return fmt.Errorf("%s not a veth device", hostVeth.Attrs().Name)
			}

			err = nettools.SetUpVeth(hostVeth)
			if err != nil {
				return fmt.Errorf("set up hostVeth error:%s", err.Error())
			}

			//塞到网桥上
			err = nettools.SetVethMaster(hostVeth, br)
			if err != nil {
				return fmt.Errorf("add hostVeth to bridge error:%s", err.Error())
			}
			return nil
		})
		return nil
	})
	//ip地址占位
	err = utils.CreateFile(fmt.Sprintf("%s/%s", ipam.IpStoragePath, podIP.IP.String()), nil, 0766)
	if err != nil {
		utils.WriteLog("create ip file error:", err.Error())
	}
	err = utils.CreateFile(fmt.Sprintf("%s/%s", ipam.ContainerIdStoragePath, containerId), []byte(podIP.IP.String()), 0766)
	if err != nil {
		utils.WriteLog("create containerId file error:", err.Error())
	}
	result := &types.Result{
		CNIVersion: pluginConfig.CNIVersion,
		IPs: []*types.IPConfig{
			{
				Address: *podIP,
				Gateway: gw.IP,
			},
		},
	}
	return result, nil
}
