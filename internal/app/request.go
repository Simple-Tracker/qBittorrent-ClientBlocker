package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	clientapi "github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
)

var fetchFailedCount = 0
var urlETagCache = make(map[string]string)
var urlLastModCache = make(map[string]string)
var requestStateMutex sync.RWMutex

var requestContext = context.Background()
var cancelRequests context.CancelFunc

func RequestContextSnapshot() context.Context {
	httpStateMutex.RLock()
	defer httpStateMutex.RUnlock()
	return requestContext
}

func WaitRequestDelay(delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-RequestContextSnapshot().Done():
		return false
	case <-timer.C:
		return true
	}
}

func HttpClientSnapshot(clientReq bool) http.Client {
	httpStateMutex.RLock()
	defer httpStateMutex.RUnlock()
	if clientReq {
		return httpClient
	}
	return httpClientExternal
}

func NewRequest(isPost bool, url string, postdata interface{}, clientReq bool, allowCache bool, withHeader *map[string]string) *http.Request {
	activeClient, clientType := CurrentClientSnapshot()
	return newRequestForClient(isPost, url, postdata, clientReq, allowCache, withHeader, activeClient, clientType)
}

func newRequestForClient(isPost bool, url string, postdata interface{}, clientReq bool, allowCache bool, withHeader *map[string]string, activeClient Client, clientType string) *http.Request {
	var request *http.Request
	var err error

	if !isPost {
		request, err = http.NewRequestWithContext(RequestContextSnapshot(), "GET", url, nil)
	} else {
		var bodyReader io.Reader
		if postdata != nil {
			switch v := postdata.(type) {
			case string:
				bodyReader = strings.NewReader(v)
			case []byte:
				bodyReader = bytes.NewReader(v)
			case io.Reader:
				bodyReader = v
			default:
				LogError("NewRequest", GetLangText("Error-NewRequest"), true, "Unsupported postdata type")
				return nil
			}
		}
		request, err = http.NewRequestWithContext(RequestContextSnapshot(), "POST", url, bodyReader)
	}

	if err != nil {
		LogError("NewRequest", GetLangText("Error-NewRequest"), true, err.Error())
		return nil
	}

	setUserAgent := false
	setContentType := false

	if withHeader != nil {
		for k, v := range *withHeader {
			switch strings.ToLower(k) {
			case "user-agent":
				setUserAgent = true
			case "content-type":
				setContentType = true
			}

			request.Header.Set(k, v)
		}
	}

	if !setUserAgent {
		request.Header.Set("User-Agent", programUserAgent)
	}

	if !setContentType && isPost {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	if clientReq {
		if session, ok := activeClient.(clientapi.SessionClient); ok && clientType == "Transmission" {
			token := session.SessionToken()
			if token != "" {
				request.Header.Set("X-Transmission-Session-Id", token)
			}
		}

		if ConfigSnapshot().UseBasicAuth && ConfigSnapshot().ClientUsername != "" {
			request.SetBasicAuth(ConfigSnapshot().ClientUsername, ConfigSnapshot().ClientPassword)
		}
	} else if !isPost && allowCache {
		requestStateMutex.RLock()
		if etag, exist := urlETagCache[url]; exist {
			request.Header.Set("If-None-Match", etag)
		}

		if lastMod, exist := urlLastModCache[url]; exist {
			request.Header.Set("If-Modified-Since", lastMod)
		}
		requestStateMutex.RUnlock()
	}

	return request
}
func Fetch(url string, tryLogin bool, clientReq bool, allowCache bool, withHeader *map[string]string) (int, http.Header, []byte) {
	activeClient, clientType := CurrentClientSnapshot()
	request := newRequestForClient(false, url, nil, clientReq, allowCache, withHeader, activeClient, clientType)
	if request == nil {
		return -1, nil, nil
	}

	var response *http.Response
	var err error

	client := HttpClientSnapshot(clientReq)
	response, err = client.Do(request)

	if err != nil {
		if ConfigSnapshot().FetchFailedThreshold > 0 && ConfigSnapshot().ExecCommand_FetchFailed != "" {
			requestStateMutex.Lock()
			fetchFailedCount++
			if fetchFailedCount >= ConfigSnapshot().FetchFailedThreshold {
				fetchFailedCount = 0
				requestStateMutex.Unlock()
				status, out, err := ExecCommand(ConfigSnapshot().ExecCommand_FetchFailed)

				if status {
					Log("Fetch", GetLangText("Success-ExecCommand"), true, out)
				} else {
					LogError("Fetch", GetLangText("Failed-ExecCommand"), true, out, err)
				}
			} else {
				requestStateMutex.Unlock()
			}
		}
		LogError("Fetch", GetLangText("Error-FetchResponse"), true, err.Error())
		return -2, nil, nil
	}

	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)

	if err != nil {
		LogError("Fetch", GetLangText("Error-ReadResponse"), true, err.Error())
		return -3, nil, nil
	}

	if response.StatusCode == 204 {
		Log("Debug-Fetch", GetLangText("Debug-Request_NoContent"), false, url)
		return 204, response.Header, nil
	}

	if response.StatusCode == 401 {
		LogError("Fetch", GetLangText("Error-NoAuth"), true)
		return 401, response.Header, nil
	}

	if response.StatusCode == 403 {
		if tryLogin {
			Login()
		}
		LogError("Fetch", GetLangText("Error-Forbidden"), true)
		return 403, response.Header, nil
	}

	if response.StatusCode == 409 {
		// 尝试从响应头获取并设置 CSRF 令牌.
		if session, ok := activeClient.(clientapi.SessionClient); ok && clientType == "Transmission" {
			trCSRFToken := response.Header.Get("X-Transmission-Session-Id")
			if trCSRFToken != "" {
				session.SetSessionToken(trCSRFToken)
				return 409, nil, nil
			}
		}

		if tryLogin {
			Login()
		}

		LogError("Fetch", GetLangText("Error-Forbidden"), true)
		return 409, response.Header, nil
	}

	if response.StatusCode == 404 {
		LogError("Fetch", GetLangText("Error-NotFound"), true)
		return 404, response.Header, nil
	}

	if response.StatusCode == 304 && (allowCache || request.Header.Get("If-None-Match") != "" || request.Header.Get("If-Modified-Since") != "") {
		Log("Debug-Fetch", GetLangText("Debug-Request_NoChange"), false, url)
		return 304, response.Header, nil
	}

	if response.StatusCode != 200 {
		LogError("Fetch", GetLangText("Error-UnknownStatusCode"), true, response.StatusCode)
		return response.StatusCode, response.Header, nil
	}

	if allowCache {
		etag := response.Header.Get("ETag")
		lastMod := response.Header.Get("Last-Modified")

		requestStateMutex.Lock()
		if etag != "" {
			urlETagCache[url] = etag
		}

		if lastMod != "" {
			urlLastModCache[url] = lastMod
		}
		requestStateMutex.Unlock()
	}

	return response.StatusCode, response.Header, responseBody
}

