package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证价卡草稿的批准与发布编排（ADR-0101 决定四至六；票 price-card-import/04）：批准先看草稿在哪一格、再读审批职责
// 规则，未登记即`未配置`、不放行，规则逐格的拒绝各成一格；发布只接`已批准`，交给既有登记用例的就是草稿本身，登记的
// 每格答复原样交回，落定才推进草稿。

const (
	draftApprover          = "https://id.syn.example/dex#SYN-OPERATOR-02"
	pricingSupervisorGrant = "SYN-GRANT-PRICING-SUPERVISOR"
	draftSourceSHA         = "9edaf27ef93004e00f73a65471897f2cf7064d5d4df05014934ef7ac5861d33d"
	draftSourceName        = "SYN-PRC-CARD-260820.xlsx"
	draftDirectionID       = "SYN-AUTH-COST-DIR-01"
)

var progressClockAt = draftClockAt.Add(time.Hour)

// draftProgressFake 是推进口的内存替身：按键读一版；推进时那一行若已不在，或被标成「读与写之间被别人动过」，各答其格。
type draftProgressFake struct {
	rows       map[string]domain.PriceCardDraft
	advanced   []domain.PriceCardDraft
	superseded bool
	loadErr    error
}

func progressKey(tenant domain.TenantID, plan domain.VersionReference) string {
	return tenant.String() + "/" + plan.ID() + "/" + plan.Version()
}

func newDraftProgressFake(drafts ...domain.PriceCardDraft) *draftProgressFake {
	fake := &draftProgressFake{rows: map[string]domain.PriceCardDraft{}}
	for _, draft := range drafts {
		fake.rows[progressKey(draft.Tenant(), draft.Plan())] = draft
	}
	return fake
}

func (fake *draftProgressFake) LoadPriceCardDraft(_ context.Context, tenant domain.TenantID, plan domain.VersionReference) (domain.PriceCardDraft, bool, error) {
	if fake.loadErr != nil {
		return domain.PriceCardDraft{}, false, fake.loadErr
	}
	draft, found := fake.rows[progressKey(tenant, plan)]
	return draft, found, nil
}

func (fake *draftProgressFake) AdvancePriceCardDraft(_ context.Context, draft domain.PriceCardDraft) (ports.PriceCardDraftAdvanceOutcome, error) {
	if fake.superseded {
		return ports.PriceCardDraftAdvanceSuperseded, nil
	}
	key := progressKey(draft.Tenant(), draft.Plan())
	if _, found := fake.rows[key]; !found {
		return ports.PriceCardDraftAdvanceNotFound, nil
	}
	fake.rows[key] = draft
	fake.advanced = append(fake.advanced, draft)
	return ports.PriceCardDraftAdvanced, nil
}

// approvalDutyRuleFake 演「租户登了 / 没登审批职责规则」，并数被读了几次。
type approvalDutyRuleFake struct {
	rule  domain.PriceCardApprovalDutyRule
	found bool
	err   error
	reads int
}

func (fake *approvalDutyRuleFake) LoadPriceCardApprovalDutyRule(context.Context, domain.TenantID) (domain.PriceCardApprovalDutyRule, bool, error) {
	fake.reads++
	return fake.rule, fake.found, fake.err
}

func draftTenant(t *testing.T) domain.TenantID {
	t.Helper()
	return mustValue(t, domain.NewTenantID, "SYN-TENANT-01")
}

func configuredRule(t *testing.T, distinct bool, requiredGrant string) *approvalDutyRuleFake {
	t.Helper()
	var required domain.OperatorGrant
	if requiredGrant != "" {
		required = mustValue(t, domain.NewOperatorGrant, requiredGrant)
	}
	rule, err := domain.NewPriceCardApprovalDutyRule(draftTenant(t), distinct, required)
	if err != nil {
		t.Fatalf("构造审批职责规则：%v", err)
	}
	return &approvalDutyRuleFake{rule: rule, found: true}
}

func subject(t *testing.T, reference string, grants ...string) domain.OperatorSubject {
	t.Helper()
	held := make([]domain.OperatorGrant, 0, len(grants))
	for _, name := range grants {
		held = append(held, mustValue(t, domain.NewOperatorGrant, name))
	}
	built, err := domain.NewOperatorSubject(reference, held)
	if err != nil {
		t.Fatalf("构造操作者主体：%v", err)
	}
	return built
}

