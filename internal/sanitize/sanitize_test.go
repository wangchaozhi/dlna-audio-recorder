package sanitize

import "testing"

func TestFileName(t *testing.T) {
	if got := FileName(`A/B:C*D?`); got != "A_B_C_D_" {
		t.Fatalf("got %q", got)
	}
}
