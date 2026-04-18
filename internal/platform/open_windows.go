package platform

import "os/exec"

func openInDefaultApp(path string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
}
