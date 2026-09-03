//go:build gui

// Package gui es la interfaz gráfica (Fyne). Solo se compila con la etiqueta de
// build `gui`; el build por defecto usa el stub de stub.go.
package gui

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"adbctl/internal/adb"
	"adbctl/internal/apps"
	"adbctl/internal/batch"
	"adbctl/internal/config"
	"adbctl/internal/install"
	"adbctl/internal/logcat"
	"adbctl/internal/mirror"
	"adbctl/internal/ui"
)

const logCap = 5000 // líneas máximas retenidas en el panel de log

// state agrupa los widgets y el estado mutable de la ventana.
//
// Panel izquierdo: elegir dispositivo -> buscar/elegir app -> acciones.
// Panel derecho: logcat en vivo de la app seleccionada, con filtro de texto.
type state struct {
	win fyne.Window
	cfg config.Config

	serial  string
	allPkgs []string
	pkg     string

	deviceSel  *widget.Select
	devSerials map[string]string

	filterEntry *widget.Entry
	appSel      *widget.Select

	appButtons []*widget.Button // acciones que requieren app seleccionada
	mirrorBtn  *widget.Button   // requiere solo dispositivo
	status     *widget.Label

	// panel de log
	logFilter *widget.Entry
	logAll    []string
	logView   []string
	logList   *widget.List
	logStart  *widget.Button
	logStop   *widget.Button
	logAllChk *widget.Check
	logRawChk *widget.Check
	streamer  *logcat.Streamer
}

// Run abre la ventana principal y bloquea hasta que se cierra.
func Run() error {
	if err := adb.CheckInstalled(); err != nil {
		return err
	}

	a := app.New()
	w := a.NewWindow("adbctl")
	w.Resize(fyne.NewSize(920, 540))

	g := &state{win: w, cfg: config.Load()}

	w.SetContent(container.NewHSplit(g.buildLeft(), g.buildRight()))
	if sp, ok := w.Content().(*container.Split); ok {
		sp.SetOffset(0.42)
	}

	w.SetOnDropped(g.onDrop)
	w.SetOnClosed(func() { g.stopLog() })

	g.reloadDevices()
	w.ShowAndRun()
	return nil
}

// ---------------------------------------------------------------------------
// Construcción de la interfaz
// ---------------------------------------------------------------------------

func (g *state) buildLeft() fyne.CanvasObject {
	g.deviceSel = widget.NewSelect(nil, func(label string) {
		g.serial = g.devSerials[label]
		if g.serial != "" && g.cfg.Device != g.serial {
			g.cfg.Device = g.serial
			_ = g.cfg.Save()
		}
		if g.serial != "" {
			g.mirrorBtn.Enable()
		} else {
			g.mirrorBtn.Disable()
		}
		g.stopLog()
		g.reloadApps()
	})
	g.deviceSel.PlaceHolder = "(sin dispositivos)"
	deviceRow := container.NewBorder(nil, nil,
		widget.NewLabel("Dispositivo"),
		widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), g.reloadDevices),
		g.deviceSel,
	)

	g.filterEntry = widget.NewEntry()
	g.filterEntry.SetPlaceHolder("buscar…")
	g.filterEntry.OnChanged = func(string) { g.applyFilter() }

	g.appSel = widget.NewSelect(nil, func(pkg string) {
		g.pkg = pkg
		g.updateActionState()
	})
	g.appSel.PlaceHolder = "(elige una app)"

	filterRow := container.NewBorder(nil, nil, widget.NewLabel("Buscar"), nil, g.filterEntry)
	appRow := container.NewBorder(nil, nil, widget.NewLabel("App"), nil, g.appSel)

	launch := widget.NewButtonWithIcon("Lanzar", theme.MediaPlayIcon(), func() { g.act("launch") })
	restart := widget.NewButtonWithIcon("Reiniciar", theme.MediaReplayIcon(), func() { g.act("restart") })
	stop := widget.NewButtonWithIcon("Forzar detención", theme.MediaStopIcon(), func() { g.act("stop") })
	toggle := widget.NewButtonWithIcon("Activar/Desactivar", theme.VisibilityOffIcon(), func() { g.act("toggle") })
	info := widget.NewButtonWithIcon("Info", theme.InfoIcon(), func() { g.act("info") })
	clear := widget.NewButtonWithIcon("Limpiar datos", theme.ContentClearIcon(), func() { g.act("clear") })
	uninstall := widget.NewButtonWithIcon("Desinstalar", theme.DeleteIcon(), func() { g.act("uninstall") })
	g.mirrorBtn = widget.NewButtonWithIcon("Espejo (scrcpy)", theme.VisibilityIcon(), func() { g.act("mirror") })
	uninstall.Importance = widget.DangerImportance
	clear.Importance = widget.DangerImportance

	g.appButtons = []*widget.Button{launch, restart, stop, toggle, info, clear, uninstall}
	for _, b := range append(g.appButtons, g.mirrorBtn) {
		b.Disable()
	}
	actions := container.NewGridWithColumns(2,
		launch, restart,
		stop, toggle,
		info, clear,
		uninstall, g.mirrorBtn,
	)

	installBtn := widget.NewButtonWithIcon("Instalar APK…", theme.FolderOpenIcon(), g.pickAndInstall)
	batchBtn := widget.NewButtonWithIcon("Lote…", theme.ListIcon(), g.batchDialog)
	extra := container.NewGridWithColumns(2, installBtn, batchBtn)

	g.status = widget.NewLabel("")
	g.status.Wrapping = fyne.TextWrapWord

	return container.NewVBox(
		deviceRow, widget.NewSeparator(),
		filterRow, appRow, widget.NewSeparator(),
		actions, extra,
		widget.NewSeparator(), g.status,
	)
}

