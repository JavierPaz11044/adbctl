// Package mirror integra scrcpy para compartir la pantalla del dispositivo.
package mirror

import (
	"fmt"
	"os"
	"os/exec"
)

// CheckInstalled verifica que scrcpy esté disponible en el PATH.
func CheckInstalled() error {
	if _, err := exec.LookPath("scrcpy"); err != nil {
		return fmt.Errorf("no se encontró 'scrcpy' en el PATH. Instálalo (ver https://github.com/Genymobile/scrcpy#installation) para poder compartir pantalla")
	}
	return nil
}

// Screen lanza scrcpy contra el dispositivo indicado. extraArgs se reenvían tal
// cual a scrcpy (--record archivo.mp4, --max-size 1024, --stay-awake, …).
func Screen(serial string, extraArgs []string) error {
	if err := CheckInstalled(); err != nil {
		return err
	}

	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, extraArgs...)

	cmd := exec.Command("scrcpy", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("Iniciando scrcpy... (cierra la ventana o Ctrl+C aquí para terminar)")
	return cmd.Run()
}
