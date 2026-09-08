package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对待批准发布的四个用例（ADR-0126 Decision 三、四）证编排：预览不落库且与录入同摘要；录入按登记册
// 落点逐名翻译、规范化不了的册答`未受理`；批准先读审批职责规则、未登记即`未配置`不放行、规则逐格的拒绝各成
// 一格；发布只接`已批准`载体、交既有发布用例、落定后推进载体，没落定的载体原样留着。

// draftKey 是替身登记册的键：版本身份四元。
type draftKey struct {
	tenant, objectID, version string
	kind                      domain.CommercialObjectKind
}

func keyOf(draft domain.PublicationDraft) draftKey {
	return draftKey{
		tenant:   draft.Tenant().String(),
		kind:     draft.Kind(),
		objectID: draft.ObjectID().String(),
		version:  draft.Version().String(),
	}
}

// publicationDraftRegistryDouble 是一份内存里的载体册，按端口约定的代数作答：同内容重放、待批准换内容修订、
// 已批准后内容固定；推进时前一格状态与摘要都对得上才落。
type publicationDraftRegistryDouble struct {
	rows    map[draftKey]domain.PublicationDraft
	loadErr error
	saveErr error
	submits int
}

func newDraftRegistryDouble() *publicationDraftRegistryDouble {
	return &publicationDraftRegistryDouble{rows: map[draftKey]domain.PublicationDraft{}}
}

func (double *publicationDraftRegistryDouble) SubmitDraft(_ context.Context, draft domain.PublicationDraft) (ports.PublicationDraftSubmitOutcome, error) {
	if double.saveErr != nil {
		return ports.PublicationDraftSubmitOutcomeInvalid, double.saveErr
	}
	double.submits++
	key := keyOf(draft)
	existing, present := double.rows[key]
	switch {
	case !present:
		double.rows[key] = draft
		return ports.PublicationDraftSaved, nil
	case existing.SameSubmissionAs(draft):
		return ports.PublicationDraftReplayed, nil
	case existing.Status() == domain.PublicationDraftPendingApproval:
		double.rows[key] = draft
		return ports.PublicationDraftRevised, nil
	default:
		return ports.PublicationDraftContentFixed, nil
	}
}

func (double *publicationDraftRegistryDouble) LoadDraft(
	_ context.Context,
	tenant domain.TenantID,
	kind domain.CommercialObjectKind,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) (domain.PublicationDraft, bool, error) {
	if double.loadErr != nil {
		return domain.PublicationDraft{}, false, double.loadErr
	}
	draft, found := double.rows[draftKey{tenant: tenant.String(), kind: kind, objectID: objectID.String(), version: version.String()}]
	return draft, found, nil
}

func (double *publicationDraftRegistryDouble) AdvanceDraft(_ context.Context, draft domain.PublicationDraft) (ports.PublicationDraftAdvanceOutcome, error) {
	if double.saveErr != nil {
		return ports.PublicationDraftAdvanceOutcomeInvalid, double.saveErr
	}
	key := keyOf(draft)
	existing, present := double.rows[key]
	if !present {
		return ports.PublicationDraftAdvanceNotFound, nil
	}
	if !existing.SameContentAs(draft) || existing.Status()+1 != draft.Status() {
		return ports.PublicationDraftAdvanceSuperseded, nil
	}
	double.rows[key] = draft
	return ports.PublicationDraftAdvanced, nil
}

// approvalDutyRuleDouble 演「租户登了 / 没登审批职责规则」。
type approvalDutyRuleDouble struct {
	rule  domain.ApprovalDutyRule
	found bool
	err   error
}

func (double approvalDutyRuleDouble) LoadApprovalDutyRule(context.Context, domain.TenantID) (domain.ApprovalDutyRule, bool, error) {
	return double.rule, double.found, double.err
}

var (
	draftSubmittedAt = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	draftApprovedAt  = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	draftPublishedAt = time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC)
)

