package netTools

import (
	"fmt"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/v2rayA/v2rayA/common/cmds"
	"github.com/v2rayA/v2rayA/pkg/util/log"
	"net"
	"strings"
)

// GetOutBoundIP 获取默认的出站ip, 多个网卡也能正常获取, 如果这时候恰好网络连接断开了就获取不到了, 如果已经用clash开启了tun代理也获取不到, 获取的ip是198.18.0.1, 这个是错误的
func GetOutBoundIP() (ip string, err error) {
	var addrSlice = []string{"223.6.6.6", "119.29.29.29", "8.8.8.8", "1.1.1.1", "208.67.222.222", "208.67.220.220", "8.8.4.4", "162.14.21.56", "162.14.21.178", "175.24.154.66"}
	for _, addr := range addrSlice {
		conn, err2 := net.Dial("udp", addr+":53")
		err = err2
		if err2 != nil {
			log.Error("GetOutBoundIP err:", err2)
			continue
		}
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		log.Info("GetOutBoundIP local ip:", localAddr.String())
		ip = strings.Split(localAddr.String(), ":")[0]
		if len(ip) > 0 {
			return
		}
	}

	return
}

// GetGatewayIp 获取网关ip和出站网卡的ip
func GetGatewayIp() (gatewayIp string, ip string) {
	var result = cmds.ExecCmd("chcp 65001 & ipconfig")
	lines := strings.Split(result, "\n")
	ipSlice := make([]string, 0)

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
				ipSlice = append(ipSlice, strings.TrimSpace(strings.Trim(ipString, "\r")))
			}
		}
	}

	if len(gatewayIp) > 0 {
		getewaySlice := strings.Split(gatewayIp, ".")
		if len(getewaySlice) != 4 {
			return
		}
		// 暂不考虑cidr ip范围为16的情况
		prefixString := fmt.Sprintf("%s.%s.%s", getewaySlice[0], getewaySlice[1], getewaySlice[2])
		for _, ipString := range ipSlice {
			if strings.HasPrefix(ipString, prefixString) {
				ip = ipString // 使用匹配的 网关和网卡ip, 如果有多个网卡, 请考虑手动指定网关ip和网卡ip
				break
			}
		}
	}

	if len(ip) == 0 {
		ip, _ = GetOutBoundIP()
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
