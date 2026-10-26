package registrationjson_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/adapters/registrationjson"
)

const endingBatch = `{
  "tenantId": "SYN-TENANT-01",
  "revocations": [{"relationshipId": "SYN-REL-01", "revision": 2, "basis": "SYN-REVOCATION/01", "at": "2026-07-01T00:00:00Z"}],
  "expirations": [{"relationshipId": "SYN-REL-02", "revision": 3, "basis": "SYN-TERM-ENDED/02", "at": "2026-06-01T00:00:00Z"}],
  "supersessions": [{"relationshipId": "SYN-REL-03", "revision": 2, "successor": "SYN-PARTY-02", "at": "2026-07-01T00:00:00Z",
    "successorRelationshipId": "SYN-REL-03-NEXT", "basis": "SYN-HANDOVER/03",
    "approval": {"reference": "SYN-APPROVAL/03", "approvedAt": "2026-06-30T00:00:00Z"}}]
}`

// Covers: 票 17「受控批量口同形加三种」的形状半边——同一份批文三节各译成对应命令，逐格不丢；替代缺批准、缺接替关系标识、
// 撤销缺依据都在翻译这一步拒，并点名是哪一格。
func TestPartyRelationshipEndingItemsTranslateIntoTheirCommands(t *testing.T) {
	var document registrationjson.PartyRelationshipEndingBatchDocument
	if err := json.Unmarshal([]byte(endingBatch), &document); err != nil {
		t.Fatalf("decode: %v", err)
	}
	tenant, err := registrationjson.DocumentTenant(document.TenantID)
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	if document.ItemCount() != 3 {
		t.Fatalf("item count = %d, want 3", document.ItemCount())
	}

	revoke, err := registrationjson.PartyRelationshipRevocationCommand(tenant, document.Revocations[0])
	if err != nil || revoke.Tenant != tenant || revoke.ID.String() != "SYN-REL-01" || revoke.Revision != 2 ||
		revoke.Basis.String() != "SYN-REVOCATION/01" || !revoke.At.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("revocation = (%+v, %v)", revoke, err)
	}
	expire, err := registrationjson.PartyRelationshipExpirationCommand(tenant, document.Expirations[0])
	if err != nil || expire.ID.String() != "SYN-REL-02" || expire.Revision != 3 || expire.Basis.String() != "SYN-TERM-ENDED/02" {
		t.Fatalf("expiration = (%+v, %v)", expire, err)
	}
	supersede, err := registrationjson.PartyRelationshipSupersessionCommand(tenant, document.Supersessions[0])
	if err != nil || supersede.Successor.String() != "SYN-PARTY-02" || supersede.SuccessorRelationship.String() != "SYN-REL-03-NEXT" ||
		supersede.Basis.String() != "SYN-HANDOVER/03" || supersede.Approval.Reference.String() != "SYN-APPROVAL/03" ||
		!supersede.Approval.ApprovedAt.Equal(time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("supersession = (%+v, %v)", supersede, err)
	}

	noApproval := document.Supersessions[0]
	noApproval.Approval = nil
	noSuccessorID := document.Supersessions[0]
	noSuccessorID.SuccessorRelationshipID = ""
	noBasis := document.Revocations[0]
	noBasis.Basis = ""
	for name, probe := range map[string]struct {
		translate func() error
		field     string
	}{
		"替代缺批准": {func() error {
			_, err := registrationjson.PartyRelationshipSupersessionCommand(tenant, noApproval)
			return err
		}, "approval"},
		"替代缺接替关系标识": {func() error {
			_, err := registrationjson.PartyRelationshipSupersessionCommand(tenant, noSuccessorID)
			return err
		}, "successorRelationshipId"},
		"撤销缺依据": {func() error {
			_, err := registrationjson.PartyRelationshipRevocationCommand(tenant, noBasis)
			return err
		}, "basis"},
	} {
		err := probe.translate()
		if err == nil || !strings.HasPrefix(err.Error(), probe.field) {
			t.Fatalf("%s: err = %v, want it to name %s", name, err, probe.field)
		}
	}
}
