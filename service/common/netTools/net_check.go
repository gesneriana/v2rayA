package netTools

import (
	"context"
	"net"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
	"github.com/v2rayA/v2rayA/pkg/util/log"
	"golang.org/x/net/proxy"
)

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
