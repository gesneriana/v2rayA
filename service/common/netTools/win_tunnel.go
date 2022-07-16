package netTools

import (
	"fmt"
	"io/ioutil"
	"net"
	"strings"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/v2rayA/v2rayA/common/cmds"
	"github.com/v2rayA/v2rayA/conf"
	"github.com/v2rayA/v2rayA/core/v2ray/where"
	"github.com/v2rayA/v2rayA/db/configure"
	"github.com/v2rayA/v2rayA/pkg/util/log"
)

// DNS的ip列表, 加到路由表
var directDnsIp = []string{"223.6.6.6", "119.29.29.29", "8.8.8.8", "1.1.1.1", "208.67.222.222", "208.67.220.220", "8.8.4.4", "162.14.21.56", "162.14.21.178", "175.24.154.66"}

// GetServerDirectIP 获取机场直连ip,加入路由表
func GetServerDirectIP() mapset.Set[string] {
	ipSet := mapset.NewSet[string]()
	subscriptions := configure.GetSubscriptionsV2()
	for _, subscription := range subscriptions {
		for ip, _ := range subscription.DirectIpSet {
			ipSet.Add(ip)
		}
	}
	return ipSet
}

func CheckAndStartWinTunnel() {
	config := conf.GetEnvironmentConfig()
	if !config.WinTunnel {
		return
	}

	variant, _, _ := where.GetV2rayServiceVersion()
	setting := configure.GetSettingNotNil()
	// 开启tun代理需要使用Xray内核, 开启Udp和流量探测, 我没有测试过其他的内核是否支持tun代理, 目前只是实验性的功能, 小范围测试和自用的
	if variant == "Xray" {
		setting.WinTun = true

	} else {
		log.Error("当前的环境可能不支持wintun代理,请使用Xray内核")
		setting.WinTun = false
	}
	_ = configure.SetSetting(setting)
	if !setting.WinTun {
		return // 为了避免因为环境因素不支持, 强行开启tun代理导致上网异常, 这里直接return
	}

	gw, ipString := GetGatewayIp()
	if len(gw) == 0 {
		log.Error("GetGatewayIp err")
		return
	}

	CloseTun()
	serverIpSet := GetServerDirectIP()
	if serverIpSet.Cardinality() == 0 {
		log.Error("请先更新订阅,把机场的域名解析为ip地址")
		return
	}
	for _, dnsIp := range directDnsIp {
		serverIpSet.Add(dnsIp) // 防止流量在本地回环死循环, 导致系统CPU和内存暴增
	}
	AddRoute(serverIpSet, gw) // 要想使用geoip分流规则, 需要设置 Socks5WithPac socks5分流的入站端口, 改用这个定制的Xray-core https://github.com/gesneriana/Xray-core

	socks5Port, socks5WithPac := configure.GetPortsNotNil().Socks5, configure.GetPortsNotNil().Socks5WithPac
	ip := net.ParseIP(ipString)
	if socks5WithPac > 0 && len(ip) > 0 {
		socks5Port = socks5WithPac // 使用分流端口开启tun代理, 必须指定出站的网卡ip
	}
	var socks5 = fmt.Sprintf("socks5://127.0.0.1:%d", socks5Port)
	waitChan := make(chan int)
	var isOpen = false // 检查Xray core是否启动成功

	go func() {
		client := GetHttpClient(socks5)
		for i := 0; i < 10; i++ {
			time.Sleep(time.Second * 3)
			rsp, err := client.Get("https://www.google.com/generate_204")
			if err != nil {
				log.Error("检查Socks入站端口 http get err:%+v", err)
				continue
			}
			data, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				log.Error("检查Socks入站端口 ReadAll err:%+v", err)
				continue
			}
			_ = rsp.Body.Close()
			if rsp.StatusCode == 204 || len(data) > 0 {
				isOpen = true
				break
			}
		}
		close(waitChan) // 为了防止协程泄露，一定次数之后关闭，释放另外两个正在等待中的协程
	}()

	go func() {
		<-waitChan
		if !isOpen {
			return
		}
		// ./tun2socks.exe -device tun://v2raya -proxy socks5://127.0.0.1:10808
		cmds.ExecCmdWithArgsAsync("./tun2socks.exe", "-device", "tun://v2raya", "-proxy", socks5)
		// 调试的时候可以使用这个启动，可以看见 tun2socks.exe 的控制台窗口
		// cmds.ExecCmdWithArgsAsync("cmd", "/c", "start", "/min", "./tun2socks.exe", "-device", "tun://v2raya", "-proxy", socks5)
	}()
	go func() {
		<-waitChan
		if !isOpen {
			return
		}
		for {
			time.Sleep(time.Second)
			var result = cmds.ExecCmd("chcp 65001 & ipconfig")
			if strings.Contains(result, "v2raya") {
				break
			}
		}

		time.Sleep(time.Second * 5)
		for i := 0; i < 10; i++ {
			var result = cmds.ExecCmd("chcp 65001 & ipconfig")
			if strings.Contains(result, "10.0.68.10") {
				break
			} else if strings.Contains(result, "169.254.") {
				// 请打开 windows系统的 计算机管理-设备管理器-网络适配器 卸载所有的 [WireGuard Tunnel] 虚拟网卡
				// https://docs.microsoft.com/zh-cn/troubleshoot/windows-server/networking/blank-default-gateway-configure-static-ip-address
				log.Error("请打开 windows系统的 计算机管理-设备管理器-网络适配器 卸载所有的 [WireGuard Tunnel] 虚拟网卡")
				log.Error("https://docs.microsoft.com/zh-cn/troubleshoot/windows-server/networking/blank-default-gateway-configure-static-ip-address")
				break
			} else {
				// netsh interface ip set address v2raya static 10.0.68.10 255.255.255.0 10.0.68.1 3
				cmds.ExecCmdWithArgs("netsh", strings.Split("interface ip set address v2raya static 10.0.68.10 255.255.255.0 10.0.68.1 3", " ")...)
				time.Sleep(time.Second * 3)
			}
		}
	}()
}

func CloseTun() {
	var tun2socksString = cmds.ExecCmd("tasklist | findstr tun2socks.exe")
	if strings.Contains(tun2socksString, "tun2socks.exe") {
		cmds.ExecCmdWithArgs("taskkill", "/f", "/im", "tun2socks.exe")
	}

	var listeningString = cmds.ExecCmd(fmt.Sprintf("netstat -ano | findstr %d | findstr LISTENING", configure.GetPortsNotNil().Socks5))
	if len(listeningString) > 0 {
		// 可能监听了多个ip, 比如 0.0.0.0 和 [::]
		listeningSlice := strings.Split(strings.Split(listeningString, "\r\n")[0], "LISTENING")
		if len(listeningSlice) == 2 {
			pidString := strings.TrimSpace(listeningSlice[1])
			cmds.ExecCmdWithArgs("taskkill", "/f", "/pid", pidString)
		}
	}

	log.Info("CloseTun is success")
}
