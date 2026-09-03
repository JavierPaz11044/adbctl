# adbctl

Herramienta en Go para administrar dispositivos Android vía `adb`, con **CLI**,
**menú interactivo** e **interfaz gráfica** opcional. Permite: listar
dispositivos y apps; lanzar / reiniciar / forzar detención; ver info de una app;
habilitar / deshabilitar sin desinstalar; desinstalar y limpiar datos (también
**en lote** por filtro/prefijo); instalar APKs; seguir el **logcat** de una app;
y compartir pantalla vía `scrcpy`.

## Requisitos

- Go 1.21+ solo para compilar (el binario final no necesita Go instalado).
- `adb` (Android Platform Tools) en el `PATH`.
- `scrcpy` en el `PATH` **solo** si vas a usar el comando `mirror` / la opción
  "Compartir pantalla" del menú.

La **CLI y el menú interactivo** no tienen dependencias externas de Go (usan
solo la librería estándar), así que no hace falta `go mod download` ni conexión
a internet para compilarlos.

La **interfaz gráfica** (`adbctl gui`) es opcional y sí trae dependencias: se
compila solo si pasas `-tags gui` (ver más abajo).

## Compilar

Binario normal (CLI + menú, sin dependencias):

```bash
go build -o adbctl .
```

Con interfaz gráfica incluida:

```bash
# Linux/X11: hace falta gcc y las libs de desarrollo de OpenGL/X11, p. ej.:
#   sudo apt install libgl1-mesa-dev xorg-dev libxxf86vm-dev
go build -tags gui -o adbctl .
```

Esto genera un binario `adbctl` que puedes mover a algún directorio de tu
`PATH`, por ejemplo:

```bash
sudo mv adbctl /usr/local/bin/
```

## Uso

Sin argumentos abre un **menú interactivo**:

```bash
adbctl
```

Al arrancar el menú **carga los dispositivos y te deja elegir uno de una vez**
(si hay uno solo lo usa; recuerda el último en `~/.adbctlrc` y lo ofrece por
defecto con Enter). El menú está agrupado en submenús: **Apps ▸**, **Lote ▸**,
**Dispositivo ▸** e **Interfaz gráfica**.

### Interfaz gráfica

Si compilaste con `-tags gui`:

```bash
adbctl gui          # o la opción 4 del menú interactivo
```

Ventana de dos paneles:

- **Izquierda**: desplegable de dispositivo, campo "Buscar" para acotar,
  desplegable de app. Al elegir una app se habilitan **Lanzar**, **Reiniciar**,
  **Forzar detención**, **Activar/Desactivar**, **Info**, **Limpiar datos** y
  **Desinstalar**. **Espejo (scrcpy)** solo necesita dispositivo. Botones
  **Instalar APK…** (o arrastra un `.apk` a la ventana) y **Lote…** (checklist
  de las apps del filtro actual + desinstalar / limpiar / forzar detención).
- **Derecha**: **logcat en vivo** de la app seleccionada (o «sin filtro» para
  todo), con campo de filtro de texto, casilla «crudo» y botones Iniciar /
  Detener / Limpiar. Sigue al proceso aunque la app se reinicie.

Las acciones destructivas piden confirmación. El dispositivo elegido se recuerda
en `~/.adbctlrc`. La GUI no usa el prefijo de paquetes: eso es de la CLI / menú.

### Subcomandos CLI

```bash
adbctl devices                                   # dispositivos conectados
adbctl apps [-s serial] [-filter t] [-3] [-p]    # paquetes instalados

# sobre una app (aceptan <corto> con -p / -prefix):
adbctl launch    <paquete>                       # lanzar
adbctl restart   <paquete>                       # force-stop + lanzar
adbctl stop      <paquete>                       # forzar detención
adbctl info      <paquete>                       # versión, SDK, ruta, tamaño, permisos…
adbctl enable    <paquete>                       # rehabilitar
adbctl disable   <paquete>                       # deshabilitar sin desinstalar
adbctl uninstall <paquete> [-y]                  # desinstalar
adbctl clear     <paquete> [-y]                  # limpiar datos/caché (pm clear)
adbctl logcat    <paquete> [--all] [--raw] [--pidcat]  # seguir el log de la app

# en lote (uninstall / clear / stop):
adbctl uninstall --all -match <substr> [-n] [-y] # -n = dry-run (solo lista)
adbctl clear     --all -prefix <pfx>   [-n] [-y]

adbctl install   <archivo.apk> [-r] [-g] [-d]    # instalar un APK local
adbctl mirror    [-s serial] [-- flags-scrcpy]   # compartir pantalla
```

### Prefijo de paquetes

Si trabajas siempre con apps bajo un mismo prefijo (p. ej. `com.perfect`), puedes
guardarlo una vez con **Dispositivo ▸ Configurar prefijo** en el menú (ofrece
sugerencias a partir de las apps de terceros instaladas). Queda en `~/.adbctlrc`:

```
prefix=com.perfect
device=192.168.100.225:5555
```

Con el prefijo activo:

- En el menú, el filtro de paquetes usa el prefijo por defecto (Enter) y la lista
  muestra los nombres **en corto** (`kpfry` en vez de `com.perfect.kpfry`).
  Escribe `*` para ver todos los paquetes.
