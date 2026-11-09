package run

import (
	"fmt"
	"io"
	"os"

	"github.com/nokia/ntt/internal/k3log"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

const (
	formatAuto  = "auto"
	formatPlain = "plain"
	formatColor = "color"
)

// formatter renders one k3log event for the console, without a trailing
// newline. Further formats belong in newFormatter.
type formatter interface {
	Format(event k3log.Event) string
}

// newFormatter selects a console output format.
//
// auto, the default, colors log events when out is a terminal and the
// environment allows color. color always colors them. plain prints the raw
// k3log text. The output file is written separately and stays raw.
func newFormatter(name string, out io.Writer) (formatter, error) {
	switch name {
	case formatPlain:
		return plainFormat{}, nil
	case formatColor:
		return colorFormat{enabled: true}, nil
	case formatAuto, "":
		return colorFormat{enabled: colorsWanted(out)}, nil
	default:
		return nil, fmt.Errorf("unknown console format %q (auto, plain, or color)", name)
	}
}

// colorsWanted reports whether auto should color out. NO_COLOR, NTT_COLORS=never or K3_COLORS=never,
// and TERM=dumb keep the console uncolored. An explicit color format does not
// consult this.
func colorsWanted(out io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" || os.Getenv("K3_COLORS") == "never" || os.Getenv("NTT_COLORS") == "never" {
		return false
	}
	return isTerminal(out)
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fd := f.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// plainFormat prints the raw k3log line.
type plainFormat struct{}

func (plainFormat) Format(event k3log.Event) string {
	return event.String()
}

// colorFormat colors log events. Error events are bold white on red. A tcfi,
// cofi, or setv is yellow for inconc, green for pass, and red for fail or
// error. plod, perf, vrsn, vach, and bctr are faint grey.
type colorFormat struct {
	enabled bool
}

func (f colorFormat) Format(event k3log.Event) string {
	text := event.String()
	if !f.enabled {
		return text
	}
	c := eventColor(event)
	if c == nil {
		return text
	}
	return c.Sprint(text)
}

// formatLine renders a k3log line for the console. A line that is not an
// event is returned unchanged.
func formatLine(f formatter, line string) string {
	event, err := k3log.ParseEvent(line)
	if err != nil {
		return line
	}
	return f.Format(event)
}

var (
	errorColor  = forcedColor(color.Bold, color.FgWhite, color.BgRed)
	inconcColor = forcedColor(color.FgYellow)
	passColor   = forcedColor(color.FgGreen)
	failColor   = forcedColor(color.FgRed)
	faintColor  = forcedColor(color.Faint, color.FgHiBlack)
)

// forcedColor builds a color that paints even when the process stdout is not a
// terminal. The format decides whether to use it.
func forcedColor(attrs ...color.Attribute) *color.Color {
	c := color.New(attrs...)
	c.EnableColor()
	return c
}

// eventColor returns the console color of one log event, or nil when the
// event keeps its ordinary text. setv is judged by its new verdict.
func eventColor(event k3log.Event) *color.Color {
	if event.IsError() {
		return errorColor
	}
	switch event.Category {
	case k3log.PLOD, k3log.PERF, k3log.VRSN, k3log.VACH, k3log.BCTR:
		return faintColor
	}
	switch verdictOf(event) {
	case "inconc":
		return inconcColor
	case "pass":
		return passColor
	case "fail", "error":
		return failColor
	default:
		return nil
	}
}

// verdictOf returns the verdict of a tcfi, cofi, or setv event. setv contributes
// its new verdict. Every other event contributes "".
func verdictOf(event k3log.Event) string {
	switch event.Category {
	case k3log.TCFI, k3log.SETV:
		return event.Field(4)
	case k3log.COFI:
		return event.Field(3)
	default:
		return ""
	}
}
