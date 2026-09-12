// Copyright © 2026 Chunyou Peng <chunyoupeng@gmail.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package libgen

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"
	MaxBodyReadLimit = 10 * 1024 * 1024 // 10MB
)

var defaultTransport = &http.Transport{
	Proxy:               http.ProxyFromEnvironment,
	MaxIdleConns:        32,
	MaxIdleConnsPerHost: 8,
	MaxConnsPerHost:     8,
	IdleConnTimeout:     90 * time.Second,
	TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
}

var defaultHTTPClient = &http.Client{
	Timeout:   HTTPClientTimeout,
	Transport: defaultTransport,
}

var downloadHTTPClient = &http.Client{
	Transport: defaultTransport,
}

// endpoint creates a cloned url.URL with the specified path and query,
// leaving the base URL untouched to prevent data races and query contamination.
func endpoint(base url.URL, path string, q url.Values) url.URL {
	u := base
	u.Path = path
	u.RawPath = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	if q != nil {
		u.RawQuery = q.Encode()
	} else {
		u.RawQuery = ""
	}
	return u
}

// getBody executes an HTTP GET request with standard browser headers and returns the response body.
func getBody(targetURL string) ([]byte, error) {
	return getBodyWithReferer(context.Background(), targetURL, "")
}

// getBodyWithReferer executes an HTTP GET request with standard browser headers and
// the provided referer. If referer is empty, it derives a same-origin index.php referer.
func getBodyWithReferer(ctx context.Context, targetURL string, referer string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,application/json,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	if referer == "" {
		if u, err := url.Parse(targetURL); err == nil && u.Scheme != "" && u.Host != "" {
			referer = fmt.Sprintf("%s://%s/index.php", u.Scheme, u.Host)
		}
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mirror %s returned HTTP %d", targetURL, resp.StatusCode)
	}

	lr := io.LimitReader(resp.Body, MaxBodyReadLimit)
	b, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}

	return b, nil
}
