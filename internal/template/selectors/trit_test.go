package selectors_test

import (
	"testing"

	"github.com/typelate/muxt/internal/template/selectors"
)

func TestTrit(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		for _, tt := range []struct {
			in   selectors.Trit
			want string
		}{
			{selectors.TritNo, "No"},
			{selectors.TritMaybe, "Maybe"},
			{selectors.TritYes, "Yes"},
		} {
			if got := tt.in.String(); got != tt.want {
				t.Errorf("%d.String() = %q, want %q", tt.in, got, tt.want)
			}
		}
	})

	t.Run("Not", func(t *testing.T) {
		for _, tt := range []struct {
			in, want selectors.Trit
		}{
			{selectors.TritYes, selectors.TritNo},
			{selectors.TritNo, selectors.TritYes},
			{selectors.TritMaybe, selectors.TritMaybe},
		} {
			if got := tt.in.Not(); got != tt.want {
				t.Errorf("%s.Not() = %s, want %s", tt.in, got, tt.want)
			}
		}
	})

	t.Run("And is Kleene min", func(t *testing.T) {
		for _, tt := range []struct {
			a, b, want selectors.Trit
		}{
			{selectors.TritYes, selectors.TritYes, selectors.TritYes},
			{selectors.TritYes, selectors.TritMaybe, selectors.TritMaybe},
			{selectors.TritYes, selectors.TritNo, selectors.TritNo},
			{selectors.TritMaybe, selectors.TritMaybe, selectors.TritMaybe},
			{selectors.TritMaybe, selectors.TritNo, selectors.TritNo},
			{selectors.TritNo, selectors.TritNo, selectors.TritNo},
		} {
			if got := tt.a.And(tt.b); got != tt.want {
				t.Errorf("%s.And(%s) = %s, want %s", tt.a, tt.b, got, tt.want)
			}
			if got := tt.b.And(tt.a); got != tt.want {
				t.Errorf("%s.And(%s) = %s, want %s (commutativity)", tt.b, tt.a, got, tt.want)
			}
		}
	})

	t.Run("Or is Kleene max", func(t *testing.T) {
		for _, tt := range []struct {
			a, b, want selectors.Trit
		}{
			{selectors.TritYes, selectors.TritYes, selectors.TritYes},
			{selectors.TritYes, selectors.TritMaybe, selectors.TritYes},
			{selectors.TritYes, selectors.TritNo, selectors.TritYes},
			{selectors.TritMaybe, selectors.TritMaybe, selectors.TritMaybe},
			{selectors.TritMaybe, selectors.TritNo, selectors.TritMaybe},
			{selectors.TritNo, selectors.TritNo, selectors.TritNo},
		} {
			if got := tt.a.Or(tt.b); got != tt.want {
				t.Errorf("%s.Or(%s) = %s, want %s", tt.a, tt.b, got, tt.want)
			}
			if got := tt.b.Or(tt.a); got != tt.want {
				t.Errorf("%s.Or(%s) = %s, want %s (commutativity)", tt.b, tt.a, got, tt.want)
			}
		}
	})
}