func (g *state) buildRight() fyne.CanvasObject {
	g.logFilter = widget.NewEntry()
	g.logFilter.SetPlaceHolder("filtrar log…")
	g.logFilter.OnChanged = func(string) { g.rebuildLogView() }

	g.logStart = widget.NewButtonWithIcon("Iniciar", theme.MediaPlayIcon(), g.startLog)
	g.logStop = widget.NewButtonWithIcon("Detener", theme.MediaStopIcon(), g.stopLog)
	clearBtn := widget.NewButtonWithIcon("Limpiar", theme.ContentClearIcon(), g.clearLog)
	g.logStop.Disable()
	g.logAllChk = widget.NewCheck("sin filtro", func(bool) { g.restartLogIfRunning() })
	g.logRawChk = widget.NewCheck("crudo", func(bool) { g.restartLogIfRunning() })

	g.logList = widget.NewList(
		func() int { return len(g.logView) },
		func() fyne.CanvasObject {
			l := widget.NewLabel("")
			l.TextStyle = fyne.TextStyle{Monospace: true}
			l.Wrapping = fyne.TextWrapOff
			return l
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(g.logView[i])
		},
	)

	head := container.NewVBox(
		container.NewHBox(g.logStart, g.logStop, clearBtn, g.logAllChk, g.logRawChk),
		g.logFilter,
	)
	return container.NewBorder(head, nil, nil, nil, g.logList)
}

// ---------------------------------------------------------------------------
// Estado / listas
// ---------------------------------------------------------------------------

func (g *state) setStatus(s string) { g.status.SetText(s) }

func (g *state) updateActionState() {
	on := g.pkg != ""
	for _, b := range g.appButtons {
		if on {
			b.Enable()
		} else {
			b.Disable()
		}
	}
}

func (g *state) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(g.filterEntry.Text))
	opts := make([]string, 0, len(g.allPkgs))
	for _, p := range g.allPkgs {
		if q == "" || strings.Contains(strings.ToLower(p), q) {
			opts = append(opts, p)
		}
	}
	g.appSel.Options = opts
	if g.pkg != "" && !slices.Contains(opts, g.pkg) {
		g.pkg = ""
		g.appSel.ClearSelected()
		g.updateActionState()
	}
	g.appSel.Refresh()

	switch {
	case len(g.allPkgs) == 0:
		g.setStatus("")
	case q == "":
		g.setStatus(fmt.Sprintf("%d apps", len(g.allPkgs)))
	default:
		g.setStatus(fmt.Sprintf("%d / %d apps", len(opts), len(g.allPkgs)))
	}
}

func (g *state) reloadDevices() {
	g.setStatus("Buscando dispositivos…")
	go func() {
		devices, err := adb.List()
		fyne.Do(func() {
			if err != nil {
				g.deviceSel.Options = nil
				g.deviceSel.Refresh()
				g.serial = ""
				g.allPkgs = nil
				g.mirrorBtn.Disable()
				g.applyFilter()
				g.setStatus("Sin dispositivos: " + err.Error())
				return
			}
			g.devSerials = make(map[string]string, len(devices))
			labels := make([]string, 0, len(devices))
			for _, d := range devices {
				label := d.Serial
				if d.Model != "" {
					label = d.Serial + "  —  " + d.Model
				}
				if d.State != "device" {
					label += "  [" + d.State + "]"
				}
				labels = append(labels, label)
				g.devSerials[label] = d.Serial
			}
			g.deviceSel.Options = labels
			g.deviceSel.Refresh()

			pick := labels[0]
			for _, l := range labels {
				if g.devSerials[l] == g.cfg.Device {
					pick = l
					break
				}
			}
			g.deviceSel.SetSelected(pick)
		})
	}()
}

