package main

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

func extractLoggedInAs() (int, error) {
	sessionCookie, err := req.Cookie("PHPSESSID")
	if err {
		return 0, nil
	}

	var session PhpSession
	session, err = getPhpSession(sessionCookie.Value)
	if err {
		return 0, nil
	}

	id, ok := session["loggedInAs"]
	if !ok {
		return 0, nil
	} else {
		return id, nil
	}
}

func PartitionerMemberIn(req *http.Request) (http.Request, error) {
	// Extract Member ID based on Cookie.
	//
	// Inject header.
}

func PartitionerMemberOut(req *http.Request) (http.Request, error) {
	// Noop?

}
