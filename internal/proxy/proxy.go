package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/pprof"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rb3ckers/trafficmirror/internal/config"
	"github.com/rb3ckers/trafficmirror/internal/mirror"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type Proxy struct {
	cfg        *config.Config
	reflector  *mirror.Reflector
	waitGroup  *sync.WaitGroup
	httpServer *http.Server
}

func NewProxy(cfg *config.Config) *Proxy {
	p := &Proxy{
		cfg:       cfg,
		reflector: mirror.NewReflector(cfg),
	}

	p.reflector.AddMirrors(cfg.Mirrors, true)

	go p.reflector.Reflect()

	return p
}

func (p *Proxy) Start(ctx context.Context) error {
	p.waitGroup = &sync.WaitGroup{}
	p.waitGroup.Add(1)

	url, _ := url.Parse(p.cfg.MainProxyTarget)
	mirrorMux := http.NewServeMux()

	h2s := &http2.Server{}
	p.httpServer = &http.Server{Addr: p.cfg.ListenAddress, Handler: h2c.NewHandler(mirrorMux, h2s)}

	targetsMux := mirrorMux
	targetsServer := p.httpServer

	if p.cfg.TargetsListenAddress != "" {
		targetsMux = http.NewServeMux()
		targetsServer = &http.Server{Addr: p.cfg.TargetsListenAddress, Handler: targetsMux}

		p.waitGroup.Add(1)
	}

	if err := p.setupTargetsMux(targetsMux); err != nil {
		return err
	}

	mirrorMux.HandleFunc("/", ReverseProxyHandler(p.reflector, url, time.Duration(p.cfg.MainTargetDelayMs)*time.Millisecond))

	// start configuration server if needed
	if p.cfg.TargetsListenAddress != "" {
		p.httpServer.RegisterOnShutdown(func() {
			if err := targetsServer.Shutdown(context.Background()); err != nil { //nolint:staticcheck
				// Already in shutdown mode, ignore error
			}
		})

		startHTTPServer(p.waitGroup, targetsServer)
	}

	// start mirror server
	startHTTPServer(p.waitGroup, p.httpServer)

	return nil
}

func startHTTPServer(wg *sync.WaitGroup, srv *http.Server) {
	go func() {
		defer wg.Done()
		// always returns error. ErrServerClosed on graceful close
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			// unexpected error. port in use?
			log.Printf("Unexpected error running server: %v", err)
		}
	}()
}

func (p *Proxy) Stop() error {
	if err := p.httpServer.Shutdown(context.TODO()); err != nil {
		panic(err) // failure/timeout shutting down the server gracefully
	}

	p.reflector.Close()

	return nil
}

func (p *Proxy) mirrorsHandler(res http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet {
		for _, target := range p.reflector.ListMirrors() {
			if target.State == mirror.StateAlive {
				fmt.Fprintf(res, "%s: %s -- queued: %d -- processed: %d\n", target.URL, target.State, target.QueuedRequests, target.Epoch)
			} else {
				fmt.Fprintf(res, "%s: %s (since: %s) -- queued: %d -- processed: %d\n", target.URL, target.State, target.FailingSince.UTC().Format(time.RFC3339), target.QueuedRequests, target.Epoch)
			}
		}

		return
	}

	if err := req.ParseForm(); err != nil {
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}

	targetURLs, inForm := req.Form["url"]

	if !inForm {
		http.Error(res, "Missing required field: 'url'.", http.StatusBadRequest)
		return
	}

	persistent := req.Form.Has("persistent")
	if persistent {
		persistent = strings.ToLower(req.Form.Get("persistent")) == "true"
	}

	if req.Method == http.MethodPut {
		p.reflector.AddMirrors(targetURLs, persistent)
	} else if req.Method == http.MethodDelete {
		p.reflector.RemoveMirrors(targetURLs)
	}
}

func parseUsernamePassword(passwordFile string) (string, string, error) {
	data, err := ioutil.ReadFile(passwordFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to load password file")
	}

	split := strings.SplitN(string(data), ":", 2) //nolint:gomnd
	if len(split) != 2 {                          //nolint:gomnd
		return "", "", fmt.Errorf("failed to parse username/password. Expected 2 find username/password separated by a new line")
	}

	return split[0], split[1], nil
}

func bufferRequest(req *http.Request) []byte {
	// Read body to buffer
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
	}

	// Restore the body so we can send it multiple times
	req.Body = ioutil.NopCloser(bytes.NewBuffer(body))

	return body
}

func (p *Proxy) setupTargetsMux(targetsMux *http.ServeMux) error {
	var username, password string

	if p.cfg.PasswordFile != "" {
		u, pwd, err := parseUsernamePassword(p.cfg.PasswordFile)
		if err != nil {
			return err
		}

		username = u
		password = pwd
	} else if p.cfg.Username != "" && p.cfg.Password != "" {
		username = p.cfg.Username
		password = p.cfg.Password
	}

	if username != "" && password != "" {
		log.Printf("/" + p.cfg.TargetsEndpoint + " is basic auth protected, username is '" + username + "'")
		targetsMux.HandleFunc("/"+p.cfg.TargetsEndpoint, BasicAuth(p.mirrorsHandler, username, password, "Please provide username and password for changing mirror targets"))
	} else {
		targetsMux.HandleFunc("/"+p.cfg.TargetsEndpoint, p.mirrorsHandler)
	}

	if p.cfg.EnablePProf {
		targetsMux.HandleFunc("/debug/pprof/", pprof.Index)
		targetsMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		targetsMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		targetsMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		targetsMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	return nil
}
