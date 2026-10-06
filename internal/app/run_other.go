//go:build !windows

package app

func Platform_Stop() {
}
func Run(version string) {
	SetVersion(version)
	defer RecoverAndStop("main", true)

	if PrepareEnv() {
		RunConsole()
	}
}
