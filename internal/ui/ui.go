// Package ui reúne los ayudantes de interacción por terminal (prompts, confirm)
// y un par de helpers de formato compartidos entre la CLI y el menú.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Stdin es un lector con búfer sobre os.Stdin compartido por la CLI y el menú
// (para no perder bytes entre lecturas sucesivas).
var Stdin = bufio.NewReader(os.Stdin)

// Prompt muestra una etiqueta y devuelve la línea introducida, sin el salto.
func Prompt(label string) string {
	fmt.Print(label)
	line, _ := Stdin.ReadString('\n')
	return strings.TrimSpace(line)
}

// Confirm pide una confirmación s/N.
func Confirm(label string) bool {
	answer := strings.ToLower(Prompt(label + " [s/N]: "))
	return answer == "s" || answer == "si" || answer == "sí" || answer == "y" || answer == "yes"
}

// Pick devuelve yes o no según cond (Go no tiene operador ternario).
func Pick(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

// Dash devuelve "—" para una cadena vacía, o la cadena tal cual.
func Dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
