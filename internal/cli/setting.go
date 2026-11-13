package cli

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"

	"github.com/dustin/go-humanize"
	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/generate"
)

// byteSize is a pflag.Value that parses a human-readable byte size, such as
// "32MB", "64MiB" or "1GB", into *p. A zero p, or a value of zero or less,
// reads as generate's default size, so a flag that was never given still shows
// the size generate would use.
type byteSize struct{ p *int64 }

func (b byteSize) String() string {
	n := int64(generate.DefaultMultipartMaxMemory)
	if b.p != nil && *b.p > 0 {
		n = *b.p
	}
	return humanize.IBytes(uint64(n))
}

func (b byteSize) Set(v string) error {
	n, err := humanize.ParseBytes(v)
	if err != nil {
		return fmt.Errorf("invalid byte size %q: %w", v, err)
	}
	if n == 0 {
		return fmt.Errorf("multipart max memory must be positive, got %q", v)
	}
	if n > math.MaxInt64 {
		return fmt.Errorf("multipart max memory %q exceeds int64 maximum", v)
	}
	*b.p = int64(n)
	return nil
}

func (byteSize) Type() string { return "bytes" }

// spec declares one setting: what a person types, what it falls back to, and
// what it says in --help. It is one value per setting so a flag, its default
// and its deprecated spellings cannot drift apart.
type spec[T any] struct {
	// Name is the flag name, which muxt.json spells with underscores.
	Name  string
	Usage string
	// Default is the value when the setting is not given.
	Default T
	// Deprecated are the old flag names: each is registered hidden and marked
	// deprecated, and names its replacement in its usage.
	Deprecated []string
	// NonEmpty means a given empty value is an error, because the setting
	// names something.
	NonEmpty bool
	// Derived, when set, is the value of a setting that was not given. It
	// reads the configuration it was built over, so it only means something
	// after the settings it depends on have their values.
	Derived func() T
	// Example is a flag argument unlike Default, for tests that exercise
	// every setting the same way.
	Example string
}

// setting is one declaration bound to one configuration field. Its closures
// are built by the constructor for the field's kind, so no code outside a
// constructor switches on a kind, and a field of an unsupported kind does not
// compile.
type setting struct {
	name       string
	deprecated []string
	nonEmpty   bool
	// kind is the JSON type muxt.json gives the setting, set by the
	// constructor for the field's kind.
	kind composite.Kind
	// example is the spec's Example, kept so a test can give every setting a
	// value the same way.
	example string
	// field is the pointer the setting is bound to, kept so a test can prove
	// every configuration field is bound once. It is an any because the kinds
	// of field differ and nothing in production reads it back.
	field any

	setDefault         func()
	register           func(fs *pflag.FlagSet)
	registerDeprecated func(fs *pflag.FlagSet, spelling string)
	// arg renders the header arguments that record the value, none when it
	// is what a run would use without being told.
	arg func() []string
	// empty reports whether the value is empty, which matters only to a
	// NonEmpty setting.
	empty func() bool
	// derive writes the Derived value, nil when the setting has none.
	derive      func()
	reportValue func() any
	// decode sets the field from the JSON value of the setting's key.
	decode func(value json.RawMessage) error
	// encode returns the JSON value that records the field in a muxt.json.
	encode func() (json.RawMessage, error)
}

// deprecatedUsage is the usage of a deprecated spelling: it says what to write
// instead, then what the setting does.
func deprecatedUsage(name, usage string) string {
	return "DEPRECATED: use --" + name + " instead. " + usage
}

// untold is the value a run uses for a setting it was not given, which the
// header leaves out. A setting with Derived compares against Derived alone:
// a value equal to Default but not to Derived must be recorded, or the
// header reads back as the derived value.
func untold[T any](s spec[T]) T {
	if s.Derived != nil {
		return s.Derived()
	}
	return s.Default
}

func stringSetting(p *string, s spec[string]) setting {
	set := setting{
		kind:       composite.KindString,
		name:       s.Name,
		deprecated: s.Deprecated,
		nonEmpty:   s.NonEmpty,
		example:    s.Example,
		field:      p,
		setDefault: func() { *p = s.Default },
		register:   func(fs *pflag.FlagSet) { fs.StringVar(p, s.Name, s.Default, s.Usage) },
		registerDeprecated: func(fs *pflag.FlagSet, spelling string) {
			fs.StringVar(p, spelling, s.Default, deprecatedUsage(s.Name, s.Usage))
			markDeprecated(fs, spelling, s.Name)
		},
		arg: func() []string {
			if *p == untold(s) {
				return nil
			}
			return []string{"--" + s.Name + "=" + *p}
		},
		empty:       func() bool { return *p == "" },
		reportValue: func() any { return *p },
		decode:      func(value json.RawMessage) error { return json.Unmarshal(value, p) },
		encode:      func() (json.RawMessage, error) { return json.Marshal(*p) },
	}
	if s.Derived != nil {
		set.derive = func() { *p = s.Derived() }
	}
	return set
}

