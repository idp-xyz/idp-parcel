package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 夹具时间线：录入 → 批准 → 发布，三刻依次向后。
var (
	draftSubmittedAt = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	draftApprovedAt  = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	draftPublishedAt = time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC)
)

func draftShell(t *testing.T, kind domain.CommercialObjectKind, objectID, version string) domain.PublicationDraftShell {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	return domain.PublicationDraftShell{
		TenantID:  commercialValue(t, domain.NewTenantID, "tenant-1"),
		Kind:      kind,
		ObjectID:  commercialValue(t, domain.NewCommercialObjectID, objectID),
		Version:   commercialValue(t, domain.NewCommercialVersionLabel, version),
		Scope:     commercialValue(t, domain.NewCommercialScopeReference, "scope-1"),
		Effective: interval,
	}
}

func creditContent(t *testing.T, minor int64) domain.PublicationContent {
	t.Helper()
	body := creditPolicyBody(t, "freight", creditAmount(t, minor), time.Time{})
	return domain.PublicationContent{Kind: domain.CreditPolicyObject, CreditPolicy: &body}
}

func operator(t *testing.T, reference string, levels ...string) domain.OperatorSubject {
	t.Helper()
	held := make([]domain.AuthorityLevel, 0, len(levels))
	for _, level := range levels {
		held = append(held, commercialValue(t, domain.NewAuthorityLevel, level))
	}
	subject, err := domain.NewOperatorSubject(commercialValue(t, domain.NewOperatorSubjectReference, reference), held)
	if err != nil {
		t.Fatalf("new operator subject: %v", err)
	}
	return subject
}

func dutyRule(t *testing.T, distinct bool, level string) domain.ApprovalDutyRule {
	t.Helper()
	var required domain.AuthorityLevel
	if level != "" {
		required = commercialValue(t, domain.NewAuthorityLevel, level)
	}
	rule, err := domain.NewApprovalDutyRule(commercialValue(t, domain.NewTenantID, "tenant-1"), distinct, required)
	if err != nil {
		t.Fatalf("new approval duty rule: %v", err)
	}
	return rule
}

func pendingCreditDraft(t *testing.T, minor int64) domain.PublicationDraft {
	t.Helper()
	draft, err := domain.SubmitPublicationDraft(
		draftShell(t, domain.CreditPolicyObject, "credit-1", "v1"),
		creditContent(t, minor),
		commercialValue(t, domain.NewOperatorSubjectReference, "op-submitter"),
		draftSubmittedAt,
	)
	if err != nil {
		t.Fatalf("submit publication draft: %v", err)
	}
	return draft
}

func approvedCreditDraft(t *testing.T) domain.PublicationDraft {
	t.Helper()
	approved, err := pendingCreditDraft(t, 500_000).Approve(
		operator(t, "op-approver", "level-approver"), dutyRule(t, true, "level-approver"), draftApprovedAt)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	return approved
}

// Covers: ADR-0126 Decision 三 — 录入形成`待批准`载体：摘要由服务端算出（表单不算摘要），壳、正文与录入者
// 原样记下，此刻没有批准者也没有发布时刻。
func TestSubmittingADraftComputesTheDigestAndStartsPendingApproval(t *testing.T) {
	draft := pendingCreditDraft(t, 500_000)

	if draft.Status() != domain.PublicationDraftPendingApproval {
		t.Fatalf("status = %s, want PENDING_APPROVAL", draft.Status())
	}
	expected := canonicalCreditPolicy(t, *creditContent(t, 500_000).CreditPolicy)
	if draft.Canonical().Digest() != expected.Digest() {
		t.Fatalf("digest = %s, want the canonical digest %s", draft.Canonical().Digest(), expected.Digest())
	}
	if draft.Canonical().Canonicalization() != "PCC-1" {
		t.Fatalf("canonicalization = %q, want PCC-1", draft.Canonical().Canonicalization())
	}
	if draft.Submitter().String() != "op-submitter" || !draft.SubmittedAt().Equal(draftSubmittedAt) {
		t.Fatalf("submitter = %s at %s", draft.Submitter(), draft.SubmittedAt())
	}
	if _, approved := draft.Approver(); approved {
		t.Fatal("a pending draft must not carry an approver")
	}
	if _, published := draft.PublishedAt(); published {
		t.Fatal("a pending draft must not carry a publication time")
	}
	spec := draft.PublicationSpec()
	if spec.ContentDigest != expected.Digest() || spec.Kind != domain.CreditPolicyObject ||
		spec.ObjectID.String() != "credit-1" || spec.Version.String() != "v1" || spec.Scope.String() != "scope-1" {
		t.Fatalf("publication spec = %#v", spec)
	}
}

