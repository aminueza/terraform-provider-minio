package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aminueza/terraform-provider-minio/v3/experiments/agentcore"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: agentcore <types|plan|converge|delete> < request.json")
	}
	verb := os.Args[1]
	ctx := context.Background()

	core, err := agentcore.Open(ctx)
	if err != nil {
		return err
	}

	if verb == "types" {
		return emit(core.ResourceTypes())
	}

	var req agentcore.Request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		return fmt.Errorf("reading the request from stdin: %w", err)
	}

	if err := core.Configure(ctx, nil); err != nil {
		return err
	}

	switch verb {
	case "plan":
		preview, err := core.Plan(ctx, req)
		if err != nil {
			return err
		}
		return emit(preview)
	case "converge":
		result, err := core.Converge(ctx, req)
		if err != nil {
			return err
		}
		return emit(result)
	case "delete":
		result, err := core.Delete(ctx, req)
		if err != nil {
			return err
		}
		return emit(result)
	default:
		return fmt.Errorf("unknown verb %q, want one of %s", verb, strings.Join([]string{"types", "plan", "converge", "delete"}, ", "))
	}
}

func emit(v interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}
