package main

import (
	"bytes"
	"crypto/subtle"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/sony/gobreaker"

	"github.com/rb3ckers/trafficmirror/datatypes"
)

var (
	Version string
	Commit  string
	Date    string
)

var netClient = &http.Client{
	Timeout: time.Second * 20,
}

var targets *datatypes.MirrorTargets

func main() {
	listenAddress := flag.String("listen", ":8080", "Address to listen on and mirror traffic from")
	proxyTarget := flag.String("main", "http://localhost:8888", "Main proxy target, its responses will be returned to the client")
	targetsEndpoint := flag.String("targets", "targets", "Path on which additional targets to mirror to can be added/deleted/listed via PUT, DELETE and GET")
	targetsAddress := flag.String("targets-address", "", "Address on which the targets endpoint is made available. Leave empty to expose it on the address that is being mirrored")
	passwordFile := flag.String("password", "", "Provide a file that contains username/password to protect the configuration 'targets' endpoint. Contains 1 username/password combination separated by ':'.")
	persistentFailureTimeout := flag.Duration("fail-after", time.Minute*30, "Remove a target when it has been failing for this duration.")
	retryAfter := flag.Duration("retry-after", time.Minute*1, "After 5 successive failures a target is temporarily disabled, it will be retried after this timeout.")

	help := flag.Bool("help", false, "Print help")

	flag.Parse()

	if *help {
		fmt.Printf("HTTP proxy that:")
		fmt.Printf("* sends requests to a main endpoint from which the response is returned")
		fmt.Printf("* can mirror the requests to any additional number of endpoints")
		fmt.Printf("")
		fmt.Printf("Additional targets are configured via PUT/DELETE on the `/targets?url=<endpoint>`.")

		flag.PrintDefaults()
		return
	}

	fmt.Printf("Traffic Mirror %s (Built on: %ss, Commit: %s)\n", Version, Date, Commit)
	fmt.Printf("Mirroring traffic from %s to %s\n", *listenAddress, *proxyTarget)
	var targetsText string
	if *targetsAddress != "" {
		targetsText = fmt.Sprintf("http://%s/%s", *targetsAddress, *targetsEndpoint)
	} else {
		targetsText = fmt.Sprintf("http://%s/%s", *listenAddress, *targetsEndpoint)
	}

	fmt.Printf("Add/remove/list mirror targets via PUT/DELETE/GET at %s:\n", targetsText)
	fmt.Printf("List  : curl %s\n", targetsText)
	fmt.Printf("Add   : curl -X PUT %s?url=http://localhost:5678\n", targetsText)
	fmt.Printf("Remove: curl -X DELETE %s?url=http://localhost:5678\n", targetsText)
	fmt.Println()

	targets = datatypes.NewMirrorTargets(datatypes.MirrorSettings{
		PersistentFailureTimeout: *persistentFailureTimeout,
		RetryAfter:               *retryAfter,
	})

	url, _ := url.Parse(*proxyTarget)

	mirrorMux := http.NewServeMux()
	var targetsMux *http.ServeMux

	if *targetsAddress != "" {
		targetsMux = http.NewServeMux()
	} else {
		targetsMux = mirrorMux
	}

	proxyTo := httputil.NewSingleHostReverseProxy(url)

	if *passwordFile != "" {
		username, password := parseUsernamePassword(*passwordFile)
		targetsMux.HandleFunc("/"+*targetsEndpoint, BasicAuth(mirrorsHandler, username, password, "Please provide username and password for changing mirror targets"))
	} else {
		targetsMux.HandleFunc("/"+*targetsEndpoint, mirrorsHandler)
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
	if *targetsAddress != "" {
		go func() {
			if err := http.ListenAndServe(*targetsAddress, targetsMux); err != nil {
				panic(err)
			}
		}()
	}

	// start mirror server
	if err := http.ListenAndServe(*listenAddress, mirrorMux); err != nil {
		panic(err)
	}
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

func sendToMirrors(req *http.Request, body []byte) {
	targets.ForEach(func(target string, breaker *gobreaker.CircuitBreaker) {
		go mirrorTo(target, req, body, breaker)
	})
}

func mirrorTo(targetURL string, req *http.Request, body []byte, breaker *gobreaker.CircuitBreaker) {
	breaker.Execute(func() (interface{}, error) {
		url := fmt.Sprintf("%s%s", targetURL, req.RequestURI)

		newRequest, _ := http.NewRequest(req.Method, url, bytes.NewReader(body))
		newRequest.Header = req.Header

		response, err := netClient.Do(newRequest)
		if err != nil {
			log.Printf("Error reading response: %v", err)
			return nil, err
		}
		defer response.Body.Close()
		// Drain the body, but discard it, to make sure connection can be reused
		io.Copy(ioutil.Discard, response.Body)
		return nil, nil
	})
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

// BasicAuth wraps a handler requiring HTTP basic auth for it using the given
// username and password and the specified realm, which shouldn't contain quotes.
// From StackOverflow: https://stackoverflow.com/questions/21936332/idiomatic-way-of-requiring-http-basic-auth-in-go/39591234#39591234
//
// Most web browser display a dialog with something like:
//
//    The website says: "<realm>"
//
// Which is really stupid so you may want to set the realm to a message rather than
// an actual realm.
func BasicAuth(handler http.HandlerFunc, username, password, realm string) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		user, pass, ok := r.BasicAuth()

		if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(username)) != 1 || subtle.ConstantTimeCompare([]byte(pass), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
			w.WriteHeader(401)
			w.Write([]byte("Unauthorised.\n"))
			return
		}

		handler(w, r)
	}
}

func parseUsernamePassword(passwordFile string) (string, string) {
	data, err := ioutil.ReadFile(passwordFile)
	if err != nil {
		panic("Failed to load password file.")
	}
	split := strings.SplitN(string(data), ":", 2)
	if len(split) != 2 {
		panic("Failed to parse username/password. Expected 2 find username/password separated by a new line.")
	}
	return split[0], split[1]
}
