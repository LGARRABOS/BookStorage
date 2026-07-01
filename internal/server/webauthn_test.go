package server

import "testing"

func TestDecodeUserIDBE(t *testing.T) {
	t.Parallel()
	tests := []struct {
		id   int
		want int
	}{
		{id: 1, want: 1},
		{id: 42, want: 42},
		{id: 999999, want: 999999},
	}
	for _, tc := range tests {
		got := decodeUserIDBE(encodeUserIDBE(tc.id))
		if got != tc.want {
			t.Fatalf("decode(encode(%d)) = %d, want %d", tc.id, got, tc.want)
		}
	}
}

func TestDecodeUserIDBE_shortHandle(t *testing.T) {
	t.Parallel()
	got := decodeUserIDBE([]byte{0, 0, 0, 7})
	if got != 7 {
		t.Fatalf("short handle: got %d, want 7", got)
	}
}