// validatedPriceCardDraft 是一份读通了的草稿：录入者 SYN-OPERATOR-01，方案与方向授权齐备。
func validatedPriceCardDraft(t *testing.T) domain.PriceCardDraft {
	t.Helper()
	plan := minimalPlan(t)
	source, err := domain.NewSourceFileIdentity(draftSourceName, draftSourceSHA)
	if err != nil {
		t.Fatalf("构造源文件身份：%v", err)
	}
	direction, err := domain.NewVersionReferenceIdentity(domain.ArtifactCommercialAuthorization, draftDirectionID, "v1")
	if err != nil {
		t.Fatalf("构造方向授权引用：%v", err)
	}
	draft, err := domain.SubmitPriceCardDraft(domain.PriceCardDraftSubmission{
		Tenant: draftTenant(t), Plan: plan.Reference(), Source: source,
		Content:   &domain.PriceCardDraftContent{Plan: plan, DirectionAuthorization: direction},
		Submitter: draftSubmitter, SubmittedAt: draftClockAt,
	})
	if err != nil {
		t.Fatalf("立草稿：%v", err)
	}
	return draft
}

func problemPriceCardDraft(t *testing.T, plan domain.VersionReference) domain.PriceCardDraft {
	t.Helper()
	source, _ := domain.NewSourceFileIdentity(draftSourceName, draftSourceSHA)
	problem, err := domain.NewPriceCardDraftProblem("价表", 3, "base", "AMOUNT_INVALID", "这一格立不住")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := domain.SubmitPriceCardDraft(domain.PriceCardDraftSubmission{
		Tenant: draftTenant(t), Plan: plan, Source: source, Problems: []domain.PriceCardDraftProblem{problem},
		Submitter: draftSubmitter, SubmittedAt: draftClockAt,
	})
	if err != nil {
		t.Fatalf("立`草稿`：%v", err)
	}
	return draft
}

func approvedPriceCardDraft(t *testing.T) domain.PriceCardDraft {
	t.Helper()
	rule := configuredRule(t, true, "")
	approved, err := validatedPriceCardDraft(t).Approve(subject(t, draftApprover), rule.rule, progressClockAt)
	if err != nil {
		t.Fatalf("批准：%v", err)
	}
	return approved
}

func newApproveHandler(t *testing.T, drafts *draftProgressFake, rules *approvalDutyRuleFake) *application.ApprovePriceCardDraftHandler {
	t.Helper()
	handler, err := application.NewApprovePriceCardDraftHandler(drafts, rules, fixedClock{at: progressClockAt})
	if err != nil {
		t.Fatalf("构造批准编排：%v", err)
	}
	return handler
}

func approve(t *testing.T, handler *application.ApprovePriceCardDraftHandler, plan domain.VersionReference, approver domain.OperatorSubject) application.ApprovePriceCardDraftResult {
	t.Helper()
	result, err := handler.Handle(context.Background(), application.ApprovePriceCardDraftCommand{
		Tenant: draftTenant(t), Plan: plan, Approver: approver,
	})
	if err != nil {
		t.Fatalf("批准编排：%v", err)
	}
	return result
}

// Covers: ADR-0101 决定六——规则未登记即`未配置`、不放行：草稿一字不动，不以「单人可批」或「必须双人」代替。
func TestApprovalAnswersNotConfiguredWhileTheTenantHasNoRule(t *testing.T) {
	validated := validatedPriceCardDraft(t)
	drafts := newDraftProgressFake(validated)

	result := approve(t, newApproveHandler(t, drafts, &approvalDutyRuleFake{}), validated.Plan(), subject(t, draftApprover, pricingSupervisorGrant))
	if result.Outcome != application.PriceCardApprovalNotConfigured || result.Outcome.String() != "NOT_CONFIGURED" {
		t.Fatalf("outcome = %v，want NOT_CONFIGURED", result.Outcome)
	}
	if len(drafts.advanced) != 0 {
		t.Fatal("未配置却推进了草稿")
	}
	if _, has := result.Draft, result.HasDraft; has {
		t.Fatal("未配置不该交回草稿")
	}
}

