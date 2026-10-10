package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

func payloadInstant() time.Time {
	return time.Date(2026, time.March, 5, 10, 11, 12, 345678901, time.FixedZone("CST", 8*3600))
}

// 承接决定口的 NOC-1 文档。字面量独立于实现：摘要是这串 JSON 的 SHA-256，前缀带形状版本。
const acceptancePayloadDocument = `{"canonicalization":"NOC-1","face":"ACCEPT_COLLABORATION","decision":"ACCEPTED","node":"NODE-1","authority":"AUTH-1","basis":"BASIS-1","accepted_units":["UNIT-A","UNIT-B"],"accepted_actions":["ISOLATE","UNSEAL"]}`

func TestAcceptancePayloadRoundTripsAsNOC1(t *testing.T) {
	document, digest, err := domain.CanonicalizeAcceptancePayload(
		domain.CollaborationAccepted,
		"NODE-1",
		"AUTH-1",
		"BASIS-1",
		[]string{"UNIT-B", "UNIT-A"},
		[]domain.CollaborationActionKind{domain.UnsealAction, domain.IsolateAction},
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(document) != acceptancePayloadDocument {
		t.Fatalf("document = %s", document)
	}
	if digest != "NOC-1:9938edb9fffaab6f67866d4ed949ee826323c990a1a5ffc322c9b7f3ad19b430" {
		t.Fatalf("digest = %s", digest)
	}
	var decoded struct {
		Canonicalization string   `json:"canonicalization"`
		Face             string   `json:"face"`
		Decision         string   `json:"decision"`
		Node             string   `json:"node"`
		Authority        string   `json:"authority"`
		Basis            string   `json:"basis"`
		AcceptedUnits    []string `json:"accepted_units"`
		AcceptedActions  []string `json:"accepted_actions"`
	}
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Canonicalization != "NOC-1" || decoded.Face != "ACCEPT_COLLABORATION" || decoded.Decision != "ACCEPTED" ||
		decoded.Node != "NODE-1" || decoded.Authority != "AUTH-1" || decoded.Basis != "BASIS-1" ||
		len(decoded.AcceptedUnits) != 2 || decoded.AcceptedUnits[0] != "UNIT-A" || decoded.AcceptedUnits[1] != "UNIT-B" ||
		len(decoded.AcceptedActions) != 2 || decoded.AcceptedActions[0] != "ISOLATE" || decoded.AcceptedActions[1] != "UNSEAL" {
		t.Fatalf("decoded = %+v", decoded)
	}
}

const executionPayloadDocument = `{"canonicalization":"NOC-1","face":"RECORD_EXECUTION","evidence":"EV-1","performed_at":"2026-03-05T02:11:12.345678901Z"}`

func TestExecutionPayloadRoundTripsAsNOC1(t *testing.T) {
	document, digest, err := domain.CanonicalizeExecutionPayload("EV-1", payloadInstant())
	if err != nil {
		t.Fatal(err)
	}
	if string(document) != executionPayloadDocument {
		t.Fatalf("document = %s", document)
	}
	if digest != "NOC-1:5c2fa49ed53cd4f27c66349d94f4fd06284895c92b3b2b91ce4a8f64d319ca83" {
		t.Fatalf("digest = %s", digest)
	}
	var decoded struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Evidence         string `json:"evidence"`
		PerformedAt      string `json:"performed_at"`
	}
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Canonicalization != "NOC-1" || decoded.Face != "RECORD_EXECUTION" || decoded.Evidence != "EV-1" ||
		decoded.PerformedAt != "2026-03-05T02:11:12.345678901Z" {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func nocSource(t *testing.T) domain.WorkFactSource {
	t.Helper()
	source, err := domain.NewWorkFactSource(
		"SRC-1",
		mustValue(t, domain.NewPerformingPartyReference, "PARTY-1"),
		mustValue(t, domain.NewExecutionEvidenceReference, "EV-1"),
		payloadInstant(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestConsolidationPayloadsRoundTripAsNOC1(t *testing.T) {
	source := nocSource(t)
	unit := mustValue(t, domain.NewConsolidationUnitID, "UNIT-1")
	cases := []struct {
		name     string
		face     string
		document string
		digest   string
		got      func() ([]byte, string, error)
	}{
		{
			name:     "open",
			face:     "OPEN_UNIT",
			document: `{"canonicalization":"NOC-1","face":"OPEN_UNIT","action":"OPEN_UNIT","performed_by":"PARTY-1","evidence":"EV-1","occurred_at":"2026-03-05T02:11:12.345678901Z","unit":"UNIT-1","asset":"ASSET-1"}`,
			digest:   "NOC-1:01ebaa0548682a10603c50a4e4b37b78b47a9b7a7fe8f6f12d43e3f68286483e",
			got: func() ([]byte, string, error) {
				return domain.CanonicalizeOpenUnitPayload(source, unit, mustValue(t, domain.NewCarrierAssetReference, "ASSET-1"))
			},
		},
		{
			name:     "add",
			face:     "ADD_MEMBER",
			document: `{"canonicalization":"NOC-1","face":"ADD_MEMBER","action":"ADD_MEMBER","performed_by":"PARTY-1","evidence":"EV-1","occurred_at":"2026-03-05T02:11:12.345678901Z","unit":"UNIT-1","member":"MEMBER-1"}`,
			digest:   "NOC-1:cd5779135aface6649e749c858b3c0ad877a7890ab51dd3481f22eff594bfd3b",
			got: func() ([]byte, string, error) {
				return domain.CanonicalizeAddMemberPayload(source, unit, mustValue(t, domain.NewHandlingUnitID, "MEMBER-1"))
			},
		},
		{
			name:     "remove",
			face:     "REMOVE_MEMBER",
			document: `{"canonicalization":"NOC-1","face":"REMOVE_MEMBER","action":"REMOVE_MEMBER","performed_by":"PARTY-1","evidence":"EV-1","occurred_at":"2026-03-05T02:11:12.345678901Z","unit":"UNIT-1","member":"MEMBER-1"}`,
			digest:   "NOC-1:9fa544cfdaace36997611c193836089bae50d0292ced9d8771a0714f092ad4c0",
			got: func() ([]byte, string, error) {
				return domain.CanonicalizeRemoveMemberPayload(source, unit, mustValue(t, domain.NewHandlingUnitID, "MEMBER-1"))
			},
		},
		{
			name:     "seal",
			face:     "SEAL_UNIT",
			document: `{"canonicalization":"NOC-1","face":"SEAL_UNIT","action":"SEAL_UNIT","performed_by":"PARTY-1","evidence":"EV-1","occurred_at":"2026-03-05T02:11:12.345678901Z","unit":"UNIT-1","seal":"SEAL-1","basis":"BASIS-1"}`,
			digest:   "NOC-1:a2b90e6a64eeb2a48116facf492a869edd180a5b9e0524d55ebe07b3e375f1eb",
			got: func() ([]byte, string, error) {
				return domain.CanonicalizeSealUnitPayload(source, unit, mustValue(t, domain.NewSealReference, "SEAL-1"), mustValue(t, domain.NewWorkBasisReference, "BASIS-1"))
			},
		},
		{
			name:     "unseal",
			face:     "UNSEAL_UNIT",
			document: `{"canonicalization":"NOC-1","face":"UNSEAL_UNIT","action":"UNSEAL_UNIT","performed_by":"PARTY-1","evidence":"EV-1","occurred_at":"2026-03-05T02:11:12.345678901Z","unit":"UNIT-1","basis":"BASIS-1"}`,
			digest:   "NOC-1:bfce2252d640ca91c46d44b4b7c9359988cf35668f7fd8c61a61cdded8a5ecc0",
			got: func() ([]byte, string, error) {
				return domain.CanonicalizeUnsealUnitPayload(source, unit, mustValue(t, domain.NewWorkBasisReference, "BASIS-1"))
			},
		},
		{
			name:     "close",
			face:     "CLOSE_UNIT",
			document: `{"canonicalization":"NOC-1","face":"CLOSE_UNIT","action":"CLOSE_UNIT","performed_by":"PARTY-1","evidence":"EV-1","occurred_at":"2026-03-05T02:11:12.345678901Z","unit":"UNIT-1","disposition":"DISP-1"}`,
			digest:   "NOC-1:ae46109e5d5592ae031c5ae7ab796d3d1f511944fb366558ea199ee4e4a75a96",
			got: func() ([]byte, string, error) {
				return domain.CanonicalizeCloseUnitPayload(source, unit, mustValue(t, domain.NewWorkBasisReference, "DISP-1"))
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			document, digest, err := testCase.got()
			if err != nil {
				t.Fatal(err)
			}
			if string(document) != testCase.document {
				t.Fatalf("document = %s", document)
			}
			if digest != testCase.digest {
				t.Fatalf("digest = %s", digest)
			}
			var decoded map[string]string
			if err := json.Unmarshal(document, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded["canonicalization"] != "NOC-1" || decoded["face"] != testCase.face || decoded["unit"] != "UNIT-1" || decoded["performed_by"] != "PARTY-1" {
				t.Fatalf("decoded = %+v", decoded)
			}
		})
	}
}

const deliveryPayloadDocument = `{"canonicalization":"NOC-1","face":"RECEIVE_DELIVERED_UNIT","unit":"UNIT-1","mark":"MARK-1","claim":"1","node":"NODE-1","occurred_at":"2026-03-05T02:11:12.345678901Z"}`

func TestDeliveryPayloadRoundTripsAsNOC1(t *testing.T) {
	document, digest, err := domain.CanonicalizeDeliveryPayload(
		mustValue(t, domain.NewHandlingUnitID, "UNIT-1"),
		"MARK-1",
		1,
		mustValue(t, domain.NewNodeReference, "NODE-1"),
		payloadInstant(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(document) != deliveryPayloadDocument {
		t.Fatalf("document = %s", document)
	}
	if digest != "NOC-1:5770380d4f333191f5fcf170f0c48e35e74c7b23e4f6008f539df9c2fb74b5da" {
		t.Fatalf("digest = %s", digest)
	}
	var decoded struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Unit             string `json:"unit"`
		Mark             string `json:"mark"`
		Claim            string `json:"claim"`
		Node             string `json:"node"`
		OccurredAt       string `json:"occurred_at"`
	}
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Face != "RECEIVE_DELIVERED_UNIT" || decoded.Unit != "UNIT-1" || decoded.Mark != "MARK-1" ||
		decoded.Claim != "1" || decoded.Node != "NODE-1" || decoded.OccurredAt != "2026-03-05T02:11:12.345678901Z" {
		t.Fatalf("decoded = %+v", decoded)
	}
}
