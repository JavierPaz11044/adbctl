package main

import (
	"fmt"
	"strings"
)

// BatchKind identifica una operación en lote sobre varios paquetes.
type BatchKind string

const (
	BatchUninstall BatchKind = "uninstall"
	BatchClear     BatchKind = "clear"
	BatchStop      BatchKind = "stop"
)

func (k BatchKind) verb() string {
	switch k {
	case BatchUninstall:
		return "Desinstalar"
	case BatchClear:
		return "Limpiar datos de"
	case BatchStop:
		return "Forzar detención de"
	default:
		return string(k)
	}
}

func (k BatchKind) apply(serial, pkg string) error {
	switch k {
	case BatchUninstall:
		return uninstallApp(serial, pkg)
	case BatchClear:
		return clearAppData(serial, pkg)
	case BatchStop:
		return forceStop(serial, pkg)
	default:
		return fmt.Errorf("operación de lote desconocida: %s", k)
	}
}

// selectBatchPackages devuelve los paquetes que contienen `match` (substring,
// sin distinguir mayúsculas). Rechaza un match vacío para no operar sobre TODO
// el dispositivo por accidente.
func selectBatchPackages(serial, match string, thirdPartyOnly bool) ([]string, error) {
	if strings.TrimSpace(match) == "" {
		return nil, fmt.Errorf("el modo lote necesita un filtro (-match o -prefix)")
	}
	pkgs, err := listPackages(serial, thirdPartyOnly, match)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, p.Name)
	}
	return out, nil
}

// BatchResult es el resultado de aplicar la operación a un paquete.
type BatchResult struct {
	Package string
	Err     error
}

// runBatch aplica k a cada paquete y devuelve un resultado por cada uno (no se
// detiene ante el primer error).
func runBatch(serial string, k BatchKind, pkgs []string) []BatchResult {
	res := make([]BatchResult, 0, len(pkgs))
	for _, p := range pkgs {
		res = append(res, BatchResult{Package: p, Err: k.apply(serial, p)})
	}
	return res
}

// summarizeBatch cuenta éxitos y errores y devuelve un texto de resumen.
func summarizeBatch(res []BatchResult) (ok int, failed int, summary string) {
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
