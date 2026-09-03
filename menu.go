package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var stdinReader = bufio.NewReader(os.Stdin)

func prompt(label string) string {
	fmt.Print(label)
	line, _ := stdinReader.ReadString('\n')
	return strings.TrimSpace(line)
}

func confirm(label string) bool {
	answer := strings.ToLower(prompt(label + " [s/N]: "))
	return answer == "s" || answer == "si" || answer == "sí" || answer == "y" || answer == "yes"
}

// pickDeviceInteractive resuelve el dispositivo a usar en modo menú, preguntando
// al usuario si hay más de uno conectado.
func pickDeviceInteractive() (string, error) {
	devices, err := listDevices()
	if err != nil {
		return "", err
	}
	if len(devices) == 1 {
		return devices[0].Serial, nil
	}

	fmt.Println("Dispositivos conectados:")
	for i, d := range devices {
		fmt.Printf("  %d) %s  (%s)  %s\n", i+1, d.Serial, d.State, d.Model)
	}
	for {
		choice := prompt("Elige un dispositivo (número): ")
		idx, err := strconv.Atoi(choice)
		if err == nil && idx >= 1 && idx <= len(devices) {
			return devices[idx-1].Serial, nil
		}
		fmt.Println("Opción inválida, intenta de nuevo.")
	}
}

// selectDeviceAtStartup lista los dispositivos al arrancar el menú interactivo y
// deja elegir uno de una vez. Si sólo hay uno conectado, lo usa. Si ~/.adbctlrc
// recuerda un serial que sigue conectado, lo ofrece por defecto (Enter para
// mantenerlo). Si no hay dispositivos, avisa y devuelve "" (se puede reintentar
// luego desde la opción 2).
func selectDeviceAtStartup(cfg Config) string {
	devices, err := listDevices()
	if err != nil {
		fmt.Println("Aviso:", err)
		return ""
	}
	if len(devices) == 1 {
		d := devices[0]
		fmt.Printf("Dispositivo: %s  (%s)  %s\n", d.Serial, d.State, d.Model)
		return d.Serial
	}

	fmt.Println("Dispositivos conectados:")
	defaultIdx := -1
	for i, d := range devices {
		marker := ""
		if d.Serial == cfg.Device {
			marker = "  <- usado la última vez"
			defaultIdx = i
		}
		fmt.Printf("  %d) %s  (%s)  %s%s\n", i+1, d.Serial, d.State, d.Model, marker)
	}
	for {
		label := "Elige un dispositivo (número): "
		if defaultIdx >= 0 {
			label = fmt.Sprintf("Elige un dispositivo (número) [Enter = %d]: ", defaultIdx+1)
		}
		choice := prompt(label)
		if choice == "" && defaultIdx >= 0 {
			return devices[defaultIdx].Serial
		}
		idx, err := strconv.Atoi(choice)
		if err == nil && idx >= 1 && idx <= len(devices) {
			return devices[idx-1].Serial
		}
		fmt.Println("Opción inválida, intenta de nuevo.")
	}
}

// promptFilter pide un filtro de paquete respetando el prefijo configurado:
// Enter usa el prefijo (si hay uno), '*' fuerza a ver todos los paquetes, y
// cualquier otro texto se usa como substring.
func promptFilter(prefix string) string {
	label := "Filtro de nombre de paquete (Enter para ver todos): "
	if prefix != "" {
		label = fmt.Sprintf("Filtro (Enter = %q · '*' = todos): ", prefix)
	}
	f := prompt(label)
	switch {
	case f == "" && prefix != "":
		return prefix
	case f == "*":
		return ""
	default:
		return f
	}
}

// pickPackageInteractive lista paquetes y deja elegir uno. Con prefijo
// configurado, se usa como filtro por defecto y los nombres se muestran en corto.
func pickPackageInteractive(serial, prefix string) (string, error) {
	filter := promptFilter(prefix)
	pkgs, err := listPackages(serial, false, filter)
	if err != nil {
		return "", err
	}
	if len(pkgs) == 0 {
		return "", fmt.Errorf("no se encontraron paquetes que coincidan con %q", filter)
	}
	for i, p := range pkgs {
		if s := shortName(p.Name, prefix); s != p.Name {
			fmt.Printf("  %d) %s  (%s)\n", i+1, s, p.Name)
		} else {
			fmt.Printf("  %d) %s\n", i+1, p.Name)
		}
	}
	for {
		choice := prompt("Elige un paquete (número): ")
		idx, err := strconv.Atoi(choice)
		if err == nil && idx >= 1 && idx <= len(pkgs) {
			return pkgs[idx-1].Name, nil
		}
		fmt.Println("Opción inválida, intenta de nuevo.")
	}
}

