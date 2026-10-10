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

// canonicalCredential 经领域登记构造出一张凭证（期限一天），再定形。额度为零即来源未提供。
func canonicalCredential(from time.Time, uses int) ([]byte, string, error) {
	id, err := domain.NewCredentialID("CRED")
	if err != nil {
		return nil, "", err
	}
	issuer, err := domain.NewRegulatoryAuthorityReference("AUTH")
	if err != nil {
		return nil, "", err
	}
	holder, err := domain.NewCredentialHolderReference("HOLDER")
	if err != nil {
		return nil, "", err
	}
	procedure, err := domain.NewCustomsProcedureReference("PROC")
	if err != nil {
		return nil, "", err
	}
	credential, err := domain.RegisterCredential(id, issuer, holder, procedure, from, from.Add(24*time.Hour), uses)
	if err != nil {
		return nil, "", err
	}
	return domain.CanonicalizeCredentialRegistrationPayload(credential)
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
		{
			name:     "credential registration",
			document: `{"canonicalization":"CCC-1","face":"REGISTER_CREDENTIAL","issuer":"AUTH","holder":"HOLDER","procedure":"PROC","valid_from":"2026-03-05T02:11:12.345678901Z","valid_to":"2026-03-06T02:11:12.345678901Z","uses":12}`,
			digest:   "CCC-1:d982368033045e3bfc00d1c0659d5cc34dbac6ca84e61654213f351e37be595c",
			call:     func() ([]byte, string, error) { return canonicalCredential(at, 12) },
		},
		{
			name:     "credential registration without a uses quota",
			document: `{"canonicalization":"CCC-1","face":"REGISTER_CREDENTIAL","issuer":"AUTH","holder":"HOLDER","procedure":"PROC","valid_from":"2026-03-05T02:11:12.345678901Z","valid_to":"2026-03-06T02:11:12.345678901Z","uses":null}`,
			digest:   "CCC-1:fe6795bc8bcc404dc86a64e5a005411d2366e60e5c3ab243949da9f4664eb041",
			call:     func() ([]byte, string, error) { return canonicalCredential(at, 0) },
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

// Covers: ADR-0014「摘要只在同一规范化版本内可比」——带 CCC-1 前缀的与本次 CCC-1 摘要比，不带前缀的
// 与无版本那一版比，认不出的前缀既不是重放也不是冲突。
func TestStoredDigestIsComparedWithinItsOwnShape(t *testing.T) {
	const current, unversioned = "CCC-1:aa", "bb"
	cases := []struct {
		name   string
		stored string
		want   domain.StoredDigestComparison
	}{
		{"CCC-1 same content", "CCC-1:aa", domain.SamePayload},
		{"CCC-1 different content", "CCC-1:cc", domain.DifferentPayload},
		{"unversioned same content", "bb", domain.SamePayload},
		{"unversioned different content", "cc", domain.DifferentPayload},
		{"an unversioned digest is not read as CCC-1", "aa", domain.DifferentPayload},
		{"a later shape", "CCC-2:aa", domain.UnknownPayloadShape},
		{"another context's shape", "TFC-1:bb", domain.UnknownPayloadShape},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := domain.CompareStoredDigest(testCase.stored, current, unversioned); got != testCase.want {
				t.Fatalf("CompareStoredDigest(%q) = %d, want %d", testCase.stored, got, testCase.want)
			}
		})
	}
}
