package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
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

// We will hold up the main request, then perform side-requests and reassemble the main response.
func (t *Turbodiv) ServeHTTP(respWriter http.ResponseWriter, req *http.Request) {
	var ok bool
	originalUrl := req.URL.String()

	// Check if we should strip session for this URL.
	t.StripSessionUrls.RLock()
	_, ok = t.StripSessionUrls.Urls[originalUrl]
	t.StripSessionUrls.RUnlock()
	if ok {
		delete(req.Header, "Cookie")
	}

	p := &IncludeProcessor{Turbodiv: t}
	esiResponse, _ := p.Process(req, "", nil)
	esiResponse.WriteTo(respWriter)

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
