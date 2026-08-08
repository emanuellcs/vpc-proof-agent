package netutil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HTTPGet performs an HTTP GET against url, returning the trimmed response
// body and status code. Transient transport errors are retried up to
// maxRetries times; non-2xx status codes are reported immediately without
// retries. The request honors ctx cancellation and a capped body read.
func HTTPGet(ctx context.Context, client *http.Client, url string, maxRetries int) (body string, status int, err error) {
	for attempt := 0; ; attempt++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", 0, ctxErr
		}

		body, status, err = httpGetOnce(ctx, client, url)
		if err == nil {
			if status >= 200 && status < 300 {
				return body, status, nil
			}
			return "", status, fmt.Errorf("GET %s returned status %d", url, status)
		}

		if attempt >= maxRetries {
			return "", 0, fmt.Errorf("GET %s after %d attempts: %w", url, attempt+1, err)
		}
	}
}

// maxBodyBytes caps how much of a response body is read.
const maxBodyBytes = 4096

// httpGetOnce performs a single HTTP GET.
func httpGetOnce(ctx context.Context, client *http.Client, url string) (body string, status int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", 0, fmt.Errorf("build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", 0, fmt.Errorf("read response body: %w", err)
	}
	return strings.TrimSpace(string(raw)), resp.StatusCode, nil
}