// Covers: ADR-0126 Decision 一 — 规范化门拒的输入录不进载体（答案是那一格，不是别的）：正文缺席、正文与壳的类别
// 不符；壳立不住（缺范围）同样拒。
func TestSubmittingADraftIsGuardedByTheCanonicalizationAndShellGates(t *testing.T) {
	submitter := commercialValue(t, domain.NewOperatorSubjectReference, "op-submitter")

	// 样本取客户服务规则：自票 admin-write-faces/18 起它已接进规范化，只有壳没有正文答的是「正文缺席」那一格
	// （「没接的册」那一格自此没有样本，见 TestCanonicalizeAnswersThreeDistinctRefusals）。
	_, err := domain.SubmitPublicationDraft(
		draftShell(t, domain.CustomerServiceRuleObject, "csr-1", "v1"),
		domain.PublicationContent{Kind: domain.CustomerServiceRuleObject},
		submitter, draftSubmittedAt)
	if !errors.Is(err, domain.ErrPublicationContentAbsent) {
		t.Fatalf("customer service rule draft without a body: err = %v, want ErrPublicationContentAbsent", err)
	}

	mismatched := creditContent(t, 1)
	mismatched.Kind = domain.SettlementPolicyObject
	_, err = domain.SubmitPublicationDraft(
		draftShell(t, domain.CreditPolicyObject, "credit-1", "v1"), mismatched, submitter, draftSubmittedAt)
	if !errors.Is(err, domain.ErrInvalidPublicationDraft) {
		t.Fatalf("shell kind ≠ content kind: err = %v, want ErrInvalidPublicationDraft", err)
	}

	blankScope := draftShell(t, domain.CreditPolicyObject, "credit-1", "v1")
	blankScope.Scope = domain.CommercialScopeReference{}
	_, err = domain.SubmitPublicationDraft(blankScope, creditContent(t, 1), submitter, draftSubmittedAt)
	if !errors.Is(err, domain.ErrInvalidCommercialVersion) {
		t.Fatalf("blank scope: err = %v, want ErrInvalidCommercialVersion", err)
	}

	_, err = domain.SubmitPublicationDraft(
		draftShell(t, domain.CreditPolicyObject, "credit-1", "v1"), creditContent(t, 1),
		domain.OperatorSubjectReference{}, draftSubmittedAt)
	if !errors.Is(err, domain.ErrInvalidPublicationDraft) {
		t.Fatalf("blank submitter: err = %v, want ErrInvalidPublicationDraft", err)
	}
}

// Covers: ADR-0126 Decision 三 — 批准是另一个操作者的动作：过了审批职责规则的载体转`已批准`，批准者与
// 时刻记下、录入者不变；发布所需的批准依据由载体交出——批准引用 = 批准者，来源 = 载体自己的引用。
func TestApprovingAPendingDraftRecordsTheApproverAndYieldsTheApprovalBasis(t *testing.T) {
	approved := approvedCreditDraft(t)

	if approved.Status() != domain.PublicationDraftApproved {
		t.Fatalf("status = %s, want APPROVED", approved.Status())
	}
	approver, ok := approved.Approver()
	if !ok || approver.String() != "op-approver" {
		t.Fatalf("approver = %s, %v", approver, ok)
	}
	if at, ok := approved.ApprovedAt(); !ok || !at.Equal(draftApprovedAt) {
		t.Fatalf("approvedAt = %s, %v", at, ok)
	}
	if approved.Submitter().String() != "op-submitter" {
		t.Fatal("approval changed the submitter")
	}

	basis, err := approved.PublicationApproval()
	if err != nil {
		t.Fatalf("publication approval: %v", err)
	}
	if basis.Reference().String() != "op-approver" || basis.Source() != approved.Reference() || !basis.ApprovedAt().Equal(draftApprovedAt) {
		t.Fatalf("approval basis = %#v", basis)
	}
	if approved.Reference().String() == "" {
		t.Fatal("a draft must carry its own reference for the approval source")
	}

	if _, err := pendingCreditDraft(t, 1).PublicationApproval(); !errors.Is(err, domain.ErrPublicationDraftNotApproved) {
		t.Fatalf("pending draft yielded an approval basis: %v", err)
	}
}

