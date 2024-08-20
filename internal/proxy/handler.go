package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"

	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/rb3ckers/trafficmirror/internal/mirror"
)

func ReverseProxyHandler(reflector *mirror.Reflector, url *url.URL) func(res http.ResponseWriter, req *http.Request) {
	// Globally incrementing epoch
	var epoch atomic.Uint64
	// Keep track of active requests
	activeRequests := cmap.NewWithCustomShardingFunction[uint64, interface{}](func(key uint64) uint32 { return uint32(key & 0x00000000FFFFFFFF) }) // Drop most significant part to go to uint32

	return func(res http.ResponseWriter, req *http.Request) {
		proxyTo := httputil.NewSingleHostReverseProxy(url)

		body := bufferRequest(req)

		// Update the headers to allow for SSL redirection
		req.URL.Host = url.Host
		req.URL.Scheme = url.Scheme
		req.Host = url.Host

		// Get the active request, these are requests that started earlier that this request can run concurrently with
		activeSnapshot := activeRequests.Items()

		// Keep track of the request epoch. We take the epoch before service the request to avoid racing (if we do it after another request might sneak in).
		requestEpoch := epoch.Add(1)
		activeRequests.Set(requestEpoch, nil)

		// Server the request to main target
		proxyTo.ServeHTTP(res, req)

		// At this point the request has been served to the main target, so we remove this as active reuqest
		activeRequests.Remove(requestEpoch)

		reflector.IncomingCh <- mirror.NewRequest(req, body, requestEpoch, activeSnapshot)
	}
}
