package main

import (
	"bytes"
	"fmt"
	"golang.org/x/net/html"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
)

type Processor interface {
	Process(origReq *http.Request, src string, extraAttr map[string]string) (*EsiResponse, error)
}

type IncludeProcessor struct {
	Turbodiv *Turbodiv
}

// Produce a new side request based on the original request.
func (p IncludeProcessor) newSideReq(origReq *http.Request, urlStr string) *http.Request {
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
		url, _ = url.Parse(urlStr)
	}

	// Find appropriate backend.
	var backendUrlString string
	var ok bool
	if backendUrlString, ok = p.Turbodiv.Config.BackendMappings[url.Host]; !ok {
		backendUrlString = p.Turbodiv.Config.BackendMappings["default"]
	}
	backendUrl, _ := url.Parse(backendUrlString)
	// Overwrite the host & scheme by using the backend mapping.
	url.Host = backendUrl.Host
	url.Scheme = backendUrl.Scheme

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

func (p IncludeProcessor) esiReplacer(match []byte) []byte {
	// The match string should contain only the esi tag as matched by regexp, might as well parse this properly.
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
			var src string
			for _, attr := range node.Attr {
				switch {
				default:
					extraAttr[attr.Key] = attr.Val
				case attr.Key == "src":
					src = attr.Val
				}
			}

			// We have found a legit esi tag - no point in searching any further.
			if src != "" {
				return []byte("TESTING"), true
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

}

func (p IncludeProcessor) Process(origReq *http.Request, src string, extraAttr map[string]string) (*EsiResponse, error) {
	// Do the side-req.
	sideReq := p.newSideReq(origReq, src)

	client := &http.Client{}
	resp, err := client.Do(sideReq)
	if err != nil {
		fmt.Printf("%#v\n", err)
	}
	defer resp.Body.Close()

	// Drain the buffer so that we can perform the replacements.
	body, _ := ioutil.ReadAll(resp.Body)

	// We cannot pass the response straight to the original client, because we want to do our substitutions.
	// Use regexp, because we don't want to force normalisation of HTML.
	// We are looking for esi tags, such as:
	// <esi:include
	//		src="http://some/url"
	//		action="replace"
	//		partitioner="Member" />
	// Maybe use regexp templates instead? (the Expand fn)
	esi := regexp.MustCompile("<esi:include[^>]*>")
	newBytes := esi.ReplaceAllFunc(body, p.esiReplacer)

	esiResp := &EsiResponse{
		Code:   resp.StatusCode,
		Header: resp.Header,
		Body:   newBytes,
	}

	return esiResp, nil
}

type EsiResponse struct {
	Code   int
	Header http.Header
	Body   []byte
}

func (e EsiResponse) WriteTo(to http.ResponseWriter) {
	copyHeader(to.Header(), e.Header)
	to.WriteHeader(e.Code)
	to.Write(e.Body)
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
