package main

import (
	"log"
	"net/http"
)

func main() {
	// Handle all asset requests directly.
	turbodiv, err := NewTurbodiv("/etc/turbodiv.json")
	if err != nil {
		panic(err)
	}
	http.HandleFunc("/", turbodiv.ServeHTTP)
	log.Fatal(http.ListenAndServe(":8002", nil))
}
