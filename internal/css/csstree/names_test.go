// Ported from lib/__tests/names.js.

package csstree

import (
	"strings"
	"testing"
)

func TestNames(t *testing.T) {
	t.Run("keyword", func(t *testing.T) {
		t.Run("base test", func(t *testing.T) {
			want := KeywordDescriptor{
				Name:     "test",
				Basename: "test",
				Prefix:   "",
				Vendor:   "",
				Custom:   false,
			}
			if got := Keyword("test"); got != want {
				t.Errorf("Keyword(%q) = %+v, want %+v", "test", got, want)
			}
		})

		t.Run("result should be immutable", func(t *testing.T) {
			data := Keyword("test")
			data.Name = "xxx"
			want := KeywordDescriptor{
				Name:     "test",
				Basename: "test",
				Prefix:   "",
				Vendor:   "",
				Custom:   false,
			}
			// Descriptors are values: changing a returned copy must not
			// change what later calls return.
			if got := Keyword("test"); got != want {
				t.Errorf("Keyword(%q) = %+v, want %+v", "test", got, want)
			}
		})

		t.Run("should normalize name to lower case", func(t *testing.T) {
			want := KeywordDescriptor{
				Name:     "test",
				Basename: "test",
				Prefix:   "",
				Vendor:   "",
				Custom:   false,
			}
			if got := Keyword("TesT"); got != want {
				t.Errorf("Keyword(%q) = %+v, want %+v", "TesT", got, want)
			}
		})

		t.Run("should detect vendor prefixes", func(t *testing.T) {
			for _, vendor := range []string{"-moz-", "-o-", "-webkit-", "-ms-", "-any-"} {
				t.Run(vendor, func(t *testing.T) {
					want := KeywordDescriptor{
						Name:     vendor + "test",
						Basename: "test",
						Prefix:   vendor,
						Vendor:   vendor,
						Custom:   false,
					}
					if got := Keyword(vendor + "test"); got != want {
						t.Errorf("Keyword(%q) = %+v, want %+v", vendor+"test", got, want)
					}
				})
			}
		})

		t.Run("should return the same object", func(t *testing.T) {
			// Go compares descriptor values; upstream compares object identity
			// of cached results.
			if a, b := Keyword("test"), Keyword("test"); a != b {
				t.Errorf("Keyword(%q) = %+v, then %+v", "test", a, b)
			}
		})

		t.Run("should return the same object for normalized names", func(t *testing.T) {
			for _, test := range []string{"Test", "-MOZ-Test"} {
				t.Run(test, func(t *testing.T) {
					if test == strings.ToLower(test) { // guard
						t.Fatalf("%q is already lower case", test)
					}
					if a, b := Keyword(test), Keyword(strings.ToLower(test)); a != b {
						t.Errorf("Keyword(%q) = %+v, want %+v", test, a, b)
					}
				})
			}
		})

		t.Run("shouldn't detect a verdor prefix for name that doesn't starts with dash", func(t *testing.T) {
			want := KeywordDescriptor{
				Name:     "test-vendor-test",
				Basename: "test-vendor-test",
				Prefix:   "",
				Vendor:   "",
				Custom:   false,
			}
			if got := Keyword("test-vendor-test"); got != want {
				t.Errorf("Keyword(%q) = %+v, want %+v", "test-vendor-test", got, want)
			}
		})

		t.Run("shouldn't detect custom property name as verdor prefix", func(t *testing.T) {
			for _, test := range []string{"--test", "--vendor-test", "--vendor-test-test"} {
				t.Run(test, func(t *testing.T) {
					want := KeywordDescriptor{
						Name:     test,
						Basename: test,
						Prefix:   "",
						Vendor:   "",
						Custom:   true,
					}
					if got := Keyword(test); got != want {
						t.Errorf("Keyword(%q) = %+v, want %+v", test, got, want)
					}
				})
			}
		})
	})

	t.Run("property", func(t *testing.T) {
		t.Run("base test", func(t *testing.T) {
			want := PropertyDescriptor{
				Name:     "test",
				Basename: "test",
				Custom:   false,
				Prefix:   "",
				Hack:     "",
				Vendor:   "",
			}
			if got := Property("test"); got != want {
				t.Errorf("Property(%q) = %+v, want %+v", "test", got, want)
			}
		})

		t.Run("result should be immutable", func(t *testing.T) {
			data := Property("test")
			data.Name = "xxx"
			want := PropertyDescriptor{
				Name:     "test",
				Basename: "test",
				Custom:   false,
				Prefix:   "",
				Hack:     "",
				Vendor:   "",
			}
			// Descriptors are values: changing a returned copy must not
			// change what later calls return.
			if got := Property("test"); got != want {
				t.Errorf("Property(%q) = %+v, want %+v", "test", got, want)
			}
		})

		t.Run("should normalize name to lower case", func(t *testing.T) {
			want := PropertyDescriptor{
				Name:     "test",
				Basename: "test",
				Custom:   false,
				Prefix:   "",
				Hack:     "",
				Vendor:   "",
			}
			if got := Property("TesT"); got != want {
				t.Errorf("Property(%q) = %+v, want %+v", "TesT", got, want)
			}
		})

		t.Run("should detect vendor prefixes", func(t *testing.T) {
			for _, vendor := range []string{"-moz-", "-o-", "-webkit-", "-ms-", "-any-"} {
				t.Run(vendor, func(t *testing.T) {
					want := PropertyDescriptor{
						Name:     vendor + "test",
						Basename: "test",
						Custom:   false,
						Prefix:   vendor,
						Hack:     "",
						Vendor:   vendor,
					}
					if got := Property(vendor + "test"); got != want {
						t.Errorf("Property(%q) = %+v, want %+v", vendor+"test", got, want)
					}
				})
			}

			t.Run("name with dashes", func(t *testing.T) {
				want := PropertyDescriptor{
					Name:     "-a-test-test",
					Basename: "test-test",
					Custom:   false,
					Prefix:   "-a-",
					Hack:     "",
					Vendor:   "-a-",
				}
				if got := Property("-a-test-test"); got != want {
					t.Errorf("Property(%q) = %+v, want %+v", "-a-test-test", got, want)
				}
			})
		})

		t.Run("should normalize vendor to lower case", func(t *testing.T) {
			want := PropertyDescriptor{
				Name:     "-vendor-test",
				Basename: "test",
				Custom:   false,
				Prefix:   "-vendor-",
				Hack:     "",
				Vendor:   "-vendor-",
			}
			if got := Property("-VenDor-TesT"); got != want {
				t.Errorf("Property(%q) = %+v, want %+v", "-VenDor-TesT", got, want)
			}
		})

		t.Run("should detect hacks", func(t *testing.T) {
			for _, hack := range []string{"*", "_", "$", "#", "+", "&", "//", "/"} {
				t.Run(hack, func(t *testing.T) {
					want := PropertyDescriptor{
						Name:     "test",
						Basename: "test",
						Custom:   false,
						Prefix:   hack,
						Hack:     hack,
						Vendor:   "",
					}
					if got := Property(hack + "test"); got != want {
						t.Errorf("Property(%q) = %+v, want %+v", hack+"test", got, want)
					}
				})
			}
		})

		t.Run("should detect custom property", func(t *testing.T) {
			want := PropertyDescriptor{
				Name:     "--test",
				Basename: "--test",
				Custom:   true,
				Prefix:   "",
				Hack:     "",
				Vendor:   "",
			}
			if got := Property("--test"); got != want {
				t.Errorf("Property(%q) = %+v, want %+v", "--test", got, want)
			}
		})

		t.Run("should detect vendor prefix and hack", func(t *testing.T) {
			want := PropertyDescriptor{
				Name:     "-moz-test",
				Basename: "test",
				Custom:   false,
				Prefix:   "//-moz-",
				Hack:     "//",
				Vendor:   "-moz-",
			}
			if got := Property("//-moz-test"); got != want {
				t.Errorf("Property(%q) = %+v, want %+v", "//-moz-test", got, want)
			}
		})

		t.Run("should detect custom property and hack", func(t *testing.T) {
			want := PropertyDescriptor{
				Name:     "--test",
				Basename: "--test",
				Custom:   true,
				Prefix:   "//",
				Hack:     "//",
				Vendor:   "",
			}
			if got := Property("//--test"); got != want {
				t.Errorf("Property(%q) = %+v, want %+v", "//--test", got, want)
			}
		})

		t.Run("should return the same object", func(t *testing.T) {
			// Go compares descriptor values; upstream compares object identity
			// of cached results.
			if a, b := Property("test"), Property("test"); a != b {
				t.Errorf("Property(%q) = %+v, then %+v", "test", a, b)
			}
		})

		t.Run("should return the same object for normalized names", func(t *testing.T) {
			for _, test := range []string{"Test", "-MOZ-Test", "//Test", "//-MOZ-Test"} {
				t.Run(test, func(t *testing.T) {
					if test == strings.ToLower(test) { // guard
						t.Fatalf("%q is already lower case", test)
					}
					if a, b := Property(test), Property(strings.ToLower(test)); a != b {
						t.Errorf("Property(%q) = %+v, want %+v", test, a, b)
					}
				})
			}
		})

		t.Run("shouldn't detect a verdor prefix for name that doesn't starts with dash", func(t *testing.T) {
			want := PropertyDescriptor{
				Name:     "test-vendor-test",
				Basename: "test-vendor-test",
				Custom:   false,
				Prefix:   "",
				Hack:     "",
				Vendor:   "",
			}
			if got := Property("test-vendor-test"); got != want {
				t.Errorf("Property(%q) = %+v, want %+v", "test-vendor-test", got, want)
			}
		})

		t.Run("shouldn't normalize custom property names", func(t *testing.T) {
			for _, want := range []PropertyDescriptor{
				{
					Name:     "--Test-Custom",
					Basename: "--Test-Custom",
					Custom:   true,
					Prefix:   "",
					Hack:     "",
					Vendor:   "",
				},
				{
					Name:     "--TEST-custom",
					Basename: "--TEST-custom",
					Custom:   true,
					Prefix:   "",
					Hack:     "",
					Vendor:   "",
				},
			} {
				if got := Property(want.Name); got != want {
					t.Errorf("Property(%q) = %+v, want %+v", want.Name, got, want)
				}
			}

			if a, b := Property("--Test"), Property("--test"); a == b {
				t.Errorf("Property(%q) = Property(%q) = %+v, want different results", "--Test", "--test", a)
			}
		})

		t.Run("shouldn't detect a verdor prefix for custom property names", func(t *testing.T) {
			for _, test := range []string{"--test", "--vendor-test", "--vendor-test-test"} {
				t.Run(test, func(t *testing.T) {
					want := PropertyDescriptor{
						Name:     test,
						Basename: test,
						Custom:   true,
						Prefix:   "",
						Hack:     "",
						Vendor:   "",
					}
					if got := Property(test); got != want {
						t.Errorf("Property(%q) = %+v, want %+v", test, got, want)
					}
				})
			}
		})
	})

	t.Run("isCustomProperty", func(t *testing.T) {
		for _, tt := range []struct {
			test string
			want bool
		}{
			{"", false},
			{"-", false},
			{"--", true},
			{"---", true},
			{"--a", true},
		} {
			t.Run(`'`+tt.test+`'`, func(t *testing.T) {
				if got := IsCustomProperty(tt.test, 0); got != tt.want {
					t.Errorf("IsCustomProperty(%q, 0) = %t, want %t", tt.test, got, tt.want)
				}
			})

			t.Run(`'$`+tt.test+`' with offset 1`, func(t *testing.T) {
				if got := IsCustomProperty("$"+tt.test, 1); got != tt.want {
					t.Errorf("IsCustomProperty(%q, 1) = %t, want %t", "$"+tt.test, got, tt.want)
				}
			})
		}
	})

	t.Run("vendorPrefix", func(t *testing.T) {
		for _, tt := range []struct {
			test string
			want string
		}{
			{"", ""},
			{"-", ""},
			{"--", ""},
			{"-a-", "-a-"},
			{"-a-b", "-a-"},
			{"--a", ""},
			{"--a-", ""},
		} {
			t.Run(`'`+tt.test+`'`, func(t *testing.T) {
				if got := VendorPrefix(tt.test, 0); got != tt.want {
					t.Errorf("VendorPrefix(%q, 0) = %q, want %q", tt.test, got, tt.want)
				}
			})

			t.Run(`'$`+tt.test+`' with offset 1`, func(t *testing.T) {
				if got := VendorPrefix("$"+tt.test, 1); got != tt.want {
					t.Errorf("VendorPrefix(%q, 1) = %q, want %q", "$"+tt.test, got, tt.want)
				}
			})
		}
	})
}
