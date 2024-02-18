package main

import (
	"fmt"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"test-cni/ipam"
	"test-cni/plugin"
	"test-cni/skel"
	"test-cni/utils"
)

func main() {
	if !utils.PathExists(ipam.IpStoragePath) {
		_ = utils.CreateDir(ipam.IpStoragePath)
	}
	if !utils.PathExists(ipam.ContainerIdStoragePath) {
		_ = utils.CreateDir(ipam.ContainerIdStoragePath)
	}
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("testcni"))
}

func cmdAdd(args *skel.CmdArgs) error {
	pluginConfig := plugin.GetConfigs(args)
	if pluginConfig == nil {
		errMsg := fmt.Errorf("add: get plugin config error, config: %s", string(args.StdinData))
		utils.WriteLog(errMsg.Error())
		return errMsg
	}

	res, err := plugin.Bootstrap(args, pluginConfig, args.ContainerID)
	if err != nil {
		utils.WriteLog("Bootstrap error: ", err.Error())
		return err
	}

	_ = cniTypes.PrintResult(res, pluginConfig.CNIVersion)
	return nil
}

func cmdDel(args *skel.CmdArgs) error {
	ipam.ReleaseIp(args.ContainerID)
	return nil
}

func cmdCheck(args *skel.CmdArgs) error {
	return nil
}
