package main

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

// checkADBInstalled verifica que el binario adb esté disponible en el PATH.
func checkADBInstalled() error {
	if _, err := exec.LookPath("adb"); err != nil {
		return fmt.Errorf("no se encontró 'adb' en el PATH. Instala Android Platform Tools y asegúrate de que esté en tu PATH")
	}
	return nil
}

// runADB ejecuta un comando adb (opcionalmente contra un serial específico) y devuelve stdout.
// Si stderr tiene contenido y el comando falla, el error incluye ese detalle.
func runADB(serial string, args ...string) (string, error) {
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

	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), fmt.Errorf("adb %s: %s", strings.Join(args, " "), msg)
	}
	return out.String(), nil
}

// runADBInteractive ejecuta adb conectando stdin/stdout/stderr directamente al terminal.
// Útil para comandos como 'adb shell' interactivo, aunque aquí lo usamos poco;
// se deja disponible para extensiones futuras.
func runADBInteractive(serial string, args ...string) error {
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

// listDevices devuelve todos los dispositivos reportados por 'adb devices -l'.
func listDevices() ([]Device, error) {
	out, err := runADB("", "devices", "-l")
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

// resolveDevice decide qué serial usar dado lo que el usuario pidió explícitamente (puede ser "").
// Reglas:
//   - Si se pasó un serial explícito, se usa tal cual (adb fallará después si no existe).
//   - Si solo hay un dispositivo conectado, se usa automáticamente.
//   - Si hay varios y no se especificó, se devuelve error pidiendo -s (modo CLI)
//     o se debe resolver antes con pickDeviceInteractive (modo menú).
func resolveDevice(explicitSerial string) (string, error) {
	if explicitSerial != "" {
		return explicitSerial, nil
	}
	devices, err := listDevices()
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
