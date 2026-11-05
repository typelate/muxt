package overlay

// Shim is the html/template shim's source.
var Shim = shim

// ModuleVersion is the github.com/typelate/muxt version for build info.
var ModuleVersion = moduleVersion

// Requirement is how the modfile requires github.com/typelate/muxt.
type Requirement = requirement

// WriteModFile writes the modfile copy of a test target's go.mod.
var WriteModFile = writeModFile

// ProbeRefusal is why the probe's go list failed, with muxt required at
// version and not replaced.
func ProbeRefusal(err error, version string) error {
	return probeRefusal(err, requirement{Version: version})
}
