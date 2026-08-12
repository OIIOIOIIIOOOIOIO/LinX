package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"linx/internal/cdp"
	"linx/internal/ui"
)

type options struct {
	endpoint    string
	extensionID string
	selector    string
	interval    time.Duration
	listTargets bool
	state       bool
	once        bool
	jsonOutput  bool
	noANSI      bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "linx:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	var opts options
	flags := flag.NewFlagSet("linx", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&opts.endpoint, "endpoint", "http://127.0.0.1:9222", "local Chrome DevTools endpoint")
	flags.StringVar(&opts.extensionID, "extension-id", cdp.DefaultExtensionID, "LINE Chrome extension ID")
	flags.StringVar(&opts.selector, "target", "", "prefer a target whose URL or title contains this text")
	flags.DurationVar(&opts.interval, "interval", 2*time.Second, "dashboard refresh interval")
	flags.BoolVar(&opts.listTargets, "list-targets", false, "list Chrome debug targets and exit")
	flags.BoolVar(&opts.state, "state", false, "emit structured LINE application state as JSON and exit")
	flags.BoolVar(&opts.once, "once", false, "capture visible text once and exit")
	flags.BoolVar(&opts.jsonOutput, "json", false, "emit JSON (used with --once or --list-targets)")
	flags.BoolVar(&opts.noANSI, "no-ansi", false, "disable the alternate screen and ANSI control codes")
	if err := flags.Parse(args); err != nil {
		return err
	}

	bridge, err := cdp.NewBridge(opts.endpoint, opts.extensionID, opts.selector)
	if err != nil {
		return err
	}
	defer bridge.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if opts.listTargets {
		return printTargets(ctx, bridge, opts.jsonOutput)
	}
	if opts.state {
		state, err := bridge.State(ctx)
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(state)
	}
	if opts.once {
		snapshot, err := bridge.Refresh(ctx)
		if err != nil {
			return err
		}
		return printSnapshot(snapshot, opts.jsonOutput)
	}

	ansi := !opts.noANSI && isCharacterDevice(os.Stdout)
	if ansi && isCharacterDevice(os.Stdin) {
		return (ui.ControlUI{
			Controller: bridge,
			In:         os.Stdin,
			Out:        os.Stdout,
			Interval:   opts.interval,
		}).Run(ctx)
	}
	return (ui.Dashboard{
		Source:   bridge,
		In:       os.Stdin,
		Out:      os.Stdout,
		Interval: opts.interval,
		ANSI:     ansi,
	}).Run(ctx)
}

func printTargets(ctx context.Context, bridge *cdp.Bridge, jsonOutput bool) error {
	targets, err := bridge.Targets(ctx)
	if err != nil {
		return err
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].URL < targets[j].URL })

	if jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(targets)
	}
	for _, target := range targets {
		fmt.Printf("%-8s  %-24s  %s\n", target.Type, target.Title, target.URL)
	}
	return nil
}

func printSnapshot(snapshot cdp.Snapshot, jsonOutput bool) error {
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(snapshot)
	}
	for _, line := range snapshot.Lines {
		fmt.Println(line)
	}
	return nil
}

func isCharacterDevice(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