func boolSetting(p *bool, s spec[bool]) setting {
	set := setting{
		kind:       composite.KindBool,
		name:       s.Name,
		deprecated: s.Deprecated,
		nonEmpty:   s.NonEmpty,
		example:    s.Example,
		field:      p,
		setDefault: func() { *p = s.Default },
		register:   func(fs *pflag.FlagSet) { fs.BoolVar(p, s.Name, s.Default, s.Usage) },
		registerDeprecated: func(fs *pflag.FlagSet, spelling string) {
			fs.BoolVar(p, spelling, s.Default, deprecatedUsage(s.Name, s.Usage))
			markDeprecated(fs, spelling, s.Name)
		},
		arg: func() []string {
			switch {
			case *p == untold(s):
				return nil
			case *p:
				return []string{"--" + s.Name}
			default:
				return []string{"--" + s.Name + "=false"}
			}
		},
		empty:       func() bool { return false },
		reportValue: func() any { return *p },
		decode:      func(value json.RawMessage) error { return json.Unmarshal(value, p) },
		encode:      func() (json.RawMessage, error) { return json.Marshal(*p) },
	}
	if s.Derived != nil {
		set.derive = func() { *p = s.Derived() }
	}
	return set
}

// stringsSetting is a list of strings. The current name splits a value on
// commas; a deprecated spelling takes one value per flag, as it always did.
func stringsSetting(p *[]string, s spec[[]string]) setting {
	set := setting{
		kind:       composite.KindList,
		name:       s.Name,
		deprecated: s.Deprecated,
		nonEmpty:   s.NonEmpty,
		example:    s.Example,
		field:      p,
		setDefault: func() { *p = slices.Clone(s.Default) },
		register:   func(fs *pflag.FlagSet) { fs.StringSliceVar(p, s.Name, s.Default, s.Usage) },
		registerDeprecated: func(fs *pflag.FlagSet, spelling string) {
			fs.StringArrayVar(p, spelling, s.Default, deprecatedUsage(s.Name, s.Usage))
			markDeprecated(fs, spelling, s.Name)
		},
		arg: func() []string {
			if slices.Equal(*p, untold(s)) {
				return nil
			}
			args := make([]string, 0, len(*p))
			for _, v := range *p {
				args = append(args, "--"+s.Name+"="+v)
			}
			return args
		},
		empty:       func() bool { return slices.Contains(*p, "") },
		reportValue: func() any { return *p },
		decode: func(value json.RawMessage) error {
			// A list of its own, so the setting never aliases what it was
			// given or what an earlier decode left.
			var list []string
			if err := json.Unmarshal(value, &list); err != nil {
				return err
			}
			*p = list
			return nil
		},
		encode: func() (json.RawMessage, error) { return json.Marshal(*p) },
	}
	if s.Derived != nil {
		set.derive = func() { *p = s.Derived() }
	}
	return set
}

// bytesSetting is a size in bytes, parsed from the human-readable form and
// written in the header as exact decimal bytes. Zero means generate's own
// default.
func bytesSetting(p *int64, s spec[int64]) setting {
	set := setting{
		// A size is written as the text the flag parses.
		kind:       composite.KindString,
		name:       s.Name,
		deprecated: s.Deprecated,
		nonEmpty:   s.NonEmpty,
		example:    s.Example,
		field:      p,
		setDefault: func() { *p = s.Default },
		register:   func(fs *pflag.FlagSet) { fs.Var(byteSize{p: p}, s.Name, s.Usage) },
		registerDeprecated: func(fs *pflag.FlagSet, spelling string) {
			fs.Var(byteSize{p: p}, spelling, deprecatedUsage(s.Name, s.Usage))
			markDeprecated(fs, spelling, s.Name)
		},
		arg: func() []string {
			if *p == untold(s) {
				return nil
			}
			return []string{"--" + s.Name + "=" + strconv.FormatInt(*p, 10)}
		},
		empty:       func() bool { return false },
		reportValue: func() any { return byteSize{p: p}.String() },
		decode: func(value json.RawMessage) error {
			// The flag's own parser turns "64MB" into bytes, so the entry
			// and the command line accept the same values.
			var text string
			if err := json.Unmarshal(value, &text); err != nil {
				return err
			}
			return byteSize{p: p}.Set(text)
		},
		// Exact bytes: a rounded "61 MiB" would read back as another size.
		encode: func() (json.RawMessage, error) { return json.Marshal(strconv.FormatInt(*p, 10)) },
	}
	if s.Derived != nil {
		set.derive = func() { *p = s.Derived() }
	}
	return set
}

// receiverTypeSetting is the receiver type, which bare muxt and generate read.
func receiverTypeSetting(p *string) setting {
	return stringSetting(p, spec[string]{
		Name:       useReceiverType,
		Usage:      useReceiverTypeHelp,
		Deprecated: []string{deprecatedReceiverType, deprecatedFindReceiverType},
		Example:    "T",
	})
}

