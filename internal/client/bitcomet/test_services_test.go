package bitcomet

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
)

func newTestClient(t *testing.T, handler http.Handler) (*Client, *client.Settings) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	settings := &client.Settings{URL: server.URL}
	return newHTTPTestClient(t, settings, server.Client()), settings
}

func newHTTPTestClient(t *testing.T, settings *client.Settings, httpClient *http.Client) *Client {
	request := func(method, url string, body any, headers *map[string]string) (int, http.Header, []byte) {
		var reader io.Reader
		switch value := body.(type) {
		case nil:
		case string:
			reader = strings.NewReader(value)
		case []byte:
			reader = bytes.NewReader(value)
		case io.Reader:
			reader = value
		default:
			t.Fatalf("unexpected body type %T", body)
		}
		req, err := http.NewRequest(method, url, reader)
		if err != nil {
			t.Fatal(err)
		}
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if headers != nil {
			for key, value := range *headers {
				req.Header.Set(key, value)
			}
		}
		response, err := httpClient.Do(req)
		if err != nil {
			return -2, nil, nil
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			return response.StatusCode, response.Header, nil
		}
		return response.StatusCode, response.Header, data
	}
	services := client.Services{
		Snapshot:    func() client.Settings { return *settings },
		SetEndpoint: func(url, username string) { settings.URL, settings.Username = url, username },
		Fetch: func(url string, _, _, _ bool, headers *map[string]string) (int, http.Header, []byte) {
			return request(http.MethodGet, url, nil, headers)
		},
		Submit: func(url string, body any, _, _ bool, headers *map[string]string) (int, http.Header, []byte) {
			return request(http.MethodPost, url, body, headers)
		},
	}
	return New(services)
}

type coverageRoundTripFunc func(*http.Request) (*http.Response, error)

func (f coverageRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func CoverageResponse(status int, body string, headers map[string]string) *http.Response {
	header := make(http.Header)
	for key, value := range headers {
		header.Set(key, value)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}
