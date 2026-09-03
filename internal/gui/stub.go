//go:build !gui

// Package gui expone Run(). En el build por defecto (sin la etiqueta `gui`) es
// un stub que explica cómo obtener la interfaz gráfica; el build con -tags gui
// aporta la implementación real con Fyne.
package gui

import "errors"

// Run es el stub sin GUI.
func Run() error {
	return errors.New("este binario se compiló sin interfaz gráfica.\n" +
		"Recompílalo con:  go build -tags gui -o adbctl .\n" +
		"(requiere gcc y, en Linux/X11, el paquete libxxf86vm-dev)")
}