// Covers: ADR-0126 Decision 三 / PAR-COM-18 — 审批职责规则逐格裁：要求不同主体时录入者自批被拒；要求持某一格
// 授予时不持它的批准者被拒；两格都不要求的规则放行同人；他租户的规则不得裁本租户的载体。
func TestApprovalIsGatedByTheApprovalDutyRule(t *testing.T) {
	pending := pendingCreditDraft(t, 500_000)

	_, err := pending.Approve(operator(t, "op-submitter", "level-approver"), dutyRule(t, true, "level-approver"), draftApprovedAt)
	if !errors.Is(err, domain.ErrApproverIsSubmitter) {
		t.Fatalf("self approval under a distinct-subjects rule: err = %v, want ErrApproverIsSubmitter", err)
	}

	_, err = pending.Approve(operator(t, "op-approver", "level-clerk"), dutyRule(t, true, "level-approver"), draftApprovedAt)
	if !errors.Is(err, domain.ErrApproverLacksRequiredLevel) {
		t.Fatalf("approver without the required level: err = %v, want ErrApproverLacksRequiredLevel", err)
	}

	if _, err := pending.Approve(operator(t, "op-submitter"), dutyRule(t, false, ""), draftApprovedAt); err != nil {
		t.Fatalf("a rule that requires nothing must let the submitter approve: %v", err)
	}

	otherTenant, err := domain.NewApprovalDutyRule(commercialValue(t, domain.NewTenantID, "tenant-2"), false, domain.AuthorityLevel{})
	if err != nil {
		t.Fatalf("new rule: %v", err)
	}
	if _, err := pending.Approve(operator(t, "op-approver"), otherTenant, draftApprovedAt); !errors.Is(err, domain.ErrApprovalDutyRuleTenantMismatch) {
		t.Fatalf("another tenant's rule: err = %v, want ErrApprovalDutyRuleTenantMismatch", err)
	}

	if _, err := pending.Approve(operator(t, "op-approver"), domain.ApprovalDutyRule{}, draftApprovedAt); !errors.Is(err, domain.ErrInvalidApprovalDutyRule) {
		t.Fatalf("zero rule: err = %v, want ErrInvalidApprovalDutyRule", err)
	}

	if _, err := pending.Approve(operator(t, "op-approver"), dutyRule(t, false, ""), draftSubmittedAt.Add(-time.Minute)); !errors.Is(err, domain.ErrInvalidPublicationDraft) {
		t.Fatalf("approval before submission: err = %v, want ErrInvalidPublicationDraft", err)
	}
}

