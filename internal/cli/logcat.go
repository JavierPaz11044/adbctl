package cli

import (
	"flag"
	"fmt"
	"os"

	"adbctl/internal/adb"
	"adbctl/internal/apps"
	"adbctl/internal/logcat"
)

func cmdLogcat(args []string) error {
	fs := flag.NewFlagSet("logcat", flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	all := fs.Bool("all", false, "no filtrar por la app (todo el logcat)")
	raw := fs.Bool("raw", false, "línea de logcat sin reformatear")
	pidcat := fs.Bool("pidcat", false, "delegar en el binario externo 'pidcat'")
	noColor := fs.Bool("no-color", false, "sin colores")
	useCfgPrefix := fs.Bool("p", false, "expande el nombre corto con el prefijo de ~/.adbctlrc")
	prefixOverride := fs.String("prefix", "", "expande el nombre corto con este prefijo")
	pos, err := parseArgsLoose(fs, args)
	if err != nil {
		return err
	}
	s, err := adb.Resolve(*serial)
	if err != nil {
		return err
	}
	prefix, err := prefixFromFlags(*useCfgPrefix, *prefixOverride)
	if err != nil {
		return err
	}
	pkg := ""
	if !*all {
		if len(pos) < 1 {
			return fmt.Errorf("uso: adbctl logcat <paquete> [-s serial]   (o 'adbctl logcat --all')")
		}
		pkg = apps.ExpandName(pos[0], prefix)
	}

	if *pidcat && !*all {
		if used, err := logcat.RunPidcatIfPresent(s, pkg); used {
			return err
		}
		return fmt.Errorf("no se encontró 'pidcat' en el PATH")
	}

	ls, err := logcat.Start(s, pkg, logcat.Opts{All: *all, Raw: *raw, Color: !*noColor})
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Ctrl+C para salir")
	for line := range ls.Lines {
		fmt.Println(line)
	}
	return ls.Err()
}
