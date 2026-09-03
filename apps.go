package main

import (
	"bufio"
	"fmt"
	"sort"
	"strings"
)

// Package representa un paquete instalado en el dispositivo.
type Package struct {
	Name string
}

// listPackages lista los paquetes instalados. Si onlyThirdParty es true, usa -3
// (excluye apps de sistema). filter es un substring opcional para acotar resultados.
func listPackages(serial string, onlyThirdParty bool, filter string) ([]Package, error) {
	args := []string{"shell", "pm", "list", "packages"}
	if onlyThirdParty {
		args = append(args, "-3")
	}
	out, err := runADB(serial, args...)
	if err != nil {
		return nil, err
	}

	var pkgs []Package
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		name := strings.TrimPrefix(line, "package:")
		if name == "" {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(filter)) {
			continue
		}
		pkgs = append(pkgs, Package{Name: name})
	}
	return pkgs, nil
}

// shortName recorta el prefijo (y el punto separador) de un nombre de paquete
// para mostrarlo más corto en los menús. Si el paquete no empieza por el
// prefijo, se devuelve tal cual.
func shortName(pkg, prefix string) string {
	if prefix == "" || pkg == prefix {
		return pkg
	}
	if strings.HasPrefix(pkg, prefix+".") {
		return pkg[len(prefix)+1:]
	}
	return pkg
}

// expandName antepone el prefijo a un nombre corto. Si name ya contiene un punto
// se asume que es un nombre de paquete completo y se deja intacto; lo mismo si no
// hay prefijo.
func expandName(name, prefix string) string {
	if prefix == "" || name == "" || strings.Contains(name, ".") {
		return name
	}
	return prefix + "." + name
}

// PrefixSuggestion es un prefijo candidato detectado a partir de las apps
// instaladas, con cuántas comparten ese prefijo.
type PrefixSuggestion struct {
	Prefix string
	Count  int
}

// suggestPrefixes agrupa las apps de terceros por sus dos primeros segmentos
// (ej. "com.perfect") y devuelve los grupos con 2 o más apps, de más a menos
// pobladas.
func suggestPrefixes(serial string) ([]PrefixSuggestion, error) {
	pkgs, err := listPackages(serial, true, "")
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, p := range pkgs {
		parts := strings.Split(p.Name, ".")
		if len(parts) < 3 {
			continue
		}
		counts[parts[0]+"."+parts[1]]++
	}
	var out []PrefixSuggestion
	for k, n := range counts {
		if n >= 2 {
			out = append(out, PrefixSuggestion{Prefix: k, Count: n})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Prefix < out[j].Prefix
	})
	return out, nil
}

// packageExists comprueba que el paquete esté instalado antes de operar sobre él,
// para dar un error claro en vez de dejar que adb falle de forma críptica.
func packageExists(serial, pkg string) (bool, error) {
	pkgs, err := listPackages(serial, false, "")
	if err != nil {
		return false, err
	}
	for _, p := range pkgs {
		if p.Name == pkg {
			return true, nil
		}
	}
	return false, nil
}

// resolveLaunchActivity intenta obtener "package/activity" para el launcher del paquete.
func resolveLaunchActivity(serial, pkg string) (string, error) {
	out, err := runADB(serial, "shell", "cmd", "package", "resolve-activity", "--brief", pkg)
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if !strings.Contains(last, "/") {
		return "", fmt.Errorf("no se pudo resolver la actividad de lanzamiento para %s", pkg)
	}
	return last, nil
}

// launchApp lanza la actividad principal de un paquete. Si no logra resolver la
// actividad exacta, cae de vuelta al comando 'monkey', que es menos preciso pero
// funciona en casi cualquier app con un launcher estándar.
func launchApp(serial, pkg string) error {
	exists, err := packageExists(serial, pkg)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("el paquete %q no está instalado en el dispositivo", pkg)
	}

	activity, err := resolveLaunchActivity(serial, pkg)
	if err == nil {
		_, err = runADB(serial, "shell", "am", "start", "-n", activity)
		if err == nil {
			return nil
		}
	}

	// Fallback: monkey dispara el intent LAUNCHER del paquete sin necesitar
	// conocer la actividad exacta.
	_, err = runADB(serial, "shell", "monkey", "-p", pkg, "-c",
		"android.intent.category.LAUNCHER", "1")
	if err != nil {
		return fmt.Errorf("no se pudo lanzar %s: %w", pkg, err)
	}
	return nil
}

// uninstallApp desinstala un paquete.
func uninstallApp(serial, pkg string) error {
	exists, err := packageExists(serial, pkg)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("el paquete %q no está instalado en el dispositivo", pkg)
	}
	_, err = runADB(serial, "uninstall", pkg)
	if err != nil {
		return fmt.Errorf("no se pudo desinstalar %s: %w", pkg, err)
	}
	return nil
}

// setEnabled activa o desactiva un paquete para el usuario 0 sin desinstalarlo
// (pm enable / pm disable-user). Útil para "bloatware" de sistema.
func setEnabled(serial, pkg string, enabled bool) error {
	exists, err := packageExists(serial, pkg)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("el paquete %q no está instalado en el dispositivo", pkg)
	}
	sub := "disable-user"
	if enabled {
		sub = "enable"
	}
	out, err := runADB(serial, "shell", "pm", sub, "--user", "0", pkg)
	if err != nil {
		return fmt.Errorf("no se pudo %s %s: %w", sub, pkg, err)
	}
	if strings.Contains(strings.ToLower(out), "failed") {
		return fmt.Errorf("pm %s falló para %s: %s", sub, pkg, strings.TrimSpace(out))
	}
	return nil
}

// forceStop mata todos los procesos del paquete (am force-stop).
func forceStop(serial, pkg string) error {
	exists, err := packageExists(serial, pkg)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("el paquete %q no está instalado en el dispositivo", pkg)
	}
	if _, err := runADB(serial, "shell", "am", "force-stop", pkg); err != nil {
		return fmt.Errorf("no se pudo forzar la detención de %s: %w", pkg, err)
	}
	return nil
}

// restartApp fuerza la detención y vuelve a lanzar la app.
func restartApp(serial, pkg string) error {
	if err := forceStop(serial, pkg); err != nil {
		return err
	}
	return launchApp(serial, pkg)
}

// clearAppData limpia datos y caché de un paquete vía 'pm clear'.
// Nota: adb no permite borrar SOLO la caché sin acceso root; 'pm clear' borra
// datos + caché juntos, que es el comportamiento estándar de "Borrar datos" en Ajustes.
func clearAppData(serial, pkg string) error {
	exists, err := packageExists(serial, pkg)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("el paquete %q no está instalado en el dispositivo", pkg)
	}
	out, err := runADB(serial, "shell", "pm", "clear", pkg)
	if err != nil {
		return fmt.Errorf("no se pudo limpiar %s: %w", pkg, err)
	}
	if !strings.Contains(out, "Success") {
		return fmt.Errorf("pm clear no confirmó éxito para %s (salida: %s)", pkg, strings.TrimSpace(out))
	}
	return nil
}