func draftShell(t *testing.T, kind domain.CommercialObjectKind, objectID, version string) domain.PublicationDraftShell {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(pubStartsAt, time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	return domain.PublicationDraftShell{
		TenantID:  pcValue(t, domain.NewTenantID, "tenant-1"),
		Kind:      kind,
		ObjectID:  pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:   pcValue(t, domain.NewCommercialVersionLabel, version),
		Scope:     pcValue(t, domain.NewCommercialScopeReference, "scope-1"),
		Effective: interval,
	}
}

func creditContent(t *testing.T, minor int64) domain.PublicationContent {
	t.Helper()
	limit, err := domain.NewCreditAmountLimit(minor)
	if err != nil {
		t.Fatalf("金额额度：%v", err)
	}
	declaration := creditPolicyBody(t, limit)
	return domain.PublicationContent{Kind: domain.CreditPolicyObject, CreditPolicy: &domain.CreditPolicyBody{
		LegalEntity: declaration.LegalEntity,
		Level:       declaration.Level,
		ChargeType:  declaration.ChargeType,
		Limit:       declaration.Limit,
		Effective:   declaration.Effective,
	}}
}

func operatorSubject(t *testing.T, reference string, levels ...string) domain.OperatorSubject {
	t.Helper()
	held := make([]domain.AuthorityLevel, 0, len(levels))
	for _, level := range levels {
		held = append(held, pcValue(t, domain.NewAuthorityLevel, level))
	}
	subject, err := domain.NewOperatorSubject(pcValue(t, domain.NewOperatorSubjectReference, reference), held)
	if err != nil {
		t.Fatalf("操作者主体：%v", err)
	}
	return subject
}

func dutyRule(t *testing.T, distinct bool, level string) domain.ApprovalDutyRule {
	t.Helper()
	var required domain.AuthorityLevel
	if level != "" {
		required = pcValue(t, domain.NewAuthorityLevel, level)
	}
	rule, err := domain.NewApprovalDutyRule(pcValue(t, domain.NewTenantID, "tenant-1"), distinct, required)
	if err != nil {
		t.Fatalf("审批职责规则：%v", err)
	}
	return rule
}

func submitCredit(t *testing.T, drafts *publicationDraftRegistryDouble, minor int64) application.SubmitPublicationDraftResult {
	t.Helper()
	return submitCreditWithShell(t, drafts, draftShell(t, domain.CreditPolicyObject, "credit-1", "v1"), minor)
}

func submitCreditWithShell(t *testing.T, drafts *publicationDraftRegistryDouble, shell domain.PublicationDraftShell, minor int64) application.SubmitPublicationDraftResult {
	t.Helper()
	handler := application.NewSubmitPublicationDraftHandler(drafts, fixedClock{at: draftSubmittedAt})
	result, err := handler.Handle(context.Background(), application.SubmitPublicationDraftCommand{
		Shell:     shell,
		Content:   creditContent(t, minor),
		Submitter: pcValue(t, domain.NewOperatorSubjectReference, "op-submitter"),
	})
	if err != nil {
		t.Fatalf("录入：%v", err)
	}
	return result
}

func draftIdentity(t *testing.T) (domain.TenantID, domain.CommercialObjectKind, domain.CommercialObjectID, domain.CommercialVersionLabel) {
	t.Helper()
	return pcValue(t, domain.NewTenantID, "tenant-1"), domain.CreditPolicyObject,
		pcValue(t, domain.NewCommercialObjectID, "credit-1"), pcValue(t, domain.NewCommercialVersionLabel, "v1")
}

func approveCredit(t *testing.T, drafts *publicationDraftRegistryDouble, rules approvalDutyRuleDouble, approver domain.OperatorSubject) application.ApprovePublicationDraftResult {
	t.Helper()
	tenant, kind, objectID, version := draftIdentity(t)
	handler := application.NewApprovePublicationDraftHandler(drafts, rules, fixedClock{at: draftApprovedAt})
	result, err := handler.Handle(context.Background(), application.ApprovePublicationDraftCommand{
		Tenant: tenant, Kind: kind, ObjectID: objectID, Version: version, Approver: approver,
	})
	if err != nil {
		t.Fatalf("批准：%v", err)
	}
	return result
}

// Covers: ADR-0126 Decision 四 — 预览交回规范化版本与摘要、不落库；同一份输入过预览与过录入逐字节同摘要；
// 没接规范化的册在预览上答`未受理`并带成因。
func TestPreviewComputesTheDigestWithoutTouchingTheRegister(t *testing.T) {
	previewer := application.NewPreviewCommercialPublicationHandler()
	preview, err := previewer.Handle(context.Background(), application.PreviewCommercialPublicationCommand{
		Shell:   draftShell(t, domain.CreditPolicyObject, "credit-1", "v1"),
		Content: creditContent(t, 500_000),
	})
	if err != nil {
		t.Fatalf("预览：%v", err)
	}
	if preview.Outcome() != application.CommercialPublicationPreviewed {
		t.Fatalf("outcome = %q, want PREVIEWED", preview.Outcome())
	}
	canonical, ok := preview.Canonical()
	if !ok || canonical.Canonicalization() != "PCC-1" {
		t.Fatalf("canonical = %#v, %v", canonical, ok)
	}

	drafts := newDraftRegistryDouble()
	submitted := submitCredit(t, drafts, 500_000)
	draft, hasDraft := submitted.Draft()
	if !hasDraft || draft.Canonical().Digest() != canonical.Digest() {
		t.Fatalf("预览摘要 %s ≠ 录入摘要 %s", canonical.Digest(), draft.Canonical().Digest())
	}

	refused, err := previewer.Handle(context.Background(), application.PreviewCommercialPublicationCommand{
		Shell:   draftShell(t, domain.SettlementPolicyObject, "settlement-1", "v1"),
		Content: domain.PublicationContent{Kind: domain.SettlementPolicyObject},
	})
	if err != nil {
		t.Fatalf("预览未接的册：%v——未受理不是 error", err)
	}
	if refused.Outcome() != application.CommercialPublicationPreviewNotAccepted || !errors.Is(refused.RefusalCause(), domain.ErrRegisterNotCanonicalized) {
		t.Fatalf("outcome = %q, cause = %v", refused.Outcome(), refused.RefusalCause())
	}
	if _, ok := refused.Canonical(); ok {
		t.Fatal("未受理的预览交回了一个摘要")
	}
}

// Covers: ADR-0126 Decision 三 — 录入形成`待批准`载体并落点逐名翻译：新行`已录入`、同内容`重放`、待批准期间
// 换内容`已修订`；正文规范化不了答`未受理`带成因且不碰登记册。
func TestSubmittingADraftTranslatesTheRegisterOutcomes(t *testing.T) {
	drafts := newDraftRegistryDouble()

	first := submitCredit(t, drafts, 500_000)
	if first.Outcome() != application.PublicationDraftSubmitted {
		t.Fatalf("first outcome = %q, want DRAFT_SUBMITTED", first.Outcome())
	}
	draft, ok := first.Draft()
	if !ok || draft.Status() != domain.PublicationDraftPendingApproval || draft.Submitter().String() != "op-submitter" {
		t.Fatalf("draft = %#v, %v", draft, ok)
	}
	if !draft.SubmittedAt().Equal(draftSubmittedAt) {
		t.Fatalf("submittedAt = %s, want the clock's now", draft.SubmittedAt())
	}

	if again := submitCredit(t, drafts, 500_000); again.Outcome() != application.PublicationDraftReplayed {
		t.Fatalf("replay outcome = %q, want DRAFT_REPLAYED", again.Outcome())
	}
	if revised := submitCredit(t, drafts, 1); revised.Outcome() != application.PublicationDraftRevised {
		t.Fatalf("revision outcome = %q, want DRAFT_REVISED", revised.Outcome())
	}

	handler := application.NewSubmitPublicationDraftHandler(drafts, fixedClock{at: draftSubmittedAt})
	refused, err := handler.Handle(context.Background(), application.SubmitPublicationDraftCommand{
		Shell:     draftShell(t, domain.SettlementPolicyObject, "settlement-1", "v1"),
		Content:   domain.PublicationContent{Kind: domain.SettlementPolicyObject},
		Submitter: pcValue(t, domain.NewOperatorSubjectReference, "op-submitter"),
	})
	if err != nil {
		t.Fatalf("录入未接的册：%v——未受理不是 error", err)
	}
	if refused.Outcome() != application.PublicationDraftNotAccepted || !errors.Is(refused.RefusalCause(), domain.ErrRegisterNotCanonicalized) {
		t.Fatalf("outcome = %q, cause = %v", refused.Outcome(), refused.RefusalCause())
	}
	if drafts.submits != 3 {
		t.Fatalf("未受理碰了登记册：submits = %d, want 3", drafts.submits)
	}
}

// Covers: ADR-0126 Decision 三 — 已批准之后换内容答`内容已固定`；同内容再录仍是重放。
func TestSubmittingAgainstAnApprovedDraftIsFixed(t *testing.T) {
	drafts := newDraftRegistryDouble()
	submitCredit(t, drafts, 500_000)
	if result := approveCredit(t, drafts, approvalDutyRuleDouble{rule: dutyRule(t, true, ""), found: true}, operatorSubject(t, "op-approver")); result.Outcome() != application.PublicationDraftApproved {
		t.Fatalf("approve outcome = %q", result.Outcome())
	}

	if fixed := submitCredit(t, drafts, 1); fixed.Outcome() != application.PublicationDraftContentFixed {
		t.Fatalf("outcome = %q, want CONTENT_FIXED", fixed.Outcome())
	}
	if replay := submitCredit(t, drafts, 500_000); replay.Outcome() != application.PublicationDraftReplayed {
		t.Fatalf("outcome = %q, want DRAFT_REPLAYED", replay.Outcome())
	}
}

// Covers: ADR-0126 Decision 三 / PAR-COM-18 — 批准先读审批职责规则：未登记即`未配置`、载体一字不动；规则在场
// 时逐格裁——录入者自批答`需换人批准`、不持要求等级答`批准者不合格`，过门的载体转`已批准`并记批准者。
func TestApprovalReadsTheDutyRuleBeforeItLetsAnyoneApprove(t *testing.T) {
	drafts := newDraftRegistryDouble()
	submitCredit(t, drafts, 500_000)
	tenant, kind, objectID, version := draftIdentity(t)

	notConfigured := approveCredit(t, drafts, approvalDutyRuleDouble{found: false}, operatorSubject(t, "op-approver", "level-approver"))
	if notConfigured.Outcome() != application.ApprovalDutyRuleNotConfigured {
		t.Fatalf("outcome = %q, want NOT_CONFIGURED", notConfigured.Outcome())
	}
	if stored, _, _ := drafts.LoadDraft(context.Background(), tenant, kind, objectID, version); stored.Status() != domain.PublicationDraftPendingApproval {
		t.Fatalf("未配置放行了批准：status = %s", stored.Status())
	}

	strict := approvalDutyRuleDouble{rule: dutyRule(t, true, "level-approver"), found: true}
	if self := approveCredit(t, drafts, strict, operatorSubject(t, "op-submitter", "level-approver")); self.Outcome() != application.PublicationDraftNeedsAnotherApprover {
		t.Fatalf("self approval outcome = %q, want NEEDS_ANOTHER_APPROVER", self.Outcome())
	}
	if clerk := approveCredit(t, drafts, strict, operatorSubject(t, "op-clerk", "level-clerk")); clerk.Outcome() != application.PublicationDraftApproverNotQualified {
		t.Fatalf("unqualified approver outcome = %q, want APPROVER_NOT_QUALIFIED", clerk.Outcome())
	}

	approved := approveCredit(t, drafts, strict, operatorSubject(t, "op-approver", "level-approver"))
	if approved.Outcome() != application.PublicationDraftApproved {
		t.Fatalf("outcome = %q, want DRAFT_APPROVED", approved.Outcome())
	}
	draft, ok := approved.Draft()
	approver, hasApprover := draft.Approver()
	if !ok || !hasApprover || approver.String() != "op-approver" {
		t.Fatalf("draft = %#v", draft)
	}
	if at, _ := draft.ApprovedAt(); !at.Equal(draftApprovedAt) {
		t.Fatalf("approvedAt = %s", at)
	}

	if twice := approveCredit(t, drafts, strict, operatorSubject(t, "op-other", "level-approver")); twice.Outcome() != application.PublicationDraftAlreadyApproved {
		t.Fatalf("second approval outcome = %q, want DRAFT_ALREADY_APPROVED", twice.Outcome())
	}
	if missing := approveCredit(t, newDraftRegistryDouble(), strict, operatorSubject(t, "op-approver", "level-approver")); missing.Outcome() != application.ApprovalDraftNotFound {
		t.Fatalf("missing draft outcome = %q, want DRAFT_NOT_FOUND", missing.Outcome())
	}
}

// Covers: 依赖故障不是业务答案——读规则或读载体失败上抛，端点按 ADR-0022 答「没形成答案」。
func TestApprovalPropagatesDependencyFailures(t *testing.T) {
	drafts := newDraftRegistryDouble()
	submitCredit(t, drafts, 500_000)
	tenant, kind, objectID, version := draftIdentity(t)
	broken := errors.New("rule store down")
	handler := application.NewApprovePublicationDraftHandler(drafts, approvalDutyRuleDouble{err: broken}, fixedClock{at: draftApprovedAt})
	if _, err := handler.Handle(context.Background(), application.ApprovePublicationDraftCommand{
		Tenant: tenant, Kind: kind, ObjectID: objectID, Version: version, Approver: operatorSubject(t, "op-approver"),
	}); !errors.Is(err, broken) {
		t.Fatalf("err = %v, want the store failure", err)
	}
}

// Covers: ADR-0126 Decision 三 — 发布只接`已批准`载体，把壳、摘要、正文与批准依据交给既有发布用例：版本按
// 载体的摘要入册（对账门恒成立）、信用政策正文随版本同笔登记、批准依据的引用是批准者、来源是载体引用、
// 角色确认由批准门代答；落定后载体转`已发布`并记发布时刻。
func TestPublishingAnApprovedDraftHandsItToTheExistingPublicationUseCase(t *testing.T) {
	drafts := newDraftRegistryDouble()
	submitCredit(t, drafts, 500_000)
	approveCredit(t, drafts, approvalDutyRuleDouble{rule: dutyRule(t, true, ""), found: true}, operatorSubject(t, "op-approver"))
	tenant, kind, objectID, version := draftIdentity(t)

	// 夹具壳的区间起点（pubStartsAt）早于发布时刻：边界已开，同一次处理内发布并取效，正文随之登记。
	registry := &publicationRegistryDouble{}
	publisher := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: draftPublishedAt}, &operatorRegistrationHandoffDouble{})
	handler := application.NewPublishPublicationDraftHandler(drafts, publisher, fixedClock{at: draftPublishedAt})
	result, err := handler.Handle(context.Background(), application.PublishPublicationDraftCommand{
		Tenant: tenant, Kind: kind, ObjectID: objectID, Version: version,
	})
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	if result.Outcome() != application.PublicationDraftPublished {
		t.Fatalf("outcome = %q, want DRAFT_PUBLISHED", result.Outcome())
	}
	publication, ok := result.Publication()
	if !ok || publication.Outcome() != application.CommercialVersionPublishedEffective {
		t.Fatalf("publication = %q, %v; want PUBLISHED_EFFECTIVE", publication.Outcome(), ok)
	}
	if len(registry.savedVersions) != 1 || len(registry.savedCredit) != 1 {
		t.Fatalf("saved %d versions / %d credit bodies, want 1 / 1", len(registry.savedVersions), len(registry.savedCredit))
	}
	if minor, ok := registry.savedCredit[0].AuthorizedLimit().AmountMinor(); !ok || minor != 500_000 {
		t.Fatalf("registered body limit = %d, %v", minor, ok)
	}
	saved := registry.savedVersions[0]
	stored, _, _ := drafts.LoadDraft(context.Background(), tenant, kind, objectID, version)
	if saved.ContentDigest() != stored.Canonical().Digest() {
		t.Fatalf("入册摘要 %s ≠ 载体摘要 %s", saved.ContentDigest(), stored.Canonical().Digest())
	}
	basis, _ := saved.ApprovalBasis()
	if basis.Reference().String() != "op-approver" || basis.Source() != stored.Reference() || !basis.ApprovedAt().Equal(draftApprovedAt) {
		t.Fatalf("approval basis = %#v", basis)
	}
	if stored.Status() != domain.PublicationDraftPublished {
		t.Fatalf("draft status = %s, want PUBLISHED", stored.Status())
	}
	if at, _ := stored.PublishedAt(); !at.Equal(draftPublishedAt) {
		t.Fatalf("publishedAt = %s", at)
	}
}

