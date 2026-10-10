package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

func cccInstant() time.Time {
	return time.Date(2026, time.March, 5, 10, 11, 12, 345678901, time.FixedZone("CST", 8*3600))
}

func TestCommandPayloadsRoundTripAsCCC1(t *testing.T) {
	at := cccInstant()
	cases := []struct {
		name     string
		document string
		digest   string
		call     func() ([]byte, string, error)
	}{
		{
			name:     "external result",
			document: `{"canonicalization":"CCC-1","face":"RECEIVE_EXTERNAL_RESULT","layer":"RELEASE_RESULT","raw_semantics":"RAW","claimed_version":"v1","attempt":2,"scope":"SCOPE","occurred_at":"2026-03-05T02:11:12.345678901Z","release":{"kind":"FULL","authority":"AUTH","condition":"COND"}}`,
			digest:   "CCC-1:586593ca999d8f96caf94c3f2bb4e3b58b9770e4948bca776742db72a8dd8814",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeExternalResultPayload(domain.ReleaseResultLayer, "RAW", "v1", 2, "SCOPE", at, &domain.ExternalReleaseContent{
					Kind: domain.FullRelease, Authority: "AUTH", Condition: "COND",
				})
			},
		},
		{
			name:     "external result without release",
			document: `{"canonicalization":"CCC-1","face":"RECEIVE_EXTERNAL_RESULT","layer":"REGULATORY_RECEIPT","raw_semantics":"RAW","claimed_version":"v1","attempt":2,"scope":"SCOPE","occurred_at":"2026-03-05T02:11:12.345678901Z","release":null}`,
			digest:   "CCC-1:b909d924e04e9a9ca3152c09d7044ecd79156001db2222fa3993e102c64588be",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeExternalResultPayload(domain.RegulatoryReceiptLayer, "RAW", "v1", 2, "SCOPE", at, nil)
			},
		},
		{
			name:     "declaration",
			document: `{"canonicalization":"CCC-1","face":"SUBMIT_DECLARATION","procedure":"PROC","dossier":"DOS","roles":"ROLES","members":["M-A","M-B"]}`,
			digest:   "CCC-1:00bb6e71b1e978fa9662e7ef19a2b467fd77f35a72805c128fad955f74becf03",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeDeclarationPayload("PROC", "DOS", "ROLES", []string{"M-B", "M-A"})
			},
		},
		{
			name:     "duty verification",
			document: `{"canonicalization":"CCC-1","face":"VERIFY_DUTY_PAYMENT","coverage":"COVERED","delta":"NO_DELTA","validity":"VALID","basis":"BASIS","procedure":"PROC","funds_version":"FV-1"}`,
			digest:   "CCC-1:4815c99da86ec41f7b6565fcad8fed149a748830fecc5d9cf7f74a897eb74de8",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeDutyVerificationPayload(domain.CoverageFull, domain.DeltaNone, domain.FundsFactValid, " BASIS ", " PROC ", " FV-1 ")
			},
		},
		{
			name:     "disposition facts",
			document: `{"canonicalization":"CCC-1","face":"VERIFY_DISPOSITION","facts":["FACT-A","FACT-B"]}`,
			digest:   "CCC-1:ebcfe7ed502079fc48a153fed71af5c5eeb231f6d0608a980811145795dead58",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeDispositionFactSetPayload([]string{"FACT-B", "FACT-A"})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			document, digest, err := tc.call()
			if err != nil {
				t.Fatalf("canonicalize: %v", err)
			}
			if string(document) != tc.document {
				t.Fatalf("document\n got %s\nwant %s", document, tc.document)
			}
			if digest != tc.digest {
				t.Fatalf("digest %s, want %s", digest, tc.digest)
			}
			var roundTrip map[string]any
			if err := json.Unmarshal(document, &roundTrip); err != nil {
				t.Fatalf("round trip: %v", err)
			}
			if roundTrip["canonicalization"] != "CCC-1" {
				t.Fatalf("canonicalization %v", roundTrip["canonicalization"])
			}
		})
	}
}
