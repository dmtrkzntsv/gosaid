package platform

// OpenInDefaultApp asks the OS to open the given path with the user's
// configured default application. Returns after spawning the process.
// Platform-specific implementation lives in open_{darwin,linux,windows}.go.
func OpenInDefaultApp(path string) error {
	return openInDefaultApp(path)
}
