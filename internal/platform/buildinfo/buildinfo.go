// Package buildinfo exposes immutable build metadata injected by the build pipeline.
package buildinfo

// Values are variables so release builds can set them with -ldflags -X.
var (
	version = "dev"
	commit  = "unknown"
	builtAt = "unknown"
)

// Info describes the source and build identity of a running binary.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"built_at"`
}

// Current returns the build identity embedded in the current binary.
func Current() Info {
	return Info{
		Version: version,
		Commit:  commit,
		BuiltAt: builtAt,
	}
}
