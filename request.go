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
		Log("NewRequest", "请求时发生了错误: %s (Part 1)", true, err.Error())
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
func Fetch(url string, tryLogin bool) []byte {
	request := NewRequest(false, url, "")
	if request == nil {
		Log("Fetch", "请求时发生了错误", true)
		return nil
	}

	response, err := httpClient.Do(request)
	if err != nil {
		Log("Fetch", "请求时发生了错误: %s", true, err.Error())
		return nil
	}
	responseBody, err := ioutil.ReadAll(response.Body)
	defer response.Body.Close()

	if err != nil {
		Log("Fetch", "读取时发生了错误: %s", true, err.Error())
		return nil
	}

	if response.StatusCode == 403 && (!tryLogin || !Login()) {
		Log("Fetch", "请求时发生了错误: 认证失败", true)
		return nil
	}

	if response.StatusCode == 404 {
		Log("Fetch", "请求时发生了错误: 资源不存在", true)
		return nil
	}

	return responseBody
}
func Submit(url string, postdata string, tryLogin bool) []byte {
	request := NewRequest(true, url, postdata)
	if request == nil {
		Log("Submit", "请求时发生了错误", true)
		return nil
	}

	response, err := httpClient.Do(request)
	if err != nil {
		Log("Submit", "请求时发生了错误: %s (Part 2)", true, err.Error())
		return nil
	}
	responseBody, err := ioutil.ReadAll(response.Body)
	defer response.Body.Close()

	if err != nil {
		Log("Submit", "读取时发生了错误: %s", true, err.Error())
		return nil
	}

	if response.StatusCode == 403 && (!tryLogin || !Login()) {
		Log("Submit", "请求时发生了错误: 认证失败", true)
		return nil
	}

	if response.StatusCode == 404 {
		Log("Submit", "请求时发生了错误: 资源不存在", true)
		return nil
	}

	return responseBody
}