// Covers: ADR-0126 Decision 三 — 状态三格封闭且单向：`已批准`不能再批，`待批准`不能发布，`已发布`两者都不能。
func TestDraftStatusMovesForwardOnly(t *testing.T) {
	approved := approvedCreditDraft(t)

	if _, err := approved.Approve(operator(t, "op-other"), dutyRule(t, false, ""), draftApprovedAt); !errors.Is(err, domain.ErrPublicationDraftNotPending) {
		t.Fatalf("approving twice: err = %v, want ErrPublicationDraftNotPending", err)
	}
	if _, err := pendingCreditDraft(t, 1).MarkPublished(draftPublishedAt); !errors.Is(err, domain.ErrPublicationDraftNotApproved) {
		t.Fatalf("publishing a pending draft: err = %v, want ErrPublicationDraftNotApproved", err)
	}
	if _, err := approved.MarkPublished(draftApprovedAt.Add(-time.Second)); !errors.Is(err, domain.ErrInvalidPublicationDraft) {
		t.Fatalf("publishing before approval: err = %v, want ErrInvalidPublicationDraft", err)
	}

	published, err := approved.MarkPublished(draftPublishedAt)
	if err != nil {
		t.Fatalf("mark published: %v", err)
	}
	if published.Status() != domain.PublicationDraftPublished {
		t.Fatalf("status = %s, want PUBLISHED", published.Status())
	}
	if at, ok := published.PublishedAt(); !ok || !at.Equal(draftPublishedAt) {
		t.Fatalf("publishedAt = %s, %v", at, ok)
	}
	if _, err := published.MarkPublished(draftPublishedAt); !errors.Is(err, domain.ErrPublicationDraftNotApproved) {
		t.Fatalf("publishing twice: err = %v, want ErrPublicationDraftNotApproved", err)
	}
	if _, err := published.Approve(operator(t, "op-other"), dutyRule(t, false, ""), draftPublishedAt); !errors.Is(err, domain.ErrPublicationDraftNotPending) {
		t.Fatalf("approving a published draft: err = %v, want ErrPublicationDraftNotPending", err)
	}
}

// Covers: ADR-0126 Decision 四 — 预览与录入同一道门：同一份壳与正文过预览与过录入得到逐字节相同的摘要；
// 录入会拒的输入预览同样拒（同一格答案），预览不形成载体。
func TestPreviewWalksTheSameGateAsSubmission(t *testing.T) {
	shell := draftShell(t, domain.CreditPolicyObject, "credit-1", "v1")
	content := creditContent(t, 500_000)

	preview, err := domain.PreviewPublication(shell, content)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.Digest() != pendingCreditDraft(t, 500_000).Canonical().Digest() {
		t.Fatalf("preview digest %s ≠ submitted digest", preview.Digest())
	}

	mismatched := content
	mismatched.Kind = domain.SettlementPolicyObject
	if _, err := domain.PreviewPublication(shell, mismatched); !errors.Is(err, domain.ErrInvalidPublicationDraft) {
		t.Fatalf("preview of a mismatched kind: err = %v", err)
	}
	blankScope := shell
	blankScope.Scope = domain.CommercialScopeReference{}
	if _, err := domain.PreviewPublication(blankScope, content); !errors.Is(err, domain.ErrInvalidCommercialVersion) {
		t.Fatalf("preview of a blank scope: err = %v", err)
	}
	if _, err := domain.PreviewPublication(draftShell(t, domain.CustomerServiceRuleObject, "csr-1", "v1"),
		domain.PublicationContent{Kind: domain.CustomerServiceRuleObject}); !errors.Is(err, domain.ErrPublicationContentAbsent) {
		t.Fatalf("preview of a wired register without its body: err = %v, want ErrPublicationContentAbsent", err)
	}
}

// Covers: ADR-0126 Decision 三「同版本同内容再录是重放，换内容是修订」的判据在领域一处：内容同不同只看
// 算出的摘要。
func TestSameContentIsJudgedByTheCanonicalDigest(t *testing.T) {
	left := pendingCreditDraft(t, 500_000)
	if !left.SameContentAs(pendingCreditDraft(t, 500_000)) {
		t.Fatal("same body submitted twice must be the same content")
	}
	if left.SameContentAs(pendingCreditDraft(t, 1)) {
		t.Fatal("a different limit must not be the same content")
	}

	// 同一次录入还要壳同：换范围再录是修订不是重放，否则新范围会被静默丢掉。
	if !left.SameSubmissionAs(pendingCreditDraft(t, 500_000)) {
		t.Fatal("same shell and body must be the same submission")
	}
	otherScope := draftShell(t, domain.CreditPolicyObject, "credit-1", "v1")
	otherScope.Scope = commercialValue(t, domain.NewCommercialScopeReference, "scope-2")
	rescoped, err := domain.SubmitPublicationDraft(otherScope, creditContent(t, 500_000),
		commercialValue(t, domain.NewOperatorSubjectReference, "op-submitter"), draftSubmittedAt)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !left.SameContentAs(rescoped) || left.SameSubmissionAs(rescoped) {
		t.Fatal("a rescoped draft has the same content but is not the same submission")
	}
}

