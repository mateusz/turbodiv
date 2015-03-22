package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {
	backend, err := url.Parse("http://localhost:8001/")
	if err != nil {
		panic(err)
	}

	// Handle all asset requests directly.
	turbodiv, err := NewTurbodiv("/etc/turbodiv.json")
	if err != nil {
		panic(err)
	}
	http.HandleFunc("/", turbodiv.ServeHTTP)
	log.Fatal(http.ListenAndServe(":8002", nil))
}
