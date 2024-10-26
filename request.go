package main

import (
	"io/ioutil"
	"net/http"
	"strings"
)

var fetchFailedCount = 0

func NewRequest(isPOST bool, url string, postdata string, clientReq bool, withHeader *map[string]string) *http.Request {
	var request *http.Request
	var err error

	if !isPOST {
		request, err = http.NewRequest("GET", url, nil)
	} else {
		request, err = http.NewRequest("POST", url, strings.NewReader(postdata))
	}

	if err != nil {
		Log("NewRequest", GetLangText("Error-NewRequest"), true, err.Error())
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

	if !setContentType && isPOST {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	if clientReq {
		if currentClientType == "Transmission" && Tr_csrfToken != "" {
			request.Header.Set("X-Transmission-Session-Id", Tr_csrfToken)
		}

		if config.UseBasicAuth && config.ClientUsername != "" {
			request.SetBasicAuth(config.ClientUsername, config.ClientPassword)
		}
	}

	return request
}
func Fetch(url string, tryLogin bool, clientReq bool, withHeader *map[string]string) (int, http.Header, []byte) {
	request := NewRequest(false, url, "", clientReq, withHeader)
	if request == nil {
		return -1, nil, nil
	}

	var response *http.Response
	var err error

	if clientReq {
		response, err = httpClient.Do(request)
	} else {
		response, err = httpClientExternal.Do(request)
	}

	if err != nil {
		if config.FetchFailedThreshold > 0 && config.ExecCommand_FetchFailed != "" {
			fetchFailedCount++
			if fetchFailedCount >= config.FetchFailedThreshold {
				fetchFailedCount = 0
				status, out, err := ExecCommand(config.ExecCommand_FetchFailed)

				if status {
					Log("Fetch", GetLangText("Success-ExecCommand"), true, out)
				} else {
					Log("Fetch", GetLangText("Failed-ExecCommand"), true, out, err)
				}
			}
		}
		Log("Fetch", GetLangText("Error-FetchResponse"), true, err.Error())
		return -2, nil, nil
	}

	responseBody, err := ioutil.ReadAll(response.Body)
	defer response.Body.Close()

	if err != nil {
		Log("Fetch", GetLangText("Error-ReadResponse"), true, err.Error())
		return -3, nil, nil
	}

	if response.StatusCode == 401 {
		Log("Fetch", GetLangText("Error-NoAuth"), true)
		return 401, response.Header, nil
	}

	if response.StatusCode == 403 {
		if tryLogin {
			Login()
		}
		Log("Fetch", GetLangText("Error-Forbidden"), true)
		return 403, response.Header, nil
	}

	if response.StatusCode == 409 {
		// 尝试获取并设置 CSRF Token.
		if currentClientType == "Transmission" {
			trCSRFToken := response.Header.Get("X-Transmission-Session-Id")
			if trCSRFToken != "" {
				Tr_SetCSRFToken(trCSRFToken)
				return 409, nil, nil
			}
		}

		if tryLogin {
			Login()
		}

		Log("Fetch", GetLangText("Error-Forbidden"), true)
		return 409, response.Header, nil
	}

	if response.StatusCode == 404 {
		Log("Fetch", GetLangText("Error-NotFound"), true)
		return 404, response.Header, nil
	}

	if response.StatusCode != 200 {
		Log("Fetch", GetLangText("Error-UnknownStatusCode"), true, response.StatusCode)
		return response.StatusCode, response.Header, nil
	}

	return response.StatusCode, response.Header, responseBody
}
func Submit(url string, postdata string, tryLogin bool, clientReq bool, withHeader *map[string]string) (int, http.Header, []byte) {
	request := NewRequest(true, url, postdata, clientReq, withHeader)
	if request == nil {
		return -1, nil, nil
	}

	var response *http.Response
	var err error

	if clientReq {
		response, err = httpClient.Do(request)
	} else {
		response, err = httpClientExternal.Do(request)
	}

	if err != nil {
		Log("Submit", GetLangText("Error-FetchResponse"), true, err.Error())
		return -2, nil, nil
	}

	responseBody, err := ioutil.ReadAll(response.Body)
	defer response.Body.Close()

	if err != nil {
		Log("Submit", GetLangText("Error-ReadResponse"), true, err.Error())
		return -3, nil, nil
	}

	if response.StatusCode == 401 {
		Log("Submit", GetLangText("Error-NoAuth"), true)
		return 401, response.Header, nil
	}

	if response.StatusCode == 403 {
		if tryLogin {
			Login()
		}
		Log("Submit", GetLangText("Error-Forbidden"), true)
		return 403, response.Header, nil
	}

	if response.StatusCode == 409 {
		// 尝试获取并设置 CSRF Token.
		if currentClientType == "Transmission" {
			trCSRFToken := response.Header.Get("X-Transmission-Session-Id")
			if trCSRFToken != "" {
				Tr_SetCSRFToken(trCSRFToken)
				return 409, response.Header, nil
			}
		}

		if tryLogin {
			Login()
		}

		Log("Fetch", GetLangText("Error-Forbidden"), true)
		return 409, response.Header, nil
	}

	if response.StatusCode == 404 {
		Log("Submit", GetLangText("Error-NotFound"), true)
		return 404, response.Header, nil
	}

	if response.StatusCode != 200 {
		Log("Submit", GetLangText("Error-UnknownStatusCode"), true, response.StatusCode)
		return response.StatusCode, response.Header, nil
	}

	return response.StatusCode, response.Header, responseBody
}