func Submit(url string, postdata interface{}, tryLogin bool, clientReq bool, withHeader *map[string]string) (int, http.Header, []byte) {
	activeClient, clientType := CurrentClientSnapshot()
	request := newRequestForClient(true, url, postdata, clientReq, false, withHeader, activeClient, clientType)
	if request == nil {
		return -1, nil, nil
	}

	var response *http.Response
	var err error

	client := HttpClientSnapshot(clientReq)
	response, err = client.Do(request)

	if err != nil {
		LogError("Submit", GetLangText("Error-FetchResponse"), true, err.Error())
		return -2, nil, nil
	}

	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)

	if err != nil {
		LogError("Submit", GetLangText("Error-ReadResponse"), true, err.Error())
		return -3, nil, nil
	}

	if response.StatusCode == 204 {
		Log("Fetch", GetLangText("Debug-Request_NoContent"), false, url)
		return 204, response.Header, nil
	}

	if response.StatusCode == 401 {
		LogError("Submit", GetLangText("Error-NoAuth"), true)
		return 401, response.Header, nil
	}

	if response.StatusCode == 403 {
		if tryLogin {
			Login()
		}
		LogError("Submit", GetLangText("Error-Forbidden"), true)
		return 403, response.Header, nil
	}

	if response.StatusCode == 409 {
		// 尝试从响应头获取并设置 CSRF 令牌.
		if session, ok := activeClient.(clientapi.SessionClient); ok && clientType == "Transmission" {
			trCSRFToken := response.Header.Get("X-Transmission-Session-Id")
			if trCSRFToken != "" {
				session.SetSessionToken(trCSRFToken)
				return 409, response.Header, nil
			}
		}

		if tryLogin {
			Login()
		}

		LogError("Fetch", GetLangText("Error-Forbidden"), true)
		return 409, response.Header, nil
	}

	if response.StatusCode == 404 {
		LogError("Submit", GetLangText("Error-NotFound"), true)
		return 404, response.Header, nil
	}

	if response.StatusCode != 200 {
		LogError("Submit", GetLangText("Error-UnknownStatusCode"), true, response.StatusCode)
		return response.StatusCode, response.Header, nil
	}

	return response.StatusCode, response.Header, responseBody
}
