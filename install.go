package main

import (
	"fmt"
	"os"
	"strings"
)

// InstallOpts son las banderas de `adb install`.
type InstallOpts struct {
	Reinstall  bool // -r: reinstalar conservando datos
	Downgrade  bool // -d: permitir bajar de versión
	GrantPerms bool // -g: conceder todos los permisos en tiempo de ejecución
}

// installAPK instala un .apk local en el dispositivo.
func installAPK(serial, path string, o InstallOpts) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("no se puede leer %q: %w", path, err)
	}
	if fi.IsDir() {
		return "", fmt.Errorf("%q es un directorio; pasa un archivo .apk", path)
	}
	if !strings.HasSuffix(strings.ToLower(path), ".apk") {
		return "", fmt.Errorf("%q no parece un .apk", path)
	}

	args := []string{"install"}
	if o.Reinstall {
		args = append(args, "-r")
	}
	if o.Downgrade {
		args = append(args, "-d")
	}
	if o.GrantPerms {
		args = append(args, "-g")
	}
	args = append(args, path)

	out, err := runADB(serial, args...)
	if err != nil {
		return "", err
	}
	if !strings.Contains(out, "Success") {
		return "", fmt.Errorf("adb install no confirmó éxito (salida: %s)", strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}
