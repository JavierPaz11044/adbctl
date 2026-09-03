package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// deviceConnected indica si el serial aparece ahora mismo en `adb devices`.
// Sirve para dar un error claro cuando un dispositivo inalámbrico cambia de IP
// o se duerme y el serial guardado deja de existir.
func deviceConnected(serial string) bool {
	if serial == "" {
		return false
	}
	devs, err := listDevices()
	if err != nil {
		return false
	}
	for _, d := range devs {
		if d.Serial == serial {
			return true
		}
	}
	return false
}

// appPIDs devuelve todos los PID del paquete (una app puede tener varios
// procesos). Lista vacía = la app no está en ejecución ahora mismo.
func appPIDs(serial, pkg string) []string {
	out, err := runADB(serial, "shell", "pidof", pkg)
	if err != nil {
		return nil
	}
	return strings.Fields(out)
}

// LogOpts controla el streaming de logcat del motor propio.
type LogOpts struct {
	All   bool // no filtrar por la app: todo el logcat
	Raw   bool // emitir la línea original de logcat, sin reformatear
	Color bool // colores ANSI (terminal sí, GUI no)
}

// LogLine es una línea de logcat ya troceada (formato -v threadtime).
type LogLine struct {
	Date, PID, TID, Level, Tag, Msg string
}

// "MM-DD hh:mm:ss.mmm  PID  TID L TAG: mensaje"
var reThreadtime = regexp.MustCompile(`^(\d\d-\d\d \d\d:\d\d:\d\d\.\d+)\s+(\d+)\s+(\d+)\s+([A-Z])\s+(.*?):\s?(.*)$`)

// LogStreamer sigue el logcat en segundo plano y entrega líneas (ya formateadas,
// salvo modo Raw) por Lines hasta que se llama a Stop o adb termina.
//
// Mientras no sea modo All, filtra por los PID del paquete y los vuelve a
// consultar cada 2 s, así que sobrevive a reinicios de la app y no necesita que
// esté corriendo al empezar (espera a que aparezca).
type LogStreamer struct {
	cancel context.CancelFunc
	done   chan struct{}
	err    error
	Lines  chan string
}

// Err devuelve el error con el que murió adb logcat (nil si fue parada limpia).
// Solo tiene sentido consultarlo una vez que Lines se ha cerrado.
func (ls *LogStreamer) Err() error {
	if ls == nil {
		return nil
	}
	return ls.err
}

func startLogStream(serial, pkg string, o LogOpts) (*LogStreamer, error) {
	if !deviceConnected(serial) {
		return nil, fmt.Errorf("el dispositivo %s ya no está conectado (¿cambió de IP o se durmió?); vuelve a seleccionarlo", serial)
	}

	full := []string{}
	if serial != "" {
		full = append(full, "-s", serial)
	}
	full = append(full, "logcat", "-v", "threadtime")

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "adb", full...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	ls := &LogStreamer{cancel: cancel, done: make(chan struct{}), Lines: make(chan string, 1024)}
	first := make(chan struct{}, 1)

	var mu sync.Mutex
	live := map[string]bool{}
	for _, p := range appPIDs(serial, pkg) {
		live[p] = true
	}
	inLive := func(pid string) bool {
		mu.Lock()
		defer mu.Unlock()
		return live[pid]
	}

	if !o.All {
		go func() { // re-resuelve los PID por si la app se reinicia
			t := time.NewTicker(2 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					pids := appPIDs(serial, pkg)
					m := make(map[string]bool, len(pids))
					for _, p := range pids {
						m[p] = true
					}
					mu.Lock()
					live = m
					mu.Unlock()
				}
			}
		}()
	}

	go func() {
		defer close(ls.done)
		defer close(ls.Lines)

		gotFirst := false
		emit := func(s string) {
			if !gotFirst {
				gotFirst = true
				select {
				case first <- struct{}{}:
				default:
				}
			}
			select {
			case ls.Lines <- s:
			case <-ctx.Done():
			}
		}

		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			raw := sc.Text()
			m := reThreadtime.FindStringSubmatch(raw)

			if !o.All {
				if m == nil || !inLive(m[2]) {
					continue
				}
			}
			if o.Raw || m == nil {
				emit(raw)
				continue
			}
			emit(formatLog(LogLine{Date: m[1], PID: m[2], TID: m[3], Level: m[4], Tag: m[5], Msg: m[6]}, o.Color))
		}

		if werr := cmd.Wait(); werr != nil && ctx.Err() == nil {
			msg := strings.TrimSpace(errBuf.String())
			if msg == "" {
				msg = werr.Error()
			}
			ls.err = fmt.Errorf("adb logcat terminó: %s", msg)
		}
	}()

	// Si adb muere enseguida (serial inválido…), devuélvelo ya. Si arranca pero
	// aún no hay líneas (la app no está corriendo), se acepta: el motor espera.
	select {
	case <-first:
	case <-ls.done:
		if ls.err != nil {
			return nil, ls.err
		}
		if msg := strings.TrimSpace(errBuf.String()); msg != "" {
			return nil, fmt.Errorf("adb logcat: %s", msg)
		}
	case <-time.After(1200 * time.Millisecond):
	}
	return ls, nil
}

// Stop detiene el stream y espera a que la goroutine termine.
func (ls *LogStreamer) Stop() {
	if ls == nil {
		return
	}
	ls.cancel()
	<-ls.done
}

// ---------------------------------------------------------------------------
// Formato tipo pidcat
// ---------------------------------------------------------------------------

const logTagWidth = 23

// paleta xterm-256 legible para colorear tags por hash
var tagPalette = []int{39, 43, 77, 113, 178, 208, 141, 168, 116, 149, 173, 110}

func tagColor(tag string) int {
	h := 0
	for _, c := range tag {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return tagPalette[h%len(tagPalette)]
}

// levelBadge devuelve " X " con fondo de color según el nivel.
func levelBadge(level string) string {
	switch level {
	case "V":
		return "\x1b[47;30m V \x1b[0m"
	case "D":
		return "\x1b[44;37m D \x1b[0m"
	case "I":
		return "\x1b[42;30m I \x1b[0m"
	case "W":
		return "\x1b[43;30m W \x1b[0m"
	case "E", "F":
		return "\x1b[41;37m " + level + " \x1b[0m"
	default:
		return " " + level + " "
	}
}

// formatLog rinde una línea al estilo pidcat: tag alineado a la derecha, badge
// de nivel y mensaje. Con color=false (GUI) va sin ANSI.
func formatLog(l LogLine, color bool) string {
	tag := l.Tag
	if len(tag) > logTagWidth {
		tag = tag[:logTagWidth]
	}
	tagCol := fmt.Sprintf("%*s", logTagWidth, tag)

	if !color {
		return fmt.Sprintf("%s  %s  %s", tagCol, l.Level, l.Msg)
	}
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m %s %s", tagColor(l.Tag), tagCol, levelBadge(l.Level), l.Msg)
}

// ---------------------------------------------------------------------------
// pidcat externo (opcional, solo terminal)
// ---------------------------------------------------------------------------

// runPidcatIfPresent delega el seguimiento del log en el binario `pidcat` si
// está en el PATH, conectándolo a la terminal. Devuelve (usado, err); usado es
// false cuando no hay pidcat.
func runPidcatIfPresent(serial, pkg string) (bool, error) {
	path, err := exec.LookPath("pidcat")
	if err != nil {
		return false, nil
	}
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, pkg)
	cmd := exec.Command(path, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return true, cmd.Run()
}
