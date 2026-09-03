package logcat

import (
	"os"
	"os/exec"
)

// RunPidcatIfPresent delega el seguimiento del log en el binario `pidcat` si
// está en el PATH, conectándolo a la terminal. Devuelve (usado, err); usado es
// false cuando no hay pidcat.
func RunPidcatIfPresent(serial, pkg string) (bool, error) {
	path, err := exec.LookPath("pidcat")
	if err != nil {
		return false, nil
	}
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, pkg)
	cmd := exec.Command(path, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return true, cmd.Run()
}
