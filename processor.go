package main

import (
	"net/http"
)

type Processor interface {
	Process(origReq *http.Request, src string, extraAttr map[string]string) ([]byte, error)
}

type ReplaceProcessor struct{}

func (p ReplaceProcessor) Process(origReq *http.Request, src string, extraAttr map[string]string) ([]byte, error) {
	// Build the side-request.
	//
	// Check other Turbodiv attributes, invoke Partitioners as needed on the request (In).
	//
	// Send the request.
	//
	// Invoke Partitioners on the response (Out).
	return nil, nil
}
