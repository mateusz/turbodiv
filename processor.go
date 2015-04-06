package main

import (
	"bytes"
	"golang.org/x/net/html"
	"net/http"
	"regexp"
	"sync"
)

type Processor interface {
	Process(origReq *http.Request, body []byte) ([]byte, error)
}

type IncludeProcessor struct {
	Turbodiv *Turbodiv
}

func (p IncludeProcessor) resolveEsi(origReq *http.Request, esi []byte) []byte {
	// The match string should contain only the esi tag as matched by regexp, might as well parse this properly.
	var (
		snippet *html.Node
		err     error
	)

	if snippet, err = html.Parse(bytes.NewBuffer(esi)); err != nil {
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
				sideResp, _ := p.Turbodiv.SideRequest(origReq, src)
				// TODO This... probably works. SS is generating something fun - including a var_dump :-)
				return sideResp.Body, true
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

// Mutex-protected buffer.
type concurrentBuffer struct {
	sync.RWMutex
	Buffer []byte
}

func (cb *concurrentBuffer) FindAll(re *regexp.Regexp) [][]byte {
	cb.RLock()
	defer cb.RUnlock()
	return re.FindAll(cb.Buffer, -1)
}

func (cb *concurrentBuffer) Replace(src []byte, dst []byte) {
	cb.Lock()
	defer cb.Unlock()
	cb.Buffer = bytes.Replace(cb.Buffer, src, dst, -1)
}

// Processes the body in place.
func (p IncludeProcessor) Process(origReq *http.Request, body []byte) ([]byte, error) {
	cbuf := concurrentBuffer{
		Buffer: body,
	}

	// We are looking for esi tags, such as:
	// <esi:include
	//		src="http://some/url"
	//		action="replace"
	//		partitioner="Member" />
	// Use regexp, because we don't want to force normalisation of HTML.
	// We want to execute in parallel so we cannot use regexp.ReplaceAllFunc.
	esi := regexp.MustCompile("<esi:include[^>]*>")
	var wg sync.WaitGroup

	// This here will continue fetching until all esi tags have been replaced.
	// It will fetch esi tags within esi tags... probably :-)
	manyMatched := cbuf.FindAll(esi)
	for manyMatched != nil {

		for _, match := range manyMatched {
			// TODO We should really only fetch the esi tags we are not already fetching. Some normalisation would be needed.
			wg.Add(1)
			go func(match []byte) {
				defer wg.Done()

				// Actual HTTP request happens inside.
				resolved := p.resolveEsi(origReq, match)
				cbuf.Replace(match, resolved)
			}(match)

		}

		// TODO This could let us go as soon as the first answer is returned. No such functionality in the library though.
		wg.Wait()
		// Subsequent round of esi fetches begins here.
		manyMatched = cbuf.FindAll(esi)
	}

	return cbuf.Buffer, nil
}
