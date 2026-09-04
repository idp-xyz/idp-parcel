package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/partycommercial"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件是两个应用层唯一相遇的地方（ADR-0025）：提供方一侧用真编排（ResolveCommercialBasisHandler
// 对着真登记册解闭包），只有权威读口、解析库、正文读口与 VE 自己的册用替身——它们各自的行为
// 由各自的包证，这里证的是翻译。夹具里的天数、日历、材料取值只是取值，不作断言依据。

var anchorAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func value[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %T from %q: %v", built, raw, err)
	}
	return built
}

// effectiveIn 用导出 API 把一个已发布生效的商业版本放进登记册并交回它（形状照抄 PS 侧同名夹具）。
func effectiveIn(
	t *testing.T,
	registry *pcdomain.CommercialRegistry,
	kind pcdomain.CommercialObjectKind,
	objectID, version, digest, scope string,
) pcdomain.CommercialVersion {
	t.Helper()

	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	draft, err := pcdomain.NewCommercialDraft(pcdomain.CommercialVersionSpec{
		TenantID:      value(t, pcdomain.NewTenantID, "tenant-1"),
		Kind:          kind,
		ObjectID:      value(t, pcdomain.NewCommercialObjectID, objectID),
		Version:       value(t, pcdomain.NewCommercialVersionLabel, version),
		Scope:         value(t, pcdomain.NewCommercialScopeReference, scope),
		ContentDigest: value(t, pcdomain.NewCommercialContentDigest, digest),
		Effective:     interval,
	})
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	basis, err := pcdomain.NewApprovalBasis(
		value(t, pcdomain.NewApprovalReference, "approval-"+objectID),
		value(t, pcdomain.NewCommercialSourceReference, "source-"+objectID),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("new approval basis: %v", err)
	}
	published, err := draft.Publish(basis, pcdomain.ApprovalRoleConfirmed, approvedAt, nil)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	live, err := published.TakeEffect(approvedAt)
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	if _, err := registry.Register(live); err != nil {
		t.Fatalf("register: %v", err)
	}
	return live
}

type authorityDouble struct {
	registry *pcdomain.CommercialRegistry
	err      error
}

func (double *authorityDouble) LoadScope(
	_ context.Context,
	_ pcdomain.TenantID,
	_ pcdomain.CommercialScopeReference,
) (*pcdomain.CommercialRegistry, error) {
	if double.err != nil {
		return nil, double.err
	}
	return double.registry, nil
}

type resolutionStoreDouble struct {
	saved []pcdomain.CommercialClosure
}

func (double *resolutionStoreDouble) LoadResolution(
	_ context.Context,
	_ pcdomain.TenantID,
	_ pcdomain.ResolutionID,
) (pcdomain.CommercialClosure, bool, error) {
	return pcdomain.CommercialClosure{}, false, nil
}

