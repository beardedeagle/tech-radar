package discovery

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

const maxErrorBodyBytes = 64 << 10

func newGitHubCollector(transport http.RoundTripper) (collector, error) {
	client, err := api.NewRESTClient(api.ClientOptions{
		Host: "github.com", APIHost: "api.github.com",
		LogIgnoreEnv: true, Timeout: 10 * time.Minute,
		Transport: errorBoundTransport{transport},
		Headers:   map[string]string{"Accept": "application/vnd.github+json"},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many GitHub redirects")
			}
			host := strings.ToLower(req.URL.Hostname())
			if req.URL.Scheme != "https" || req.URL.User != nil || (req.URL.Port() != "" && req.URL.Port() != "443") || (host != "api.github.com" && host != "codeload.github.com") {
				return errors.New("GitHub redirected outside the permitted API and archive hosts")
			}
			return nil
		},
	})
	if err != nil {
		return collector{}, errors.New("initialize GitHub API client: existing github.com authentication is required")
	}
	return collector{fetch: func(ctx context.Context, endpoint string, out io.Writer) error {
		resp, err := client.RequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			failure := &apiError{cause: err}
			var httpError *api.HTTPError
			if errors.As(err, &httpError) {
				failure.status, failure.message = httpError.StatusCode, httpError.Message
			}
			return failure
		}
		defer resp.Body.Close()
		_, err = io.Copy(out, resp.Body)
		return err
	}}, nil
}

// Limit error bodies before go-gh parses them.
// Metadata and archive readers enforce their own limits on successful responses.
type errorBoundTransport struct{ http.RoundTripper }

func (t errorBoundTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.RoundTripper.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body = struct {
			io.Reader
			io.Closer
		}{io.LimitReader(resp.Body, maxErrorBodyBytes), resp.Body}
	}
	return resp, nil
}
