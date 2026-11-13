package cli

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toy is a configuration of one field of each kind a setting binds.
type toy struct {
	Name  string
	Route string
	Loud  bool
	Quiet bool
	Vars  []string
	Limit int64
}

// toySettings declares each field of c, in the order the header lists them.
func toySettings(c *toy) []setting {
	return []setting{
		stringsSetting(&c.Vars, spec[[]string]{Name: useTemplatesVariable, Default: []string{"templates"}, NonEmpty: true, Deprecated: []string{deprecatedFindTemplatesVariable}, Usage: "variables"}),
		stringSetting(&c.Name, spec[string]{Name: "name", Default: "Routes", NonEmpty: true, Deprecated: []string{"old-name"}, Usage: "the name"}),
		stringSetting(&c.Route, spec[string]{Name: "route", Derived: func() string { return c.Name + "Route" }, Usage: "the route"}),
		boolSetting(&c.Loud, spec[bool]{Name: "loud", Usage: "loud"}),
		boolSetting(&c.Quiet, spec[bool]{Name: "quiet", Default: true, Usage: "quiet"}),
		bytesSetting(&c.Limit, spec[int64]{Name: "limit", Usage: "limit"}),
	}
}

// parsedToy registers settings on a fresh flag set, parses commandLine, and
// returns the configuration, the flag set and the settings.
func parsedToy(t *testing.T, commandLine ...string) (*toy, *pflag.FlagSet, []setting) {
	t.Helper()
	var c toy
	settings := toySettings(&c)
	fs := pflag.NewFlagSet("toy", pflag.ContinueOnError)
	registerWithDeprecated(fs, settings)
	require.NoError(t, fs.Parse(commandLine), "parsing %v", commandLine)
	return &c, fs, settings
}

func TestSetDefaults(t *testing.T) {
	var c toy
	setDefaults(toySettings(&c))

	assert.Equal(t, toy{Name: "Routes", Quiet: true, Vars: []string{"templates"}}, c, "defaults")
}

func TestRegisterSetsDefaults(t *testing.T) {
	c, _, _ := parsedToy(t)

	assert.Equal(t, toy{Name: "Routes", Quiet: true, Vars: []string{"templates"}}, *c, "configuration after parsing no flags")
}

func TestRegisterAddsCurrentNamesOnly(t *testing.T) {
	var c toy
	fs := pflag.NewFlagSet("toy", pflag.ContinueOnError)
	register(fs, toySettings(&c))

	assert.NotNil(t, fs.Lookup("name"), "name")
	assert.Nil(t, fs.Lookup("old-name"), "old-name")
	assert.Nil(t, fs.Lookup(deprecatedFindTemplatesVariable), "find-templates-variable")
}

func TestRegisterWithDeprecatedHidesOldSpellings(t *testing.T) {
	_, fs, _ := parsedToy(t)

	old := fs.Lookup("old-name")
	require.NotNil(t, old, "old-name")
	assert.True(t, old.Hidden, "hidden")
	assert.Equal(t, "use --name instead", old.Deprecated, "deprecation message")
	assert.Equal(t, "DEPRECATED: use --name instead. the name", old.Usage, "usage")
	assert.Equal(t, "Routes", old.DefValue, "default")
	assert.Equal(t, "stringArray", fs.Lookup(deprecatedFindTemplatesVariable).Value.Type(), "a deprecated list spelling takes one value per flag")
	assert.Equal(t, "stringSlice", fs.Lookup(useTemplatesVariable).Value.Type(), "the current list spelling splits on commas")
	assert.Equal(t, "32 MiB", fs.Lookup("limit").DefValue, "a byte size shows generate's default")
}

func TestRegisteredSpellingsWriteTheField(t *testing.T) {
	c, _, _ := parsedToy(t, "--old-name=Legacy", "--use-templates-variable=a,b", "--loud", "--quiet=false", "--limit=64MB")

	assert.Equal(t, toy{Name: "Legacy", Vars: []string{"a", "b"}, Loud: true, Limit: 64_000_000}, *c, "configuration")
}

func TestArgsRecordWhatDiffersFromTheDefault(t *testing.T) {
	for _, tt := range []struct {
		name        string
		commandLine []string
		want        []string
	}{
		{name: "nothing given", want: nil},
		{name: "a string whose derived setting follows it", commandLine: []string{"--name=Other"}, want: []string{"--name=Other"}},
		{name: "a derived string differing from what it derives", commandLine: []string{"--route=Mine"}, want: []string{"--route=Mine"}},
		{name: "a derived string given its default rather than what it derives", commandLine: []string{"--route="}, want: []string{"--route="}},
		{name: "a switch turned on", commandLine: []string{"--loud"}, want: []string{"--loud"}},
		{name: "a switch that defaults on turned off", commandLine: []string{"--quiet=false"}, want: []string{"--quiet=false"}},
		{name: "a list, one argument each", commandLine: []string{"--use-templates-variable=a,b"}, want: []string{"--use-templates-variable=a", "--use-templates-variable=b"}},
		{name: "a byte size, as exact bytes", commandLine: []string{"--limit=64MB"}, want: []string{"--limit=64000000"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, fs, settings := parsedToy(t, tt.commandLine...)
			derive(settings, givenFlags(fs, settings))
			assert.Equal(t, tt.want, args(settings), "args after %v, configuration %+v", tt.commandLine, c)
		})
	}
}

