package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"golang.org/x/net/html"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"regexp"
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
	// This is probably completely wrong - it feels like we should create only one reverse proxy per origin?
	proxy := httputil.NewSingleHostReverseProxy(backendUrl)

	// We cannot pass the response straight to the original client, because we want to do our substitutions.
	buffering := newBufferingResponseWriter(respWriter)
	proxy.ServeHTTP(buffering, req)

	// Use regexp, because we don't want to force normalisation of HTML.
	// We are looking for esi tags, such as:
	// <esi:include
	//		src="http://some/url"
	//		processor="replace"
	//		partitioner="Member" />
	// Maybe use regexp templates instead? (the Expand fn)
	esi := regexp.MustCompile("<esi:include[^>]*>")
	newBytes := esi.ReplaceAllFunc(buffering.BodyBuffer.Bytes(), func(match []byte) []byte {
		// We are going to replace the esi tag though, so we can parse this properly.
		var (
			snippet *html.Node
			err     error
		)

		if snippet, err = html.Parse(bytes.NewBuffer(match)); err != nil {
			return []byte("<!-- invalid esi tag -->")
		}

		// Iteratively descend into the node tree to find esi tags.
		var descend func(*html.Node) ([]byte, bool)
		descend = func(node *html.Node) ([]byte, bool) {
			if node.Type == html.ElementNode && node.Data == "esi:include" {

				// Process the esi:include
				extraAttr := make(map[string]string)
				var (
					src       string
					processor Processor
				)

				for _, attr := range node.Attr {
					switch {
					default:
						extraAttr[attr.Key] = attr.Val
					case attr.Key == "src":
						src = attr.Val
					case attr.Key == "processor":
						switch attr.Val {
						case "replace":
							processor = ReplaceProcessor{}
						}
					}
				}

				// We have found a legit esi tag - no point in searching any further.
				if src != "" && processor != nil {
					if replacement, err := processor.Process(req, src, extraAttr); err == nil {
						return replacement, true
					}
				}

			}

			// Descend into sub-nodes, looking for first match. We are still within the string matched by regexp.
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if replacement, found := descend(child); found {
					return replacement, true
				}
			}

			// Nothing of interest found. Could be no tag, or broken tag in the matched string.
			return nil, false
		}

		var (
			replacement []byte
			found       bool
		)

		if replacement, found = descend(snippet); !found {
			return []byte("<!-- invalid esi tag -->")
		}

		return replacement

	})

	buffering.ReplaceBody(newBytes)
	buffering.Flush()

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

// bufferingResponseWriter buffers the entire response so that it can be modified.
// Once ready, the response can be flushed to the original destination.
type bufferingResponseWriter struct {
	BodyBuffer     bytes.Buffer
	CodeBuffer     int
	HeaderBuffer   http.Header
	OriginalWriter http.ResponseWriter
}

func newBufferingResponseWriter(respWriter http.ResponseWriter) *bufferingResponseWriter {
	return &bufferingResponseWriter{
		HeaderBuffer:   make(http.Header),
		OriginalWriter: respWriter,
	}
}

// Meaningless before flush.
func (rw *bufferingResponseWriter) Header() http.Header {
	return rw.HeaderBuffer
}

func (rw *bufferingResponseWriter) Write(buf []byte) (int, error) {
	return rw.BodyBuffer.Write(buf)
}

func (rw *bufferingResponseWriter) WriteHeader(code int) {
	rw.CodeBuffer = code
}

// Flush the content to the downstream writer.
func (rw *bufferingResponseWriter) Flush() (int, error) {
	copyHeader(rw.OriginalWriter.Header(), rw.HeaderBuffer)
	rw.OriginalWriter.WriteHeader(rw.CodeBuffer)
	return rw.OriginalWriter.Write(rw.BodyBuffer.Bytes())
}

func (rw *bufferingResponseWriter) ReplaceBody(newBytes []byte) {
	rw.BodyBuffer.Reset()
	rw.BodyBuffer.Write(newBytes)
	rw.Header().Set("Content-Length", fmt.Sprintf("%d", len(newBytes)))
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
