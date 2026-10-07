package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/chelinho139/wow-ai/go-bridge/internal/bridge"
	"github.com/chelinho139/wow-ai/go-bridge/internal/config"
	"github.com/chelinho139/wow-ai/go-bridge/internal/publish"
	"github.com/chelinho139/wow-ai/go-bridge/internal/ui"
)

func main() {
	headless := flag.Bool("headless", false, "run without opening the browser UI")
	installSlots := flag.Bool("install-slots", false, "create slot addons + signal files and exit")
	inject := flag.String("inject", "", "inject a plain-text user message (for testing)")
	cfgPathFlag := flag.String("config", "", "path to config.json (default: user config dir)")
	flag.Parse()

	cfg, cfgPath, err := loadConfig(*cfgPathFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	guessPrimer(&cfg)

	if *installSlots {
		made, kept, err := publish.InstallSlots(cfg.AddonDir, cfg.TocInterface, cfg.Slots, cfg.ActMax, cfg.PresenceMax, cfg.MaxInstances)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("slots: created %d, already present %d (instances 1..%d)\n", made, kept, cfg.MaxInstances)
		fmt.Println("Fully quit and relaunch WoW so it sees the new files.")
		return
	}

	eng := bridge.New(cfg, cfgPath, func(line string) {
		fmt.Println(line)
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *inject != "" {
		if err := eng.Start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		id, err := eng.Inject(*inject, 1)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("injected #%d; Ctrl+C to quit (waiting for LLM reply)…\n", id)
		<-ctx.Done()
		eng.Stop()
		return
	}

	if *headless {
		if err := eng.Start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("headless bridge running; Ctrl+C to stop")
		<-ctx.Done()
		eng.Stop()
		return
	}

	app := ui.New(eng, &cfg, cfgPath)
	if err := app.Run(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	eng.Stop()
}

func loadConfig(explicit string) (config.Config, string, error) {
	if explicit == "" {
		return config.Load()
	}
	cfg := config.Default()
	data, err := os.ReadFile(explicit)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, explicit, nil
		}
		return cfg, explicit, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, explicit, err
	}
	cfg.ApplyDefaults()
	return cfg, explicit, nil
}

func guessPrimer(cfg *config.Config) {
	if cfg.PrimerFile != "" {
		return
	}
	candidates := []string{
		filepath.Join("docs", "WOW-ADDON-PRIMER.md"),
		filepath.Join("..", "docs", "WOW-ADDON-PRIMER.md"),
		filepath.Join("..", "..", "docs", "WOW-ADDON-PRIMER.md"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(c)
			cfg.PrimerFile = abs
			return
		}
	}
}
