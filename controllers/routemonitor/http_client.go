package routemonitor

import (
	"io"
	"net/http"
	"strings"
)

// HTTPClient is an interface that wraps the Do and Head methods.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
	Head(url string) (*http.Response, error)
}

// RealHTTPClient is a real implementation of HTTPClient interface.
type RealHTTPClient struct {
	Client *http.Client
}

// Do sends an HTTP request and returns an HTTP response.
func (c *RealHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.Client.Do(req)
}

// Head performs a HEAD request to the specified URL.
func (c *RealHTTPClient) Head(url string) (*http.Response, error) {
	return c.Client.Head(url)
}

// NewBody returns an io.ReadCloser for the provided content.
func NewBody(content string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(content))
}

func NewRealHTTPClient() *RealHTTPClient {
	return &RealHTTPClient{
		Client: &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}
