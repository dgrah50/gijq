package autocomplete

import (
	"testing"

	"github.com/dayangraham/gijq/internal/jq"
)

func TestSuggest(t *testing.T) {
	jsonData := `{"users":[{"name":"alice","age":30}],"meta":{"version":"1.0"}}`
	jqSvc, _ := jq.NewService([]byte(jsonData))
	svc := NewService(jqSvc)

	tests := []struct {
		name   string
		filter string
		want   []string
	}{
		{"root empty", ".", []string{"meta", "users"}},
		{"root partial", ".me", []string{"meta"}},
		{"root partial users", ".us", []string{"users"}},
		{"nested", ".meta.", []string{"version"}},
		{"nested partial", ".meta.ver", []string{"version"}},
		{"array element", ".users[0].", []string{"age", "name"}},
		{"array partial", ".users[0].na", []string{"name"}},
		{"no match", ".xyz", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestions, _ := svc.Suggest(tt.filter)
			if !equalSlices(suggestions, tt.want) {
				t.Errorf("Suggest(%q) = %v, want %v", tt.filter, suggestions, tt.want)
			}
		})
	}
}

func TestApply(t *testing.T) {
	jsonData := `{"users":[{"name":"alice"}]}`
	jqSvc, _ := jq.NewService([]byte(jsonData))
	svc := NewService(jqSvc)

	tests := []struct {
		filter   string
		selected string
		want     string
	}{
		{".us", "users", ".users"},
		{".users[0].na", "name", ".users[0].name"},
		{".meta.", "version", ".meta.version"},
		{".", "users", ".users"},
	}

	for _, tt := range tests {
		t.Run(tt.filter+"->"+tt.selected, func(t *testing.T) {
			_, ctx := svc.Suggest(tt.filter)
			got := svc.Apply(tt.filter, ctx, tt.selected)
			if got != tt.want {
				t.Errorf("Apply(%q, %q) = %q, want %q", tt.filter, tt.selected, got, tt.want)
			}
		})
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
