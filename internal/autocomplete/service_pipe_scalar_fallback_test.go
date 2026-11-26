package autocomplete

import (
	"testing"

	"github.com/dayangraham/gijq/internal/jq"
)

func TestSuggestAfterPipeFromScalarDoesNotOfferRootKeys(t *testing.T) {
	jqSvc, err := jq.NewService([]byte(`{"name":"x","user":{"id":1},"tags":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(jqSvc)
	for _, filter := range []string{`.name | .`, `.tags[] | .`} {
		if got, _ := svc.Suggest(filter); len(got) != 0 {
			t.Errorf("Suggest(%q) = %v, want no suggestions (left side yields no object)", filter, got)
		}
	}
}
