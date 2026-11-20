package printer

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/nokia/ntt/control"
)

type PlainPrinter struct{}

func NewPlainPrinter() *PlainPrinter {
	return &PlainPrinter{}
}

func (p *PlainPrinter) Print(ev control.Event) {
	switch ev := ev.(type) {
	case control.LogEvent:
		ColorLog.Fprintln(os.Stderr, strings.TrimRightFunc(ev.Text, unicode.IsSpace))
	case control.StartEvent:
	case control.TickerEvent:
	case control.StopEvent:
		c := Colors(ev.Verdict)
		c.Printf("%s\t%s\t%.2f\n", ev.Verdict, ev.Name, ev.Time().Sub(ev.Begin).Seconds())
	case control.ErrorEvent:
		msg := "error: "
		if job := control.UnwrapJob(ev); job != nil {
			msg += fmt.Sprintf("%s: ", job.Name)
		}
		msg += ev.Err.Error()
		ColorFatal.Fprintln(os.Stderr, msg)
	default:
		panic(fmt.Sprintf("event type %T not implemented", ev))
	}
}
