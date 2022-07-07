package netTools

import (
	"bytes"
	"context"
	"fmt"
	"github.com/v2rayA/v2rayA/conf"
	"github.com/v2rayA/v2rayA/core/v2ray/where"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/pkg/errors"
	"github.com/v2rayA/v2rayA/db/configure"
	"github.com/v2rayA/v2rayA/pkg/util/log"
	"golang.org/x/net/proxy"
)

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

func GetGatewayIp() (gatewayIp string, ip string) {
	var result = ExecCmd("chcp 65001 & ipconfig")
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

func ExecCmd(s string) string {
	if len(s) == 0 {
		return ""
	}
	command := exec.Command("cmd.exe", "/c", s)
	log.Info("ExecCmd Run:%s", command.String())

	var buffer bytes.Buffer
	command.Stdout = &buffer //设置输入
	if err := command.Start(); err != nil {
		log.Error("ExecCmd Start err:%s", errors.WithStack(err).Error())
		return ""
	}
	if err := command.Wait(); err != nil {
		log.Error("ExecCmd Wait err:%s, buffer:%s", errors.WithStack(err).Error(), buffer.String())
		return ""
	}
	var result = buffer.String()
	buffer.Reset()

	return result
}

func ExecCmdWithArgsAsync(cmd string, args ...string) int {
	if len(cmd) == 0 {
		return 0
	}
	command := exec.Command(cmd, args...)
	log.Info("ExecCmdWithArgsAsync Run:%s", command.String())

	command.Stdout = os.Stdout //设置输入
	if err := command.Start(); err != nil {
		log.Error("ExecCmdWithArgsAsync Start err:%s", errors.WithStack(err).Error())
		return 0
	}

	return command.Process.Pid
}

func ExecCmdWithArgs(cmd string, args ...string) {
	if len(cmd) == 0 {
		return
	}
	command := exec.Command(cmd, args...)
	log.Info("ExecCmdWithArgs Run:%s", command.String())

	command.Stdout = os.Stdout //设置输入
	if err := command.Run(); err != nil {
		log.Error("ExecCmdWithArgs Run err:%s", errors.WithStack(err).Error())
		return
	}
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
			ExecCmd(cmdString)
			commandSet.Clear()
		}
	}

	cmdString := strings.Join(commandSet.ToSlice(), " & ")
	ExecCmd(cmdString)
	commandSet.Clear()
}

// DNS的ip列表, 加到路由表
var directDnsIp = []string{"223.6.6.6", "119.29.29.29", "8.8.8.8", "1.1.1.1", "208.67.222.222", "208.67.220.220", "8.8.4.4", "162.14.21.56", "162.14.21.178", "175.24.154.66"}

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
		setting.WinTun = false
	}
	_ = configure.SetSetting(setting)
	if !setting.WinTun {
		return // 为了避免因为环境因素不支持, 强行开启tun代理导致上网异常, 这里直接return
	}

	gw, _ := GetGatewayIp()
	if len(gw) == 0 {
		log.Error("GetGatewayIp err")
		return
	}

	CloseTun()
	serverIpSet := GetServerDirectIP()
	for _, dnsIp := range directDnsIp {
		serverIpSet.Add(dnsIp) // 防止流量在本地回环死循环, 导致系统CPU和内存暴增
	}
	AddRoute(serverIpSet, gw)

	var socks5 = fmt.Sprintf("socks5://127.0.0.1:%d", configure.GetPortsNotNil().Socks5)
	waitChan := make(chan int)
	var isOpen = false

	go func() {
		client := GetHttpClient(socks5)
		for i := 0; i < 5; i++ {
			rsp, err := client.Get("https://www.google.com/generate_204")
			if err != nil {
				continue
			}
			data, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				continue
			}
			_ = rsp.Body.Close()
			if rsp.StatusCode == 204 || len(data) > 0 {
				isOpen = true
				close(waitChan)
				return
			}
			time.Sleep(time.Second * 3)
		}
		close(waitChan) // 为了防止协程泄露，一定次数之后关闭，释放另外两个正在等待中的协程
	}()

	go func() {
		<-waitChan
		if !isOpen {
			return
		}
		// v2raya 开启的socks5代理没办法作为tun代理，可能是没有开启udp和流量探测, v2rayn的就可以
		// ./tun2socks.exe -device tun://v2raya -proxy socks5://127.0.0.1:10808
		ExecCmdWithArgsAsync("cmd", "/c", "start", "/min", "./tun2socks.exe", "-device", "tun://v2raya", "-proxy", socks5)
	}()
	go func() {
		<-waitChan
		if !isOpen {
			return
		}
		for {
			time.Sleep(time.Second)
			var result = ExecCmd("chcp 65001 & ipconfig")
			if strings.Contains(result, "v2raya") {
				break
			}
		}

		time.Sleep(time.Second * 5)
		for i := 0; i < 10; i++ {
			var result = ExecCmd("chcp 65001 & ipconfig")
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
				ExecCmdWithArgs("netsh", strings.Split("interface ip set address v2raya static 10.0.68.10 255.255.255.0 10.0.68.1 3", " ")...)
				time.Sleep(time.Second * 3)
			}
		}
	}()
}

var proxyClient = &http.Client{}

// GetHttpClient 获取全局的http代理客户端
func GetHttpClient(socks5 string) *http.Client {
	// 使用socks5代理初始化http客户端
	tgProxyURL, err := url.Parse(socks5)
	if err != nil {
		log.Error("Failed to parse proxy URL:%s\n", errors.WithStack(err).Error())
		return proxyClient
	}
	proxyDialer, err := proxy.FromURL(tgProxyURL, proxy.Direct)
	if err != nil {
		log.Error("Failed to obtain proxy dialer: %s\n", errors.WithStack(err).Error())
		return proxyClient
	}
	var dialContext = func(ctx context.Context, network string, addr string) (net.Conn, error) {
		return proxyDialer.Dial(network, addr)
	}
	tgTransport := &http.Transport{
		DialContext: dialContext,
	}
	proxyClient.Transport = tgTransport // 使用全局的HttpClient不需要释放连接
	return proxyClient
}

func CloseTun() {
	var tun2socksString = ExecCmd("tasklist | findstr tun2socks.exe")
	if strings.Contains(tun2socksString, "tun2socks.exe") {
		ExecCmdWithArgs("taskkill", "/f", "/im", "tun2socks.exe")
	}

	var listeningString = ExecCmd(fmt.Sprintf("netstat -ano | findstr %d | findstr LISTENING", configure.GetPortsNotNil().Socks5))
	if len(listeningString) > 0 {
		// 可能监听了多个ip, 比如 0.0.0.0 和 [::]
		listeningSlice := strings.Split(strings.Split(listeningString, "\r\n")[0], "LISTENING")
		if len(listeningSlice) == 2 {
			pidString := strings.TrimSpace(listeningSlice[1])
			ExecCmdWithArgs("taskkill", "/f", "/pid", pidString)
		}
	}

	log.Info("CloseTun is success")
}
