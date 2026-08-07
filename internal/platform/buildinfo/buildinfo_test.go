package buildinfo

import "testing"

func TestCurrentHasDevelopmentDefaults(t *testing.T) {
	info := Current()
	if info.Version == "" || info.Commit == "" || info.BuiltAt == "" {
		t.Fatalf("Current() returned empty build metadata: %#v", info)
	}
}
