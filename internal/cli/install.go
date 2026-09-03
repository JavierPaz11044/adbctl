package cli

import (
	"flag"
	"fmt"

	"adbctl/internal/adb"
	"adbctl/internal/install"
)

func cmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	r := fs.Bool("r", false, "reinstalar conservando datos")
	g := fs.Bool("g", false, "conceder todos los permisos en tiempo de ejecución")
	d := fs.Bool("d", false, "permitir bajar de versión")
	pos, err := parseArgsLoose(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return fmt.Errorf("uso: adbctl install <archivo.apk> [-r] [-g] [-d] [-s serial]")
	}
	s, err := adb.Resolve(*serial)
	if err != nil {
		return err
	}
	fmt.Println("Instalando", pos[0], "…")
	out, err := install.APK(s, pos[0], install.Opts{Reinstall: *r, GrantPerms: *g, Downgrade: *d})
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}