// Covers: 票 04 第 3 条——规则在场时逐格裁：主体相同、授予不足各答一格且草稿不动；两格都过即转`已批准`，记下批准者与批准
// 时刻（取编排的时钟）。
func TestApprovalGateAnswersEachCellOfTheRule(t *testing.T) {
	cases := []struct {
		name     string
		rule     func(*testing.T) *approvalDutyRuleFake
		approver func(*testing.T) domain.OperatorSubject
		want     application.ApprovePriceCardDraftOutcome
		wantName string
	}{
		{"主体相同", func(t *testing.T) *approvalDutyRuleFake { return configuredRule(t, true, "") },
			func(t *testing.T) domain.OperatorSubject { return subject(t, draftSubmitter, pricingSupervisorGrant) },
			application.PriceCardApprovalNeedsAnotherApprover, "NEEDS_ANOTHER_APPROVER"},
		{"授予不足", func(t *testing.T) *approvalDutyRuleFake { return configuredRule(t, false, pricingSupervisorGrant) },
			func(t *testing.T) domain.OperatorSubject {
				return subject(t, draftApprover, "REGISTRY_CONFIGURATION_WRITE")
			},
			application.PriceCardApprovalApproverNotQualified, "APPROVER_NOT_QUALIFIED"},
		{"通过", func(t *testing.T) *approvalDutyRuleFake { return configuredRule(t, true, pricingSupervisorGrant) },
			func(t *testing.T) domain.OperatorSubject { return subject(t, draftApprover, pricingSupervisorGrant) },
			application.PriceCardDraftApproved, "DRAFT_APPROVED"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			validated := validatedPriceCardDraft(t)
			drafts := newDraftProgressFake(validated)
			result := approve(t, newApproveHandler(t, drafts, test.rule(t)), validated.Plan(), test.approver(t))
			if result.Outcome != test.want || result.Outcome.String() != test.wantName {
				t.Fatalf("outcome = %v，want %s", result.Outcome, test.wantName)
			}
			if test.want != application.PriceCardDraftApproved {
				if len(drafts.advanced) != 0 {
					t.Fatal("被拒的批准推进了草稿")
				}
				return
			}
			draft, has := result.Draft, result.HasDraft
			if !has || draft.Status() != domain.PriceCardDraftStatusApproved || len(drafts.advanced) != 1 {
				t.Fatalf("通过却没推进：has=%v advanced=%d", has, len(drafts.advanced))
			}
			if approver, _ := draft.Approver(); approver != draftApprover {
				t.Fatalf("approver = %q", approver)
			}
			if at, _ := draft.ApprovedAt(); !at.Equal(progressClockAt) {
				t.Fatalf("approvedAt = %v，want 编排时钟 %v", at, progressClockAt)
			}
		})
	}
}

// Covers: 票 04 第 3 条「只接`已校验`」——状态不对按草稿此刻在哪一格答，不读规则、不重复批；册上没有这一版答不在。
func TestApprovalAnswersFromTheCellTheDraftIsIn(t *testing.T) {
	validated := validatedPriceCardDraft(t)
	approved := approvedPriceCardDraft(t)
	published, err := approved.MarkPublished(progressClockAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		draft domain.PriceCardDraft
		want  string
	}{
		{"草稿", problemPriceCardDraft(t, validated.Plan()), "DRAFT_NOT_VALIDATED"},
		{"已批准", approved, "DRAFT_ALREADY_APPROVED"},
		{"已发布", published, "DRAFT_ALREADY_PUBLISHED"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			drafts := newDraftProgressFake(test.draft)
			rules := configuredRule(t, false, "")
			result := approve(t, newApproveHandler(t, drafts, rules), test.draft.Plan(), subject(t, draftApprover))
			if result.Outcome.String() != test.want {
				t.Fatalf("outcome = %v，want %s", result.Outcome, test.want)
			}
			if rules.reads != 0 || len(drafts.advanced) != 0 {
				t.Fatalf("状态不对却读了规则（%d 次）或推进了草稿（%d 次）", rules.reads, len(drafts.advanced))
			}
		})
	}

	missing := approve(t, newApproveHandler(t, newDraftProgressFake(), configuredRule(t, false, "")), validated.Plan(), subject(t, draftApprover))
	if missing.Outcome != application.PriceCardApprovalDraftNotFound || missing.Outcome.String() != "DRAFT_NOT_FOUND" {
		t.Fatalf("册上没有：outcome = %v", missing.Outcome)
	}
}

