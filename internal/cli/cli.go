// Package cli parsea los argumentos de línea de comandos y despacha a cada
// feature. Sin argumentos abre el menú interactivo; con "gui", la interfaz
// gráfica.
package cli

import (
	"flag"
	"fmt"
	"os"

	"adbctl/internal/adb"
	"adbctl/internal/batch"
	"adbctl/internal/config"
	"adbctl/internal/gui"
	"adbctl/internal/menu"
)

// Main es el punto de entrada real; devuelve el código de salida del proceso.
func Main() int {
	if len(os.Args) < 2 {
		menu.Run()
		return 0
	}

	cmd, args := os.Args[1], os.Args[2:]

	switch cmd {
	case "-h", "--help", "help":
		printUsage()
		return 0
	case "gui":
		if err := gui.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}
		return 0
	}

	if err := adb.CheckInstalled(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}

	if err := dispatch(cmd, args); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

func dispatch(cmd string, args []string) error {
	switch cmd {
	case "devices":
		return cmdDevices(args)
	case "apps":
		return cmdApps(args)
	case "launch":
		return cmdLaunch(args)
	case "uninstall":
		return cmdAppAction(batch.Uninstall, args)
	case "clear":
		return cmdAppAction(batch.Clear, args)
	case "stop":
		return cmdAppAction(batch.Stop, args)
	case "restart":
		return cmdRestart(args)
	case "enable":
		return cmdSetEnabled(true, args)
	case "disable":
		return cmdSetEnabled(false, args)
	case "info":
		return cmdInfo(args)
	case "install":
		return cmdInstall(args)
	case "logcat":
		return cmdLogcat(args)
	case "mirror":
		return cmdMirror(args)
	default:
		fmt.Fprintf(os.Stderr, "Comando desconocido: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
		return nil
	}
}

// prefixFromFlags decide qué prefijo aplicar a partir de -prefix (explícito) y
// -p (usa el de ~/.adbctlrc). "" si el usuario no pidió ninguno: en CLI nunca se
// adivina.
func prefixFromFlags(useConfigPrefix bool, override string) (string, error) {
	if override != "" {
		return trimDot(override), nil
	}
	if useConfigPrefix {
		p := config.Load().PackagePrefix
		if p == "" {
			return "", fmt.Errorf("-p requiere un prefijo configurado en ~/.adbctlrc (defínelo desde el menú, o usa -prefix)")
		}
		return p, nil
	}
	return "", nil
}

func trimDot(s string) string {
	for len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
}

// parseArgsLoose parsea las flags de fs aunque estén intercaladas con
// argumentos posicionales (p. ej. "launch ajustes -p"); flag se detiene en el
// primer token que no es flag.
func parseArgsLoose(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// requirePositional parsea fs (tolerando flags tras el posicional) y exige al
// menos un argumento posicional —el nombre del paquete—.
func requirePositional(fs *flag.FlagSet, args []string, cmdName, argName string) (string, error) {
	pos, err := parseArgsLoose(fs, args)
	if err != nil {
		return "", err
	}
	if len(pos) < 1 {
		return "", fmt.Errorf("uso: adbctl %s %s [-s serial]", cmdName, argName)
	}
	return pos[0], nil
}

func printUsage() {
	fmt.Println(`adbctl - administración rápida de dispositivos Android vía adb

Uso:
  adbctl                                    menú interactivo
  adbctl gui                                interfaz gráfica (binario con -tags gui)
  adbctl devices                            listar dispositivos conectados
  adbctl apps      [-s serial] [-filter t] [-3] [-p|-prefix pfx]  listar paquetes

  Sobre una app (aceptan <corto> con -p / -prefix):
  adbctl launch    <paquete>               lanzar
  adbctl restart   <paquete>               forzar detención y relanzar
  adbctl stop      <paquete>               forzar detención (am force-stop)
  adbctl info      <paquete>               versión, SDK, ruta, tamaño, permisos…
  adbctl enable    <paquete>               volver a habilitar
  adbctl disable   <paquete>               deshabilitar sin desinstalar
  adbctl uninstall <paquete> [-y]          desinstalar
  adbctl clear     <paquete> [-y]          limpiar datos/caché (pm clear)
  adbctl logcat    <paquete> [--all] [--raw] [--pidcat]   seguir el log (Ctrl+C sale)

  En lote (uninstall / clear / stop):
  adbctl uninstall --all -match <substr> [-n] [-y]
  adbctl clear     --all -prefix <pfx>    [-n] [-y]

  Otros:
  adbctl install   <archivo.apk> [-r] [-g] [-d]   instalar un APK local
  adbctl mirror    [-s serial] [-- flags]         compartir pantalla con scrcpy

Flags comunes:
  -s serial     serial del dispositivo (ver 'adbctl devices')
  -y            no pedir confirmación en acciones destructivas
  -n            dry-run: mostrar qué se haría sin ejecutarlo (uninstall/clear/stop)
  --all         modo lote; con -match <substr> o -prefix <pfx> selecciona paquetes
  -filter texto filtrar paquetes por substring (comando 'apps')
  -3            solo apps de terceros, no de sistema (comando 'apps')
  -p            expande un nombre corto (o filtra) con el prefijo de ~/.adbctlrc
  -prefix pfx   igual que -p pero con un prefijo explícito
  install:  -r reinstalar  -g conceder permisos  -d permitir downgrade
  logcat:   --raw línea original  --no-color sin ANSI  --pidcat usar 'pidcat' externo
            (sigue a la app por PID; no hace falta que esté corriendo, aguanta reinicios)

El prefijo por defecto se configura desde el menú interactivo y se guarda en
~/.adbctlrc como 'prefix=com.perfect'.`)
}
