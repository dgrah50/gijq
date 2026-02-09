package main

import (
	"strings"
	"testing"

	"github.com/dayangraham/gijq/internal/autocomplete"
	"github.com/dayangraham/gijq/internal/jq"
)

func TestFullPipeline(t *testing.T) {
	jsonData := []byte(`{
		"users": [
			{"name": "alice", "age": 30},
			{"name": "bob", "age": 25}
		],
		"config": {
			"version": "1.0",
			"debug": true
		}
	}`)

	// Create services
	jqSvc, err := jq.NewService(jsonData)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	acSvc := autocomplete.NewService(jqSvc)

	// Test jq execution
	t.Run("execute filter", func(t *testing.T) {
		result := jqSvc.Execute(".users[0].name")
		if result.Error != nil {
			t.Fatalf("Execute failed: %v", result.Error)
		}
		if !strings.Contains(result.Raw, "alice") {
			t.Errorf("expected 'alice' in result, got %s", result.Raw)
		}
	})

	// Test autocomplete suggestions
	t.Run("autocomplete at root", func(t *testing.T) {
		suggestions, _ := acSvc.Suggest(".")
		if len(suggestions) != 2 {
			t.Errorf("expected 2 suggestions, got %d: %v", len(suggestions), suggestions)
		}
	})

	t.Run("autocomplete partial", func(t *testing.T) {
		suggestions, _ := acSvc.Suggest(".us")
		if len(suggestions) != 1 || suggestions[0] != "users" {
			t.Errorf("expected [users], got %v", suggestions)
		}
	})

	// Test apply completion
	t.Run("apply completion", func(t *testing.T) {
		_, ctx := acSvc.Suggest(".users[0].na")
		result := acSvc.Apply(".users[0].na", ctx, "name")
		if result != ".users[0].name" {
			t.Errorf("expected '.users[0].name', got %s", result)
		}
	})

	// Test chained operations
	t.Run("chained workflow", func(t *testing.T) {
		// Start with root
		suggestions, _ := acSvc.Suggest(".")
		if !contains(suggestions, "users") {
			t.Fatal("expected 'users' in suggestions")
		}

		// Apply users
		_, ctx := acSvc.Suggest(".us")
		filter := acSvc.Apply(".us", ctx, "users")
		if filter != ".users" {
			t.Fatalf("expected '.users', got %s", filter)
		}

		// Execute
		result := jqSvc.Execute(filter)
		if result.Error != nil {
			t.Fatalf("Execute failed: %v", result.Error)
		}

		// Continue to array element
		filter = filter + "[0]."
		suggestions, _ = acSvc.Suggest(filter)
		if !contains(suggestions, "name") || !contains(suggestions, "age") {
			t.Errorf("expected name and age in suggestions, got %v", suggestions)
		}
	})
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
