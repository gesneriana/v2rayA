package netTools

import (
	"bytes"
	"fmt"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/pkg/errors"
	"github.com/v2rayA/v2rayA/db/configure"
	"github.com/v2rayA/v2rayA/pkg/util/log"
	"os/exec"
	"strings"
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

func TrimContentSpace(s string, num int) string {
	for i := 0; i < num; i++ {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

func GetGatewayIp() (gatewayIp string, ip string) {

	command := exec.Command("route", "print")
	var buffer bytes.Buffer
	command.Stdout = &buffer //设置输入
	if err := command.Start(); err != nil {
		log.Error("GetGatewayIp Start err:%s", errors.WithStack(err).Error())
		return "", ""
	}
	if err := command.Wait(); err != nil {
		log.Error("GetGatewayIp Wait err:%s", errors.WithStack(err).Error())
		return "", ""
	}
	fmt.Printf("%s", buffer.Bytes())
	var result = buffer.String()
	lines := strings.Split(result, "\n")

	for _, line := range lines {
		if strings.Count(line, "0.0.0.0") == 2 {
			var gwString = TrimContentSpace(strings.TrimSpace(strings.Trim(line, "\r")), 10)
			ipSlice := strings.Split(gwString, " ")
			if len(ipSlice) >= 5 {
				return ipSlice[2], ipSlice[3]
			}
		}
	}
	return "", ""
}

func ExecCmd(s string) {
	if len(s) == 0 {
		return
	}
	command := exec.Command("cmd.exe", "/c", s)
	var buffer bytes.Buffer
	command.Stdout = &buffer //设置输入
	if err := command.Start(); err != nil {
		log.Error("AddRoute Start err:%s", errors.WithStack(err).Error())
		return
	}
	if err := command.Wait(); err != nil {
		log.Error("AddRoute Wait err:%s, buffer:%s", errors.WithStack(err).Error(), buffer.String())
		return
	}
	buffer.Reset()
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

func InitRoute() {
	gw, _ := GetGatewayIp()
	serverIpSet := GetServerDirectIP()
	AddRoute(serverIpSet, gw)
}
