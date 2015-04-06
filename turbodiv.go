package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
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

	sideResp, _ := t.SideRequest(req, "")
	p := &IncludeProcessor{Turbodiv: t}
	sideResp.Body, _ = p.Process(req, sideResp.Body)
	sideResp.WriteTo(respWriter)

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

// Simplified structure for handling side responses.
type SideResponse struct {
	Code   int
	Header http.Header
	Body   []byte
}

func (s SideResponse) WriteTo(to http.ResponseWriter) {
	copyHeader(to.Header(), s.Header)
	to.Header().Set("Content-Length", fmt.Sprintf("%d", len(s.Body)))
	to.WriteHeader(s.Code)
	to.Write(s.Body)
}

func (t *Turbodiv) SideRequest(origReq *http.Request, src string) (*SideResponse, error) {
	// Do the side-req.
	sideReq := t.newSideReq(origReq, src)

	client := &http.Client{}
	resp, err := client.Do(sideReq)
	if err != nil {
		fmt.Printf("%#v\n", err)
	}
	defer resp.Body.Close()

	// Drain the entire response into the buffer.
	body, _ := ioutil.ReadAll(resp.Body)
	sideResp := &SideResponse{
		Code:   resp.StatusCode,
		Header: resp.Header,
		Body:   body,
	}

	return sideResp, nil
}

// Produce a new side request based on the original request.
func (t *Turbodiv) newSideReq(origReq *http.Request, urlStr string) *http.Request {
	var url *url.URL
	if urlStr == "" {
		// Deep copy the URL. This is probably totally wrong :-)
		newUrl := *origReq.URL
		if origReq.URL.User != nil {
			user := *(origReq.URL.User)
			newUrl.User = &user
		}
		url = &newUrl
	} else {
		// Let's fix the HTML-originating URL first - can have no scheme, be relative, etc.
		if strings.HasPrefix(urlStr, "//") {
			urlStr = "http:" + urlStr
		}
		if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
			// TODO This here needs the BaseURL. Probably needs to be already present in urlStr.
			urlStr = "http://localhost:8002/ssorg/" + urlStr
		}

		url, _ = url.Parse(urlStr)
	}

	// Find appropriate backend.
	var backendUrlString string
	var ok bool
	if backendUrlString, ok = t.Config.BackendMappings[url.Host]; !ok {
		backendUrlString = t.Config.BackendMappings["default"]
	}
	backendUrl, _ := url.Parse(backendUrlString)
	// Overwrite the host & scheme by using the backend mapping.
	url.Host = backendUrl.Host
	url.Scheme = backendUrl.Scheme
	fmt.Printf("Side request: %#v\n", url)

	// Bypass NewRequest for better control - e.g. we have parsed the url already.
	req := &http.Request{
		Method:     "GET",
		URL:        url,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Host:       backendUrl.Host,
	}
	copyHeader(req.Header, origReq.Header)
	return req
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