// configurePrefixInteractive fija o borra el prefijo de paquetes guardado en
// ~/.adbctlrc. Con un dispositivo seleccionado, ofrece sugerencias a partir de
// las apps de terceros instaladas.
func configurePrefixInteractive(cfg *Config, serial string) {
	if cfg.PackagePrefix != "" {
		fmt.Printf("Prefijo actual: %s\n", cfg.PackagePrefix)
	} else {
		fmt.Println("No hay prefijo configurado.")
	}

	var sugs []PrefixSuggestion
	if serial != "" {
		if s, err := suggestPrefixes(serial); err == nil && len(s) > 0 {
			sugs = s
			fmt.Println("Sugerencias (por apps de terceros instaladas):")
			for i, ps := range sugs {
				fmt.Printf("  %d) %s  (%d apps)\n", i+1, ps.Prefix, ps.Count)
			}
			fmt.Println("Elige un número, teclea el prefijo a mano, o Enter para borrarlo.")
		}
	}

	in := prompt("Prefijo: ")
	if idx, err := strconv.Atoi(in); err == nil && idx >= 1 && idx <= len(sugs) {
		cfg.PackagePrefix = sugs[idx-1].Prefix
	} else {
		cfg.PackagePrefix = strings.TrimSuffix(strings.TrimSpace(in), ".")
	}

	if err := cfg.save(); err != nil {
		fmt.Println("Error guardando ~/.adbctlrc:", err)
		return
	}
	if cfg.PackagePrefix == "" {
		fmt.Println("Prefijo borrado.")
	} else {
		fmt.Println("Prefijo guardado:", cfg.PackagePrefix)
	}
}

