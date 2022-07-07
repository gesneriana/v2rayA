package netTools

import (
	"fmt"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/v2rayA/v2rayA/common/cmds"
)

func GetGatewayIp() (gatewayIp string, ip string) {
	var result = cmds.ExecCmd("chcp 65001 & ipconfig")
	lines := strings.Split(result, "\n")

	for i, line := range lines {
		if strings.Contains(line, "默认网关") || strings.Contains(line, "Default Gateway") {
			gw1, gw2 := line, ""
			if len(line) > i+1 {
				gw2 = lines[i+1]
			}
			if strings.Count(gw1, ".") == 3 {
				gatewayLineSlice := strings.Split(gw1, ":")
				gatewayString := gatewayLineSlice[len(gatewayLineSlice)-1]
				gatewayIp = strings.TrimSpace(strings.Trim(gatewayString, "\r"))
			} else if strings.Count(gw2, ".") == 3 {
				gatewayLineSlice := strings.Split(gw2, ":")
				gatewayString := gatewayLineSlice[len(gatewayLineSlice)-1]
				gatewayIp = strings.TrimSpace(strings.Trim(gatewayString, "\r"))
			}
		}
		if len(ip) == 0 && strings.Contains(line, "IPv4 地址") || strings.Contains(line, "IPv4 Address") {
			ipLineSlice := strings.Split(line, ":")
			ipString := ipLineSlice[len(ipLineSlice)-1]
			if strings.Count(ipString, ".") == 3 {
				ip = strings.TrimSpace(strings.Trim(ipString, "\r"))
			}
		}
	}
	return
}

// AddRoute  添加路由到路由表, 添加路由表需要管理员权限
func AddRoute(ipSet mapset.Set[string], gateway string) {

	var count = 0
	var ipSlice = ipSet.ToSlice()
	commandSet := mapset.NewSet[string]()
	for _, ipString := range ipSlice {
		count++
		var commandString = fmt.Sprintf("route add %s %s metric 5", ipString, gateway)
		commandSet.Add(commandString)
		if count%100 == 0 {
			cmdString := strings.Join(commandSet.ToSlice(), " & ")
			cmds.ExecCmd(cmdString)
			commandSet.Clear()
		}
	}

	cmdString := strings.Join(commandSet.ToSlice(), " & ")
	cmds.ExecCmd(cmdString)
	commandSet.Clear()
}
