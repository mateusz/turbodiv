package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	. "github.com/yvasiyarov/php_session_decoder"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

type Config struct {
	Processors struct {
		Name string `json:"name"`
	}
	Partitioners struct {
		Name string `json:"name"`
	}
}

type Turbodiv struct {
	Config *Config
}

func NewTurbodiv(configPath string) (*Turbodiv, error) {

	config := &Config{}
	/*
		jsonStr, err := ioutil.ReadFile(configPath)
		if err != nil {
			return nil, err
		}

		// Use field tags http://weekly.golang.org/pkg/encoding/json/#Marshal
		err = json.Unmarshal(jsonStr, config)
		if err != nil {
			return nil, err
		}
	*/

	return turbodiv, nil
}

func (t *Turbodiv) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	// Proxy the request.
	//
	// If response is HTML, parse it, looking for Turbodiv markup.
	//
	// Invoke processors as needed depending on what is found in the markup.
}