// Covers: 发布用例的既有语义「挂在`已计划生效`版本上的声明整项拒绝，届期改为到界发布」在载体上的落法——载体永远
// 带正文，所以区间未开的载体不交给发布用例，答`等待生效边界`：载体留在`已批准`、一个字节不写，到界后再发布。
func TestPublishingADraftBeforeItsEffectiveStartWaitsForTheBoundary(t *testing.T) {
	drafts := newDraftRegistryDouble()
	future := draftShell(t, domain.CreditPolicyObject, "credit-1", "v1")
	interval, err := domain.NewEffectiveInterval(draftPublishedAt.Add(30*24*time.Hour), time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	future.Effective = interval
	submitCreditWithShell(t, drafts, future, 500_000)
	approveCredit(t, drafts, approvalDutyRuleDouble{rule: dutyRule(t, false, ""), found: true}, operatorSubject(t, "op-approver"))
	tenant, kind, objectID, version := draftIdentity(t)

	registry := &publicationRegistryDouble{}
	publisher := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: draftPublishedAt}, &operatorRegistrationHandoffDouble{})
	result, err := application.NewPublishPublicationDraftHandler(drafts, publisher, fixedClock{at: draftPublishedAt}).Handle(
		context.Background(), application.PublishPublicationDraftCommand{Tenant: tenant, Kind: kind, ObjectID: objectID, Version: version})
	if err != nil {
		t.Fatalf("发布：%v——等边界不是 error", err)
	}
	if result.Outcome() != application.PublicationDraftAwaitsEffectiveStart {
		t.Fatalf("outcome = %q, want DRAFT_AWAITS_EFFECTIVE_START", result.Outcome())
	}
	if _, ok := result.Publication(); ok {
		t.Fatal("没交给发布用例却带了发布答案")
	}
	if len(registry.savedVersions) != 0 || registry.loads != 0 {
		t.Fatalf("等边界写了 %d 版本、读了 %d 次整册", len(registry.savedVersions), registry.loads)
	}
	if stored, _, _ := drafts.LoadDraft(context.Background(), tenant, kind, objectID, version); stored.Status() != domain.PublicationDraftApproved {
		t.Fatalf("draft status = %s, want APPROVED", stored.Status())
	}

	// 到界之后同一载体发布落定：`已计划生效`不会出现，因为到界时刻本身就让版本在同一次处理内取效。
	later := fixedClock{at: interval.StartsAt()}
	laterPublisher := application.NewPublishCommercialAuthorityHandler(registry, later, &operatorRegistrationHandoffDouble{})
	result, err = application.NewPublishPublicationDraftHandler(drafts, laterPublisher, later).Handle(
		context.Background(), application.PublishPublicationDraftCommand{Tenant: tenant, Kind: kind, ObjectID: objectID, Version: version})
	if err != nil || result.Outcome() != application.PublicationDraftPublished {
		t.Fatalf("到界发布：outcome = %q, err = %v", result.Outcome(), err)
	}
}

