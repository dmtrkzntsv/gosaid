package platform

import "os/exec"

func openInDefaultApp(path string) error {
	return exec.Command("open", path).Start()
}
