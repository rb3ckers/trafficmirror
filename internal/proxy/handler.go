package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/rb3ckers/trafficmirror/internal/mirror"
)

func ReverseProxyHandler(reflector *mirror.Reflector, url *url.URL) func(res http.ResponseWriter, req *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		proxyTo := httputil.NewSingleHostReverseProxy(url)

		body := bufferRequest(req)

		// Update the headers to allow for SSL redirection
		req.URL.Host = url.Host
		req.URL.Scheme = url.Scheme
		req.Host = url.Host

		proxyTo.ServeHTTP(res, req)
		reflector.IncomingCh <- mirror.NewRequest(req, body)
	}
}
