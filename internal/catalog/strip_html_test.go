package catalog

import "testing"

func TestStripHTML(t *testing.T) {
	s := StripHTML(`<p>Hello <br/> <b>world</b> &amp; co</p>`)
	if s != "Hello world & co" {
		t.Fatalf("got %q", s)
	}
}
