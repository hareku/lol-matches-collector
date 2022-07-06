package lolmatch

import "net/http"

type RiotAuth struct {
	Token string
	Base  http.RoundTripper
}

func (r *RiotAuth) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-Riot-Token", r.Token)
	return r.Base.RoundTrip(req)
}