func TestArgsFollowListOrder(t *testing.T) {
	_, fs, settings := parsedToy(t, "--limit=1KB", "--loud", "--name=N", "--use-templates-variable=v")
	derive(settings, givenFlags(fs, settings))

	assert.Equal(t, []string{"--use-templates-variable=v", "--name=N", "--loud", "--limit=1000"}, args(settings))
}

func TestDeriveKeepsAGivenValue(t *testing.T) {
	c, fs, settings := parsedToy(t, "--name=N", "--route=Mine")

	derive(settings, givenFlags(fs, settings))

	assert.Equal(t, "Mine", c.Route, "route given")
}

func TestDeriveFillsWhatWasNotGiven(t *testing.T) {
	c, fs, settings := parsedToy(t, "--name=N")

	derive(settings, givenFlags(fs, settings))

	assert.Equal(t, "NRoute", c.Route, "route derived from the name given")
}

func TestGivenFlagsNamesTheSettingForEverySpelling(t *testing.T) {
	_, fs, settings := parsedToy(t, "--old-name=A", "--name=B", "--loud", "--find-templates-variable=x")

	got := givenFlags(fs, settings)

	assert.Equal(t, givenSettings{
		"name":               {"name", "old-name"},
		"loud":               {"loud"},
		useTemplatesVariable: {deprecatedFindTemplatesVariable},
	}, got, "given")
	assert.True(t, got.has("name"), "has name")
	assert.False(t, got.has("quiet"), "has quiet")
	assert.Equal(t, []string{"find-templates-variable", "loud", "name", "old-name"}, got.spellings(), "spellings")
}

func TestGivenFlagsLeavesOutWhatIsNotASetting(t *testing.T) {
	var c toy
	settings := toySettings(&c)
	fs := pflag.NewFlagSet("toy", pflag.ContinueOnError)
	registerWithDeprecated(fs, settings)
	fs.Bool("verbose", false, "")
	require.NoError(t, fs.Parse([]string{"--verbose", "--name=N"}))

	assert.Equal(t, givenSettings{"name": {"name"}}, givenFlags(fs, settings))
}

func TestGivenFlagsFilesTheHandSpellingUnderTheTemplatesVariable(t *testing.T) {
	var c toy
	var deprecated string
	settings := toySettings(&c)
	fs := pflag.NewFlagSet("toy", pflag.ContinueOnError)
	registerWithDeprecated(fs, settings)
	addDeprecatedTemplatesVariable(fs, &deprecated)
	require.NoError(t, fs.Parse([]string{"--templates-variable", "x"}))

	got := givenFlags(fs, settings)

	assert.Equal(t, givenSettings{useTemplatesVariable: {deprecatedTemplatesVariable}}, got, "given")
	assert.Equal(t, "x", deprecated, "the hand spelling fills its own variable")
	hidden := fs.Lookup(deprecatedTemplatesVariable)
	assert.True(t, hidden.Hidden, "hidden")
	assert.Equal(t, "use --use-templates-variable instead", hidden.Deprecated, "deprecation message")
}

func TestSettingEmptyAndReportValue(t *testing.T) {
	c, _, settings := parsedToy(t, "--use-templates-variable=a,", "--limit=64MiB")
	byName := map[string]setting{}
	for _, s := range settings {
		byName[s.name] = s
	}

	assert.True(t, byName[useTemplatesVariable].empty(), "a list with an empty element")
	assert.False(t, byName["name"].empty(), "a name")
	c.Name = ""
	assert.True(t, byName["name"].empty(), "an empty name")
	assert.Equal(t, []string{"a", ""}, byName[useTemplatesVariable].reportValue(), "list")
	assert.Equal(t, "64 MiB", byName["limit"].reportValue(), "bytes")
	assert.Equal(t, true, byName["quiet"].reportValue(), "bool")
	assert.True(t, byName[useTemplatesVariable].nonEmpty, "nonEmpty")
	assert.False(t, byName["route"].nonEmpty, "not nonEmpty")
}

func TestReceiverSettings(t *testing.T) {
	var typeName, pkg string
	typ, pk := receiverTypeSetting(&typeName), receiverPackageSetting(&pkg)

	assert.Equal(t, useReceiverType, typ.name, "type name")
	assert.Equal(t, []string{deprecatedReceiverType, deprecatedFindReceiverType}, typ.deprecated, "type deprecated")
	assert.Equal(t, useReceiverTypePackage, pk.name, "package name")
	assert.Equal(t, []string{deprecatedReceiverTypePackage, deprecatedFindReceiverTypePackage}, pk.deprecated, "package deprecated")

	fs := pflag.NewFlagSet("toy", pflag.ContinueOnError)
	registerWithDeprecated(fs, []setting{typ, pk})
	require.NoError(t, fs.Parse([]string{"--find-receiver-type=T", "--receiver-type-package=p"}))
	assert.Equal(t, "T", typeName, "type")
	assert.Equal(t, "p", pkg, "package")
	assert.Equal(t, []string{"--use-receiver-type=T", "--use-receiver-type-package=p"}, args([]setting{typ, pk}), "args")
}
