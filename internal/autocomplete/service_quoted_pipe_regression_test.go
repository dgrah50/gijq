package autocomplete

import (
	"testing"

	"github.com/dayangraham/gijq/internal/jq"
)

func TestSuggestQuotedPipeKey(t *testing.T) {
	jqSvc, err := jq.NewService([]byte(`{"a|b":{"child":1},"x":2}`))
	if err != nil {
		t.Fatal(err)
	}

	got, _ := NewService(jqSvc).Suggest(`.["a|b"].`)
	if len(got) != 1 || got[0] != "child" {
		t.Fatalf("Suggest quoted pipe key = %v, want [child]", got)
	}
}
