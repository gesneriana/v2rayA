package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/v2rayA/v2rayA/common/netTools"

	"github.com/gin-gonic/gin"
	_ "github.com/v2rayA/v2rayA/conf/report"
	_ "github.com/v2rayA/v2rayA/pkg/plugin/pingtunnel"
	_ "github.com/v2rayA/v2rayA/pkg/plugin/simpleobfs"
	_ "github.com/v2rayA/v2rayA/pkg/plugin/socks5"
	_ "github.com/v2rayA/v2rayA/pkg/plugin/ss"
	_ "github.com/v2rayA/v2rayA/pkg/plugin/ssr"
	_ "github.com/v2rayA/v2rayA/pkg/plugin/tcp"
	_ "github.com/v2rayA/v2rayA/pkg/plugin/tls"
	_ "github.com/v2rayA/v2rayA/pkg/plugin/trojanc"
	_ "github.com/v2rayA/v2rayA/pkg/plugin/ws"
	"github.com/v2rayA/v2rayA/pkg/util/log"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	checkEnvironment()
	if runtime.GOOS == "linux" {
		checkTProxySupportability()
	} else if runtime.GOOS == "windows" {
		netTools.CheckAndStartWinTunnel()
	}
	initConfigure()
	checkUpdate()
	hello()

	go func() {
		if err := run(); err != nil {
			log.Fatal("main: %v", err)
		}
	}()

	if runtime.GOOS == "windows" {
		// 监控两个信号
		// TERM信号（kill + 进程号 触发）
		// 中断信号（ctrl + c 触发）
		osc := make(chan os.Signal, 1)
		signal.Notify(osc, syscall.SIGTERM, syscall.SIGINT)
		s := <-osc
		fmt.Println("监听到退出信号,s=", s)

		// 退出前的清理操作
		netTools.CloseTun()
		log.Info("v2raya server is stop")
	}
}
