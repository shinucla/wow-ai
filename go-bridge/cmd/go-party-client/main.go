// go-party-client short-circuits the auto-party path: load saved API config,
// send an incoming party line through the same coach prompt + JSON parse as the bridge,
// and print the structured decision. No WoW / pixel strip required.
//
// Incoming text should look like in-game party lines: "Name: message"
//
//	go-party-client                                # interactive REPL
//	go-party-client "Bob: ready for pulls?"        # one-shot
//	go-party-client -config /path/config.json
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chelinho139/wow-ai/go-bridge/internal/config"
	"github.com/chelinho139/wow-ai/go-bridge/internal/llm"
	"github.com/chelinho139/wow-ai/go-bridge/internal/party"
)

func main() {
	cfgPathFlag := flag.String("config", "", "path to config.json (default: user config dir / WOW_AI_GO_CONFIG)")
	ctxFlag := flag.String("ctx", "", "optional in-game context string passed to the coach prompt")
	timeout := flag.Duration("timeout", 2*time.Minute, "per-request LLM timeout")
	flag.Parse()

	cfg, cfgPath, err := loadConfig(*cfgPathFlag)
	if err != nil {
		fatal(err)
	}
	if strings.TrimSpace(cfg.LLM.APIKey) == "" {
		fatal(fmt.Errorf("API key is empty in %s — set it in the bridge Settings UI first", cfgPath))
	}

	client := llm.New(cfg.LLM, "")
	fmt.Fprintf(os.Stderr, "config: %s\nmodel:  %s / %s\nmode:   party\n", cfgPath, cfg.LLM.Provider, cfg.LLM.Model)

	args := flag.Args()
	if len(args) > 0 {
		runOne(client, strings.Join(args, " "), *ctxFlag, *timeout)
		return
	}

	fmt.Fprintln(os.Stderr, "type an incoming party line as Name: message (empty line or Ctrl-D to quit)")
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for {
		fmt.Fprint(os.Stderr, "> ")
		if !in.Scan() {
			fmt.Fprintln(os.Stderr)
			break
		}
		line := strings.TrimSpace(in.Text())
		if line == "" {
			break
		}
		runOne(client, line, *ctxFlag, *timeout)
	}
	if err := in.Err(); err != nil {
		fatal(err)
	}
}

func runOne(client *llm.Client, incoming, gameCtx string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	res, err := party.Decide(ctx, client, incoming, gameCtx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	out := map[string]interface{}{
		"incoming": incoming,
		"raw":      res.Raw,
		"action":   res.Decision.Action,
		"text":     res.Decision.Text,
		"would":    would(res),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

func would(res party.Result) string {
	if res.Decision.Action == "reply" {
		return fmt.Sprintf("SendChatMessage(%q, \"PARTY\")", res.Decision.Text)
	}
	return "no party message sent"
}

func loadConfig(explicit string) (config.Config, string, error) {
	if explicit != "" {
		cfg := config.Default()
		data, err := os.ReadFile(explicit)
		if err != nil {
			return cfg, explicit, err
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, explicit, err
		}
		cfg.ApplyDefaults()
		return cfg, explicit, nil
	}
	return config.Load()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