// Covers: ADR-0126 Decision 三 — `待批准`的载体发布不了；没有载体答`未找到`；已发布的载体再发布答`已发布`；
// 发布用例没落定（这里演`内容冲突`）时载体留在`已批准`、结果带出发布用例的答案。
func TestPublishingRefusesDraftsThatAreNotApprovedAndKeepsUnlandedOnes(t *testing.T) {
	tenant, kind, objectID, version := draftIdentity(t)
	command := application.PublishPublicationDraftCommand{Tenant: tenant, Kind: kind, ObjectID: objectID, Version: version}

	pending := newDraftRegistryDouble()
	submitCredit(t, pending, 500_000)
	publisher := application.NewPublishCommercialAuthorityHandler(&publicationRegistryDouble{}, fixedClock{at: draftPublishedAt}, &operatorRegistrationHandoffDouble{})
	result, err := application.NewPublishPublicationDraftHandler(pending, publisher, fixedClock{at: draftPublishedAt}).Handle(context.Background(), command)
	if err != nil || result.Outcome() != application.PublicationDraftNotApproved {
		t.Fatalf("pending draft: outcome = %q, err = %v; want DRAFT_NOT_APPROVED", result.Outcome(), err)
	}

	result, err = application.NewPublishPublicationDraftHandler(newDraftRegistryDouble(), publisher, fixedClock{at: draftPublishedAt}).Handle(context.Background(), command)
	if err != nil || result.Outcome() != application.PublicationDraftNotFound {
		t.Fatalf("missing draft: outcome = %q, err = %v; want DRAFT_NOT_FOUND", result.Outcome(), err)
	}

	conflicting := newDraftRegistryDouble()
	submitCredit(t, conflicting, 500_000)
	approveCredit(t, conflicting, approvalDutyRuleDouble{rule: dutyRule(t, false, ""), found: true}, operatorSubject(t, "op-approver"))
	registry := &publicationRegistryDouble{versionOutcome: ports.PublicationContentConflict}
	conflictPublisher := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: draftPublishedAt}, &operatorRegistrationHandoffDouble{})
	result, err = application.NewPublishPublicationDraftHandler(conflicting, conflictPublisher, fixedClock{at: draftPublishedAt}).Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("发布冲突：%v——没落定不是 error", err)
	}
	if result.Outcome() != application.PublicationDraftPublicationNotLanded {
		t.Fatalf("outcome = %q, want PUBLICATION_NOT_LANDED", result.Outcome())
	}
	if publication, ok := result.Publication(); !ok || publication.Outcome() != application.CommercialPublicationConflicted {
		t.Fatalf("publication = %q, %v", publication.Outcome(), ok)
	}
	if stored, _, _ := conflicting.LoadDraft(context.Background(), tenant, kind, objectID, version); stored.Status() != domain.PublicationDraftApproved {
		t.Fatalf("没落定却推进了载体：status = %s", stored.Status())
	}

	published := newDraftRegistryDouble()
	submitCredit(t, published, 500_000)
	approveCredit(t, published, approvalDutyRuleDouble{rule: dutyRule(t, false, ""), found: true}, operatorSubject(t, "op-approver"))
	landing := application.NewPublishPublicationDraftHandler(published, publisher, fixedClock{at: draftPublishedAt})
	if first, err := landing.Handle(context.Background(), command); err != nil || first.Outcome() != application.PublicationDraftPublished {
		t.Fatalf("first publication: %q, %v", first.Outcome(), err)
	}
	if again, err := landing.Handle(context.Background(), command); err != nil || again.Outcome() != application.PublicationDraftAlreadyPublished {
		t.Fatalf("second publication: %q, %v; want DRAFT_ALREADY_PUBLISHED", again.Outcome(), err)
	}
}

