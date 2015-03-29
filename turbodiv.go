package main

import (
	"encoding/json"
	"fmt"
	//"golang.org/x/net/html"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Config struct {
	BackendMappings map[string]string `json:"backendMappings"`
}

type stringToTimeMap struct {
	sync.RWMutex
	Urls map[string]time.Time
}

type Turbodiv struct {
	Config           *Config
	StripSessionUrls *stringToTimeMap
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
		Config: config,
		StripSessionUrls: &stringToTimeMap{
			Urls: make(map[string]time.Time),
		},
	}

	return turbodiv, nil
}

func bridge(from *httptest.ResponseRecorder, to http.ResponseWriter) {
	to.WriteHeader(from.Code)
	for name, value := range from.Header() {
		to.Header()[name] = value
	}
	to.Write(from.Body.Bytes())
}

func (t *Turbodiv) ServeHTTP(respWriter http.ResponseWriter, req *http.Request) {
	var backendUrlString string
	var ok bool
	originalUrl := req.URL.String()

	// Check if we should strip session for this URL.
	t.StripSessionUrls.RLock()
	_, ok = t.StripSessionUrls.Urls[originalUrl]
	t.StripSessionUrls.RUnlock()
	if ok {
		delete(req.Header, "Cookie")
	}

	// Proxy the request by finding a relevant backend mapping.
	if backendUrlString, ok = t.Config.BackendMappings[req.Host]; !ok {
		backendUrlString = t.Config.BackendMappings["default"]
	}
	backendUrl, _ := url.Parse(backendUrlString)
	proxy := httputil.NewSingleHostReverseProxy(backendUrl)

	// We cannot pass the response straight to the original client, because we want to do our substitutions.
	amending := &amendingResponseWriter{
		AmendFunc: func(buf []byte) {
			fmt.Printf("Amending!\n")
			// If response is HTML, parse it, looking for ESI markup, i.e. something like this:
			//
			//	<esi:include
			//		src="//localhost:8002/ssorg/toolbar/profile"
			//		processor="replace"
			//		partitioner="Member" />
			//html.ParseFragment

			// Invoke processors as needed depending on what is found in the markup.
		},
		OriginalWriter: respWriter,
	}
	proxy.ServeHTTP(amending, req)

	// Check Cache-Control header to see if we need to do anything special in the future for this URL.
	cacheControl := respWriter.Header()["Cache-Control"]
	if len(cacheControl) > 0 {
		for _, verb := range strings.Split(cacheControl[0], ",") {
			verb = strings.TrimSpace(verb)
			if verb == "strip-session" {
				// We should remove request cookies next time around, because backend doesn't care about them.
				// This will allow Varnish to respond with a cached response, regardless of cookies in the request.
				t.StripSessionUrls.Lock()
				t.StripSessionUrls.Urls[originalUrl] = time.Now()
				t.StripSessionUrls.Unlock()
			}
		}
	}

}

type amendingResponseWriter struct {
	AmendFunc      func([]byte)
	OriginalWriter http.ResponseWriter
}

func (rw *amendingResponseWriter) Header() http.Header {
	return rw.OriginalWriter.Header()
}

func (rw *amendingResponseWriter) Write(buf []byte) (int, error) {
	rw.AmendFunc(buf)
	return rw.OriginalWriter.Write(buf)
}

func (rw *amendingResponseWriter) WriteHeader(code int) {
	rw.OriginalWriter.WriteHeader(code)
}
