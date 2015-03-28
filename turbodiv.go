package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	BackendMappings map[string]string `json:"backendMappings"`
}

type Turbodiv struct {
	Config           *Config
	StripSessionUrls map[string]time.Time
}

func NewTurbodiv(configPath string) (*Turbodiv, error) {

	config := &Config{}

	jsonStr, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(jsonStr, config)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Loaded backends:\n%v\n", config.BackendMappings)

	turbodiv := &Turbodiv{
		Config:           config,
		StripSessionUrls: make(map[string]time.Time),
	}

	return turbodiv, nil
}

func (t *Turbodiv) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	var backendUrlString string
	var ok bool
	originalUrl := req.URL.String()

	// Check if we should strip session for this URL.
	if _, ok := t.StripSessionUrls[originalUrl]; ok {
		delete(req.Header, "Cookie")
	}

	// Proxy the request by finding a relevant backend mapping.
	if backendUrlString, ok = t.Config.BackendMappings[req.Host]; !ok {
		backendUrlString = t.Config.BackendMappings["default"]
	}
	backendUrl, _ := url.Parse(backendUrlString)
	proxy := httputil.NewSingleHostReverseProxy(backendUrl)
	proxy.ServeHTTP(resp, req)

	// Check Cache-Control header to see if we need to do anything special in the future for this URL.
	cacheControl := resp.Header()["Cache-Control"]
	if len(cacheControl) > 0 {
		for _, verb := range strings.Split(cacheControl[0], ",") {
			verb = strings.TrimSpace(verb)
			if verb == "strip-session" {
				// We should remove request cookies next time around, because backend doesn't care about them.
				// This will allow Varnish to respond with a cached response, regardless of cookies in the request.
				t.StripSessionUrls[originalUrl] = time.Now()
			}
		}
	}

	// If response is HTML, parse it, looking for ESI markup, i.e. something like this:
	//
	//	<esi:include
	//		src="//localhost:8002/ssorg/toolbar/profile"
	//		processor="replace"
	//		partitioner="Member" />

	// Invoke processors as needed depending on what is found in the markup.
}