- En CLI, el prefijo **no se aplica solo**: pásalo con `-p` (usa el de
  `~/.adbctlrc`) o `-prefix com.otra` (explícito). Un nombre corto sin puntos se
  expande (`adbctl launch kpfry -p` → `com.perfect.kpfry`); uno que ya tiene
  puntos se respeta tal cual. Las flags pueden ir antes o después del paquete.

### Ejemplos

```bash
# Ver dispositivos
adbctl devices

# Buscar apps que contengan "whatsapp" en el nombre de paquete
adbctl apps -filter whatsapp

# Listar solo las apps bajo el prefijo guardado en ~/.adbctlrc
adbctl apps -p

# Lanzar Chrome en un dispositivo específico
adbctl launch com.android.chrome -s emulator-5554

# Lanzar com.perfect.kpfry escribiendo solo el nombre corto
adbctl launch kpfry -p

# Desinstalar sin pedir confirmación
adbctl uninstall com.example.app -y

# Ver qué desinstalaría el lote (dry-run), y luego hacerlo sin preguntar
adbctl uninstall --all -match com.perfect -n
adbctl uninstall --all -match com.perfect -y

# Deshabilitar bloatware sin desinstalarlo (reversible con 'enable')
adbctl disable com.fabricante.app.molesta

# Seguir el log de una app (Ctrl+C sale)
adbctl logcat kpfry -p

# Instalar un APK reinstalando y concediendo permisos
adbctl install ~/Descargas/app.apk -r -g

# Compartir pantalla, pasando flags extra directamente a scrcpy
adbctl mirror -- --max-size 1024 --stay-awake
```

## Notas importantes

- **`clear` borra datos + caché juntos.** ADB no permite borrar *solo* la
  caché sin acceso root; `pm clear` es el mismo comportamiento que "Borrar
  datos" en Ajustes del sistema, y es destructivo (borra sesión, archivos
  locales de la app, etc.). Por eso pide confirmación salvo que uses `-y`.
- **El modo lote nunca opera "sobre todo".** Exige `-match <substr>` o un
  prefijo (`-prefix`/`-p`), muestra la lista de paquetes afectados y pide **una**
  confirmación (`-y` la salta, `-n` solo simula).
- **`disable`** usa `pm disable-user --user 0`: la app desaparece del lanzador
  pero sigue instalada; se revierte con `adbctl enable`.
- **`info`** mide el tamaño con `du` sobre el `codePath`; en algunos dispositivos
  ese dato puede faltar por permisos y se omite.
- **`logcat`**: motor propio inspirado en *pidcat* — sigue a la app por sus PID
  (los reconsulta cada 2 s, así que **no necesita que esté corriendo** al empezar
  y **sobrevive a reinicios**), y reformatea al estilo pidcat (tag alineado,
  badge de nivel, color por tag). `--raw` = línea original de logcat;
  `--all` = todo el buffer sin filtrar; `--no-color` = sin ANSI;
  `--pidcat` = delegar en el binario externo `pidcat` si lo tienes en el `PATH`.
- Si hay **un solo dispositivo** conectado, se usa automáticamente sin pedir
  `-s`. Si hay varios, el modo CLI exige `-s <serial>` y el menú interactivo
  te deja elegir de una lista al arrancar (y recuerda el último elegido).
- `~/.adbctlrc` es opcional y editable a mano; si no existe, no pasa nada.
- `mirror` requiere `scrcpy` instalado aparte (no es parte de Android
  Platform Tools). Ver: https://github.com/Genymobile/scrcpy#installation

## Estructura del proyecto

Arquitectura modular por *feature*: `main.go` solo llama a `cli.Main()`; toda
la lógica vive en paquetes bajo `internal/`.

```
adbctl/
├── main.go                  # 5 líneas: os.Exit(cli.Main())
└── internal/
    ├── adb/       # ejecución de adb; Device, List, Resolve, Connected
    ├── config/    # ~/.adbctlrc (Config, Load, Save)
    ├── ui/        # Prompt, Confirm, helpers de formato (CLI + menú)
    ├── apps/      # ciclo de vida de apps (apps.go) + 'info' (info.go) + prefijos
    ├── batch/     # uninstall/clear/stop sobre una selección   → usa apps
    ├── logcat/    # motor estilo pidcat: engine + format + pidcat  → usa adb
    ├── install/   # adb install                                 → usa adb
    ├── mirror/    # scrcpy
    ├── cli/       # parseo de flags y despacho; un cmd*.go por feature
    ├── menu/      # menú interactivo con submenús
    └── gui/       # interfaz gráfica Fyne (gui.go //go:build gui, stub.go si no)
```

Dependencias: `adb` y `config` son la base; cada feature depende solo de lo que
necesita; `cli` / `menu` / `gui` orquestan. Añadir un comando nuevo: función en
el paquete de la feature + un `cmd*` en `internal/cli` + su `case` en el
dispatcher (y opcionalmente una entrada en `internal/menu`).

La CLI y el menú siguen usando solo la librería estándar (`flag`); las
dependencias de `internal/gui` (Fyne) solo entran con `-tags gui`.