// 读与写之间那一行被别人动过（修订或已被推进）：答`草稿已变`让后到的重读再来，不覆盖；读口与规则读不动是依赖故障，上抛。
func TestApprovalThatLosesTheRaceOrItsDependenciesIsNotAnApproval(t *testing.T) {
	validated := validatedPriceCardDraft(t)
	drafts := newDraftProgressFake(validated)
	drafts.superseded = true
	result := approve(t, newApproveHandler(t, drafts, configuredRule(t, false, "")), validated.Plan(), subject(t, draftApprover))
	if result.Outcome != application.PriceCardApprovalDraftChanged || result.Outcome.String() != "DRAFT_CHANGED" {
		t.Fatalf("outcome = %v，want DRAFT_CHANGED", result.Outcome)
	}

	unavailable := errors.New("规则册读不动")
	handler := newApproveHandler(t, newDraftProgressFake(validated), &approvalDutyRuleFake{err: unavailable})
	if _, err := handler.Handle(context.Background(), application.ApprovePriceCardDraftCommand{
		Tenant: draftTenant(t), Plan: validated.Plan(), Approver: subject(t, draftApprover),
	}); !errors.Is(err, unavailable) {
		t.Fatalf("规则读不动：err = %v", err)
	}
	broken := newDraftProgressFake()
	broken.loadErr = errors.New("草稿册读不动")
	if _, err := newApproveHandler(t, broken, configuredRule(t, false, "")).Handle(context.Background(), application.ApprovePriceCardDraftCommand{
		Tenant: draftTenant(t), Plan: validated.Plan(), Approver: subject(t, draftApprover),
	}); !errors.Is(err, broken.loadErr) {
		t.Fatalf("草稿册读不动：err = %v", err)
	}
}

func newPublishHandler(t *testing.T, drafts *draftProgressFake, catalog *priceCardCatalogDouble) *application.PublishPriceCardDraftHandler {
	t.Helper()
	registrar := application.NewRegisterPriceCardHandler(application.RegisterPriceCardDeps{Catalog: catalog})
	handler, err := application.NewPublishPriceCardDraftHandler(drafts, registrar, fixedClock{at: progressClockAt.Add(time.Minute)})
	if err != nil {
		t.Fatalf("构造发布编排：%v", err)
	}
	return handler
}

// Covers: ADR-0101 决定四、五——交给登记用例的就是草稿上那一份：摘要与规范化版本逐字节等于录入时算出的那个，源文件身份、
// 方向授权引用照抄，发布批准责任方是批准者主体而不是录入者；登记落定后草稿转`已发布`，发布时刻取编排的时钟。
func TestPublishingHandsTheApprovedDraftItselfToTheRegistration(t *testing.T) {
	approved := approvedPriceCardDraft(t)
	content, _ := approved.Content()
	submittedDigest, submittedCanonicalization := content.Plan.ContentDigest(), content.Plan.CanonicalizationVersion()
	drafts := newDraftProgressFake(approved)
	catalog := &priceCardCatalogDouble{outcome: ports.PriceCardRegistered}

	result, err := newPublishHandler(t, drafts, catalog).Handle(context.Background(),
		application.PublishPriceCardDraftCommand{Tenant: draftTenant(t), Plan: approved.Plan()})
	if err != nil {
		t.Fatalf("发布编排：%v", err)
	}
	if result.Outcome != application.PriceCardDraftPublished || result.Outcome.String() != "DRAFT_PUBLISHED" {
		t.Fatalf("outcome = %v，want DRAFT_PUBLISHED", result.Outcome)
	}
	if len(catalog.registered) != 1 {
		t.Fatalf("登记用例收到 %d 份，want 1", len(catalog.registered))
	}
	handed := catalog.registered[0]
	if handed.Plan().ContentDigest() != submittedDigest || handed.Plan().CanonicalizationVersion() != submittedCanonicalization {
		t.Fatalf("摘要漂移：交出 %s/%s，录入时 %s/%s", handed.Plan().CanonicalizationVersion(), handed.Plan().ContentDigest(),
			submittedCanonicalization, submittedDigest)
	}
	if handed.SourceFile().Name() != draftSourceName || handed.SourceFile().SHA256() != draftSourceSHA ||
		handed.DirectionAuthorization().ID() != draftDirectionID || handed.Tenant() != draftTenant(t) {
		t.Fatalf("登记没照抄草稿：%+v", handed)
	}
	if handed.PublicationApprover() != draftApprover {
		t.Fatalf("publicationApprover = %q，want 批准者 %q", handed.PublicationApprover(), draftApprover)
	}
	draft, has := result.Draft, result.HasDraft
	if !has || draft.Status() != domain.PriceCardDraftStatusPublished || len(drafts.advanced) != 1 {
		t.Fatalf("登记落定却没推进草稿：has=%v advanced=%d", has, len(drafts.advanced))
	}
	if at, _ := draft.PublishedAt(); !at.Equal(progressClockAt.Add(time.Minute)) {
		t.Fatalf("publishedAt = %v", at)
	}
}

