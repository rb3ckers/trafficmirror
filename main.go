package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
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
	mirrorsEndpoint := flag.String("mirrors", "mirrors", "Path on which additional mirrors can be added/deleted/listed")

	help := flag.Bool("help", false, "Print help")

	flag.Parse()

	if *help {
		fmt.Printf("HTTP proxy that:")
		fmt.Printf("* sends requests to a main endpoint from which the response is returned")
		fmt.Printf("* can mirror the requests to any additional number of endpoints")
		fmt.Printf("")
		fmt.Printf("Additional targets are configured via POST/DELETE on the `/mirrorTargets?url=<endpoint>`.")

		flag.PrintDefaults()
		return
	}

	url, _ := url.Parse(*proxyTarget)

	proxyTo := httputil.NewSingleHostReverseProxy(url)

	http.HandleFunc("/"+*mirrorsEndpoint, mirrorsHandler)

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
