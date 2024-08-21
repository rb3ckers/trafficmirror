package proxy

import (
	"github.com/rb3ckers/trafficmirror/internal/mirror"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

func ReverseProxyHandler(reflector *mirror.Reflector, url *url.URL, sendDelay time.Duration) func(res http.ResponseWriter, req *http.Request) {
	tracker := MakeRequestTracker()

	return func(res http.ResponseWriter, req *http.Request) {
		proxyTo := httputil.NewSingleHostReverseProxy(url)

		body := bufferRequest(req)

		// Update the headers to allow for SSL redirection
		req.URL.Host = url.Host
		req.URL.Scheme = url.Scheme
		req.Host = url.Host

		// Get the active request, these are requests that started earlier that this request can run concurrently with
		// Keep track of the request epoch. We take the epoch before service the request to avoid racing (if we do it after another request might sneak in).
		requestEpoch, activeSnapshot := tracker.NewRequest()

		// Catch panic in serving HTTP
		defer func() {
			if p := recover(); p != nil {
				// At this point the request has been served to the main target, so we remove this as active request
				tracker.RequestDone(requestEpoch)

				reflector.IncomingCh <- mirror.NewRequest(req, body, requestEpoch, activeSnapshot)

				panic(p)
			}
		}()

		time.Sleep(sendDelay)

		// Server the request to main target
		proxyTo.ServeHTTP(res, req)

		// At this point the request has been served to the main target, so we remove this as active request
		tracker.RequestDone(requestEpoch)

		reflector.IncomingCh <- mirror.NewRequest(req, body, requestEpoch, activeSnapshot)
	}
}
