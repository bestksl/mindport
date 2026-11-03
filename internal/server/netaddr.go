// 本机可达地址枚举。不关心用户用的是哪种组网（蒲公英、WireGuard、
// ZeroTier 或别的虚拟网络）：默认路由网卡上的地址归为「局域网」，
// 其余活动网卡上的全局单播地址一律归为「组网地址」。
package server

import (
	"net"
	"sort"
)

// ListenAddresses 返回本机可用的 IPv4 访问地址。
// primary：默认路由网卡（通常是 Wi-Fi/以太网）上的地址；
// mesh：其他活动网卡（各类组网虚拟网卡）上的地址。
// 枚举失败时返回空切片，不报错——调用方只用于展示与配对。
func ListenAddresses() (primary, mesh []string) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, nil
	}

	primaryIP := defaultRouteSource()
	primaryName := ""
	if primaryIP != "" {
		for _, ifc := range ifaces {
			if ifaceHasIP(ifc, primaryIP) {
				primaryName = ifc.Name
				break
			}
		}
	}

	seen := map[string]bool{}
	add := func(dst *[]string, ip string) {
		if ip == "" || seen[ip] {
			return
		}
		seen[ip] = true
		*dst = append(*dst, ip)
	}

	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		isPrimary := ifc.Name == primaryName
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil || !ip4.IsGlobalUnicast() {
				continue
			}
			s := ip4.String()
			if isPrimary {
				add(&primary, s)
			} else {
				add(&mesh, s)
			}
		}
	}
	sort.Strings(primary)
	sort.Strings(mesh)
	return primary, mesh
}

// defaultRouteSource 用一个不会真正发包的 UDP「连接」探测默认路由的源地址。
func defaultRouteSource() string {
	conn, err := net.Dial("udp4", "1.1.1.1:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	if a, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return a.IP.String()
	}
	return ""
}

func ifaceHasIP(ifc net.Interface, ip string) bool {
	addrs, err := ifc.Addrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if ipNet, ok := a.(*net.IPNet); ok && ipNet.IP.String() == ip {
			return true
		}
	}
	return false
}
