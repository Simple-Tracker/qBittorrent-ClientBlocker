package main

import (
	"strings"
	"io/ioutil"
)

func Fetch(url string, tryLogin bool) []byte {
	response, err := httpClient.Get(url)
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
		Log("Fetch", "请求时发生了错误: 认证失败 %s", true, responseBody)
		return nil
	}

	if response.StatusCode == 404 {
		Log("Fetch", "请求时发生了错误: 资源不存在", true)
		return nil
	}

	return responseBody
}
func Submit(url string, postdata string, tryLogin bool) []byte {
	response, err := httpClient.Post(url, "application/x-www-form-urlencoded", strings.NewReader(postdata))
	if err != nil {
		Log("Submit", "请求时发生了错误: %s", true, err.Error())
		return nil
	}
	responseBody, err := ioutil.ReadAll(response.Body)
	defer response.Body.Close()

	if err != nil {
		Log("Submit", "读取时发生了错误: %s", true, err.Error())
		return nil
	}

	if response.StatusCode == 403 && (!tryLogin || !Login()) {
		Log("Submit", "请求时发生了错误: 认证失败 %s", true, responseBody)
		return nil
	}

	if response.StatusCode == 404 {
		Log("Submit", "请求时发生了错误: 资源不存在", true)
		return nil
	}

	return responseBody
}