// Covers: 四个用例的结果都有名字（enum 门禁的镜像），零值没有。
func TestPublicationDraftOutcomesAreNamed(t *testing.T) {
	if application.PreviewCommercialPublicationOutcomeInvalid.String() != "" ||
		application.SubmitPublicationDraftOutcomeInvalid.String() != "" ||
		application.ApprovePublicationDraftOutcomeInvalid.String() != "" ||
		application.PublishPublicationDraftOutcomeInvalid.String() != "" {
		t.Fatal("zero outcomes must have no name")
	}
	for _, name := range []string{
		application.CommercialPublicationPreviewed.String(), application.CommercialPublicationPreviewNotAccepted.String(),
		application.PublicationDraftSubmitted.String(), application.PublicationDraftReplayed.String(),
		application.PublicationDraftRevised.String(), application.PublicationDraftContentFixed.String(),
		application.PublicationDraftNotAccepted.String(),
		application.PublicationDraftApproved.String(), application.ApprovalDraftNotFound.String(),
		application.ApprovalDutyRuleNotConfigured.String(), application.PublicationDraftNeedsAnotherApprover.String(),
		application.PublicationDraftApproverNotQualified.String(), application.PublicationDraftAlreadyApproved.String(),
		application.ApprovalDraftAlreadyPublished.String(), application.ApprovalDraftChanged.String(),
		application.PublicationDraftPublished.String(), application.PublicationDraftNotFound.String(),
		application.PublicationDraftNotApproved.String(), application.PublicationDraftAlreadyPublished.String(),
		application.PublicationDraftAwaitsEffectiveStart.String(), application.PublicationDraftPublicationNotLanded.String(),
	} {
		if name == "" {
			t.Fatal("an outcome has no name")
		}
	}
}
