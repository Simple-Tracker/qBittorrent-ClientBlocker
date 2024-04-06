package main

import (
	"net/http"
	"strings"
	"io/ioutil"
)

func NewRequest(isPOST bool, url string, postdata string) *http.Request {
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

	request.Header.Set("User-Agent", programName + "/" + programVersion)

	if isPOST {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	if config.UseBasicAuth && config.QBUsername != "" {
		request.SetBasicAuth(config.QBUsername, config.QBPassword)
	}

	return request
}
func Fetch(url string, tryLogin bool, withCookie bool) []byte {
	request := NewRequest(false, url, "")
	if request == nil {
		return nil
	}

	var response *http.Response
	var err error

	if withCookie {
		response, err = httpClient.Do(request)
	} else {
		response, err = httpClientWithoutCookie.Do(request)
	}

	if err != nil {
		Log("Fetch", GetLangText("Error-FetchResponse"), true, err.Error())
		return nil
	}

	responseBody, err := ioutil.ReadAll(response.Body)
	defer response.Body.Close()

	if err != nil {
		Log("Fetch", GetLangText("Error-ReadResponse"), true, err.Error())
		return nil
	}

	if response.StatusCode == 403 && (!tryLogin || !Login()) {
		Log("Fetch", GetLangText("Error-Forbidden"), true)
		return nil
	}

	if response.StatusCode == 404 {
		Log("Fetch", GetLangText("Error-NotFound"), true)
		return nil
	}

	return responseBody
}
func Submit(url string, postdata string, tryLogin bool, withCookie bool) []byte {
	request := NewRequest(true, url, postdata)
	if request == nil {
		return nil
	}

	var response *http.Response
	var err error

	if withCookie {
		response, err = httpClient.Do(request)
	} else {
		response, err = httpClientWithoutCookie.Do(request)
	}

	if err != nil {
		Log("Submit", GetLangText("Error-FetchResponse"), true, err.Error())
		return nil
	}
	responseBody, err := ioutil.ReadAll(response.Body)
	defer response.Body.Close()

	if err != nil {
		Log("Submit", GetLangText("Error-ReadResponse"), true, err.Error())
		return nil
	}

	if response.StatusCode == 403 && (!tryLogin || !Login()) {
		Log("Submit", GetLangText("Error-Forbidden"), true)
		return nil
	}

	if response.StatusCode == 404 {
		Log("Submit", GetLangText("Error-NotFound"), true)
		return nil
	}

	return responseBody
}