// runInteractiveMenu es el bucle principal del modo sin argumentos. El menú
// está agrupado en submenús (Apps / Lote / Dispositivo) para no saturar.
func runInteractiveMenu() {
	if err := checkADBInstalled(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	cfg := loadConfig()
	serial := selectDeviceAtStartup(cfg)
	if serial != "" && serial != cfg.Device {
		cfg.Device = serial
		_ = cfg.save()
	}

	for {
		fmt.Println()
		fmt.Println("==== adbctl ====")
		if serial != "" {
			fmt.Printf("Dispositivo: %s", serial)
		} else {
			fmt.Print("Dispositivo: (ninguno)")
		}
		if cfg.PackagePrefix != "" {
			fmt.Printf("   ·   Prefijo: %s", cfg.PackagePrefix)
		}
		fmt.Println()
		fmt.Println("1) Apps ▸")
		fmt.Println("2) Lote ▸")
		fmt.Println("3) Dispositivo ▸")
		fmt.Println("4) Interfaz gráfica (GUI)")
		fmt.Println("0) Salir")

		switch prompt("> ") {
		case "1":
			s, err := ensureSerial(&serial, &cfg)
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			appsSubmenu(s, &cfg)
		case "2":
			s, err := ensureSerial(&serial, &cfg)
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			batchSubmenu(s, &cfg)
		case "3":
			deviceSubmenu(&serial, &cfg)
		case "4":
			if err := runGUI(); err != nil {
				fmt.Println("Error:", err)
				continue
			}
			cfg = loadConfig() // la GUI pudo cambiar prefijo/dispositivo
		case "0", "q", "Q":
			fmt.Println("Hasta luego.")
			return
		default:
			fmt.Println("Opción inválida.")
		}
	}
}

// ensurePkg reutiliza la app ya seleccionada en el submenú o pide elegir una.
func ensurePkg(pkg *string, serial, prefix string) (string, error) {
	if *pkg != "" {
		return *pkg, nil
	}
	p, err := pickPackageInteractive(serial, prefix)
	if err != nil {
		return "", err
	}
	*pkg = p
	return p, nil
}

// appsSubmenu agrupa todas las acciones sobre una app. Recuerda la app elegida
// mientras no se vuelva atrás.
func appsSubmenu(serial string, cfg *Config) {
	var pkg string
	for {
		fmt.Println()
		fmt.Print("-- Apps --")
		if pkg != "" {
			fmt.Printf("  (%s)", pkg)
		}
		fmt.Println()
		fmt.Println(" 1) Elegir app")
		fmt.Println(" 2) Listar apps instaladas")
		fmt.Println(" 3) Info de la app")
		fmt.Println(" 4) Lanzar")
		fmt.Println(" 5) Reiniciar (force-stop + lanzar)")
		fmt.Println(" 6) Forzar detención")
		fmt.Println(" 7) Habilitar / deshabilitar")
		fmt.Println(" 8) Desinstalar")
		fmt.Println(" 9) Limpiar datos/caché")
		fmt.Println("10) Ver log (logcat) de la app")
		fmt.Println(" 0) Volver")

		choice := prompt("> ")
		if choice == "0" {
			return
		}

		if choice == "2" {
			pkgs, err := listPackages(serial, false, promptFilter(cfg.PackagePrefix))
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			for _, p := range pkgs {
				if sn := shortName(p.Name, cfg.PackagePrefix); sn != p.Name {
					fmt.Printf("  - %s  (%s)\n", sn, p.Name)
				} else {
					fmt.Println("  -", p.Name)
				}
			}
			continue
		}
		if choice == "1" {
			p, err := pickPackageInteractive(serial, cfg.PackagePrefix)
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			pkg = p
			continue
		}

		p, err := ensurePkg(&pkg, serial, cfg.PackagePrefix)
		if err != nil {
			fmt.Println("Error:", err)
			continue
		}

		switch choice {
		case "3":
			info, err := getAppInfo(serial, p)
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			fmt.Print(info.String())
		case "4":
			menuRun("App lanzada", p, launchApp(serial, p))
		case "5":
			menuRun("Reiniciada", p, restartApp(serial, p))
		case "6":
			menuRun("Detenida", p, forceStop(serial, p))
		case "7":
			info, err := getAppInfo(serial, p)
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			target := !info.Enabled
			menuRun(pick(target, "Habilitada", "Deshabilitada"), p, setEnabled(serial, p, target))
		case "8":
			if !confirm(fmt.Sprintf("¿Desinstalar %s de %s?", p, serial)) {
				fmt.Println("Cancelado.")
				continue
			}
			if menuRun("Desinstalada", p, uninstallApp(serial, p)) {
				pkg = ""
			}
		case "9":
			if !confirm(fmt.Sprintf("¿Borrar datos y caché de %s? Esto es irreversible", p)) {
				fmt.Println("Cancelado.")
				continue
			}
			menuRun("Datos y caché limpiados", p, clearAppData(serial, p))
		case "10":
			streamLogcatMenu(serial, p)
		default:
			fmt.Println("Opción inválida.")
		}
	}
}

// menuRun imprime "ok: pkg" o el error, y devuelve true si no hubo error.
func menuRun(okMsg, pkg string, err error) bool {
	if err != nil {
		fmt.Println("Error:", err)
		return false
	}
	fmt.Printf("%s: %s\n", okMsg, pkg)
	return true
}

// streamLogcatMenu sigue el logcat de la app hasta que el usuario pulse Enter.
// El motor sigue al proceso por am_proc_start/died, así que no hace falta que la
// app esté ya corriendo.
func streamLogcatMenu(serial, pkg string) {
	ls, err := startLogStream(serial, pkg, LogOpts{Color: true})
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Println("(siguiendo el log; pulsa Enter para detener)")
	go func() {
		_, _ = stdinReader.ReadString('\n')
		ls.Stop()
	}()
	n := 0
	for line := range ls.Lines {
		fmt.Println(line)
		n++
	}
	if n == 0 {
		if e := ls.Err(); e != nil {
			fmt.Println("Error:", e)
		} else {
			fmt.Println("(sin líneas; ¿la app llegó a arrancar?)")
		}
	}
}

// batchSubmenu ejecuta operaciones sobre varios paquetes elegidos por filtro.
func batchSubmenu(serial string, cfg *Config) {
	for {
		fmt.Println()
		fmt.Println("-- Lote --")
		fmt.Println("1) Desinstalar por filtro/prefijo")
		fmt.Println("2) Limpiar datos por filtro/prefijo")
		fmt.Println("3) Forzar detención por filtro/prefijo")
		fmt.Println("0) Volver")
		switch prompt("> ") {
		case "1":
			runBatchMenu(serial, cfg.PackagePrefix, BatchUninstall)
		case "2":
			runBatchMenu(serial, cfg.PackagePrefix, BatchClear)
		case "3":
			runBatchMenu(serial, cfg.PackagePrefix, BatchStop)
		case "0":
			return
		default:
			fmt.Println("Opción inválida.")
		}
	}
}

// runBatchMenu pide el filtro, muestra la lista, y ofrece ejecutar o simular.
func runBatchMenu(serial, prefix string, kind BatchKind) {
	match := promptFilter(prefix)
	if strings.TrimSpace(match) == "" {
		fmt.Println("Necesitas un filtro (o un prefijo configurado). No se opera sobre todo.")
		return
	}
	pkgs, err := selectBatchPackages(serial, match, false)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if len(pkgs) == 0 {
		fmt.Printf("Ningún paquete coincide con %q.\n", match)
		return
	}
	fmt.Printf("%s %d paquete(s):\n", kind.verb(), len(pkgs))
	for _, p := range pkgs {
		fmt.Println("  -", p)
	}
	switch strings.ToLower(prompt("Ejecutar? [s = sí · n = solo simular · otra = cancelar]: ")) {
	case "n":
		fmt.Println("(simulación: no se hizo nada)")
		return
	case "s", "si", "sí", "y", "yes":
		okN, failN, summary := summarizeBatch(runBatch(serial, kind, pkgs))
		fmt.Print(summary)
		fmt.Printf("Hecho: %d ok, %d con error.\n", okN, failN)
	default:
		fmt.Println("Cancelado.")
	}
}

// deviceSubmenu agrupa lo relativo al dispositivo y a la configuración.
func deviceSubmenu(serial *string, cfg *Config) {
	for {
		fmt.Println()
		fmt.Println("-- Dispositivo --")
		fmt.Println("1) Listar dispositivos")
		fmt.Println("2) Seleccionar dispositivo")
		fmt.Println("3) Instalar APK (ruta local)")
		fmt.Println("4) Compartir pantalla (scrcpy)")
		fmt.Println("5) Configurar prefijo de paquetes")
		fmt.Println("0) Volver")

		switch prompt("> ") {
		case "1":
			devices, err := listDevices()
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			for _, d := range devices {
				fmt.Printf("  - %s (%s) %s\n", d.Serial, d.State, d.Model)
			}
		case "2":
			s, err := pickDeviceInteractive()
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			*serial = s
			if cfg.Device != s {
				cfg.Device = s
				_ = cfg.save()
			}
			fmt.Println("Dispositivo seleccionado:", s)
		case "3":
			s, err := ensureSerial(serial, cfg)
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			path := prompt("Ruta del .apk: ")
			if path == "" {
				continue
			}
			fmt.Println("Instalando…")
			out, err := installAPK(s, path, InstallOpts{Reinstall: true})
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			fmt.Println(out)
		case "4":
			s, err := ensureSerial(serial, cfg)
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			extra := prompt("Flags extra para scrcpy (Enter para ninguno): ")
			var extraArgs []string
			if extra != "" {
				extraArgs = strings.Fields(extra)
			}
			if err := mirrorScreen(s, extraArgs); err != nil {
				fmt.Println("Error:", err)
			}
		case "5":
			configurePrefixInteractive(cfg, *serial)
		case "0":
			return
		default:
			fmt.Println("Opción inválida.")
		}
	}
}

// ensureSerial garantiza que haya un dispositivo seleccionado, resolviéndolo
// automáticamente si solo hay uno conectado, o pidiéndolo si hay varios.
// Persiste el serial elegido en ~/.adbctlrc.
func ensureSerial(serial *string, cfg *Config) (string, error) {
	if *serial != "" {
		return *serial, nil
	}
	s, err := pickDeviceInteractive()
	if err != nil {
		return "", err
	}
	*serial = s
	if cfg.Device != s {
		cfg.Device = s
		_ = cfg.save()
	}
	return s, nil
}
