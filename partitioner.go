package main

import (
	"fmt"
	. "github.com/yvasiyarov/php_session_decoder"
	"io/ioutil"
	"net/http"
)

func getPhpSession(sessionId string) (PhpSession, error) {
	sessionFile := fmt.Sprintf("/var/tmp/sess_%s", sessionId)
	sessionBytes, err := ioutil.ReadFile(sessionFile)
	if err != nil {
		return nil, err
	}

	decoder := NewPhpDecoder(string(sessionBytes))
	session, err := decoder.Decode()
	if err != nil {
		return nil, err
	}

	return session, nil
}

func extractLoggedInAs(req *http.Request) (int, error) {
	sessionCookie, err := req.Cookie("PHPSESSID")
	if err != nil {
		return 0, nil
	}

	var session PhpSession
	session, err = getPhpSession(sessionCookie.Value)
	if err != nil {
		return 0, nil
	}

	id, ok := session["loggedInAs"]
	if !ok {
		return 0, nil
	} else {
		return id.(int), nil
	}
}

/*
func PartitionerMemberIn(req *http.Request) (http.Request, error) {
	// Extract Member ID based on Cookie.
	//
	// Inject header.
	return nil, nil
}

func PartitionerMemberOut(req *http.Request) (http.Request, error) {
	// Noop?
	return nil, nil
}
*/
