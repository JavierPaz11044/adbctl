// adbctl es una herramienta de línea de comandos para administrar dispositivos
// Android vía adb: lanzar apps, desinstalarlas, limpiar su caché/datos, y
// compartir pantalla en vivo mediante scrcpy.
//
// Uso:
//
//	adbctl                     -> menú interactivo (submenús agrupados)
//	adbctl gui                 -> interfaz gráfica (build con -tags gui)
//	adbctl devices
//	adbctl apps      [-s serial] [-filter texto] [-3] [-p | -prefix pfx]
//	adbctl launch|restart|stop|info|enable|disable|uninstall|clear <paquete|corto>
//	adbctl uninstall|clear|stop --all -match <substr> [-n] [-y]   (lote)
//	adbctl logcat    <paquete> [--all]
//	adbctl install   <archivo.apk> [-r] [-g] [-d]
//	adbctl mirror    [-s serial] [-- flags-para-scrcpy...]
//
// -p expande un nombre corto (ej. "ajustes") con el prefijo guardado en
// ~/.adbctlrc; -prefix hace lo mismo con un prefijo explícito. Ver 'adbctl help'.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		runInteractiveMenu()
		return
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	if cmd == "-h" || cmd == "--help" || cmd == "help" {
		printUsage()
		return
	}

	if err := checkADBInstalled(); err != nil {
		exitErr(err)
	}

	var err error
	switch cmd {
	case "gui":
		err = runGUI()
	case "devices":
		err = cmdDevices(args)
	case "apps":
		err = cmdApps(args)
	case "launch":
		err = cmdLaunch(args)
	case "uninstall":
		err = cmdAppAction(BatchUninstall, args)
	case "clear":
		err = cmdAppAction(BatchClear, args)
	case "stop":
		err = cmdAppAction(BatchStop, args)
	case "restart":
		err = cmdRestart(args)
	case "enable":
		err = cmdSetEnabled(true, args)
	case "disable":
		err = cmdSetEnabled(false, args)
	case "info":
		err = cmdInfo(args)
	case "install":
		err = cmdInstall(args)
	case "logcat":
		err = cmdLogcat(args)
	case "mirror":
		err = cmdMirror(args)
	default:
		fmt.Fprintf(os.Stderr, "Comando desconocido: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		exitErr(err)
	}
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
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

func cmdDevices(args []string) error {
	devices, err := listDevices()
	if err != nil {
		return err
	}
	for _, d := range devices {
		fmt.Printf("%s\t%s\t%s\n", d.Serial, d.State, d.Model)
	}
	return nil
}

func cmdApps(args []string) error {
	fs := flag.NewFlagSet("apps", flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	filter := fs.String("filter", "", "filtrar por substring")
	thirdParty := fs.Bool("3", false, "solo apps de terceros")
	useCfgPrefix := fs.Bool("p", false, "usa el prefijo de ~/.adbctlrc como filtro por defecto")
	prefixOverride := fs.String("prefix", "", "usa este prefijo como filtro por defecto")
	fs.Parse(args)

	s, err := resolveDevice(*serial)
	if err != nil {
		return err
	}

	effFilter := *filter
	if effFilter == "" {
		p, err := prefixFromFlags(*useCfgPrefix, *prefixOverride)
		if err != nil {
			return err
		}
		effFilter = p
	}

	pkgs, err := listPackages(s, *thirdParty, effFilter)
	if err != nil {
		return err
	}
	for _, p := range pkgs {
		fmt.Println(p.Name)
	}
	return nil
}

func cmdLaunch(args []string) error {
	fs := flag.NewFlagSet("launch", flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	useCfgPrefix := fs.Bool("p", false, "expande el nombre corto con el prefijo de ~/.adbctlrc")
	prefixOverride := fs.String("prefix", "", "expande el nombre corto con este prefijo")

	pkg, err := requirePositional(fs, args, "launch", "<paquete>")
	if err != nil {
		return err
	}
	prefix, err := prefixFromFlags(*useCfgPrefix, *prefixOverride)
	if err != nil {
		return err
	}
	pkg = expandName(pkg, prefix)

	s, err := resolveDevice(*serial)
	if err != nil {
		return err
	}
	if err := launchApp(s, pkg); err != nil {
		return err
	}
	fmt.Println("App lanzada:", pkg)
	return nil
}

// cmdAppAction implementa uninstall/clear/stop, en modo individual (un paquete)
// o en lote (--all / -match <substr>, opcionalmente acotado por el prefijo).
func cmdAppAction(kind BatchKind, args []string) error {
	name := string(kind)
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	yes := fs.Bool("y", false, "no pedir confirmación")
	dry := fs.Bool("n", false, "dry-run: solo muestra qué se haría")
	all := fs.Bool("all", false, "modo lote sobre los paquetes que coincidan")
	match := fs.String("match", "", "modo lote: substring para elegir paquetes")
	useCfgPrefix := fs.Bool("p", false, "usa/expande con el prefijo de ~/.adbctlrc")
	prefixOverride := fs.String("prefix", "", "usa/expande con este prefijo")

	pos, err := parseArgsLoose(fs, args)
	if err != nil {
		return err
	}
	s, err := resolveDevice(*serial)
	if err != nil {
		return err
	}
	prefix, err := prefixFromFlags(*useCfgPrefix, *prefixOverride)
	if err != nil {
		return err
	}

	// ---------- modo lote ----------
	if *all || *match != "" {
		sel := *match
		if sel == "" {
			sel = prefix
		}
		pkgs, err := selectBatchPackages(s, sel, false)
		if err != nil {
			return err
		}
		if len(pkgs) == 0 {
			fmt.Printf("Ningún paquete coincide con %q.\n", sel)
			return nil
		}
		fmt.Printf("%s %d paquete(s) en %s:\n", kind.verb(), len(pkgs), s)
		for _, p := range pkgs {
			fmt.Println("  -", p)
		}
		if *dry {
			fmt.Println("\n(dry-run: no se hizo nada)")
			return nil
		}
		if !*yes && !confirm(fmt.Sprintf("\n¿Continuar con %s %d paquete(s)?", strings.ToLower(kind.verb()), len(pkgs))) {
			fmt.Println("Cancelado.")
			return nil
		}
		okN, failN, summary := summarizeBatch(runBatch(s, kind, pkgs))
		fmt.Print(summary)
		fmt.Printf("Hecho: %d ok, %d con error.\n", okN, failN)
		if failN > 0 {
			return fmt.Errorf("%d de %d fallaron", failN, len(pkgs))
		}
		return nil
	}

	// ---------- modo individual ----------
	if len(pos) < 1 {
		return fmt.Errorf("uso: adbctl %s <paquete|corto> [-s serial]   (o --all -match <substr>)", name)
	}
	pkg := expandName(pos[0], prefix)
	if *dry {
		fmt.Printf("(dry-run) %s %s\n", kind.verb(), pkg)
		return nil
	}
	if !*yes && kind != BatchStop {
		msg := fmt.Sprintf("¿%s %s en %s?", kind.verb(), pkg, s)
		if kind == BatchClear {
			msg = fmt.Sprintf("¿Borrar datos y caché de %s? Esto es irreversible", pkg)
		}
		if !confirm(msg) {
			fmt.Println("Cancelado.")
			return nil
		}
	}
	if err := kind.apply(s, pkg); err != nil {
		return err
	}
	fmt.Printf("%s: %s\n", pastVerb(kind), pkg)
	return nil
}

func pastVerb(k BatchKind) string {
	switch k {
	case BatchUninstall:
		return "Desinstalado"
	case BatchClear:
		return "Datos y caché limpiados"
	case BatchStop:
		return "Detenido"
	default:
		return string(k)
	}
}

func cmdRestart(args []string) error {
	fs := flag.NewFlagSet("restart", flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	useCfgPrefix := fs.Bool("p", false, "expande el nombre corto con el prefijo de ~/.adbctlrc")
	prefixOverride := fs.String("prefix", "", "expande el nombre corto con este prefijo")
	pkg, err := requirePositional(fs, args, "restart", "<paquete>")
	if err != nil {
		return err
	}
	prefix, err := prefixFromFlags(*useCfgPrefix, *prefixOverride)
	if err != nil {
		return err
	}
	s, err := resolveDevice(*serial)
	if err != nil {
		return err
	}
	pkg = expandName(pkg, prefix)
	if err := restartApp(s, pkg); err != nil {
		return err
	}
	fmt.Println("Reiniciada:", pkg)
	return nil
}

func cmdSetEnabled(enabled bool, args []string) error {
	name := "disable"
	if enabled {
		name = "enable"
	}
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	useCfgPrefix := fs.Bool("p", false, "expande el nombre corto con el prefijo de ~/.adbctlrc")
	prefixOverride := fs.String("prefix", "", "expande el nombre corto con este prefijo")
	pkg, err := requirePositional(fs, args, name, "<paquete>")
	if err != nil {
		return err
	}
	prefix, err := prefixFromFlags(*useCfgPrefix, *prefixOverride)
	if err != nil {
		return err
	}
	s, err := resolveDevice(*serial)
	if err != nil {
		return err
	}
	pkg = expandName(pkg, prefix)
	if err := setEnabled(s, pkg, enabled); err != nil {
		return err
	}
	fmt.Printf("%s: %s\n", pick(enabled, "Habilitada", "Deshabilitada"), pkg)
	return nil
}

func cmdInfo(args []string) error {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	useCfgPrefix := fs.Bool("p", false, "expande el nombre corto con el prefijo de ~/.adbctlrc")
	prefixOverride := fs.String("prefix", "", "expande el nombre corto con este prefijo")
	pkg, err := requirePositional(fs, args, "info", "<paquete>")
	if err != nil {
		return err
	}
	prefix, err := prefixFromFlags(*useCfgPrefix, *prefixOverride)
	if err != nil {
		return err
	}
	s, err := resolveDevice(*serial)
	if err != nil {
		return err
	}
	info, err := getAppInfo(s, expandName(pkg, prefix))
	if err != nil {
		return err
	}
	fmt.Print(info.String())
	return nil
}

func cmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	r := fs.Bool("r", false, "reinstalar conservando datos")
	g := fs.Bool("g", false, "conceder todos los permisos en tiempo de ejecución")
	d := fs.Bool("d", false, "permitir bajar de versión")
	pos, err := parseArgsLoose(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return fmt.Errorf("uso: adbctl install <archivo.apk> [-r] [-g] [-d] [-s serial]")
	}
	s, err := resolveDevice(*serial)
	if err != nil {
		return err
	}
	fmt.Println("Instalando", pos[0], "…")
	out, err := installAPK(s, pos[0], InstallOpts{Reinstall: *r, GrantPerms: *g, Downgrade: *d})
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func cmdLogcat(args []string) error {
	fs := flag.NewFlagSet("logcat", flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	all := fs.Bool("all", false, "no filtrar por la app (todo el logcat)")
	raw := fs.Bool("raw", false, "línea de logcat sin reformatear")
	pidcat := fs.Bool("pidcat", false, "delegar en el binario externo 'pidcat'")
	noColor := fs.Bool("no-color", false, "sin colores")
	useCfgPrefix := fs.Bool("p", false, "expande el nombre corto con el prefijo de ~/.adbctlrc")
	prefixOverride := fs.String("prefix", "", "expande el nombre corto con este prefijo")
	pos, err := parseArgsLoose(fs, args)
	if err != nil {
		return err
	}
	s, err := resolveDevice(*serial)
	if err != nil {
		return err
	}
	prefix, err := prefixFromFlags(*useCfgPrefix, *prefixOverride)
	if err != nil {
		return err
	}
	pkg := ""
	if !*all {
		if len(pos) < 1 {
			return fmt.Errorf("uso: adbctl logcat <paquete> [-s serial]   (o 'adbctl logcat --all')")
		}
		pkg = expandName(pos[0], prefix)
	}

	if *pidcat && !*all {
		if used, err := runPidcatIfPresent(s, pkg); used {
			return err
		}
		return fmt.Errorf("no se encontró 'pidcat' en el PATH")
	}

	ls, err := startLogStream(s, pkg, LogOpts{All: *all, Raw: *raw, Color: !*noColor})
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Ctrl+C para salir")
	for line := range ls.Lines {
		fmt.Println(line)
	}
	return ls.Err()
}

func cmdMirror(args []string) error {
	fs := flag.NewFlagSet("mirror", flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	fs.Parse(args)

	s, err := resolveDevice(*serial)
	if err != nil {
		return err
	}
	// Todo lo que quede tras el parseo de flags propios se reenvía a scrcpy,
	// permitiendo cosas como: adbctl mirror -s XXXX -- --record salida.mp4
	return mirrorScreen(s, fs.Args())
}

// parseArgsLoose parsea las flags de fs aunque estén intercaladas con argumentos
// posicionales (p. ej. "launch ajustes -p"), algo que el paquete flag no soporta
// de serie porque se detiene en el primer token que no es flag. Devuelve los
// argumentos posicionales en el orden en que aparecieron.
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
// menos un argumento posicional —el nombre del paquete—, con un mensaje de error
// consistente si falta.
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
