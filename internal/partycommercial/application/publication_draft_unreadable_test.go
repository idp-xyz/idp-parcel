package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证批准与发布遇到本构建折不回正文的载体行（AT-PC-071 的编排那一半）：那是一格业务结论，不是 error——重发不会好，
// 恢复动作在操作者手里（ADR-0022、ADR-0029）。判法只凭库上剩下的事实：内容身份先于状态，`已发布`如实答已发布，
// 其余答`读不回`；行本身坏了仍是 error。

// unreadableRow 造一行读不回的载体：规范化版本是本构建不认得的 PCC-9，摘要串随之换前缀，状态与痕迹照 status 配齐。
func unreadableRow(t *testing.T, status domain.PublicationDraftStatus) *domain.UnreadablePublicationDraft {
	t.Helper()
	seen := submitCredit(t, newDraftRegistryDouble(), 500_000)
	draft, _ := seen.Draft()
	spec := domain.RehydratePublicationDraftSpec{
		Shell: domain.PublicationDraftShell{
			TenantID: draft.Tenant(), Kind: draft.Kind(), ObjectID: draft.ObjectID(), Version: draft.Version(),
			Scope: draft.Scope(), Effective: draft.Effective(),
		},
		Canonicalization: "PCC-9",
		Document:         draft.Canonical().Document(),
		Digest:           pcValue(t, domain.NewCommercialContentDigest, "PCC-9:future-build"),
		Submitter:        draft.Submitter(),
		SubmittedAt:      draft.SubmittedAt(),
		Status:           status,
	}
	if status >= domain.PublicationDraftApproved {
		spec.Approver = pcValue(t, domain.NewOperatorSubjectReference, "op-approver")
		spec.ApprovedAt = draftApprovedAt
	}
	if status == domain.PublicationDraftPublished {
		spec.PublishedAt = draftPublishedAt
	}
	_, err := domain.RehydratePublicationDraft(spec)
	var unreadable *domain.UnreadablePublicationDraft
	if !errors.As(err, &unreadable) {
		t.Fatalf("夹具应读不回：err = %v", err)
	}
	return unreadable
}

func approveUnreadable(
	t *testing.T, loadErr error, rules approvalDutyRuleDouble, digest, shellDigest string,
) (application.ApprovePublicationDraftResult, error) {
	t.Helper()
	drafts := newDraftRegistryDouble()
	drafts.loadErr = loadErr
	tenant, kind, objectID, version := draftIdentity(t)
	handler := application.NewApprovePublicationDraftHandler(drafts, rules, fixedClock{at: draftApprovedAt})
	return handler.Handle(context.Background(), application.ApprovePublicationDraftCommand{
		Tenant: tenant, Kind: kind, ObjectID: objectID, Version: version, Approver: operatorSubject(t, "op-approver"),
		ExpectedDigest: digest, ExpectedShellDigest: shellDigest,
	})
}

// Covers: AT-PC-071——批准遇到读不回的载体：`待批准`与`已批准`答 DRAFT_NOT_READABLE（带载体状态与成因），一个字节不写，也不去读审批职责规则
// （规则未登记时读得回的载体答「未配置」，读不回的这一格先于它）；`已发布`答 DRAFT_ALREADY_PUBLISHED——终点不需要正文就能如实答。
func TestApprovingAnUnreadableDraftAnswersByItsStatusWithoutTouchingTheBody(t *testing.T) {
	noRule := approvalDutyRuleDouble{found: false}
	for _, status := range []domain.PublicationDraftStatus{domain.PublicationDraftPendingApproval, domain.PublicationDraftApproved} {
		unreadable := unreadableRow(t, status)
		result, err := approveUnreadable(t, unreadable, noRule, "", "")
		if err != nil {
			t.Fatalf("%s：读不回的载体不是 error：%v", status, err)
		}
		if result.Outcome() != application.ApprovalDraftNotReadable || result.Outcome().String() != "DRAFT_NOT_READABLE" {
			t.Fatalf("%s：outcome = %q, want DRAFT_NOT_READABLE", status, result.Outcome())
		}
		got, ok := result.Unreadable()
		if !ok || got.Status() != status || !errors.Is(got.Cause(), domain.ErrCanonicalizationUnsupported) {
			t.Fatalf("%s：读不回一格的事实 = %v %v", status, got, ok)
		}
		if _, hasDraft := result.Draft(); hasDraft {
			t.Fatalf("%s：读不回却交出了载体", status)
		}
	}

	published, err := approveUnreadable(t, unreadableRow(t, domain.PublicationDraftPublished), noRule, "", "")
	if err != nil || published.Outcome() != application.ApprovalDraftAlreadyPublished {
		t.Fatalf("已发布的读不回行：%q, %v; want DRAFT_ALREADY_PUBLISHED", published.Outcome(), err)
	}
	if _, ok := published.Unreadable(); ok {
		t.Fatal("已发布一格不该带读不回的事实")
	}
}

