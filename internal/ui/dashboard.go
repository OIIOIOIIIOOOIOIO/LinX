package ui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"linx/internal/cdp"
)

type Source interface {
	Refresh(context.Context) (cdp.Snapshot, error)
}

type Dashboard struct {
	Source   Source
	In       io.Reader
	Out      io.Writer
	Interval time.Duration
	ANSI     bool
	MaxLines int
}

type viewState struct {
	Snapshot cdp.Snapshot
	Err      error
	Updated  time.Time
	Loading  bool
}

func (d Dashboard) Run(ctx context.Context) error {
	if d.Source == nil {
		return fmt.Errorf("dashboard source is required")
	}
	if d.In == nil {
		d.In = os.Stdin
	}
	if d.Out == nil {
		d.Out = os.Stdout
	}
	if d.Interval <= 0 {
		d.Interval = 2 * time.Second
	}
	if d.MaxLines <= 0 {
		d.MaxLines = terminalLines()
	}

	if d.ANSI {
		fmt.Fprint(d.Out, "\x1b[?1049h\x1b[?25l")
		defer fmt.Fprint(d.Out, "\x1b[?25h\x1b[?1049l")
	}

	commands := make(chan string)
	go scanCommands(d.In, commands)

	state := viewState{Loading: true}
	d.render(state)
	refresh := make(chan viewState, 1)
	startRefresh := func() {
		go func() {
			callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			snapshot, err := d.Source.Refresh(callCtx)
			select {
			case refresh <- viewState{Snapshot: snapshot, Err: err, Updated: time.Now()}:
			case <-ctx.Done():
			}
		}()
	}

	startRefresh()
	ticker := time.NewTicker(d.Interval)
	defer ticker.Stop()
	refreshing := true

	for {
		select {
		case <-ctx.Done():
			return nil
		case result := <-refresh:
			state = result
			refreshing = false
			d.render(state)
		case <-ticker.C:
			if !refreshing {
				refreshing = true
				state.Loading = true
				d.render(state)
				startRefresh()
			}
		case command, ok := <-commands:
			if !ok {
				commands = nil
				continue
			}
			switch strings.ToLower(strings.TrimSpace(command)) {
			case "q", "quit", "exit":
				return nil
			case "", "r", "refresh":
				if !refreshing {
					refreshing = true
					state.Loading = true
					d.render(state)
					startRefresh()
				}
			case "h", "help", "?":
				state.Err = fmt.Errorf("commands: r/Enter refresh, q quit")
				d.render(state)
			}
		}
	}
}

func scanCommands(in io.Reader, commands chan<- string) {
	defer close(commands)
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		commands <- scanner.Text()
	}
}

func (d Dashboard) render(state viewState) {
	var out strings.Builder
	if d.ANSI {
		out.WriteString("\x1b[2J\x1b[H")
	}

	out.WriteString("LINX · LINE Chrome → terminal\n")
	out.WriteString(strings.Repeat("─", 56))
	out.WriteByte('\n')

	switch {
	case state.Loading && state.Updated.IsZero():
		out.WriteString("status  connecting…\n")
	case state.Loading:
		out.WriteString("status  refreshing…\n")
	case state.Err != nil:
		out.WriteString("status  waiting: ")
		out.WriteString(oneLine(state.Err.Error()))
		out.WriteByte('\n')
	default:
		out.WriteString("status  connected · ")
		out.WriteString(state.Snapshot.Source)
		out.WriteString(" · ")
		out.WriteString(state.Updated.Format("15:04:05"))
		out.WriteByte('\n')
		out.WriteString("target  ")
		out.WriteString(oneLine(state.Snapshot.Target.Title))
		out.WriteByte('\n')
		out.WriteString("url     ")
		out.WriteString(oneLine(state.Snapshot.Target.URL))
		out.WriteByte('\n')
	}

	out.WriteString(strings.Repeat("─", 56))
	out.WriteByte('\n')

	lines := state.Snapshot.Lines
	if len(lines) == 0 {
		out.WriteString("ยังไม่มีข้อความที่อ่านได้จากหน้าต่าง LINE\n")
	} else {
		start := 0
		if len(lines) > d.MaxLines {
			start = len(lines) - d.MaxLines
		}
		for _, line := range lines[start:] {
			out.WriteString("  ")
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}

	out.WriteString(strings.Repeat("─", 56))
	out.WriteByte('\n')
	out.WriteString("[Enter/r] refresh  [q] quit  (พิมพ์แล้วกด Enter)\n")
	fmt.Fprint(d.Out, out.String())
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func terminalLines() int {
	if raw := os.Getenv("LINES"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 10 {
			return value - 10
		}
	}
	return 25
}
