package overlay

// Shim is the html/template shim's source.
var Shim = shim

// RequirementFor is how the modfile requires github.com/typelate/htt, given
// build information and the engine's source file.
var RequirementFor = requirementFor

// EngineFile is the path the compiler recorded for the engine's source.
var EngineFile = engineFile

// Requirement is how the modfile requires github.com/typelate/htt.
type Requirement = requirement

// WriteModFile writes the modfile copy of a test target's go.mod.
var WriteModFile = writeModFile

// ProbeRefusal is why the probe's go list failed, given how htt is required.
var ProbeRefusal = probeRefusal