func (g *state) reloadApps() {
	serial := g.serial
	g.pkg = ""
	g.appSel.ClearSelected()
	g.updateActionState()
	if serial == "" {
		g.allPkgs = nil
		g.applyFilter()
		return
	}
	g.setStatus("Cargando apps…")
	go func() {
		pkgs, err := apps.List(serial, false, "")
		fyne.Do(func() {
			if err != nil {
				dialog.ShowError(err, g.win)
				g.setStatus("Error al listar apps")
				return
			}
			names := make([]string, len(pkgs))
			for i, p := range pkgs {
				names[i] = p.Name
			}
			sort.Strings(names)
			g.allPkgs = names
			g.applyFilter()
		})
	}()
}

// ---------------------------------------------------------------------------
// Acciones sobre una app
// ---------------------------------------------------------------------------

var actionLabels = map[string][2]string{
	"launch":    {"Lanzando", "App lanzada"},
	"restart":   {"Reiniciando", "Reiniciada"},
	"stop":      {"Deteniendo", "Detenida"},
	"uninstall": {"Desinstalando", "Desinstalada"},
	"clear":     {"Limpiando", "Datos y caché limpiados"},
}

func (g *state) act(kind string) {
	if g.serial == "" {
		dialog.ShowInformation("Sin dispositivo", "Selecciona un dispositivo primero.", g.win)
		return
	}

	if kind == "mirror" {
		if err := mirror.CheckInstalled(); err != nil {
			dialog.ShowError(err, g.win)
			return
		}
		g.setStatus("Abriendo scrcpy…")
		go func() {
			err := mirror.Screen(g.serial, nil)
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(err, g.win)
					g.setStatus("scrcpy terminó con error")
					return
				}
				g.setStatus("scrcpy cerrado")
			})
		}()
		return
	}

	if g.pkg == "" {
		dialog.ShowInformation("Sin selección", "Elige una app primero.", g.win)
		return
	}
	pkg := g.pkg

	switch kind {
	case "info":
		g.showInfo(pkg)
		return
	case "toggle":
		g.toggleEnabled(pkg)
		return
	}

	simple := func() {
		lbl := actionLabels[kind]
		g.setStatus(lbl[0] + " " + pkg + "…")
		go func() {
			var err error
			switch kind {
			case "launch":
				err = apps.Launch(g.serial, pkg)
			case "restart":
				err = apps.Restart(g.serial, pkg)
			case "stop":
				err = apps.ForceStop(g.serial, pkg)
			case "uninstall":
				err = apps.Uninstall(g.serial, pkg)
			case "clear":
				err = apps.Clear(g.serial, pkg)
			}
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(err, g.win)
					g.setStatus("Error")
					return
				}
				g.setStatus(lbl[1] + ": " + pkg)
				if kind == "uninstall" {
					g.reloadApps()
				}
			})
		}()
	}

	switch kind {
	case "uninstall":
		dialog.ShowConfirm("Desinstalar",
			fmt.Sprintf("¿Desinstalar %s de %s?", pkg, g.serial),
			func(ok bool) {
				if ok {
					simple()
				}
			}, g.win)
	case "clear":
		dialog.ShowConfirm("Limpiar datos",
			fmt.Sprintf("¿Borrar datos y caché de %s?\nEsto es irreversible.", pkg),
			func(ok bool) {
				if ok {
					simple()
				}
			}, g.win)
	default:
		simple()
	}
}

func (g *state) showInfo(pkg string) {
	g.setStatus("Consultando " + pkg + "…")
	go func() {
		info, err := apps.GetInfo(g.serial, pkg)
		fyne.Do(func() {
			if err != nil {
				dialog.ShowError(err, g.win)
				g.setStatus("Error")
				return
			}
			g.setStatus("")
			lbl := widget.NewLabel(info.String())
			lbl.TextStyle = fyne.TextStyle{Monospace: true}
			sc := container.NewVScroll(lbl)
			sc.SetMinSize(fyne.NewSize(460, 340))
			dialog.ShowCustom("Info: "+pkg, "Cerrar", sc, g.win)
		})
	}()
}

