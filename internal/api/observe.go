package api

import (
	"net/netip"

	"github.com/labstack/echo/v4"

	"ondaire/internal/id"
)

// observeMiddleware records a peer's real source IP into the cluster's
// observation map (§3.1). A request carrying X-Ondaire-Proxied:1 came directly
// from a peer node's socket, and X-Ondaire-From names that peer — so its
// RemoteAddr IP is a genuine observed address for THAT peer. We never trust
// X-Forwarded-For (§3.1 trust model); RemoteAddr only.
func (s *Server) observeMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		req := c.Request()
		if req.Header.Get(proxiedHeader) != "" {
			if from := req.Header.Get(fromHeader); from != "" {
				if peer, err := id.Parse(from); err == nil {
					if ip := remoteIP(req.RemoteAddr); ip.IsValid() {
						s.cfg.Cluster.Observe(peer, ip)
					}
				}
			}
		}
		return next(c)
	}
}

// remoteIP parses the IP from a "host:port" RemoteAddr (host may be a bare IP).
func remoteIP(remoteAddr string) netip.Addr {
	if ap, err := netip.ParseAddrPort(remoteAddr); err == nil {
		return ap.Addr().Unmap()
	}
	if a, err := netip.ParseAddr(remoteAddr); err == nil {
		return a.Unmap()
	}
	return netip.Addr{}
}
