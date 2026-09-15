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
		return fmt.Errorf("usage: agentcore <types|schema|plan|converge|delete> < request.json")
	}
	verb := os.Args[1]
	ctx := context.Background()

	core, err := open(ctx)
	if err != nil {
		return err
	}
	defer core.Close()

	if verb == "types" {
		return emit(core.ResourceTypes())
	}

	var req agentcore.Request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		return fmt.Errorf("reading the request from stdin: %w", err)
	}

	if verb == "schema" {
		attributes, err := core.Schema(req.Type)
		if err != nil {
			return err
		}
		return emit(attributes)
	}

	var providerConfig map[string]interface{}
	if raw := os.Getenv("AGENTCORE_PROVIDER_CONFIG"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &providerConfig); err != nil {
			return fmt.Errorf("AGENTCORE_PROVIDER_CONFIG is not a JSON object: %w", err)
		}
	}
	if err := core.Configure(ctx, providerConfig); err != nil {
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
		return fmt.Errorf("unknown verb %q, want one of %s", verb, strings.Join([]string{"types", "schema", "plan", "converge", "delete"}, ", "))
	}
}

func open(ctx context.Context) (*agentcore.Core, error) {
	if path := os.Getenv("AGENTCORE_PROVIDER"); path != "" {
		return agentcore.OpenBinary(ctx, path)
	}
	return agentcore.Open(ctx)
}

func emit(v interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}