func (g *state) toggleEnabled(pkg string) {
	g.setStatus("Consultando estado de " + pkg + "…")
	go func() {
		info, err := apps.GetInfo(g.serial, pkg)
		if err != nil {
			fyne.Do(func() { dialog.ShowError(err, g.win); g.setStatus("Error") })
			return
		}
		target := !info.Enabled
		err = apps.SetEnabled(g.serial, pkg, target)
		fyne.Do(func() {
			if err != nil {
				dialog.ShowError(err, g.win)
				g.setStatus("Error")
				return
			}
			g.setStatus(ui.Pick(target, "Habilitada", "Deshabilitada") + ": " + pkg)
		})
	}()
}

// ---------------------------------------------------------------------------
// Instalar APK (botón + arrastrar y soltar)
// ---------------------------------------------------------------------------

func (g *state) pickAndInstall() {
	if g.serial == "" {
		dialog.ShowInformation("Instalar APK", "Elige un dispositivo primero.", g.win)
		return
	}
	fd := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
		if err != nil || rc == nil {
			return
		}
		p := rc.URI().Path()
		_ = rc.Close()
		g.confirmInstall(p)
	}, g.win)
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".apk"}))
	fd.Show()
}

func (g *state) onDrop(_ fyne.Position, uris []fyne.URI) {
	if g.serial == "" {
		dialog.ShowInformation("Instalar APK", "Elige un dispositivo primero.", g.win)
		return
	}
	for _, u := range uris {
		if strings.HasSuffix(strings.ToLower(u.Path()), ".apk") {
			g.confirmInstall(u.Path())
			return
		}
	}
	dialog.ShowInformation("Instalar APK", "Suelta un archivo .apk.", g.win)
}

func (g *state) confirmInstall(path string) {
	base := filepath.Base(path)
	dialog.ShowConfirm("Instalar APK",
		fmt.Sprintf("¿Instalar %s en %s?\n(se reinstala conservando datos si ya existe)", base, g.serial),
		func(ok bool) {
			if !ok {
				return
			}
			g.setStatus("Instalando " + base + "…")
			go func() {
				out, err := install.APK(g.serial, path, install.Opts{Reinstall: true})
				fyne.Do(func() {
					if err != nil {
						dialog.ShowError(err, g.win)
						g.setStatus("Error al instalar")
						return
					}
					g.setStatus("Instalado: " + base + " (" + out + ")")
					g.reloadApps()
				})
			}()
		}, g.win)
}

// ---------------------------------------------------------------------------
// Lote (checklist)
// ---------------------------------------------------------------------------

func (g *state) batchDialog() {
	if g.serial == "" {
		dialog.ShowInformation("Lote", "Elige un dispositivo primero.", g.win)
		return
	}
	q := strings.ToLower(strings.TrimSpace(g.filterEntry.Text))
	var cands []string
	for _, p := range g.allPkgs {
		if q == "" || strings.Contains(strings.ToLower(p), q) {
			cands = append(cands, p)
		}
	}
	if len(cands) == 0 {
		dialog.ShowInformation("Lote", "No hay apps que coincidan con el filtro actual.", g.win)
		return
	}

	checks := make([]*widget.Check, len(cands))
	box := container.NewVBox()
	for i, p := range cands {
		c := widget.NewCheck(p, nil)
		checks[i] = c
		box.Add(c)
	}
	sc := container.NewVScroll(box)
	sc.SetMinSize(fyne.NewSize(380, 300))

	action := widget.NewSelect([]string{"Desinstalar", "Limpiar datos", "Forzar detención"}, nil)
	action.SetSelectedIndex(0)

	content := container.NewBorder(
		container.NewVBox(widget.NewLabel("Filtro: "+ui.Dash(q)), action, widget.NewSeparator()),
		nil, nil, nil, sc,
	)
	dialog.ShowCustomConfirm("Lote", "Ejecutar", "Cancelar", content, func(ok bool) {
		if !ok {
			return
		}
		var sel []string
		for i, c := range checks {
			if c.Checked {
				sel = append(sel, cands[i])
			}
		}
		if len(sel) == 0 {
			return
		}
		kind := batch.Uninstall
		switch action.Selected {
		case "Limpiar datos":
			kind = batch.Clear
		case "Forzar detención":
			kind = batch.Stop
		}
		g.setStatus(fmt.Sprintf("%s %d paquete(s)…", kind.Verb(), len(sel)))
		go func() {
			okN, failN, summary := batch.Summarize(batch.Run(g.serial, kind, sel))
			fyne.Do(func() {
				g.setStatus(fmt.Sprintf("Lote: %d ok, %d con error", okN, failN))
				if failN > 0 {
					lbl := widget.NewLabel(summary)
					lbl.TextStyle = fyne.TextStyle{Monospace: true}
					dialog.ShowCustom("Lote — resultado", "Cerrar", container.NewVScroll(lbl), g.win)
				}
				if kind == batch.Uninstall {
					g.reloadApps()
				}
			})
		}()
	}, g.win)
}

