//go:build !windows
package main

func main() {
	if PrepareEnv() {
		RunConsole()
	}
}