// receiverPackageSetting is the package that holds the receiver type.
func receiverPackageSetting(p *string) setting {
	return stringSetting(p, spec[string]{
		Name:       useReceiverTypePackage,
		Usage:      useReceiverTypePackageHelp,
		Deprecated: []string{deprecatedReceiverTypePackage, deprecatedFindReceiverTypePackage},
		Example:    "example.com/m/types",
	})
}

// setDefaults gives every setting its Default, so a configuration built with
// no flag parsing still starts where the flags would.
func setDefaults(settings []setting) {
	for _, s := range settings {
		s.setDefault()
	}
}

// register adds each setting's current name to fs. Registering sets the
// field to its default.
func register(fs *pflag.FlagSet, settings []setting) {
	for _, s := range settings {
		s.register(fs)
	}
}

// registerWithDeprecated is register, then each deprecated spelling, hidden
// and marked, so the old command lines keep working.
func registerWithDeprecated(fs *pflag.FlagSet, settings []setting) {
	register(fs, settings)
	for _, s := range settings {
		for _, spelling := range s.deprecated {
			s.registerDeprecated(fs, spelling)
		}
	}
}

// addDeprecatedTemplatesVariable registers --templates-variable, the one
// deprecated spelling that is not a setting of its own: it fills p, which the
// command reads when it finalizes the configuration, since an old command
// line may carry only this spelling.
func addDeprecatedTemplatesVariable(fs *pflag.FlagSet, p *string) {
	fs.StringVar(p, deprecatedTemplatesVariable, "", deprecatedUsage(useTemplatesVariable, useTemplatesVariableHelp))
	markDeprecated(fs, deprecatedTemplatesVariable, useTemplatesVariable)
}

// givenSettings maps a setting's name to the spellings that gave it, sorted.
// A setting absent from the map was not given.
type givenSettings map[string][]string

// has reports whether the setting called name was given by any spelling.
func (g givenSettings) has(name string) bool {
	_, ok := g[name]
	return ok
}

// spellings returns every spelling that gave a setting, sorted and without
// repeats.
func (g givenSettings) spellings() []string {
	var all []string
	for _, name := range slices.Sorted(maps.Keys(g)) {
		all = append(all, g[name]...)
	}
	slices.Sort(all)
	return slices.Compact(all)
}

// givenFlags returns the settings fs was given, by the spelling typed. A
// flag that is not a spelling of a setting, such as --verbose, is not
// configuration and is left out. --templates-variable is filed under the
// templates variable setting when the list has one.
func givenFlags(fs *pflag.FlagSet, settings []setting) givenSettings {
	given := givenSettings{}
	fs.Visit(func(f *pflag.Flag) {
		if name, ok := settingNamed(settings, f.Name); ok {
			given[name] = append(given[name], f.Name)
		}
	})
	for _, spellings := range given {
		slices.Sort(spellings)
	}
	return given
}

// settingNamed returns the name of the setting that spelling is the flag name
// or a deprecated spelling of.
func settingNamed(settings []setting, spelling string) (string, bool) {
	for _, s := range settings {
		if s.name == spelling || slices.Contains(s.deprecated, spelling) {
			return s.name, true
		}
		if spelling == deprecatedTemplatesVariable && s.name == useTemplatesVariable {
			return s.name, true
		}
	}
	return "", false
}

// derive writes the Derived value of each setting that has one and was not
// given, in list order, so a later setting sees the earlier ones.
func derive(settings []setting, g givenSettings) {
	for _, s := range settings {
		if s.derive != nil && !g.has(s.name) {
			s.derive()
		}
	}
}

// args returns the generated-file header arguments that record a finalized
// configuration, in list order, leaving out each setting that has the value a
// run would use anyway.
func args(settings []setting) []string {
	var out []string
	for _, s := range settings {
		out = append(out, s.arg()...)
	}
	return out
}

// givenKeys returns the settings an entry gives: each setting whose key the
// entry holds, by its own name, the same as a command line that typed the
// current flag. composite drops a null at read, so a key present is given.
func givenKeys(e composite.Entry, settings []setting) givenSettings {
	given := givenSettings{}
	for _, s := range settings {
		if _, ok := e[composite.Key(s.name)]; ok {
			given[s.name] = []string{s.name}
		}
	}
	return given
}

// decode sets the field of each setting whose key e holds. A value the
// setting's parser rejects is a *composite.ValueError naming the key.
func decode(settings []setting, e composite.Entry) error {
	for _, s := range settings {
		key := composite.Key(s.name)
		value, ok := e[key]
		if !ok {
			continue
		}
		if err := s.decode(value); err != nil {
			return &composite.ValueError{Key: key, Err: err}
		}
	}
	return nil
}

// encode returns the entry that records each setting g gives, under its key,
// with the value the field holds. A setting given by a deprecated spelling is
// recorded under its current key, since the file has no other spelling.
func encode(settings []setting, g givenSettings) (composite.Entry, error) {
	e := composite.Entry{}
	for _, s := range settings {
		if !g.has(s.name) {
			continue
		}
		value, err := s.encode()
		if err != nil {
			return nil, fmt.Errorf("encoding %s: %w", composite.Key(s.name), err)
		}
		e[composite.Key(s.name)] = value
	}
	return e, nil
}
