package apps

import (
	"fmt"
	"regexp"
	"strings"

	"adbctl/internal/adb"
	"adbctl/internal/ui"
)

// Info resume los datos de un paquete instalado que devuelve `dumpsys package`.
type Info struct {
	Package      string
	VersionName  string
	VersionCode  string
	MinSDK       string
	TargetSDK    string
	CodePath     string
	APKPaths     []string
	ABI          string
	Installer    string
	FirstInstall string
	LastUpdate   string
	System       bool
	Enabled      bool
	APKSize      string   // tamaño legible del codePath, "" si no se pudo medir
	Permissions  []string // permisos runtime concedidos, sin el prefijo android.permission.
}

var reSystemPath = regexp.MustCompile(`^/(system|product|vendor|apex|system_ext|odm)/`)

// GetInfo consulta el dispositivo y arma un Info. Los campos que no se puedan
// extraer quedan vacíos en vez de fallar.
func GetInfo(serial, pkg string) (*Info, error) {
	if err := mustExist(serial, pkg); err != nil {
		return nil, err
	}

	out, err := adb.Run(serial, "shell", "dumpsys", "package", pkg)
	if err != nil {
		return nil, err
	}

	find := func(re string) string {
		if m := regexp.MustCompile(re).FindStringSubmatch(out); len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
		return ""
	}

	info := &Info{
		Package:      pkg,
		Enabled:      true,
		VersionName:  find(`versionName=(\S+)`),
		VersionCode:  find(`versionCode=(\d+)`),
		MinSDK:       find(`minSdk=(\d+)`),
		TargetSDK:    find(`targetSdk=(\d+)`),
		CodePath:     find(`codePath=(\S+)`),
		ABI:          find(`primaryCpuAbi=(\S+)`),
		Installer:    find(`installerPackageName=(\S+)`),
		FirstInstall: find(`firstInstallTime=(.+)`),
		LastUpdate:   find(`lastUpdateTime=(.+)`),
	}

	flags := find(`pkgFlags=\[(.*?)\]`) + " " + find(`flags=\[(.*?)\]`)
	if strings.Contains(flags, "SYSTEM") || reSystemPath.MatchString(info.CodePath) {
		info.System = true
	}

	if m := regexp.MustCompile(`\benabled=(\d+)`).FindStringSubmatch(out); len(m) > 1 {
		// 0=default(habilitada), 1=habilitada; 2,3,4=deshabilitada
		info.Enabled = m[1] == "0" || m[1] == "1"
	}

	for _, m := range regexp.MustCompile(`(?m)^\s+android\.permission\.(\S+): granted=true`).FindAllStringSubmatch(out, -1) {
		info.Permissions = append(info.Permissions, m[1])
	}

	if p, err := adb.Run(serial, "shell", "pm", "path", pkg); err == nil {
		for _, l := range strings.Split(p, "\n") {
			l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "package:"))
			if l != "" {
				info.APKPaths = append(info.APKPaths, l)
			}
		}
	}
	if info.CodePath != "" {
		// El codePath suele acabar en "==" (base64); se cita para que adb shell
		// no lo malinterprete. Se ignora el código de salida: du devuelve != 0
		// por subdirectorios sin permiso (p. ej. /oat) pero igual imprime el total.
		d, _ := adb.Run(serial, "shell", "du -sh '"+info.CodePath+"' 2>/dev/null")
		for _, ln := range strings.Split(d, "\n") {
			if f := strings.Fields(ln); len(f) > 0 {
				info.APKSize = f[0]
				break
			}
		}
	}

	return info, nil
}

// String formatea el Info en un bloque legible para la CLI y el menú.
func (a *Info) String() string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "%s\n", a.Package)
	line := func(k, v string) {
		if v != "" {
			fmt.Fprintf(b, "  %-13s %s\n", k, v)
		}
	}
	line("Versión", a.VersionName)
	line("versionCode", a.VersionCode)
	if a.MinSDK != "" || a.TargetSDK != "" {
		line("SDK", fmt.Sprintf("min %s / target %s", ui.Dash(a.MinSDK), ui.Dash(a.TargetSDK)))
	}
	line("ABI", a.ABI)
	line("APK", strings.Join(a.APKPaths, "\n                "))
	line("Tamaño", a.APKSize)
	line("codePath", a.CodePath)
	line("Instalador", a.Installer)
	line("Instalada", a.FirstInstall)
	line("Actualizada", a.LastUpdate)
	line("Tipo", ui.Pick(a.System, "app de sistema", "app de terceros"))
	line("Estado", ui.Pick(a.Enabled, "habilitada", "DESHABILITADA"))
	if len(a.Permissions) > 0 {
		line("Permisos", fmt.Sprintf("%d concedidos: %s", len(a.Permissions), strings.Join(a.Permissions, ", ")))
	}
	return b.String()
}
