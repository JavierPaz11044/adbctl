//go:build !gui

package main

import "errors"

// runGUI es el stub que se compila cuando el binario se construye SIN la
// etiqueta `gui`. Mantiene el build por defecto libre de cgo y de dependencias
// externas.
func runGUI() error {
	return errors.New("este binario se compiló sin interfaz gráfica.\n" +
		"Recompílalo con:  go build -tags gui -o adbctl .\n" +
		"(requiere gcc y, en Linux/X11, el paquete libxxf86vm-dev)")
}
