package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rb3ckers/trafficmirror/internal/config"
	"github.com/rb3ckers/trafficmirror/internal/mirror"
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

	p.reflector.AddMirrors(cfg.Mirrors)

	go p.reflector.Reflect()

	return p
}

func (p *Proxy) Start(ctx context.Context) error {
	p.waitGroup = &sync.WaitGroup{}
	p.waitGroup.Add(1)

	url, _ := url.Parse(p.cfg.MainProxyTarget)
	mirrorMux := http.NewServeMux()
	p.httpServer = &http.Server{Addr: p.cfg.ListenAddress, Handler: mirrorMux}

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

	mirrorMux.HandleFunc("/", ReverseProxyHandler(p.reflector, url))

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
				fmt.Fprintf(res, "%s: %s\n", target.URL, target.State)
			} else {
				fmt.Fprintf(res, "%s: %s (since: %s)\n", target.URL, target.State, target.FailingSince.UTC().Format(time.RFC3339))
			}
		}

		return
	}

	req.ParseForm()

	targetURLs, inForm := req.Form["url"]

	if !inForm {
		http.Error(res, "Missing required field: 'url'.", http.StatusBadRequest)
		return
	}

	if req.Method == http.MethodPut {
		p.reflector.AddMirrors(targetURLs)
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
	if p.cfg.PasswordFile != "" {
		username, password, err := parseUsernamePassword(p.cfg.PasswordFile)
		if err != nil {
			return err
		}

		targetsMux.HandleFunc("/"+p.cfg.TargetsEndpoint, BasicAuth(p.mirrorsHandler, username, password, "Please provide username and password for changing mirror targets"))
	} else {
		targetsMux.HandleFunc("/"+p.cfg.TargetsEndpoint, p.mirrorsHandler)
	}

	return nil
}
