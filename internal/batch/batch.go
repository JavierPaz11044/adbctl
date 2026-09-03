// Package batch aplica una operación (desinstalar / limpiar / forzar detención)
// a un conjunto de paquetes seleccionados por filtro.
package batch

import (
	"fmt"
	"strings"

	"adbctl/internal/apps"
)

// Kind identifica una operación en lote.
type Kind string

const (
	Uninstall Kind = "uninstall"
	Clear     Kind = "clear"
	Stop      Kind = "stop"
)

// Verb devuelve el infinitivo con el que presentar la operación al usuario.
func (k Kind) Verb() string {
	switch k {
	case Uninstall:
		return "Desinstalar"
	case Clear:
		return "Limpiar datos de"
	case Stop:
		return "Forzar detención de"
	default:
		return string(k)
	}
}

// Apply ejecuta la operación sobre un único paquete.
func (k Kind) Apply(serial, pkg string) error {
	switch k {
	case Uninstall:
		return apps.Uninstall(serial, pkg)
	case Clear:
		return apps.Clear(serial, pkg)
	case Stop:
		return apps.ForceStop(serial, pkg)
	default:
		return fmt.Errorf("operación de lote desconocida: %s", k)
	}
}

// SelectPackages devuelve los paquetes que contienen `match` (substring, sin
// distinguir mayúsculas). Rechaza un match vacío para no operar sobre TODO el
// dispositivo por accidente.
func SelectPackages(serial, match string, thirdPartyOnly bool) ([]string, error) {
	if strings.TrimSpace(match) == "" {
		return nil, fmt.Errorf("el modo lote necesita un filtro (-match o -prefix)")
	}
	pkgs, err := apps.List(serial, thirdPartyOnly, match)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, p.Name)
	}
	return out, nil
}

// Result es el resultado de aplicar la operación a un paquete.
type Result struct {
	Package string
	Err     error
}

// Run aplica k a cada paquete y devuelve un resultado por cada uno (no se
// detiene ante el primer error).
func Run(serial string, k Kind, pkgs []string) []Result {
	res := make([]Result, 0, len(pkgs))
	for _, p := range pkgs {
		res = append(res, Result{Package: p, Err: k.Apply(serial, p)})
	}
	return res
}

// Summarize cuenta éxitos y errores y devuelve un texto de resumen.
func Summarize(res []Result) (ok int, failed int, summary string) {
	b := &strings.Builder{}
	for _, r := range res {
		if r.Err != nil {
			failed++
			fmt.Fprintf(b, "  ✗ %s — %v\n", r.Package, r.Err)
		} else {
			ok++
			fmt.Fprintf(b, "  ✓ %s\n", r.Package)
		}
	}
	return ok, failed, b.String()
}