// ---------------------------------------------------------------------------
// Panel de log
// ---------------------------------------------------------------------------

func (g *state) startLog() {
	if g.streamer != nil {
		return
	}
	all := g.logAllChk.Checked
	if !all && g.pkg == "" {
		dialog.ShowInformation("Log", "Elige una app o marca «sin filtro».", g.win)
		return
	}
	// logcat.Start hace llamadas a adb y puede tardar ~1s; fuera del hilo UI.
	g.logStart.Disable()
	g.setStatus("Conectando al log…")
	serial, pkg := g.serial, g.pkg
	opts := logcat.Opts{All: all, Raw: g.logRawChk.Checked, Color: false}
	go func() {
		ls, err := logcat.Start(serial, pkg, opts)
		fyne.Do(func() {
			if err != nil {
				g.logStart.Enable()
				g.setStatus("Log: error")
				dialog.ShowError(err, g.win)
				return
			}
			g.attachStream(ls)
		})
	}()
}

// restartLogIfRunning reinicia el stream para aplicar un cambio de «sin filtro»
// o «crudo».
func (g *state) restartLogIfRunning() {
	if g.streamer == nil {
		return
	}
	g.stopLog()
	g.startLog()
}

// attachStream engancha un Streamer ya arrancado al panel (en el hilo UI).
func (g *state) attachStream(ls *logcat.Streamer) {
	g.streamer = ls
	g.logStart.Disable()
	g.logStop.Enable()
	g.setStatus("Log en marcha…")

	var mu sync.Mutex
	var pending []string
	drained := make(chan struct{})

	go func() {
		for line := range ls.Lines {
			mu.Lock()
			pending = append(pending, line)
			mu.Unlock()
		}
		close(drained)
	}()

	go func() {
		t := time.NewTicker(200 * time.Millisecond)
		defer t.Stop()
		flush := func() {
			mu.Lock()
			if len(pending) == 0 {
				mu.Unlock()
				return
			}
			chunk := pending
			pending = nil
			mu.Unlock()
			fyne.Do(func() { g.appendLog(chunk) })
		}
		for {
			select {
			case <-drained:
				flush()
				e := ls.Err()
				fyne.Do(func() { g.onLogEnded(e) })
				return
			case <-t.C:
				flush()
			}
		}
	}()
}

func (g *state) appendLog(lines []string) {
	g.logAll = append(g.logAll, lines...)
	if len(g.logAll) > logCap {
		g.logAll = g.logAll[len(g.logAll)-logCap:]
	}
	g.rebuildLogView()
}

// onLogEnded se llama cuando el stream termina por sí solo (adb murió). Si fue
// una parada manual, g.streamer ya es nil y no hace nada.
func (g *state) onLogEnded(err error) {
	if g.streamer == nil {
		return
	}
	g.streamer = nil
	g.logStart.Enable()
	g.logStop.Disable()
	if err != nil {
		g.setStatus("Log: " + err.Error())
	} else {
		g.setStatus("Log detenido")
	}
}

func (g *state) stopLog() {
	if g.streamer == nil {
		return
	}
	s := g.streamer
	g.streamer = nil
	g.logStart.Enable()
	g.logStop.Disable()
	go s.Stop()
}

func (g *state) clearLog() {
	g.logAll = nil
	g.logView = nil
	g.logList.Refresh()
}

func (g *state) rebuildLogView() {
	q := strings.ToLower(strings.TrimSpace(g.logFilter.Text))
	g.logView = g.logView[:0]
	for _, l := range g.logAll {
		if q == "" || strings.Contains(strings.ToLower(l), q) {
			g.logView = append(g.logView, l)
		}
	}
	g.logList.Refresh()
	if n := len(g.logView); n > 0 {
		g.logList.ScrollTo(n - 1)
	}
}
