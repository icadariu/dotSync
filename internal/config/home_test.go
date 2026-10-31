package config_test

import (
	"testing"

	"github.com/icadariu/dotSync/internal/config"
)

func TestIsHomeOrAncestor(t *testing.T) {
	for _, tc := range []struct {
		dst, home string
		want      bool
	}{
		{"/", "/home/u", true},
		{"/home", "/home/u", true},
		{"/home/u", "/home/u", true},
		{"/home/u/", "/home/u", true},
		{"/home/u/.zshrc", "/home/u", false},
		{"/home/uu", "/home/u", false},
		{"/opt", "/home/u", false},
	} {
		if got := config.IsHomeOrAncestor(tc.dst, tc.home); got != tc.want {
			t.Errorf("IsHomeOrAncestor(%q, %q) = %v, want %v", tc.dst, tc.home, got, tc.want)
		}
	}
}
