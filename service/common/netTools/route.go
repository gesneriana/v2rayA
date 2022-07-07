package netTools

import (
	"bytes"
	"context"
	"fmt"
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

func GetIpConfig() string {
	command := exec.Command("cmd.exe", "/c", "chcp 65001 & ipconfig")
	var buffer bytes.Buffer
	command.Stdout = &buffer //设置输入
	if err := command.Start(); err != nil {
		log.Error("GetGatewayIp Start err:%s", errors.WithStack(err).Error())
		return ""
	}
	if err := command.Wait(); err != nil {
		log.Error("GetGatewayIp Wait err:%s", errors.WithStack(err).Error())
		return ""
	}

	var result = buffer.String()
	return result
}

func GetGatewayIp() (gatewayIp string, ip string) {

	var result = GetIpConfig()
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

func ExecCmdWithArgsAsync(cmd string, args ...string) *os.Process {
	if len(cmd) == 0 {
		return nil
	}
	command := exec.Command(cmd, args...)

	command.Stdout = os.Stdout //设置输入
	if err := command.Start(); err != nil {
		log.Error("ExecCmdWithArgsAsync Start err:%s", errors.WithStack(err).Error())
		return nil
	}

	return command.Process
}

func ExecCmdWithArgs(cmd string, args ...string) {
	if len(cmd) == 0 {
		return
	}
	command := exec.Command(cmd, args...)

	command.Stdout = os.Stdout //设置输入
	if err := command.Run(); err != nil {
		log.Error("ExecCmdWithArgs Run err:%s", errors.WithStack(err).Error())
		return
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
			ExecCmd(cmdString)
			commandSet.Clear()
		}
	}

	cmdString := strings.Join(commandSet.ToSlice(), " & ")
	ExecCmd(cmdString)
	commandSet.Clear()
}

var tunProcess *os.Process

func InitRoute() {
	gw, _ := GetGatewayIp()
	if len(gw) == 0 {
		log.Error("GetGatewayIp err")
		return
	}

	var tun2socksString = ExecCmd("tasklist | findstr tun2socks.exe")
	if strings.Contains(tun2socksString, "tun2socks.exe") {
		log.Info("exec cmd: taskkill /f /im tun2socks.exe")
		ExecCmdWithArgs("taskkill", "/f", "/im", "tun2socks.exe")
	}

	var listeningString = ExecCmd(fmt.Sprintf("netstat -ano | findstr %d | findstr LISTENING", configure.GetPortsNotNil().Socks5))
	if len(listeningString) > 0 {
		listeningSlice := strings.Split(listeningString, "LISTENING")
		if len(listeningSlice) == 2 {
			pidString := strings.TrimSpace(strings.Trim(listeningSlice[1], "\r\n"))
			log.Info("exec cmd: taskkill /f /pid %s", pidString)
			ExecCmdWithArgs("taskkill", "/f", "/pid", pidString)
		}
	}
	serverIpSet := GetServerDirectIP()
	AddRoute(serverIpSet, gw)

	waitChan := make(chan int)
	go func() {
		var socks5 = fmt.Sprintf("socks5://127.0.0.1:%d", configure.GetPortsNotNil().Socks5)
		for {
			client := GetHttpClient(socks5)
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
				close(waitChan)
				break
			}

			time.Sleep(time.Second * 3)
		}

		log.Info("exec cmd: ./tun2socks.exe -device tun://v2raya -proxy %s", socks5)
		tunProcess = ExecCmdWithArgsAsync("./tun2socks.exe", "-device", "tun://v2raya", "-proxy", socks5)
	}()
	go func() {
		<-waitChan
		for {
			time.Sleep(time.Second)
			var result = GetIpConfig()
			if strings.Contains(result, "v2raya") {
				break
			}
		}

		// netsh interface ip set address v2raya static 10.0.68.10 255.255.255.0 10.0.68.1 3
		log.Info("exec cmd: netsh interface ip set address v2raya static 10.0.68.10 255.255.255.0 10.0.68.1 3")
		ExecCmdWithArgs("netsh", strings.Split("interface ip set address v2raya static 10.0.68.10 255.255.255.0 10.0.68.1 3", " ")...)
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
	log.Info("exec cmd: taskkill /f /im tun2socks.exe")
	ExecCmdWithArgs("taskkill", "/f", "/im", "tun2socks.exe")

	var listeningString = ExecCmd(fmt.Sprintf("netstat -ano | findstr %d | findstr LISTENING", configure.GetPortsNotNil().Socks5))
	if len(listeningString) > 0 {
		listeningSlice := strings.Split(listeningString, "LISTENING")
		if len(listeningSlice) == 2 {
			pidString := strings.TrimSpace(strings.Trim(listeningSlice[1], "\r\n"))
			log.Info("exec cmd: taskkill /f /pid %s", pidString)
			ExecCmdWithArgs("taskkill", "/f", "/pid", pidString)
		}
	}

	log.Info("CloseTun is success")
}