// Covers: AT-PC-069 与 AT-PC-071 的交会——摘要绑定对读不回的行照样成立（用列上的摘要与壳摘要比）：对着别的摘要批准答 DRAFT_CHANGED，
// 先于状态答案；对着列上的摘要与壳摘要才落到「读不回」或「已发布」。
func TestTheDigestBindingStillHoldsOnAnUnreadableDraft(t *testing.T) {
	noRule := approvalDutyRuleDouble{found: false}
	pending := unreadableRow(t, domain.PublicationDraftPendingApproval)
	published := unreadableRow(t, domain.PublicationDraftPublished)

	for name, row := range map[string]*domain.UnreadablePublicationDraft{"待批准": pending, "已发布": published} {
		if stale, err := approveUnreadable(t, row, noRule, "PCC-1:seen-before", ""); err != nil || stale.Outcome() != application.ApprovalDraftChanged {
			t.Fatalf("%s 的读不回行对着旧内容摘要批准：%q, %v; want DRAFT_CHANGED", name, stale.Outcome(), err)
		}
		if stale, err := approveUnreadable(t, row, noRule, row.ContentDigest().String(), "PCS-1:seen-before"); err != nil || stale.Outcome() != application.ApprovalDraftChanged {
			t.Fatalf("%s 的读不回行对着旧壳摘要批准：%q, %v; want DRAFT_CHANGED", name, stale.Outcome(), err)
		}
	}
	if bound, err := approveUnreadable(t, pending, noRule, pending.ContentDigest().String(), pending.ShellDigest()); err != nil || bound.Outcome() != application.ApprovalDraftNotReadable {
		t.Fatalf("带着列上的两个摘要：%q, %v; want DRAFT_NOT_READABLE", bound.Outcome(), err)
	}
	if bound, err := approveUnreadable(t, published, noRule, published.ContentDigest().String(), published.ShellDigest()); err != nil || bound.Outcome() != application.ApprovalDraftAlreadyPublished {
		t.Fatalf("已发布带着列上的两个摘要：%q, %v; want DRAFT_ALREADY_PUBLISHED", bound.Outcome(), err)
	}
}

// Covers: AT-PC-071 的反面——行本身坏了（重建门因行数据而拒）与读取失败仍是 error，不被收进「读不回」：批准口与发布口都一样。
func TestABrokenRowAndAFailedReadStayErrorsOnBothCommandPaths(t *testing.T) {
	for name, loadErr := range map[string]error{
		"行坏了":  errors.Join(errors.New("load publication draft"), domain.ErrInvalidRehydratedPublicationDraft),
		"读取失败": errors.New("connection reset"),
	} {
		if _, err := approveUnreadable(t, loadErr, approvalDutyRuleDouble{found: true}, "", ""); err == nil {
			t.Fatalf("批准口 %s 被吞成了答案", name)
		}
		drafts := newDraftRegistryDouble()
		drafts.loadErr = loadErr
		tenant, kind, objectID, version := draftIdentity(t)
		publisher := application.NewPublishCommercialAuthorityHandler(&publicationRegistryDouble{}, fixedClock{at: draftPublishedAt}, &operatorRegistrationHandoffDouble{})
		handler := application.NewPublishPublicationDraftHandler(drafts, publisher, fixedClock{at: draftPublishedAt})
		if _, err := handler.Handle(context.Background(), application.PublishPublicationDraftCommand{
			Tenant: tenant, Kind: kind, ObjectID: objectID, Version: version,
		}); err == nil {
			t.Fatalf("发布口 %s 被吞成了答案", name)
		}
	}
}

// Covers: AT-PC-071——发布遇到读不回的载体：`待批准`与`已批准`都答 DRAFT_NOT_READABLE（`待批准`不答「还没批准」——批准它同样要正文），不交给发布用例、
// 不写任何版本，答复不带发布用例的答案；`已发布`答 DRAFT_ALREADY_PUBLISHED；旧摘要答 DRAFT_CHANGED，先于状态。
func TestPublishingAnUnreadableDraftIsAnAnswerAndNeverReachesThePublisher(t *testing.T) {
	publish := func(row *domain.UnreadablePublicationDraft, digest string) application.PublishPublicationDraftResult {
		t.Helper()
		drafts := newDraftRegistryDouble()
		drafts.loadErr = row
		registry := &publicationRegistryDouble{}
		publisher := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: draftPublishedAt}, &operatorRegistrationHandoffDouble{})
		handler := application.NewPublishPublicationDraftHandler(drafts, publisher, fixedClock{at: draftPublishedAt})
		tenant, kind, objectID, version := draftIdentity(t)
		result, err := handler.Handle(context.Background(), application.PublishPublicationDraftCommand{
			Tenant: tenant, Kind: kind, ObjectID: objectID, Version: version, ExpectedDigest: digest,
		})
		if err != nil {
			t.Fatalf("读不回的载体不是 error：%v", err)
		}
		if len(registry.savedVersions) != 0 {
			t.Fatalf("读不回的载体仍写了 %d 个版本", len(registry.savedVersions))
		}
		return result
	}

	for _, status := range []domain.PublicationDraftStatus{domain.PublicationDraftPendingApproval, domain.PublicationDraftApproved} {
		result := publish(unreadableRow(t, status), "")
		if result.Outcome() != application.PublicationDraftNotReadable || result.Outcome().String() != "DRAFT_NOT_READABLE" {
			t.Fatalf("%s：outcome = %q, want DRAFT_NOT_READABLE", status, result.Outcome())
		}
		if got, ok := result.Unreadable(); !ok || got.Status() != status {
			t.Fatalf("%s：读不回一格的事实 = %v %v", status, got, ok)
		}
		if _, hasPublication := result.Publication(); hasPublication {
			t.Fatalf("%s：读不回的载体却带了发布用例的答案", status)
		}
	}
	published := unreadableRow(t, domain.PublicationDraftPublished)
	if result := publish(published, ""); result.Outcome() != application.PublicationDraftAlreadyPublished {
		t.Fatalf("已发布的读不回行：%q, want DRAFT_ALREADY_PUBLISHED", result.Outcome())
	}
	if result := publish(published, "PCC-1:seen-before"); result.Outcome() != application.PublicationDraftChanged {
		t.Fatalf("旧摘要：%q, want DRAFT_CHANGED", result.Outcome())
	}
}
