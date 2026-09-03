package logcat

import "fmt"

const tagWidth = 23

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

// formatLine rinde una línea al estilo pidcat: tag alineado a la derecha, badge
// de nivel y mensaje. Con color=false (GUI) va sin ANSI.
func formatLine(l Line, color bool) string {
	tag := l.Tag
	if len(tag) > tagWidth {
		tag = tag[:tagWidth]
	}
	tagCol := fmt.Sprintf("%*s", tagWidth, tag)

	if !color {
		return fmt.Sprintf("%s  %s  %s", tagCol, l.Level, l.Msg)
	}
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m %s %s", tagColor(l.Tag), tagCol, levelBadge(l.Level), l.Msg)
}
