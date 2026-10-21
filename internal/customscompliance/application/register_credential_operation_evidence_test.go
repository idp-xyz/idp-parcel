package application_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// Covers: AT-CC-446（编排半边）——命令上的操作证据随这一版凭证交给写口；它不是凭证内容，另一位操作者以同内容重登答已在册，册上
// 留的仍是首次那一位；不带操作证据的一路记空。
func TestCredentialRegistrationHandsTheOperationEvidenceToTheRegisterAndAReplayKeepsTheFirst(t *testing.T) {
	store := &credentialStoreDouble{}
	command := credentialCommand(t)
	command.OperationEvidence = "OPERATOR/https://id.syn.example/dex#SYN-CUSTOMS-01"

	if outcome, err := newCredentialHandler(store).Handle(t.Context(), command); err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("凭证登记：err=%v outcome=%v", err, outcome)
	}
	replay := command
	replay.OperationEvidence = "OPERATOR/https://id.syn.example/dex#SYN-CUSTOMS-02"
	if outcome, err := newCredentialHandler(store).Handle(t.Context(), replay); err != nil || outcome != application.ConfigurationExisting {
		t.Fatalf("另一位操作者同内容重登：err=%v outcome=%v——操作证据进了内容比对", err, outcome)
	}
	if len(store.rows) != 1 || store.rows[0].evidence != command.OperationEvidence {
		t.Fatalf("册上的操作证据走样：%+v", store.rows)
	}

	plain := credentialCommand(t)
	plain.ID = configValue(t, domain.NewCredentialID, "SYN-CRED-02")
	if outcome, err := newCredentialHandler(store).Handle(t.Context(), plain); err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("不记登记人的一路：err=%v outcome=%v", err, outcome)
	}
	if store.rows[1].evidence != "" {
		t.Fatalf("没给操作证据却记了 %q", store.rows[1].evidence)
	}
}
