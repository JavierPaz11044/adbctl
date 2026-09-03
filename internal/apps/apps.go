// Package apps cubre el ciclo de vida de las aplicaciones instaladas: listar,
// lanzar, reiniciar, detener, habilitar/deshabilitar, desinstalar y limpiar
// datos, más helpers de prefijos y (en info.go) el detalle de una app.
package apps

import (
	"bufio"
	"fmt"
	"sort"
	"strings"

	"adbctl/internal/adb"
)

// Package representa un paquete instalado en el dispositivo.
type Package struct {
	Name string
}

// List lista los paquetes instalados. Si onlyThirdParty es true, usa -3
// (excluye apps de sistema). filter es un substring opcional para acotar.
func List(serial string, onlyThirdParty bool, filter string) ([]Package, error) {
	args := []string{"shell", "pm", "list", "packages"}
	if onlyThirdParty {
		args = append(args, "-3")
	}
	out, err := adb.Run(serial, args...)
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

// ShortName recorta el prefijo (y el punto separador) de un nombre de paquete
// para mostrarlo más corto. Si no empieza por el prefijo, se devuelve tal cual.
func ShortName(pkg, prefix string) string {
	if prefix == "" || pkg == prefix {
		return pkg
	}
	if strings.HasPrefix(pkg, prefix+".") {
		return pkg[len(prefix)+1:]
	}
	return pkg
}

// ExpandName antepone el prefijo a un nombre corto. Si name ya contiene un punto
// se asume que es un nombre de paquete completo y se deja intacto; lo mismo si
// no hay prefijo.
func ExpandName(name, prefix string) string {
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

// SuggestPrefixes agrupa las apps de terceros por sus dos primeros segmentos
// (ej. "com.perfect") y devuelve los grupos con 2 o más apps, de más a menos
// pobladas.
func SuggestPrefixes(serial string) ([]PrefixSuggestion, error) {
	pkgs, err := List(serial, true, "")
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

// Exists comprueba que el paquete esté instalado antes de operar sobre él, para
// dar un error claro en vez de dejar que adb falle de forma críptica.
func Exists(serial, pkg string) (bool, error) {
	pkgs, err := List(serial, false, "")
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

func mustExist(serial, pkg string) error {
	ok, err := Exists(serial, pkg)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("el paquete %q no está instalado en el dispositivo", pkg)
	}
	return nil
}

// resolveLaunchActivity intenta obtener "package/activity" para el launcher.
func resolveLaunchActivity(serial, pkg string) (string, error) {
	out, err := adb.Run(serial, "shell", "cmd", "package", "resolve-activity", "--brief", pkg)
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

// Launch lanza la actividad principal de un paquete. Si no logra resolver la
// actividad exacta, cae de vuelta a 'monkey', menos preciso pero funciona en
// casi cualquier app con un launcher estándar.
func Launch(serial, pkg string) error {
	if err := mustExist(serial, pkg); err != nil {
		return err
	}

	if activity, err := resolveLaunchActivity(serial, pkg); err == nil {
		if _, err := adb.Run(serial, "shell", "am", "start", "-n", activity); err == nil {
			return nil
		}
	}

	if _, err := adb.Run(serial, "shell", "monkey", "-p", pkg, "-c",
		"android.intent.category.LAUNCHER", "1"); err != nil {
		return fmt.Errorf("no se pudo lanzar %s: %w", pkg, err)
	}
	return nil
}

// Uninstall desinstala un paquete.
func Uninstall(serial, pkg string) error {
	if err := mustExist(serial, pkg); err != nil {
		return err
	}
	if _, err := adb.Run(serial, "uninstall", pkg); err != nil {
		return fmt.Errorf("no se pudo desinstalar %s: %w", pkg, err)
	}
	return nil
}

// SetEnabled activa o desactiva un paquete para el usuario 0 sin desinstalarlo
// (pm enable / pm disable-user). Útil para "bloatware" de sistema.
func SetEnabled(serial, pkg string, enabled bool) error {
	if err := mustExist(serial, pkg); err != nil {
		return err
	}
	sub := "disable-user"
	if enabled {
		sub = "enable"
	}
	out, err := adb.Run(serial, "shell", "pm", sub, "--user", "0", pkg)
	if err != nil {
		return fmt.Errorf("no se pudo %s %s: %w", sub, pkg, err)
	}
	if strings.Contains(strings.ToLower(out), "failed") {
		return fmt.Errorf("pm %s falló para %s: %s", sub, pkg, strings.TrimSpace(out))
	}
	return nil
}

// ForceStop mata todos los procesos del paquete (am force-stop).
func ForceStop(serial, pkg string) error {
	if err := mustExist(serial, pkg); err != nil {
		return err
	}
	if _, err := adb.Run(serial, "shell", "am", "force-stop", pkg); err != nil {
		return fmt.Errorf("no se pudo forzar la detención de %s: %w", pkg, err)
	}
	return nil
}

// Restart fuerza la detención y vuelve a lanzar la app.
func Restart(serial, pkg string) error {
	if err := ForceStop(serial, pkg); err != nil {
		return err
	}
	return Launch(serial, pkg)
}

// Clear limpia datos y caché de un paquete vía 'pm clear'. adb no permite borrar
// SOLO la caché sin root; 'pm clear' borra datos + caché juntos, como "Borrar
// datos" en Ajustes.
func Clear(serial, pkg string) error {
	if err := mustExist(serial, pkg); err != nil {
		return err
	}
	out, err := adb.Run(serial, "shell", "pm", "clear", pkg)
	if err != nil {
		return fmt.Errorf("no se pudo limpiar %s: %w", pkg, err)
	}
	if !strings.Contains(out, "Success") {
		return fmt.Errorf("pm clear no confirmó éxito para %s (salida: %s)", pkg, strings.TrimSpace(out))
	}
	return nil
}
