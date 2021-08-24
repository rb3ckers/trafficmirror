package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/rb3ckers/trafficmirror/datatypes"
	"github.com/rb3ckers/trafficmirror/internal/config"
	"github.com/sony/gobreaker"
)

var netClient = &http.Client{
	Timeout: time.Second * 20,
}

var targets *datatypes.MirrorTargets

func Start(ctx context.Context, cfg *config.Config) error {
	targets = datatypes.NewMirrorTargets(datatypes.MirrorSettings{
		PersistentFailureTimeout: time.Duration(cfg.PersistentFailureTimeout) * time.Minute,
		RetryAfter:               time.Duration(cfg.RetryAfter) * time.Minute,
	})

	url, _ := url.Parse(cfg.MainProxyTarget)

	mirrorMux := http.NewServeMux()

	var targetsMux *http.ServeMux

	if cfg.TargetsListenAddress != "" {
		targetsMux = http.NewServeMux()
	} else {
		targetsMux = mirrorMux
	}

	proxyTo := httputil.NewSingleHostReverseProxy(url)

	if cfg.PasswordFile != "" {
		username, password, err := parseUsernamePassword(cfg.PasswordFile)
		if err != nil {
			return err
		}

		targetsMux.HandleFunc("/"+cfg.TargetsEndpoint, BasicAuth(mirrorsHandler, username, password, "Please provide username and password for changing mirror targets"))
	} else {
		targetsMux.HandleFunc("/"+cfg.TargetsEndpoint, mirrorsHandler)
	}

	mirrorMux.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		body := bufferRequest(req)

		// Update the headers to allow for SSL redirection
		req.URL.Host = url.Host
		req.URL.Scheme = url.Scheme
		req.Host = url.Host

		proxyTo.ServeHTTP(res, req)
		go sendToMirrors(req, body)
	})

	// start configuration server if needed
	if cfg.TargetsListenAddress != "" {
		go func() {
			if err := http.ListenAndServe(cfg.TargetsListenAddress, targetsMux); err != nil {
				panic(err)
			}
		}()
	}

	// start mirror server
	if err := http.ListenAndServe(cfg.ListenAddress, mirrorMux); err != nil {
		panic(err)
	}

	return nil
}

func mirrorsHandler(res http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet {
		for _, target := range targets.ListTargets() {
			if target.State == "alive" {
				fmt.Fprintf(res, "%s: %s\n", target.Name, target.State)
			} else {
				fmt.Fprintf(res, "%s: %s (since: %s)\n", target.Name, target.State, target.FailingSince.UTC().Format(time.RFC3339))
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
		targets.Add(targetURLs)
	} else if req.Method == http.MethodDelete {
		targets.Delete(targetURLs)
	}
}

func sendToMirrors(req *http.Request, body []byte) {
	targets.ForEach(func(target string, breaker *gobreaker.CircuitBreaker) {
		go mirrorTo(target, req, body, breaker)
	})
}

func mirrorTo(targetURL string, req *http.Request, body []byte, breaker *gobreaker.CircuitBreaker) {
	breaker.Execute(func() (interface{}, error) { //nolint:errcheck
		url := fmt.Sprintf("%s%s", targetURL, req.RequestURI)

		newRequest, _ := http.NewRequest(req.Method, url, bytes.NewReader(body)) //nolint:noctx
		newRequest.Header = req.Header

		response, err := netClient.Do(newRequest)
		if err != nil {
			log.Printf("Error reading response: %v", err)
			return nil, err
		}
		defer response.Body.Close()
		// Drain the body, but discard it, to make sure connection can be reused
		return io.Copy(ioutil.Discard, response.Body)
	})
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
