package main

import "testing"

func TestUnpackPackedLabel(t *testing.T) {
	cases := []struct {
		name string
		in   uint32
		want uint32
	}{
		{"zero background", 0, 0},
		{"cell 1 mask 1", (1 << 8) | 1, 1},
		{"cell 5 mask 2", (5 << 8) | 2, 5},
		{"cell 3 mask 0 bbox bg", (3 << 8) | 0, 0},
		{"cell 10 mask 1", (10 << 8) | 1, 10},
		{"cell 255 mask 2", (255 << 8) | 2, 255},
		{"cell 1 mask 0", (1 << 8) | 0, 0},
		{"small unpadded value", 1, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := unpackPackedLabel(c.in); got != c.want {
				t.Errorf("unpackPackedLabel(%d) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}
