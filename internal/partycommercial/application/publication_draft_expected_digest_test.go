package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件钉批准与发布的前置条件（ADR-0314 决定二）：调用方带上它看到的那份内容的摘要，载体此刻的摘要与它不同就答
// `已变更`，不去批、不去发一份调用方没看过的内容；内容身份先于状态；不带摘要的调用方（手上本就握着载体）照旧。

func approveCreditExpecting(
	t *testing.T,
	drafts *publicationDraftRegistryDouble,
	rules approvalDutyRuleDouble,
	approver domain.OperatorSubject,
	digest string,
) application.ApprovePublicationDraftResult {
	t.Helper()
	tenant, kind, objectID, version := draftIdentity(t)
	handler := application.NewApprovePublicationDraftHandler(drafts, rules, fixedClock{at: draftApprovedAt})
	result, err := handler.Handle(context.Background(), application.ApprovePublicationDraftCommand{
		Tenant: tenant, Kind: kind, ObjectID: objectID, Version: version, Approver: approver, ExpectedDigest: digest,
	})
	if err != nil {
		t.Fatalf("批准：%v", err)
	}
	return result
}

// Covers: AT-PC-069——批准带的摘要与载体此刻的摘要相同才批；换过内容的载体（待批准期间被修订）答已变更，载体仍待批准、批准者没有被记下。
func TestApprovalIsBoundToTheDigestTheApproverSaw(t *testing.T) {
	drafts := newDraftRegistryDouble()
	seen := submitCredit(t, drafts, 500_000)
	draft, _ := seen.Draft()
	seenDigest := draft.Canonical().Digest().String()
	rules := approvalDutyRuleDouble{rule: dutyRule(t, true, ""), found: true}
	approver := operatorSubject(t, "op-approver")
	tenant, kind, objectID, version := draftIdentity(t)

	// 录入者在批准者翻看之后改了额度：同一版载体被修订，摘要随之变了。
	submitCredit(t, drafts, 900_000)
	changed := approveCreditExpecting(t, drafts, rules, approver, seenDigest)
	if changed.Outcome() != application.ApprovalDraftChanged {
		t.Fatalf("对着旧摘要批准：outcome = %q, want DRAFT_CHANGED", changed.Outcome())
	}
	stored, _, _ := drafts.LoadDraft(context.Background(), tenant, kind, objectID, version)
	if stored.Status() != domain.PublicationDraftPendingApproval {
		t.Fatalf("已变更仍放行了批准：status = %s", stored.Status())
	}
	if _, approved := stored.Approver(); approved {
		t.Fatal("已变更却记下了批准者")
	}

	currentDigest := stored.Canonical().Digest().String()
	if currentDigest == seenDigest {
		t.Fatal("夹具没换内容，摘要没变")
	}
	if approved := approveCreditExpecting(t, drafts, rules, approver, currentDigest); approved.Outcome() != application.PublicationDraftApproved {
		t.Fatalf("对着此刻摘要批准：outcome = %q, want DRAFT_APPROVED", approved.Outcome())
	}
}

// Covers: AT-PC-069——内容身份先于状态：载体已批准时，带旧摘要的调用方得到已变更，不是「已批准」——后者会让他以为批的是自己看过的那份；
// 不带摘要的调用方仍按状态答。
func TestTheDigestCheckComesBeforeTheStatusAnswer(t *testing.T) {
	drafts := newDraftRegistryDouble()
	submitCredit(t, drafts, 500_000)
	rules := approvalDutyRuleDouble{rule: dutyRule(t, true, ""), found: true}
	approveCredit(t, drafts, rules, operatorSubject(t, "op-approver"))

	stale := approveCreditExpecting(t, drafts, rules, operatorSubject(t, "op-other"), "PCC-1:stale")
	if stale.Outcome() != application.ApprovalDraftChanged {
		t.Fatalf("已批准载体上带旧摘要：outcome = %q, want DRAFT_CHANGED", stale.Outcome())
	}
	if plain := approveCreditExpecting(t, drafts, rules, operatorSubject(t, "op-other"), ""); plain.Outcome() != application.PublicationDraftAlreadyApproved {
		t.Fatalf("不带摘要：outcome = %q, want DRAFT_ALREADY_APPROVED", plain.Outcome())
	}
}

// Covers: AT-PC-069——发布同样带摘要：已批准载体对着旧摘要发布答已变更，一个字节不写；对着此刻摘要才交给受控发布。
func TestPublishingIsBoundToTheDigestTheCallerSaw(t *testing.T) {
	drafts := newDraftRegistryDouble()
	submitCredit(t, drafts, 500_000)
	approveCredit(t, drafts, approvalDutyRuleDouble{rule: dutyRule(t, true, ""), found: true}, operatorSubject(t, "op-approver"))
	tenant, kind, objectID, version := draftIdentity(t)
	stored, _, _ := drafts.LoadDraft(context.Background(), tenant, kind, objectID, version)

	registry := &publicationRegistryDouble{}
	publisher := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: draftPublishedAt}, &operatorRegistrationHandoffDouble{})
	handler := application.NewPublishPublicationDraftHandler(drafts, publisher, fixedClock{at: draftPublishedAt})
	command := application.PublishPublicationDraftCommand{Tenant: tenant, Kind: kind, ObjectID: objectID, Version: version}

	command.ExpectedDigest = "PCC-1:stale"
	stale, err := handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	if stale.Outcome() != application.PublicationDraftChanged {
		t.Fatalf("对着旧摘要发布：outcome = %q, want DRAFT_CHANGED", stale.Outcome())
	}
	if len(registry.savedVersions) != 0 {
		t.Fatalf("已变更仍写了 %d 个版本", len(registry.savedVersions))
	}
	if still, _, _ := drafts.LoadDraft(context.Background(), tenant, kind, objectID, version); still.Status() != domain.PublicationDraftApproved {
		t.Fatalf("已变更动了载体：status = %s", still.Status())
	}

	command.ExpectedDigest = stored.Canonical().Digest().String()
	published, err := handler.Handle(context.Background(), command)
	if err != nil || published.Outcome() != application.PublicationDraftPublished {
		t.Fatalf("对着此刻摘要发布：%q, %v; want DRAFT_PUBLISHED", published.Outcome(), err)
	}
}

func TestTheChangedOutcomeOfPublishingIsNamed(t *testing.T) {
	if application.PublicationDraftChanged.String() != "DRAFT_CHANGED" {
		t.Fatalf("name = %q", application.PublicationDraftChanged.String())
	}
}