func (double *resolutionStoreDouble) Save(
	_ context.Context,
	closure pcdomain.CommercialClosure,
) (pcports.ResolutionSaveOutcome, error) {
	double.saved = append(double.saved, closure)
	return pcports.ResolutionSaved, nil
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

type ownRulesDouble struct {
	rules    veports.EligibilityRules
	declared bool
	err      error
	asked    []veports.EligibilityQuery
}

func (double *ownRulesDouble) RulesForClaim(
	_ context.Context,
	query veports.EligibilityQuery,
) (veports.EligibilityRules, bool, error) {
	double.asked = append(double.asked, query)
	if double.err != nil {
		return veports.EligibilityRules{}, false, double.err
	}
	return double.rules, double.declared, nil
}

type keySourceDouble struct {
	key    pcdomain.ClosureResolutionKey
	formed bool
	err    error
	asked  int
}

func (double *keySourceDouble) FormRuleResolutionKey(
	_ context.Context,
	_ veports.EligibilityQuery,
) (pcdomain.ClosureResolutionKey, bool, error) {
	double.asked++
	return double.key, double.formed, double.err
}

type contentsDouble struct {
	rule        pcdomain.CustomerServiceRuleVersion
	found       bool
	err         error
	askedTenant pcdomain.TenantID
	askedRule   pcdomain.CommercialVersion
}

func (double *contentsDouble) LoadCustomerServiceRule(
	_ context.Context,
	tenant pcdomain.TenantID,
	rule pcdomain.CommercialVersion,
) (pcdomain.CustomerServiceRuleVersion, bool, error) {
	double.askedTenant, double.askedRule = tenant, rule
	if double.err != nil {
		return pcdomain.CustomerServiceRuleVersion{}, false, double.err
	}
	return double.rule, double.found, nil
}

var _ pcports.CustomerServiceRuleContentView = (*contentsDouble)(nil)

type fixture struct {
	registry  *pcdomain.CommercialRegistry
	authority *authorityDouble
	store     *resolutionStoreDouble
	version   pcdomain.CommercialVersion
	own       *ownRulesDouble
	keys      *keySourceDouble
	contents  *contentsDouble
}

// newFixture 造一份「VE 自己的册在场且类型在保、PC 里恰有一版生效的客户服务规则、键来源能成键」
// 的世界；正文由各用例按需塞进 contents。
func newFixture(t *testing.T) *fixture {
	t.Helper()

	registry := pcdomain.NewCommercialRegistry()
	version := effectiveIn(t, registry, pcdomain.CustomerServiceRuleObject, "csr-1", "v1", "sha256:csr1", "scope-a")
	anchor, err := pcdomain.NewSelectionAnchor(anchorAt, value(t, pcdomain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("new selection anchor: %v", err)
	}
	return &fixture{
		registry:  registry,
		authority: &authorityDouble{registry: registry},
		store:     &resolutionStoreDouble{},
		version:   version,
		own: &ownRulesDouble{
			declared: true,
			rules: veports.EligibilityRules{
				RuleVersion: "SYN-CLAIM-RULES-1",
				KindCovered: true,
				Authorization: veports.AuthorizationCatalogue{
					Registered:  true,
					RuleVersion: "SYN-AUTH-1",
					AuthorizedApplicants: []vedomain.ApplicantReference{
						value(t, vedomain.NewApplicantReference, "applicant-1"),
					},
				},
			},
		},
		keys: &keySourceDouble{formed: true, key: pcdomain.ClosureResolutionKey{
			TenantID:             value(t, pcdomain.NewTenantID, "tenant-1"),
			CustomerAccountID:    value(t, pcdomain.NewCustomerAccountID, "customer-1"),
			LegalEntityCandidate: value(t, pcdomain.NewLegalEntityReference, "legal-1"),
			Scope:                value(t, pcdomain.NewCommercialScopeReference, "scope-a"),
			Purpose:              pcdomain.AcceptanceControlPurpose,
			Anchor:               anchor,
			RequiredBases:        []pcdomain.CommercialObjectKind{pcdomain.CustomerServiceRuleObject},
		}},
		contents: &contentsDouble{},
	}
}

func (fixture *fixture) adapter(t *testing.T, keys adapter.RuleResolutionKeySource) *adapter.ClaimServiceRules {
	t.Helper()
	built, err := adapter.NewClaimServiceRules(adapter.ClaimServiceRulesDeps{
		Rules:    fixture.own,
		Resolve:  pcapplication.NewResolveCommercialBasisHandler(fixture.authority, fixture.store, fixedClock{at: anchorAt}),
		Contents: fixture.contents,
		Keys:     keys,
	})
	if err != nil {
		t.Fatalf("new claim service rules: %v", err)
	}
	return built
}

func (fixture *fixture) query(t *testing.T) veports.EligibilityQuery {
	t.Helper()
	return veports.EligibilityQuery{
		Tenant:    value(t, vedomain.NewTenantID, "tenant-1"),
		Batch:     value(t, vedomain.NewClaimBatchReference, "batch-1"),
		Item:      value(t, vedomain.NewClaimItemID, "claim-1"),
		Customer:  value(t, vedomain.NewCustomerAccountReference, "customer-1"),
		Contract:  value(t, vedomain.NewContractScopeReference, "contract-1/liability"),
		Target:    value(t, vedomain.NewRequestScopeReference, "parcel-1"),
		Kind:      value(t, vedomain.NewClaimKindReference, "claim-loss"),
		Applicant: value(t, vedomain.NewApplicantReference, "applicant-1"),
	}
}

func (fixture *fixture) deadline(t *testing.T, kind pcdomain.ClaimDeadlineKind, event string, days int, calendar string) pcdomain.ClaimDeadlineRule {
	t.Helper()
	rule, err := pcdomain.NewClaimDeadlineRule(kind,
		value(t, pcdomain.NewDeadlineStartEventReference, event), days,
		value(t, pcdomain.NewBusinessCalendarReference, calendar))
	if err != nil {
		t.Fatalf("new claim deadline rule: %v", err)
	}
	return rule
}

func (fixture *fixture) materials(t *testing.T, claimKind string, materials ...string) pcdomain.MinimumMaterialsRule {
	t.Helper()
	references := make([]pcdomain.MaterialRequirementReference, 0, len(materials))
	for _, material := range materials {
		references = append(references, value(t, pcdomain.NewMaterialRequirementReference, material))
	}
	rule, err := pcdomain.NewMinimumMaterialsRule(value(t, pcdomain.NewClaimKindReference, claimKind), references)
	if err != nil {
		t.Fatalf("new minimum materials rule: %v", err)
	}
	return rule
}

// content 把一份正文挂到夹具那一版规则壳上并放进正文读口。
func (fixture *fixture) content(t *testing.T, deadlines []pcdomain.ClaimDeadlineRule, materials []pcdomain.MinimumMaterialsRule) {
	t.Helper()
	rule, err := pcdomain.NewCustomerServiceRuleVersion(
		fixture.version,
		pcdomain.CustomerServiceRuleAppliesToServiceProduct(value(t, pcdomain.NewCommercialObjectID, "product-1")),
		value(t, pcdomain.NewPartyID, "operator-1"),
		value(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		deadlines, materials,
	)
	if err != nil {
		t.Fatalf("new customer service rule version: %v", err)
	}
	fixture.contents.rule, fixture.contents.found = rule, true
}

// Covers: 票面「要做什么」第 2、3 条与 ADR-0104 Decision 五——两维从 PC 正文翻译：Registered 为真、
// RuleVersion 冻三段版本引用、起算事件与日历照引用转写、Scope 取索赔目标范围、Required 与 PC 条目逐项
// 相等；Deadline / SupplementDeadline / Notice 是票面「裁决」留的格，必须仍是零值——填了就是造实例参数。
// VE 自己的册交出的其余三样一字不改，正文按闭包采用的那一版、以查询租户点读。
func TestClaimServiceRulesOverlayBothDimensionsFromTheResolvedRule(t *testing.T) {
	fixture := newFixture(t)
	fixture.content(t,
		[]pcdomain.ClaimDeadlineRule{
			fixture.deadline(t, pcdomain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn"),
			fixture.deadline(t, pcdomain.MaterialSupplementDeadline, "event-materials-requested", 10, "calendar-cn"),
		},
		[]pcdomain.MinimumMaterialsRule{
			fixture.materials(t, "claim-loss", "material-photo", "material-invoice"),
			fixture.materials(t, "claim-damage", "material-photo"),
		},
	)

	rules, declared, err := fixture.adapter(t, fixture.keys).RulesForClaim(context.Background(), fixture.query(t))
	if err != nil || !declared {
		t.Fatalf("declared=%v err=%v", declared, err)
	}

	deadline := rules.FilingDeadline
	if !deadline.Registered {
		t.Fatal("PC 登了 FIRST_CLAIM 那一行，首次索赔期限维却答未登记")
	}
	if deadline.RuleVersion != "tenant-1/csr-1/v1" {
		t.Fatalf("RuleVersion = %q，要冻租户/对象/版本号三段引用", deadline.RuleVersion)
	}
	if deadline.StartEvent != "event-delivered" || deadline.Calendar != "calendar-cn" {
		t.Fatalf("起算事件/日历 = %q/%q，要照 PC 引用转写", deadline.StartEvent, deadline.Calendar)
	}
	if deadline.Scope != "parcel-1" {
		t.Fatalf("Scope = %q，要取索赔自己固定的目标范围", deadline.Scope)
	}
	if !deadline.Deadline.IsZero() {
		t.Fatalf("Deadline = %v；起算事实源与业务日历今天都没有，截止时刻只能留格", deadline.Deadline)
	}

	materials := rules.Materials
	if !materials.Registered {
		t.Fatal("PC 登了本索赔类型的材料清单，最低材料维却答未登记")
	}
	if materials.RuleVersion != "tenant-1/csr-1/v1" {
		t.Fatalf("材料维 RuleVersion = %q", materials.RuleVersion)
	}
	if len(materials.Required) != 2 ||
		materials.Required[0].String() != "material-invoice" ||
		materials.Required[1].String() != "material-photo" {
		t.Fatalf("Required = %v，要与 PC 条目逐项相等（PC 按稳定顺序交回）", materials.Required)
	}
	if materials.Notice.String() != "" || !materials.SupplementDeadline.IsZero() {
		t.Fatalf("Notice=%q SupplementDeadline=%v；两样今天都没有来源，只能留格", materials.Notice, materials.SupplementDeadline)
	}

	if rules.RuleVersion != "SYN-CLAIM-RULES-1" || !rules.KindCovered ||
		!rules.Authorization.Registered || len(rules.Authorization.AuthorizedApplicants) != 1 {
		t.Fatalf("VE 自己的册交出的三维被改动了：%#v", rules)
	}
	if fixture.contents.askedTenant.String() != "tenant-1" ||
		fixture.contents.askedRule.ObjectID().String() != "csr-1" ||
		fixture.contents.askedRule.Version().String() != "v1" {
		t.Fatalf("点读用的租户/版本 = %q/%q/%q，要是查询租户与闭包采用的那一版",
			fixture.contents.askedTenant, fixture.contents.askedRule.ObjectID(), fixture.contents.askedRule.Version())
	}
	if len(fixture.store.saved) != 1 {
		t.Fatalf("唯一解析要固定进 PC 解析库（ADR-0027），实得 %d 笔", len(fixture.store.saved))
	}
}

// Covers: 端口合同「第二个返回值为 false 只有『合同的索赔资格声明不在场』一个意思」——VE 自己的册
// 说不在场时原样交回，不去问 PC：连「这个类型在不在保」都无从谈起，两维也没有可挂的地方。
func TestClaimServiceRulesPassThroughWhenTheOwnCatalogueIsAbsent(t *testing.T) {
	fixture := newFixture(t)
	fixture.own.declared = false
	fixture.content(t, []pcdomain.ClaimDeadlineRule{fixture.deadline(t, pcdomain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn")}, nil)

	rules, declared, err := fixture.adapter(t, fixture.keys).RulesForClaim(context.Background(), fixture.query(t))
	if err != nil || declared {
		t.Fatalf("declared=%v err=%v，要原样交回「声明不在场」", declared, err)
	}
	if rules.FilingDeadline.Registered || rules.Materials.Registered {
		t.Fatal("声明不在场却给两维挂了登记")
	}
	if fixture.keys.asked != 0 {
		t.Fatal("声明不在场还去问了 PC")
	}
}

// Covers: 票面「裁决」——解析键的翻译是实例半边，没有键来源就是显式未配置：两维如实答未登记（恢复
// 方向是去登记），VE 自己的册交出的三维照旧。
func TestClaimServiceRulesAnswerUnregisteredWithoutAKeySource(t *testing.T) {
	fixture := newFixture(t)
	fixture.content(t, []pcdomain.ClaimDeadlineRule{fixture.deadline(t, pcdomain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn")}, nil)

	rules, declared, err := fixture.adapter(t, nil).RulesForClaim(context.Background(), fixture.query(t))
	if err != nil || !declared {
		t.Fatalf("declared=%v err=%v", declared, err)
	}
	if rules.FilingDeadline.Registered || rules.Materials.Registered {
		t.Fatalf("没有键来源却答了登记：%#v", rules)
	}
	if rules.RuleVersion != "SYN-CLAIM-RULES-1" || !rules.KindCovered || !rules.Authorization.Registered {
		t.Fatalf("VE 自己的册交出的三维被改动了：%#v", rules)
	}
}

// Covers: 票面「裁决」第一条——Registered 各维按「PC 那一项有没有行」答，版本壳在场不等于两维都登记。
func TestClaimServiceRulesAnswerEachDimensionByItsOwnRow(t *testing.T) {
	cases := map[string]struct {
		arrange      func(t *testing.T, fixture *fixture)
		wantDeadline bool
		wantMaterial bool
	}{
		"只登了首次索赔期限": {
			arrange: func(t *testing.T, fixture *fixture) {
				fixture.content(t, []pcdomain.ClaimDeadlineRule{fixture.deadline(t, pcdomain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn")}, nil)
			},
			wantDeadline: true,
		},
		"只登了本类型的材料清单": {
			arrange: func(t *testing.T, fixture *fixture) {
				fixture.content(t, nil, []pcdomain.MinimumMaterialsRule{fixture.materials(t, "claim-loss", "material-photo")})
			},
			wantMaterial: true,
		},
		"只登了别的类型的材料清单": {
			arrange: func(t *testing.T, fixture *fixture) {
				fixture.content(t, nil, []pcdomain.MinimumMaterialsRule{fixture.materials(t, "claim-damage", "material-photo")})
			},
		},
		"只登了别的种类的期限": {
			arrange: func(t *testing.T, fixture *fixture) {
				fixture.content(t, []pcdomain.ClaimDeadlineRule{fixture.deadline(t, pcdomain.ConclusionReviewDeadline, "event-conclusion-notified", 15, "calendar-cn")}, nil)
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t)
			tc.arrange(t, fixture)

			rules, declared, err := fixture.adapter(t, fixture.keys).RulesForClaim(context.Background(), fixture.query(t))
			if err != nil || !declared {
				t.Fatalf("declared=%v err=%v", declared, err)
			}
			if rules.FilingDeadline.Registered != tc.wantDeadline {
				t.Fatalf("FilingDeadline.Registered = %v, want %v", rules.FilingDeadline.Registered, tc.wantDeadline)
			}
			if rules.Materials.Registered != tc.wantMaterial {
				t.Fatalf("Materials.Registered = %v, want %v", rules.Materials.Registered, tc.wantMaterial)
			}
		})
	}
}

// Covers: 提供方没东西可交的三种缺席都是「未登记」而不是 error——键来源答形不成键（映射没登）、闭包
// `无适用依据`（这个范围里没有生效的规则版本）、点读 found=false（壳在正文没登）。三种的恢复动作都是
// 去登记，只是登的东西不同；一律不折成报错，报错的恢复动作是重试依赖，那对这三种都无用。
func TestClaimServiceRulesAnswerUnregisteredWhenTheProviderHasNothing(t *testing.T) {
	cases := map[string]func(t *testing.T, fixture *fixture){
		"键来源形不成键": func(_ *testing.T, fixture *fixture) {
			fixture.keys.formed = false
		},
		"范围里没有生效的规则版本": func(t *testing.T, fixture *fixture) {
			fixture.authority.registry = pcdomain.NewCommercialRegistry()
			fixture.content(t, []pcdomain.ClaimDeadlineRule{fixture.deadline(t, pcdomain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn")}, nil)
		},
		"壳在正文没登": func(_ *testing.T, fixture *fixture) {
			fixture.contents.found = false
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t)
			arrange(t, fixture)

			rules, declared, err := fixture.adapter(t, fixture.keys).RulesForClaim(context.Background(), fixture.query(t))
			if err != nil || !declared {
				t.Fatalf("declared=%v err=%v，缺席不是 error", declared, err)
			}
			if rules.FilingDeadline.Registered || rules.Materials.Registered {
				t.Fatalf("提供方没东西却答了登记：%#v", rules)
			}
			if !rules.KindCovered || !rules.Authorization.Registered {
				t.Fatalf("VE 自己的册交出的维被改动了：%#v", rules)
			}
		})
	}
}

// Covers: 端口合同「依赖调不通作为错误返回」与 ADR-0025 全函数——下列每一格都不许折成「未登记」：
// 折了会把租户支去登一份其实已经存在（或根本不是缺登记）的东西。键配错（缺客户服务规则那一类、
// 或答成别的租户）是配置缺陷走哨兵；`适用冲突`要商业责任方修重叠、权威读不到要重试，两者走
// ErrCustomerServiceRuleUnresolved；VE 自己的册、键来源与正文读口的 error 原样上抛。
func TestClaimServiceRulesReportErrorsInsteadOfFoldingThemIntoUnregistered(t *testing.T) {
	ownErr := errors.New("own catalogue unreachable")
	keysErr := errors.New("key store unreachable")
	contentsErr := errors.New("content store unreachable")
	authorityErr := errors.New("authority unreachable")

	cases := map[string]struct {
		arrange func(t *testing.T, fixture *fixture)
		want    error
	}{
		"VE 自己的册报错": {
			arrange: func(_ *testing.T, fixture *fixture) { fixture.own.err = ownErr },
			want:    ownErr,
		},
		"键来源报错": {
			arrange: func(_ *testing.T, fixture *fixture) { fixture.keys.err = keysErr },
			want:    keysErr,
		},
		"键没把客户服务规则列为必需依据": {
			arrange: func(_ *testing.T, fixture *fixture) {
				fixture.keys.key.RequiredBases = []pcdomain.CommercialObjectKind{pcdomain.CustomerContractObject}
			},
			want: adapter.ErrUntranslatableAnswer,
		},
		"键答成了别的租户": {
			arrange: func(t *testing.T, fixture *fixture) {
				fixture.keys.key.TenantID = value(t, pcdomain.NewTenantID, "tenant-2")
			},
			want: adapter.ErrUntranslatableAnswer,
		},
		"同范围两版规则都生效": {
			arrange: func(t *testing.T, fixture *fixture) {
				effectiveIn(t, fixture.registry, pcdomain.CustomerServiceRuleObject, "csr-2", "v1", "sha256:csr2", "scope-a")
			},
			want: adapter.ErrCustomerServiceRuleUnresolved,
		},
		"权威读不到": {
			arrange: func(_ *testing.T, fixture *fixture) { fixture.authority.err = authorityErr },
			want:    adapter.ErrCustomerServiceRuleUnresolved,
		},
		"正文读口报错": {
			arrange: func(_ *testing.T, fixture *fixture) { fixture.contents.err = contentsErr },
			want:    contentsErr,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t)
			fixture.content(t, []pcdomain.ClaimDeadlineRule{fixture.deadline(t, pcdomain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn")}, nil)
			tc.arrange(t, fixture)

			rules, declared, err := fixture.adapter(t, fixture.keys).RulesForClaim(context.Background(), fixture.query(t))
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if declared || rules.FilingDeadline.Registered || rules.Materials.Registered {
				t.Fatalf("报错的同时还交了答案：declared=%v rules=%#v", declared, rules)
			}
		})
	}
}

// Covers: 装配缺陷在构造期就拒——三个非可选协作方缺一即 error。Keys 可缺（显式未配置），其余不可：
// 本适配器一旦装上就是要真去问商业侧的，缺一半而静默答未登记会让装配疏漏与租户没登记长得一样。
func TestClaimServiceRulesRefuseToBeBuiltWithoutTheirCollaborators(t *testing.T) {
	fixture := newFixture(t)
	resolve := pcapplication.NewResolveCommercialBasisHandler(fixture.authority, fixture.store, fixedClock{at: anchorAt})
	cases := map[string]adapter.ClaimServiceRulesDeps{
		"缺 VE 自己的册": {Resolve: resolve, Contents: fixture.contents},
		"缺解析编排":     {Rules: fixture.own, Contents: fixture.contents},
		"缺正文读口":     {Rules: fixture.own, Resolve: resolve},
	}
	for name, deps := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := adapter.NewClaimServiceRules(deps); err == nil {
				t.Fatal("缺协作方却装配成功")
			}
		})
	}
}
