package httpntlm

import (
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/Mostafa-Alaa-494/go-ntlm/tree/master/ntlm"
)

// NtlmTransport is implementation of http.RoundTripper interface
type NtlmTransport struct {
	Domain   string
	User     string
	Password string
	http.RoundTripper
}

// RoundTrip method send http request and tries to perform NTLM authentication
func (t NtlmTransport) RoundTrip(req *http.Request) (res *http.Response, err error) {
	// first send NTLM Negotiate header
	r, _ := http.NewRequest("GET", req.URL.String(), strings.NewReader(""))
	r.Header.Add("Authorization", "NTLM "+encBase64(negotiate()))

	client := http.Client{}
	if t.RoundTripper != nil {
		client.Transport = t.RoundTripper
	}

	resp, err := client.Do(r)
	if err != nil {
		return nil, err
	}

	if err == nil && resp.StatusCode == http.StatusUnauthorized {
		// it's necessary to reuse the same http connection
		// in order to do that it's required to read Body and close it
		_, err = io.Copy(ioutil.Discard, resp.Body)
		if err != nil {
			return nil, err
		}
		err = resp.Body.Close()
		if err != nil {
			return nil, err
		}

		// retrieve Www-Authenticate header from response
		authHeaders := resp.Header.Values("WWW-Authenticate")
		if len(authHeaders) == 0 {
			return nil, errors.New("WWW-Authenticate header missing")
		}

		// there could be multiple WWW-Authenticate headers, so we need to pick the one that starts with NTLM
		var ntlmChallengeString string
		for _, h := range authHeaders {
			if strings.HasPrefix(h, "NTLM") {
				ntlmChallengeString = strings.Replace(h, "NTLM ", "", -1)
				break
			}
		}
		if ntlmChallengeString == "" {
			return nil, errors.New("wrong WWW-Authenticate header")
		}

		challengeBytes, err := decBase64(ntlmChallengeString)
		if err != nil {
			return nil, err
		}

		session, err := ntlm.CreateClientSession(ntlm.Version2, ntlm.ConnectionlessMode)
		if err != nil {
			return nil, err
		}

		session.SetUserInfo(t.User, t.Password, t.Domain)

		// parse NTLM challenge
		challenge, err := ntlm.ParseChallengeMessage(challengeBytes)
		if err != nil {
			return nil, err
		}

		err = session.ProcessChallengeMessage(challenge)
		if err != nil {
			return nil, err
		}

		// authenticate user
		authenticate, err := session.GenerateAuthenticateMessage()
		if err != nil {
			return nil, err
		}

		// set NTLM Authorization header
		req.Header.Set("Authorization", "NTLM "+encBase64(authenticate.Bytes()))
		return client.Do(req)
	}

	return resp, err
}
