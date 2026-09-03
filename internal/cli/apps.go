package cli

import (
	"flag"
	"fmt"
	"strings"

	"adbctl/internal/adb"
	"adbctl/internal/apps"
	"adbctl/internal/batch"
	"adbctl/internal/ui"
)

func cmdApps(args []string) error {
	fs := flag.NewFlagSet("apps", flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	filter := fs.String("filter", "", "filtrar por substring")
	thirdParty := fs.Bool("3", false, "solo apps de terceros")
	useCfgPrefix := fs.Bool("p", false, "usa el prefijo de ~/.adbctlrc como filtro por defecto")
	prefixOverride := fs.String("prefix", "", "usa este prefijo como filtro por defecto")
	fs.Parse(args)

	s, err := adb.Resolve(*serial)
	if err != nil {
		return err
	}

	effFilter := *filter
	if effFilter == "" {
		p, err := prefixFromFlags(*useCfgPrefix, *prefixOverride)
		if err != nil {
			return err
		}
		effFilter = p
	}

	pkgs, err := apps.List(s, *thirdParty, effFilter)
	if err != nil {
		return err
	}
	for _, p := range pkgs {
		fmt.Println(p.Name)
	}
	return nil
}

func cmdLaunch(args []string) error {
	fs := flag.NewFlagSet("launch", flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	useCfgPrefix := fs.Bool("p", false, "expande el nombre corto con el prefijo de ~/.adbctlrc")
	prefixOverride := fs.String("prefix", "", "expande el nombre corto con este prefijo")

	pkg, err := requirePositional(fs, args, "launch", "<paquete>")
	if err != nil {
		return err
	}
	prefix, err := prefixFromFlags(*useCfgPrefix, *prefixOverride)
	if err != nil {
		return err
	}
	pkg = apps.ExpandName(pkg, prefix)

	s, err := adb.Resolve(*serial)
	if err != nil {
		return err
	}
	if err := apps.Launch(s, pkg); err != nil {
		return err
	}
	fmt.Println("App lanzada:", pkg)
	return nil
}

// cmdAppAction implementa uninstall/clear/stop, en modo individual (un paquete)
// o en lote (--all / -match <substr>, opcionalmente acotado por el prefijo).
func cmdAppAction(kind batch.Kind, args []string) error {
	name := string(kind)
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	yes := fs.Bool("y", false, "no pedir confirmación")
	dry := fs.Bool("n", false, "dry-run: solo muestra qué se haría")
	all := fs.Bool("all", false, "modo lote sobre los paquetes que coincidan")
	match := fs.String("match", "", "modo lote: substring para elegir paquetes")
	useCfgPrefix := fs.Bool("p", false, "usa/expande con el prefijo de ~/.adbctlrc")
	prefixOverride := fs.String("prefix", "", "usa/expande con este prefijo")

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

	// ---------- modo lote ----------
	if *all || *match != "" {
		sel := *match
		if sel == "" {
			sel = prefix
		}
		pkgs, err := batch.SelectPackages(s, sel, false)
		if err != nil {
			return err
		}
		if len(pkgs) == 0 {
			fmt.Printf("Ningún paquete coincide con %q.\n", sel)
			return nil
		}
		fmt.Printf("%s %d paquete(s) en %s:\n", kind.Verb(), len(pkgs), s)
		for _, p := range pkgs {
			fmt.Println("  -", p)
		}
		if *dry {
			fmt.Println("\n(dry-run: no se hizo nada)")
			return nil
		}
		if !*yes && !ui.Confirm(fmt.Sprintf("\n¿Continuar con %s %d paquete(s)?", strings.ToLower(kind.Verb()), len(pkgs))) {
			fmt.Println("Cancelado.")
			return nil
		}
		okN, failN, summary := batch.Summarize(batch.Run(s, kind, pkgs))
		fmt.Print(summary)
		fmt.Printf("Hecho: %d ok, %d con error.\n", okN, failN)
		if failN > 0 {
			return fmt.Errorf("%d de %d fallaron", failN, len(pkgs))
		}
		return nil
	}

	// ---------- modo individual ----------
	if len(pos) < 1 {
		return fmt.Errorf("uso: adbctl %s <paquete|corto> [-s serial]   (o --all -match <substr>)", name)
	}
	pkg := apps.ExpandName(pos[0], prefix)
	if *dry {
		fmt.Printf("(dry-run) %s %s\n", kind.Verb(), pkg)
		return nil
	}
	if !*yes && kind != batch.Stop {
		msg := fmt.Sprintf("¿%s %s en %s?", kind.Verb(), pkg, s)
		if kind == batch.Clear {
			msg = fmt.Sprintf("¿Borrar datos y caché de %s? Esto es irreversible", pkg)
		}
		if !ui.Confirm(msg) {
			fmt.Println("Cancelado.")
			return nil
		}
	}
	if err := kind.Apply(s, pkg); err != nil {
		return err
	}
	fmt.Printf("%s: %s\n", pastVerb(kind), pkg)
	return nil
}

func pastVerb(k batch.Kind) string {
	switch k {
	case batch.Uninstall:
		return "Desinstalado"
	case batch.Clear:
		return "Datos y caché limpiados"
	case batch.Stop:
		return "Detenido"
	default:
		return string(k)
	}
}

func cmdRestart(args []string) error {
	fs := flag.NewFlagSet("restart", flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	useCfgPrefix := fs.Bool("p", false, "expande el nombre corto con el prefijo de ~/.adbctlrc")
	prefixOverride := fs.String("prefix", "", "expande el nombre corto con este prefijo")
	pkg, err := requirePositional(fs, args, "restart", "<paquete>")
	if err != nil {
		return err
	}
	prefix, err := prefixFromFlags(*useCfgPrefix, *prefixOverride)
	if err != nil {
		return err
	}
	s, err := adb.Resolve(*serial)
	if err != nil {
		return err
	}
	pkg = apps.ExpandName(pkg, prefix)
	if err := apps.Restart(s, pkg); err != nil {
		return err
	}
	fmt.Println("Reiniciada:", pkg)
	return nil
}

func cmdSetEnabled(enabled bool, args []string) error {
	name := "disable"
	if enabled {
		name = "enable"
	}
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	useCfgPrefix := fs.Bool("p", false, "expande el nombre corto con el prefijo de ~/.adbctlrc")
	prefixOverride := fs.String("prefix", "", "expande el nombre corto con este prefijo")
	pkg, err := requirePositional(fs, args, name, "<paquete>")
	if err != nil {
		return err
	}
	prefix, err := prefixFromFlags(*useCfgPrefix, *prefixOverride)
	if err != nil {
		return err
	}
	s, err := adb.Resolve(*serial)
	if err != nil {
		return err
	}
	pkg = apps.ExpandName(pkg, prefix)
	if err := apps.SetEnabled(s, pkg, enabled); err != nil {
		return err
	}
	fmt.Printf("%s: %s\n", ui.Pick(enabled, "Habilitada", "Deshabilitada"), pkg)
	return nil
}

func cmdInfo(args []string) error {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	serial := fs.String("s", "", "serial del dispositivo")
	useCfgPrefix := fs.Bool("p", false, "expande el nombre corto con el prefijo de ~/.adbctlrc")
	prefixOverride := fs.String("prefix", "", "expande el nombre corto con este prefijo")
	pkg, err := requirePositional(fs, args, "info", "<paquete>")
	if err != nil {
		return err
	}
	prefix, err := prefixFromFlags(*useCfgPrefix, *prefixOverride)
	if err != nil {
		return err
	}
	s, err := adb.Resolve(*serial)
	if err != nil {
		return err
	}
	info, err := apps.GetInfo(s, apps.ExpandName(pkg, prefix))
	if err != nil {
		return err
	}
	fmt.Print(info.String())
	return nil
}
