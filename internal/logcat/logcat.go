// Package logcat implementa un motor de logcat inspirado en pidcat: sigue a la
// app por sus PID (reconsultados cada 2 s, así que aguanta reinicios y no
// necesita que esté corriendo al empezar) y reformatea la salida.
package logcat

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"adbctl/internal/adb"
)

// appPIDs devuelve todos los PID del paquete (una app puede tener varios
// procesos). Lista vacía = la app no está en ejecución ahora mismo.
func appPIDs(serial, pkg string) []string {
	out, err := adb.Run(serial, "shell", "pidof", pkg)
	if err != nil {
		return nil
	}
	return strings.Fields(out)
}

// Opts controla el streaming de logcat.
type Opts struct {
	All   bool // no filtrar por la app: todo el logcat
	Raw   bool // emitir la línea original de logcat, sin reformatear
	Color bool // colores ANSI (terminal sí, GUI no)
}

// Line es una línea de logcat ya troceada (formato -v threadtime).
type Line struct {
	Date, PID, TID, Level, Tag, Msg string
}

// "MM-DD hh:mm:ss.mmm  PID  TID L TAG: mensaje"
var reThreadtime = regexp.MustCompile(`^(\d\d-\d\d \d\d:\d\d:\d\d\.\d+)\s+(\d+)\s+(\d+)\s+([A-Z])\s+(.*?):\s?(.*)$`)

// Streamer sigue el logcat en segundo plano y entrega líneas (ya formateadas,
// salvo modo Raw) por Lines hasta que se llama a Stop o adb termina.
type Streamer struct {
	cancel context.CancelFunc
	done   chan struct{}
	err    error
	Lines  chan string
}

// Err devuelve el error con el que murió adb logcat (nil si fue parada limpia).
// Solo tiene sentido consultarlo una vez que Lines se ha cerrado.
func (ls *Streamer) Err() error {
	if ls == nil {
		return nil
	}
	return ls.err
}

// Start arranca el stream. Devuelve error si adb muere de inmediato (serial
// inválido, dispositivo desconectado…).
func Start(serial, pkg string, o Opts) (*Streamer, error) {
	if !adb.Connected(serial) {
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

	ls := &Streamer{cancel: cancel, done: make(chan struct{}), Lines: make(chan string, 1024)}
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
			emit(formatLine(Line{Date: m[1], PID: m[2], TID: m[3], Level: m[4], Tag: m[5], Msg: m[6]}, o.Color))
		}

		if werr := cmd.Wait(); werr != nil && ctx.Err() == nil {
			msg := strings.TrimSpace(errBuf.String())
			if msg == "" {
				msg = werr.Error()
			}
			ls.err = fmt.Errorf("adb logcat terminó: %s", msg)
		}
	}()

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
func (ls *Streamer) Stop() {
	if ls == nil {
		return
	}
	ls.cancel()
	<-ls.done
}
