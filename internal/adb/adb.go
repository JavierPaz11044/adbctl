// Package adb envuelve la ejecución del binario `adb` y el listado/selección de
// dispositivos. Es la base sobre la que se apoyan el resto de features.
package adb

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrNoDevices se devuelve cuando no hay ningún dispositivo/emulador conectado.
var ErrNoDevices = errors.New("no hay dispositivos conectados (revisa 'adb devices' y que el depurador USB esté autorizado)")

// Device representa un dispositivo Android visible para adb.
type Device struct {
	Serial string
	State  string // device, offline, unauthorized...
	Model  string
}

// CheckInstalled verifica que el binario adb esté disponible en el PATH.
func CheckInstalled() error {
	if _, err := exec.LookPath("adb"); err != nil {
		return fmt.Errorf("no se encontró 'adb' en el PATH. Instala Android Platform Tools y asegúrate de que esté en tu PATH")
	}
	return nil
}

// Run ejecuta un comando adb (opcionalmente contra un serial específico) y
// devuelve stdout. Si stderr tiene contenido y el comando falla, el error
// incluye ese detalle.
func Run(serial string, args ...string) (string, error) {
	fullArgs := []string{}
	if serial != "" {
		fullArgs = append(fullArgs, "-s", serial)
	}
	fullArgs = append(fullArgs, args...)

	cmd := exec.Command("adb", fullArgs...)
	var out strings.Builder
	var errOut strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), fmt.Errorf("adb %s: %s", strings.Join(args, " "), msg)
	}
	return out.String(), nil
}

// RunInteractive ejecuta adb conectando stdin/stdout/stderr directamente al
// terminal (para comandos interactivos como 'adb shell').
func RunInteractive(serial string, args ...string) error {
	fullArgs := []string{}
	if serial != "" {
		fullArgs = append(fullArgs, "-s", serial)
	}
	fullArgs = append(fullArgs, args...)

	cmd := exec.Command("adb", fullArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// List devuelve todos los dispositivos reportados por 'adb devices -l'.
func List() ([]Device, error) {
	out, err := Run("", "devices", "-l")
	if err != nil {
		return nil, err
	}

	var devices []Device
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "List of devices") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		d := Device{Serial: fields[0], State: fields[1]}
		for _, f := range fields[2:] {
			if strings.HasPrefix(f, "model:") {
				d.Model = strings.TrimPrefix(f, "model:")
			}
		}
		devices = append(devices, d)
	}

	if len(devices) == 0 {
		return nil, ErrNoDevices
	}
	return devices, nil
}

// Resolve decide qué serial usar dado lo que el usuario pidió explícitamente
// (puede ser ""):
//   - serial explícito -> se usa tal cual (adb fallará después si no existe);
//   - un solo dispositivo conectado -> se usa automáticamente;
//   - varios y ninguno indicado -> error pidiendo -s <serial>.
func Resolve(explicitSerial string) (string, error) {
	if explicitSerial != "" {
		return explicitSerial, nil
	}
	devices, err := List()
	if err != nil {
		return "", err
	}
	if len(devices) == 1 {
		return devices[0].Serial, nil
	}
	var b strings.Builder
	b.WriteString("hay varios dispositivos conectados, especifica uno con -s <serial>:\n")
	for _, d := range devices {
		fmt.Fprintf(&b, "  - %s (%s) %s\n", d.Serial, d.State, d.Model)
	}
	return "", errors.New(b.String())
}

// Connected indica si el serial aparece ahora mismo en `adb devices`. Sirve para
// dar un error claro cuando un dispositivo inalámbrico cambia de IP o se duerme
// y el serial guardado deja de existir.
func Connected(serial string) bool {
	if serial == "" {
		return false
	}
	devs, err := List()
	if err != nil {
		return false
	}
	for _, d := range devs {
		if d.Serial == serial {
			return true
		}
	}
	return false
}
