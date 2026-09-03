// adbctl es una herramienta para administrar dispositivos Android vía adb, con
// CLI, menú interactivo e interfaz gráfica opcional (build con -tags gui).
//
// La lógica vive en paquetes por feature bajo internal/:
//
//	adb      ejecución de adb, dispositivos          config   ~/.adbctlrc
//	apps     ciclo de vida de apps + info + prefijos  batch    operaciones en lote
//	logcat   motor de logcat estilo pidcat            install  adb install
//	mirror   scrcpy                                   ui       prompts/helpers
//	cli      parseo de flags y despacho               menu     menú interactivo
//	gui      interfaz gráfica (Fyne, //go:build gui)
//
// Ver 'adbctl help'.
package main

import (
	"os"

	"adbctl/internal/cli"
)

func main() {
	os.Exit(cli.Main())
}