// Covers: ADR-0126 Decision 三 — 载体的正文快照就是规范化文档：文档能折回正文，折回再算得到同一个摘要；
// 本构建不认识的规范化版本折不回来（答不支持，不是猜）。
func TestTheCanonicalDocumentRoundTripsTheContent(t *testing.T) {
	canonical := canonicalCreditPolicy(t, *creditContent(t, 500_000).CreditPolicy)

	content, err := domain.RehydratePublicationContent(canonical.Canonicalization(), canonical.Document())
	if err != nil {
		t.Fatalf("rehydrate content: %v", err)
	}
	again, err := domain.CanonicalizePublicationContent(content)
	if err != nil {
		t.Fatalf("re-canonicalize: %v", err)
	}
	if again.Digest() != canonical.Digest() {
		t.Fatalf("round trip changed the digest: %s vs %s", again.Digest(), canonical.Digest())
	}
	if content.CreditPolicy == nil || content.CreditPolicy.ChargeType.String() != "freight" {
		t.Fatalf("content = %#v", content)
	}

	if _, err := domain.RehydratePublicationContent("PCC-9", canonical.Document()); !errors.Is(err, domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("unknown canonicalization: err = %v, want ErrCanonicalizationUnsupported", err)
	}
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), []byte(`{"kind":"CREDIT_POLICY"}`)); err == nil {
		t.Fatal("a document without its body must not rehydrate into content")
	}
}

// Covers: ADR-0028 在载体上的落法 — 重建门相信输入但校不变量：三格各自带该带的痕迹、快照折回后摘要与列上
// 一致；缺痕迹、多痕迹或摘要对不上的行拒。
func TestRehydratingADraftChecksItsInvariantsWithoutRecomputingJudgments(t *testing.T) {
	published, err := approvedCreditDraft(t).MarkPublished(draftPublishedAt)
	if err != nil {
		t.Fatalf("mark published: %v", err)
	}
	approver, _ := published.Approver()
	approvedAt, _ := published.ApprovedAt()
	publishedAt, _ := published.PublishedAt()
	spec := domain.RehydratePublicationDraftSpec{
		Shell: domain.PublicationDraftShell{
			TenantID: published.Tenant(), Kind: published.Kind(), ObjectID: published.ObjectID(),
			Version: published.Version(), Scope: published.Scope(), Effective: published.Effective(),
		},
		Canonicalization: published.Canonical().Canonicalization(),
		Document:         published.Canonical().Document(),
		Digest:           published.Canonical().Digest(),
		Submitter:        published.Submitter(),
		SubmittedAt:      published.SubmittedAt(),
		Status:           domain.PublicationDraftPublished,
		Approver:         approver,
		ApprovedAt:       approvedAt,
		PublishedAt:      publishedAt,
	}

	rehydrated, err := domain.RehydratePublicationDraft(spec)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if rehydrated.Status() != domain.PublicationDraftPublished || !rehydrated.SameContentAs(published) {
		t.Fatalf("rehydrated = %#v", rehydrated)
	}
	if basis, err := rehydrated.PublicationApproval(); err != nil || basis.Source() != published.Reference() {
		t.Fatalf("rehydrated approval basis = %#v, %v", basis, err)
	}

	corruptDigest := spec
	corruptDigest.Digest = commercialValue(t, domain.NewCommercialContentDigest, "PCC-1:deadbeef")
	if _, err := domain.RehydratePublicationDraft(corruptDigest); !errors.Is(err, domain.ErrInvalidRehydratedPublicationDraft) {
		t.Fatalf("digest ≠ document: err = %v, want ErrInvalidRehydratedPublicationDraft", err)
	}

	pendingWithApprover := spec
	pendingWithApprover.Status = domain.PublicationDraftPendingApproval
	if _, err := domain.RehydratePublicationDraft(pendingWithApprover); !errors.Is(err, domain.ErrInvalidRehydratedPublicationDraft) {
		t.Fatalf("pending with approval traces: err = %v", err)
	}

	approvedWithoutApprover := spec
	approvedWithoutApprover.Status = domain.PublicationDraftApproved
	approvedWithoutApprover.PublishedAt = time.Time{}
	approvedWithoutApprover.Approver = domain.OperatorSubjectReference{}
	if _, err := domain.RehydratePublicationDraft(approvedWithoutApprover); !errors.Is(err, domain.ErrInvalidRehydratedPublicationDraft) {
		t.Fatalf("approved without approver: err = %v", err)
	}

	publishedWithoutTime := spec
	publishedWithoutTime.PublishedAt = time.Time{}
	if _, err := domain.RehydratePublicationDraft(publishedWithoutTime); !errors.Is(err, domain.ErrInvalidRehydratedPublicationDraft) {
		t.Fatalf("published without time: err = %v", err)
	}
}

