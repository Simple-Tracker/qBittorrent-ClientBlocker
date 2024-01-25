//go:build (!windows) || (windows && (arm || arm64))
package main

func main() {
	RunConsole()
}