// Covers: ADR-0101 决定五「答案代数一格不改」——登记用例的每格答复原样交回：入册与重放算落定、草稿转`已发布`；冲突、
// 规范化不可比与依赖故障都没落定，草稿留在`已批准`，依赖故障连原因一起上抛。
func TestEveryRegistrationAnswerIsHandedBackAsIs(t *testing.T) {
	unavailable := errors.New("价卡册写不动")
	cases := []struct {
		name      string
		catalog   *priceCardCatalogDouble
		want      string
		published bool
	}{
		{"RECORDED", &priceCardCatalogDouble{outcome: ports.PriceCardRegistered}, "DRAFT_PUBLISHED", true},
		{"ALREADY_REGISTERED", &priceCardCatalogDouble{outcome: ports.PriceCardAlreadyRegistered}, "DRAFT_PUBLISHED", true},
		{"CONTENT_CONFLICT", &priceCardCatalogDouble{outcome: ports.PriceCardContentConflict}, "PUBLICATION_NOT_LANDED", false},
		{"CANONICALIZATION_DIFFERS", &priceCardCatalogDouble{outcome: ports.PriceCardCanonicalizationDiffers}, "PUBLICATION_NOT_LANDED", false},
		{"UNDECIDED", &priceCardCatalogDouble{err: unavailable}, "PUBLICATION_NOT_LANDED", false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			approved := approvedPriceCardDraft(t)
			drafts := newDraftProgressFake(approved)
			result, err := newPublishHandler(t, drafts, test.catalog).Handle(context.Background(),
				application.PublishPriceCardDraftCommand{Tenant: draftTenant(t), Plan: approved.Plan()})
			if test.name == "UNDECIDED" {
				if !errors.Is(err, unavailable) {
					t.Fatalf("依赖故障：err = %v，want 原因随错误上抛", err)
				}
			} else if err != nil {
				t.Fatalf("发布编排：%v", err)
			}
			registration, handed := result.Registration, result.HasRegistration
			if !handed || registration.String() != test.name {
				t.Fatalf("登记答复 = %v（%v），want 原样的 %s", registration, handed, test.name)
			}
			if result.Outcome.String() != test.want {
				t.Fatalf("outcome = %v，want %s", result.Outcome, test.want)
			}
			if published := len(drafts.advanced) == 1; published != test.published {
				t.Fatalf("推进了 %d 次，want 落定才推进（%v）", len(drafts.advanced), test.published)
			}
		})
	}
}

// Covers: 票 04 第 4 条「只接`已批准`」——`草稿`与`已校验`答未批准、`已发布`答已发布，三者都不交给登记用例；册上没有答不在。
func TestPublishingAnswersFromTheCellTheDraftIsIn(t *testing.T) {
	validated := validatedPriceCardDraft(t)
	published, err := approvedPriceCardDraft(t).MarkPublished(progressClockAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		draft domain.PriceCardDraft
		want  string
	}{
		{"草稿", problemPriceCardDraft(t, validated.Plan()), "DRAFT_NOT_APPROVED"},
		{"已校验", validated, "DRAFT_NOT_APPROVED"},
		{"已发布", published, "DRAFT_ALREADY_PUBLISHED"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			catalog := &priceCardCatalogDouble{outcome: ports.PriceCardRegistered}
			result, err := newPublishHandler(t, newDraftProgressFake(test.draft), catalog).Handle(context.Background(),
				application.PublishPriceCardDraftCommand{Tenant: draftTenant(t), Plan: test.draft.Plan()})
			if err != nil {
				t.Fatalf("发布编排：%v", err)
			}
			if result.Outcome.String() != test.want || len(catalog.registered) != 0 {
				t.Fatalf("outcome = %v（登记收到 %d 份），want %s 且不交登记", result.Outcome, len(catalog.registered), test.want)
			}
		})
	}

	missing, err := newPublishHandler(t, newDraftProgressFake(), &priceCardCatalogDouble{}).Handle(context.Background(),
		application.PublishPriceCardDraftCommand{Tenant: draftTenant(t), Plan: validated.Plan()})
	if err != nil || missing.Outcome.String() != "DRAFT_NOT_FOUND" {
		t.Fatalf("册上没有：outcome = %v err = %v", missing.Outcome, err)
	}
}

// 登记落定之后草稿行读与写之间被动过：登记已写、草稿没跟上，上抛让整笔事务回滚、两边一起重来。
func TestAPublicationWhoseDraftChangedUnderneathRollsBack(t *testing.T) {
	approved := approvedPriceCardDraft(t)
	drafts := newDraftProgressFake(approved)
	drafts.superseded = true
	_, err := newPublishHandler(t, drafts, &priceCardCatalogDouble{outcome: ports.PriceCardRegistered}).Handle(context.Background(),
		application.PublishPriceCardDraftCommand{Tenant: draftTenant(t), Plan: approved.Plan()})
	if err == nil {
		t.Fatal("草稿被替换却没上抛：登记与草稿会分岔")
	}
}
