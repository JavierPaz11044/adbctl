package cli

import (
	"flag"
	"fmt"

	"adbctl/internal/adb"
	"adbctl/internal/mirror"
)

func cmdDevices(args []string) error {
	devices, err := adb.List()
	if err != nil {
		return err
	}
	for _, d := range devices {
		fmt.Printf("%s\t%s\t%s\n", d.Serial, d.State, d.Model)
	}
	return nil
}

func cmdMirror(args []string) error {
	fs := flag.NewFlagSet("mirror", flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	fs.Parse(args)

	s, err := adb.Resolve(*serial)
	if err != nil {
		return err
	}
	// Todo lo que quede tras el parseo se reenvía a scrcpy:
	//   adbctl mirror -s XXXX -- --record salida.mp4
	return mirror.Screen(s, fs.Args())
}
