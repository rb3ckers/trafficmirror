package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

var netClient = &http.Client{
	Timeout: time.Second * 10,
}

var mirrors = make(map[string]bool)

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
		fmt.Printf("Additional endpoints are configured via POST/DELETE on the `/mirrors?url=<endpoint>`.")

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
	for mirrorURL := range mirrors {
		go mirrorTo(mirrorURL, req, body)
	}
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
}

func mirrorsHandler(res http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet {
		for url := range mirrors {
			fmt.Fprintln(res, url)
		}
		return
	}

	req.ParseForm()

	mirrorURLs, inForm := req.Form["url"]

	if !inForm {
		http.Error(res, "Missing required field: 'url'.", http.StatusBadRequest)
		return
	}

	if req.Method == http.MethodPost {
		log.Printf("Adding '%s' to mirror list.", mirrorURLs)
		for _, url := range mirrorURLs {
			mirrors[url] = true
		}
	} else if req.Method == http.MethodDelete {
		log.Printf("Removing '%s' from mirror list.", mirrorURLs)
		for _, url := range mirrorURLs {
			delete(mirrors, url)
		}
	}
}
