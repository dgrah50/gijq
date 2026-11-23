package history

import (
	"slices"
	"testing"
)

func TestStoreAddTrimsAndDeduplicatesQueries(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}

	s.Add("f", ".items")
	s.Add("f", ".items ")
	s.Add("f", " .items")
	s.Add("f", "   ")

	if got, want := s.Get("f"), []string{".items"}; !slices.Equal(got, want) {
		t.Fatalf("Get = %q, want %q", got, want)
	}
}
