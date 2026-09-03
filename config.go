package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config guarda preferencias persistentes de adbctl en ~/.adbctlrc, con formato
// sencillo clave=valor (una por línea). Las líneas en blanco y las que empiezan
// con '#' se ignoran. Claves reconocidas:
//
//	prefix=com.perfect     prefijo por defecto para nombres de paquete
//	device=SERIAL          último serial seleccionado en el menú interactivo
type Config struct {
	PackagePrefix string
	Device        string
}

// configPath devuelve la ruta de ~/.adbctlrc (o "./.adbctlrc" si no se puede
// determinar el home).
func configPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".adbctlrc"
	}
	return filepath.Join(home, ".adbctlrc")
}

// loadConfig lee ~/.adbctlrc. Si no existe o no se puede leer, devuelve una
// Config vacía sin error: la configuración es totalmente opcional.
func loadConfig() Config {
	var c Config
	f, err := os.Open(configPath())
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

// save reescribe ~/.adbctlrc con el contenido actual de la Config.
func (c Config) save() error {
	var b strings.Builder
	b.WriteString("# Configuración de adbctl (editable a mano)\n")
	if c.PackagePrefix != "" {
		b.WriteString("prefix=" + c.PackagePrefix + "\n")
	}
	if c.Device != "" {
		b.WriteString("device=" + c.Device + "\n")
	}
	return os.WriteFile(configPath(), []byte(b.String()), 0o644)
}

// prefixFromFlags decide qué prefijo aplicar en modo CLI a partir de las flags
// -prefix (explícito) y -p (usa el de ~/.adbctlrc). Devuelve "" si el usuario no
// pidió ninguno: en CLI nunca se adivina.
func prefixFromFlags(useConfigPrefix bool, override string) (string, error) {
	if override != "" {
		return strings.TrimSuffix(override, "."), nil
	}
	if useConfigPrefix {
		p := loadConfig().PackagePrefix
		if p == "" {
			return "", fmt.Errorf("-p requiere un prefijo configurado en ~/.adbctlrc (defínelo con el menú interactivo, opción 8, o usa -prefix)")
		}
		return p, nil
	}
	return "", nil
}
