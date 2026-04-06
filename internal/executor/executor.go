package executor

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/smallfish/httprun/internal/resolver"
)

type Config struct {
	Timeout time.Duration
}

type Result struct {
	Request  resolver.ResolvedRequest
	Response *http.Response
	Body     []byte
	Duration time.Duration
}

type Session struct {
	config Config
	jar    http.CookieJar
}

func NewSession(config Config) (*Session, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	return &Session{
		config: config,
		jar:    jar,
	}, nil
}

func (s *Session) Execute(ctx context.Context, request resolver.ResolvedRequest) (Result, error) {
	client := &http.Client{
		Timeout:       timeoutForRequest(request, s.config.Timeout),
		Jar:           jarForRequest(s.jar, request.NoCookieJar),
		CheckRedirect: redirectPolicy(request.NoRedirect),
		Transport:     transportForRequest(request),
	}

	httpRequest, err := http.NewRequestWithContext(ctx, request.Method, request.URL, strings.NewReader(string(request.Body)))
	if err != nil {
		return Result{}, err
	}
	httpRequest.Header = request.Headers.Clone()

	start := time.Now()
	response, err := client.Do(httpRequest)
	if err != nil {
		return Result{}, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Request:  request,
		Response: response,
		Body:     body,
		Duration: time.Since(start),
	}, nil
}

func timeoutForRequest(request resolver.ResolvedRequest, fallback time.Duration) time.Duration {
	if request.Timeout != nil {
		return *request.Timeout
	}
	return fallback
}

func transportForRequest(request resolver.ResolvedRequest) http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if request.ConnectionTimeout != nil {
		dialer := &net.Dialer{
			Timeout:   *request.ConnectionTimeout,
			KeepAlive: 30 * time.Second,
		}
		transport.DialContext = dialer.DialContext
	}
	return transport
}

func redirectPolicy(noRedirect bool) func(req *http.Request, via []*http.Request) error {
	if !noRedirect {
		return nil
	}

	return func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
}

func jarForRequest(jar http.CookieJar, noCookieJar bool) http.CookieJar {
	if !noCookieJar {
		return jar
	}
	return suppressSetCookiesJar{inner: jar}
}

type suppressSetCookiesJar struct {
	inner http.CookieJar
}

func (j suppressSetCookiesJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
}

func (j suppressSetCookiesJar) Cookies(u *url.URL) []*http.Cookie {
	if j.inner == nil {
		return nil
	}
	return j.inner.Cookies(u)
}
