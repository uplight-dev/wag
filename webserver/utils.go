package webserver

import (
	"net"
	"net/http"
	"strings"
	"wag/utils"
)

func getIPFromRequest(r *http.Request) string {

	//Do not respect the X-Forwarded-For header until we are explictly told we are being proxied.
	if isProxied {
		ips := r.Header.Get("X-Forwarded-For")

		addresses := strings.Split(ips, ",")
		if ips != "" && len(addresses) > 0 && net.ParseIP(addresses[0]) != nil {
			return addresses[0]
		}
	}

	return utils.GetIP(r.RemoteAddr)
}