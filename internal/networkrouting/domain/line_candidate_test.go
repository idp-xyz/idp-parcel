package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

func TestALineCandidateIdentifierGivesBackItsLineAndVersion(t *testing.T) {
	for _, tc := range []struct {
		line    string
		version int32
	}{
		{"SYN-LINE-CN-SG-01", 1},
		{"SYN-LINE-XA-XB", 37},
		{"LINE@WITH@SEPARATOR", 2},
	} {
		id, err := domain.NewLineCandidateID(tc.line, tc.version)
		if err != nil {
			t.Fatal(err)
		}
		code, version, ok := id.LineVersion()
		if !ok || code != tc.line || version != tc.version {
			t.Fatalf("%s 解出 %q 版本 %d（ok=%v），想要 %q 版本 %d", id, code, version, ok, tc.line, tc.version)
		}
	}
}

func TestAnIdentifierNotMintedAsALineCandidateHasNoLineVersion(t *testing.T) {
	for _, raw := range []string{
		"SYN-LINE-CN-SG-01/1", "SYN-LINE-CN-SG-01", "@1", " @1", "SYN-LINE-CN-SG-01@", "SYN-LINE-CN-SG-01@v1",
		"SYN-LINE-CN-SG-01@01", "SYN-LINE-CN-SG-01@+1", "SYN-LINE-CN-SG-01@0", "SYN-LINE-CN-SG-01@-1",
	} {
		id, err := domain.NewCandidateID(raw)
		if err != nil {
			t.Fatal(err)
		}
		if code, version, ok := id.LineVersion(); ok {
			t.Fatalf("%q 不是首版形态铸的标识，却解出 %q 版本 %d", raw, code, version)
		}
	}
}

func TestALineCandidateNeedsALineCodeAndAPositiveVersion(t *testing.T) {
	for _, tc := range []struct {
		line    string
		version int32
	}{
		{"", 1},
		{"  ", 1},
		{"SYN-LINE-CN-SG-01", 0},
		{"SYN-LINE-CN-SG-01", -1},
	} {
		if id, err := domain.NewLineCandidateID(tc.line, tc.version); !errors.Is(err, domain.ErrInvalidLineCandidate) {
			t.Fatalf("线路 %q 版本 %d 铸出 %q（err=%v），想要 ErrInvalidLineCandidate", tc.line, tc.version, id, err)
		}
	}
}
