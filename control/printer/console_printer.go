package printer

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/nokia/ntt/control"
)

type ConsolePrinter struct {
}

func NewConsolePrinter() *ConsolePrinter {
	return &ConsolePrinter{}
}

func (p *ConsolePrinter) Print(ev control.Event) {
	switch ev := ev.(type) {
	case control.LogEvent:
		ColorLog.Fprintln(os.Stderr, strings.TrimRightFunc(ev.Text, unicode.IsSpace))
	case control.StartEvent:
		ColorStart.Printf("=== RUN %s\n", ev.Name)
	case control.TickerEvent:
		ColorRunning.Printf("... active %s\n", ev.Name)
	case control.StopEvent:
		c := Colors(ev.Verdict)
		c.Printf("--- %s %s\t(duration=%.2fs)\n", ev.Verdict, ev.Name, ev.Time().Sub(ev.Begin).Seconds())
	case control.ErrorEvent:
		msg := "+++ fatal "
		if job := control.UnwrapJob(ev); job != nil {
			msg += fmt.Sprintf("%s: ", job.Name)
		}
		msg += ev.Err.Error()
		ColorFatal.Println(msg)
	default:
		panic(fmt.Sprintf("event type %T not implemented", ev))
	}
}