// Covers: 操作者主体是「引用 + 授予集」这一个消费面（ADR-0126 Decision 五）：空引用立不住；持不持某一格
// 授予按等级逐格答；授予集可为空（信封里没带等级不是错，只是批不了要求等级的载体）。
func TestOperatorSubjectHoldsItsGrantedLevels(t *testing.T) {
	subject := operator(t, "op-1", "level-a", "level-b")
	if !subject.Holds(commercialValue(t, domain.NewAuthorityLevel, "level-a")) || subject.Holds(commercialValue(t, domain.NewAuthorityLevel, "level-c")) {
		t.Fatal("Holds answered the wrong levels")
	}
	if len(subject.Levels()) != 2 {
		t.Fatalf("levels = %v", subject.Levels())
	}
	if _, err := domain.NewOperatorSubject(domain.OperatorSubjectReference{}, nil); !errors.Is(err, domain.ErrInvalidOperatorSubject) {
		t.Fatalf("blank reference: err = %v", err)
	}
	if _, err := domain.NewOperatorSubject(commercialValue(t, domain.NewOperatorSubjectReference, "op-2"), []domain.AuthorityLevel{{}}); !errors.Is(err, domain.ErrInvalidOperatorSubject) {
		t.Fatalf("blank level in the grant set: err = %v", err)
	}
}

// Covers: 审批职责规则本身的构造门：租户必带；要求的等级可缺（零值 = 不要求）；两格都不要求仍是一条合法的
// 租户声明（单人可批是租户说的，不是系统默认的）。
func TestApprovalDutyRuleIsATenantDeclaration(t *testing.T) {
	if _, err := domain.NewApprovalDutyRule(domain.TenantID{}, true, domain.AuthorityLevel{}); !errors.Is(err, domain.ErrInvalidApprovalDutyRule) {
		t.Fatalf("blank tenant: err = %v", err)
	}
	rule := dutyRule(t, false, "")
	if rule.RequiresDistinctSubjects() {
		t.Fatal("rule declared no distinct-subjects requirement")
	}
	if _, required := rule.RequiredApproverLevel(); required {
		t.Fatal("rule declared no approver level")
	}
	strict := dutyRule(t, true, "level-approver")
	if level, required := strict.RequiredApproverLevel(); !required || level.String() != "level-approver" {
		t.Fatalf("required level = %s, %v", level, required)
	}
	if strict.Tenant().String() != "tenant-1" {
		t.Fatalf("tenant = %s", strict.Tenant())
	}
}

// Covers: 状态取值都有名字（enum 门禁的镜像）。
func TestPublicationDraftStatusNames(t *testing.T) {
	names := map[domain.PublicationDraftStatus]string{
		domain.PublicationDraftPendingApproval: "PENDING_APPROVAL",
		domain.PublicationDraftApproved:        "APPROVED",
		domain.PublicationDraftPublished:       "PUBLISHED",
	}
	for status, want := range names {
		if status.String() != want {
			t.Fatalf("%d.String() = %q, want %q", status, status.String(), want)
		}
	}
	if domain.PublicationDraftStatusInvalid.String() != "" {
		t.Fatal("the zero status must have no name")
	}
}
