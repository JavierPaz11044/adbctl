// Package config lee y escribe las preferencias persistentes de adbctl en
// ~/.adbctlrc (formato sencillo clave=valor).
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Config guarda preferencias persistentes. Claves reconocidas del archivo:
//
//	prefix=com.perfect     prefijo por defecto para nombres de paquete
//	device=SERIAL          último serial seleccionado en el menú interactivo
type Config struct {
	PackagePrefix string
	Device        string
}

// Path devuelve la ruta de ~/.adbctlrc (o "./.adbctlrc" si no se puede
// determinar el home).
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".adbctlrc"
	}
	return filepath.Join(home, ".adbctlrc")
}

// Load lee ~/.adbctlrc. Si no existe o no se puede leer, devuelve una Config
// vacía sin error: la configuración es totalmente opcional.
func Load() Config {
	var c Config
	f, err := os.Open(Path())
	if err != nil {
		return c
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "prefix":
			c.PackagePrefix = strings.TrimSuffix(val, ".")
		case "device":
			c.Device = val
		}
	}
	return c
}

// Save reescribe ~/.adbctlrc con el contenido actual de la Config.
func (c Config) Save() error {
	var b strings.Builder
	b.WriteString("# Configuración de adbctl (editable a mano)\n")
	if c.PackagePrefix != "" {
		b.WriteString("prefix=" + c.PackagePrefix + "\n")
	}
	if c.Device != "" {
		b.WriteString("device=" + c.Device + "\n")
	}
	return os.WriteFile(Path(), []byte(b.String()), 0o644)
}
