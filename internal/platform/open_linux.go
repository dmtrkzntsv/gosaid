package platform

import "os/exec"

func openInDefaultApp(path string) error {
	return exec.Command("xdg-open", path).Start()
}
