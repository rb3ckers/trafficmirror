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

	"github.com/rb3ckers/trafficmirror/datatypes"
)

var netClient = &http.Client{
	Timeout: time.Second * 10,
}

var targets = datatypes.NewMirrorTargets()

func main() {
	listenPort := flag.String("port", "7071", "Port to listen on")
	proxyTarget := flag.String("main", "http://localhost:7072", "Main proxy target, its responses will be returned")
	mirrorsEndpoint := flag.String("targets", "targets", "Path on which additional targets to mirror to can be added/deleted/listed")
	passwordFile := flag.String("password", "", "Provide a file that contains username/password to protect the configuration 'targets' endpoint. Contains 1 username/password combination separated by '\n'.")

	help := flag.Bool("help", false, "Print help")

	flag.Parse()

	if *help {
		fmt.Printf("HTTP proxy that:")
		fmt.Printf("* sends requests to a main endpoint from which the response is returned")
		fmt.Printf("* can mirror the requests to any additional number of endpoints")
		fmt.Printf("")
		fmt.Printf("Additional targets are configured via POST/DELETE on the `/targets?url=<endpoint>`.")

		flag.PrintDefaults()
		return
	}

	url, _ := url.Parse(*proxyTarget)

	proxyTo := httputil.NewSingleHostReverseProxy(url)

	if *passwordFile != "" {
		username, password := parseUsernamePassword(*passwordFile)
		http.HandleFunc("/"+*mirrorsEndpoint, BasicAuth(mirrorsHandler, username, password, "Please provide username and password for changing mirror targets"))
	} else {
		http.HandleFunc("/"+*mirrorsEndpoint, mirrorsHandler)
	}

	http.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		body := bufferRequest(req)

		proxyTo.ServeHTTP(res, req)
		go sendToMirrors(req, body)
	})

	// start server
	if err := http.ListenAndServe(":"+*listenPort, nil); err != nil {
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
	targets.ForEach(func(target string) {
		go mirrorTo(target, req, body)
	})
}

func mirrorTo(targetURL string, req *http.Request, body []byte) {
	url := fmt.Sprintf("%s%s", targetURL, req.RequestURI)

	newRequest, _ := http.NewRequest(req.Method, url, bytes.NewReader(body))
	newRequest.Header = req.Header

	response, err := netClient.Do(newRequest)
	if err != nil {
		log.Printf("Error reading response: %v", err)
		return
	}
	defer response.Body.Close()
	// Drain the body, but discard it, to make sure connection can be reused
	io.Copy(ioutil.Discard, response.Body)
}

func mirrorsHandler(res http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet {
		targets.ForEach(func(target string) {
			fmt.Fprintln(res, target)
		})
		return
	}

	req.ParseForm()

	targetURLs, inForm := req.Form["url"]

	if !inForm {
		http.Error(res, "Missing required field: 'url'.", http.StatusBadRequest)
		return
	}

	if req.Method == http.MethodPost {
		log.Printf("Adding '%s' to targets list.", targetURLs)
		targets.Add(targetURLs)
	} else if req.Method == http.MethodDelete {
		log.Printf("Removing '%s' from targets list.", targetURLs)
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
	split := strings.SplitN(string(data), "\n", 2)
	if len(split) != 2 {
		panic("Failed to parse username/password. Expected 2 find username/password separated by a new line.")
	}
	return split[0], split[1]
}
