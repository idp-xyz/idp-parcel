package partycommercial_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/partycommercial"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证的是翻译（ADR-0025）：两个提供方的读口——parcel-shipment 的回指读口与 party-commercial 的闭包读口、
// 正文读口——与 VE 自己的册全用替身，各自的行为由各自的包证。闭包本身用 PC 领域的 ResolveCommercialClosure 对着
// 真登记册解出来，而不是手搓一份：适配器读的是「接受时固定的闭包」，它的形状（采用了哪些类别、回指怎么算）由
// 提供方定义，替身只负责把它按（租户，回指）交回来。夹具里的天数、日历、材料取值只是取值，不作断言依据。

var anchorAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func value[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %T from %q: %v", built, raw, err)
	}
	return built
}

// liveVersion 用导出 API 造一个已发布生效的商业版本（形状照抄 PS 侧同名夹具）。
func liveVersion(
	t *testing.T,
	tenant string,
	kind pcdomain.CommercialObjectKind,
	objectID, version, digest, scope string,
) pcdomain.CommercialVersion {
	t.Helper()

	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	draft, err := pcdomain.NewCommercialDraft(pcdomain.CommercialVersionSpec{
		TenantID:      value(t, pcdomain.NewTenantID, tenant),
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
	return live
}

// effectiveIn 把一个已发布生效的商业版本放进登记册并交回它。
func effectiveIn(
	t *testing.T,
	registry *pcdomain.CommercialRegistry,
	tenant string,
	kind pcdomain.CommercialObjectKind,
	objectID, version, digest, scope string,
) pcdomain.CommercialVersion {
	t.Helper()
	live := liveVersion(t, tenant, kind, objectID, version, digest, scope)
	if _, err := registry.Register(live); err != nil {
		t.Fatalf("register: %v", err)
	}
	return live
}

type referenceSourceDouble struct {
	reference   psdomain.CommercialResolutionID
	present     bool
	err         error
	asked       int
	askedTenant psdomain.TenantID
	askedParcel psdomain.DeclaredParcelID
}

func (double *referenceSourceDouble) LoadCommercialResolutionReference(
	_ context.Context,
	tenant psdomain.TenantID,
	parcel psdomain.DeclaredParcelID,
) (psdomain.CommercialResolutionID, bool, error) {
	double.asked++
	double.askedTenant, double.askedParcel = tenant, parcel
	if double.err != nil {
		return psdomain.CommercialResolutionID{}, false, double.err
	}
	return double.reference, double.present, nil
}

var _ adapter.CommercialResolutionReferenceSource = (*referenceSourceDouble)(nil)

type closureViewDouble struct {
	closure         pcdomain.CommercialClosure
	found           bool
	err             error
	asked           int
	askedTenant     pcdomain.TenantID
	askedResolution pcdomain.ResolutionID
}

func (double *closureViewDouble) LoadResolution(
	_ context.Context,
	tenant pcdomain.TenantID,
	resolution pcdomain.ResolutionID,
) (pcdomain.CommercialClosure, bool, error) {
	double.asked++
	double.askedTenant, double.askedResolution = tenant, resolution
	if double.err != nil {
		return pcdomain.CommercialClosure{}, false, double.err
	}
	return double.closure, double.found, nil
}

var _ pcports.CommercialResolutionView = (*closureViewDouble)(nil)

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

type contentsDouble struct {
	rule        pcdomain.CustomerServiceRuleVersion
	found       bool
	err         error
	asked       int
	askedTenant pcdomain.TenantID
	askedRule   pcdomain.CommercialVersion
}

func (double *contentsDouble) LoadCustomerServiceRule(
	_ context.Context,
	tenant pcdomain.TenantID,
	rule pcdomain.CommercialVersion,
) (pcdomain.CustomerServiceRuleVersion, bool, error) {
	double.asked++
	double.askedTenant, double.askedRule = tenant, rule
	if double.err != nil {
		return pcdomain.CustomerServiceRuleVersion{}, false, double.err
	}
	return double.rule, double.found, nil
}

var _ pcports.CustomerServiceRuleContentView = (*contentsDouble)(nil)

type fixture struct {
	registry   *pcdomain.CommercialRegistry
	contract   pcdomain.CommercialVersion
	version    pcdomain.CommercialVersion
	anchor     pcdomain.SelectionAnchor
	closure    pcdomain.CommercialClosure
	own        *ownRulesDouble
	references *referenceSourceDouble
	closures   *closureViewDouble
	contents   *contentsDouble
}

// newFixture 造一份「VE 自己的册在场且类型在保、PC 里恰有一版客户合同与一版客户服务规则生效、目标包裹所属委托
// 接受时固定的闭包同时采用了这两版、PS 按目标包裹答得出那份闭包的回指」的世界；正文由各用例按需塞进 contents。
func newFixture(t *testing.T) *fixture {
	t.Helper()

	registry := pcdomain.NewCommercialRegistry()
	contract := effectiveIn(t, registry, "tenant-1", pcdomain.CustomerContractObject, "contract-1", "v1", "sha256:contract1", "scope-a")
	version := effectiveIn(t, registry, "tenant-1", pcdomain.CustomerServiceRuleObject, "csr-1", "v1", "sha256:csr1", "scope-a")
	anchor, err := pcdomain.NewSelectionAnchor(anchorAt, value(t, pcdomain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("new selection anchor: %v", err)
	}
	fixture := &fixture{
		registry: registry,
		contract: contract,
		version:  version,
		anchor:   anchor,
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
		references: &referenceSourceDouble{},
		closures:   &closureViewDouble{},
		contents:   &contentsDouble{},
	}
	fixture.hold(t, fixture.closureFor(t, "tenant-1", pcdomain.CustomerContractObject, pcdomain.CustomerServiceRuleObject))
	return fixture
}

// closureFor 让 PC 领域对着夹具登记册解一份唯一闭包——必需依据由用例指名，闭包采用了什么由解析决定。
func (fixture *fixture) closureFor(t *testing.T, tenant string, bases ...pcdomain.CommercialObjectKind) pcdomain.CommercialClosure {
	t.Helper()
	closure := pcdomain.ResolveCommercialClosure(fixture.registry, pcdomain.ClosureResolutionKey{
		TenantID:             value(t, pcdomain.NewTenantID, tenant),
		CustomerAccountID:    value(t, pcdomain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: value(t, pcdomain.NewLegalEntityReference, "legal-1"),
		Scope:                value(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		Purpose:              pcdomain.AcceptanceControlPurpose,
		Anchor:               fixture.anchor,
		RequiredBases:        bases,
	}, nil)
	if closure.Outcome() != pcdomain.UniquelyResolved {
		t.Fatalf("closure outcome = %q reason %q, want UNIQUELY_RESOLVED", closure.Outcome(), closure.Reason())
	}
	return closure
}

// hold 让两个提供方替身对同一份闭包说话：PS 按目标包裹答它的回指，PC 按回指交回它。
func (fixture *fixture) hold(t *testing.T, closure pcdomain.CommercialClosure) {
	t.Helper()
	fixture.closure = closure
	fixture.references.reference = value(t, psdomain.NewCommercialResolutionID, closure.ResolutionID().String())
	fixture.references.present = true
	fixture.closures.closure = closure
	fixture.closures.found = true
}

func (fixture *fixture) adapter(t *testing.T) *adapter.ClaimServiceRules {
	t.Helper()
	built, err := adapter.NewClaimServiceRules(fixture.deps())
	if err != nil {
		t.Fatalf("new claim service rules: %v", err)
	}
	return built
}

func (fixture *fixture) deps() adapter.ClaimServiceRulesDeps {
	return adapter.ClaimServiceRulesDeps{
		Rules:      fixture.own,
		References: fixture.references,
		Closures:   fixture.closures,
		Contents:   fixture.contents,
	}
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
		pcdomain.CustomerServiceRuleAppliesToCustomerContract(value(t, pcdomain.NewCommercialObjectID, "contract-1")),
		value(t, pcdomain.NewPartyID, "operator-1"),
		value(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		deadlines, materials,
	)
	if err != nil {
		t.Fatalf("new customer service rule version: %v", err)
	}
	fixture.contents.rule, fixture.contents.found = rule, true
}

func (fixture *fixture) firstDeadlineContent(t *testing.T) {
	t.Helper()
	fixture.content(t, []pcdomain.ClaimDeadlineRule{fixture.deadline(t, pcdomain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn")}, nil)
}

// Covers: 判据 2「已登记」格与 ADR-0136 决定二三段——目标包裹原样作声明包裹身份问 PS、回指原样作解析标识问 PC、
// 正文按闭包采用的那一版以查询租户点读；两维从正文翻译：Registered 为真、RuleVersion 冻三段版本引用、起算事件
// 与日历照引用转写、Scope 取索赔目标范围、Required 与 PC 条目逐项相等；Deadline / SupplementDeadline / Notice 是
// 票 03「裁决」留的格，必须仍是零值——填了就是造实例参数。VE 自己的册交出的其余三样一字不改。
func TestClaimServiceRulesOverlayBothDimensionsFromTheAcceptanceTimeRule(t *testing.T) {
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

	rules, declared, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
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
	if fixture.references.askedTenant.String() != "tenant-1" || fixture.references.askedParcel.String() != "parcel-1" {
		t.Fatalf("问 PS 用的租户/包裹 = %q/%q，要是查询租户与原样的目标范围引用",
			fixture.references.askedTenant, fixture.references.askedParcel)
	}
	if fixture.closures.askedTenant.String() != "tenant-1" || fixture.closures.askedResolution != fixture.closure.ResolutionID() {
		t.Fatalf("问 PC 用的租户/回指 = %q/%q，要是查询租户与 PS 答的回指原样",
			fixture.closures.askedTenant, fixture.closures.askedResolution)
	}
	if fixture.contents.askedTenant.String() != "tenant-1" ||
		fixture.contents.askedRule.ObjectID().String() != "csr-1" ||
		fixture.contents.askedRule.Version().String() != "v1" {
		t.Fatalf("点读用的租户/版本 = %q/%q/%q，要是查询租户与闭包采用的那一版",
			fixture.contents.askedTenant, fixture.contents.askedRule.ObjectID(), fixture.contents.askedRule.Version())
	}
}

// Covers: 端口合同「第二个返回值为 false 只有『合同的索赔资格声明不在场』一个意思」——VE 自己的册
// 说不在场时原样交回，不去问任何提供方：连「这个类型在不在保」都无从谈起，两维也没有可挂的地方。
func TestClaimServiceRulesPassThroughWhenTheOwnCatalogueIsAbsent(t *testing.T) {
	fixture := newFixture(t)
	fixture.own.declared = false
	fixture.firstDeadlineContent(t)

	rules, declared, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
	if err != nil || declared {
		t.Fatalf("declared=%v err=%v，要原样交回「声明不在场」", declared, err)
	}
	if rules.FilingDeadline.Registered || rules.Materials.Registered {
		t.Fatal("声明不在场却给两维挂了登记")
	}
	if fixture.references.asked != 0 || fixture.closures.asked != 0 || fixture.contents.asked != 0 {
		t.Fatal("声明不在场还去问了提供方")
	}
}

// Covers: 票 03「裁决」第一条——Registered 各维按「PC 那一项有没有行」答，版本壳在场不等于两维都登记。
func TestClaimServiceRulesAnswerEachDimensionByItsOwnRow(t *testing.T) {
	cases := map[string]struct {
		arrange      func(t *testing.T, fixture *fixture)
		wantDeadline bool
		wantMaterial bool
	}{
		"只登了首次索赔期限": {
			arrange:      func(t *testing.T, fixture *fixture) { fixture.firstDeadlineContent(t) },
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

			rules, declared, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
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

// Covers: 判据 2 的三个「未登记」格（ADR-0136 决定二第三段），恢复动作都是去登记、登的东西各不同，一律不折成
// 报错——报错的恢复动作是重试依赖，对这三格都无用：
//   - PS 答「没有」：目标不属任何已接受委托的成员集合（含目标是明确服务责任范围）——不再往 PC 问，与本票之前
//     键来源未配置那一行同一可观察行为；
//   - 闭包在场、合同也采用了，却未采用客户服务规则版本：租户没在 PS 解析键登记面把它列进必需依据——正文读口
//     不该被问到，问了就是在替一次没固定的接受补一版规则；
//   - 规则版本在场而正文未登（found=false）：去 PC 登正文。
func TestClaimServiceRulesAnswerUnregisteredWhenTheProviderHasNothing(t *testing.T) {
	cases := map[string]struct {
		arrange func(t *testing.T, fixture *fixture)
		assert  func(t *testing.T, fixture *fixture)
	}{
		"目标不属任何已接受委托": {
			arrange: func(t *testing.T, fixture *fixture) {
				fixture.references.present = false
				fixture.references.reference = psdomain.CommercialResolutionID{}
				fixture.firstDeadlineContent(t)
			},
			assert: func(t *testing.T, fixture *fixture) {
				if fixture.closures.asked != 0 {
					t.Fatal("PS 答没有回指，却还去 PC 问了闭包")
				}
			},
		},
		"接受时闭包未采用客户服务规则版本": {
			arrange: func(t *testing.T, fixture *fixture) {
				fixture.hold(t, fixture.closureFor(t, "tenant-1", pcdomain.CustomerContractObject))
				fixture.firstDeadlineContent(t)
			},
			assert: func(t *testing.T, fixture *fixture) {
				if fixture.contents.asked != 0 {
					t.Fatal("闭包没采用客户服务规则版本，却还去点读了正文")
				}
			},
		},
		"规则版本在场而正文没登": {
			arrange: func(_ *testing.T, fixture *fixture) { fixture.contents.found = false },
			assert:  func(_ *testing.T, _ *fixture) {},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t)
			tc.arrange(t, fixture)

			rules, declared, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
			if err != nil || !declared {
				t.Fatalf("declared=%v err=%v，缺席不是 error", declared, err)
			}
			if rules.FilingDeadline.Registered || rules.Materials.Registered {
				t.Fatalf("提供方没东西却答了登记：%#v", rules)
			}
			if !rules.KindCovered || !rules.Authorization.Registered {
				t.Fatalf("VE 自己的册交出的维被改动了：%#v", rules)
			}
			tc.assert(t, fixture)
		})
	}
}

// Covers: 判据 2 的两个 error 格与「租户不符仍报 ErrUntranslatableAnswer」，加端口合同「依赖调不通作为错误返回」
// 与 ADR-0025 全函数——下列每一格都不许折成「未登记」，折了会把租户支去登一份其实已经存在（或根本不是缺登记）
// 的东西：
//   - 闭包不在场（ErrCommercialClosureAbsent）：对一份已接受委托的回指是提供方缺数据；
//   - 闭包在场却未采用客户合同版本（ErrCustomerContractNotAdopted）：接受流的装配缺陷（ADR-0136 决定三）；
//   - 提供方阶段契约被打破（ErrUntranslatableAnswer）：PS 答在场却没有回指、PC 交回别的租户或别的回指的闭包、
//     闭包在客户服务规则那一格采用了客户合同版本冒名；
//   - VE 自己的册、PS 回指读口、PC 闭包读口与正文读口的 error 原样上抛。
func TestClaimServiceRulesReportErrorsInsteadOfFoldingThemIntoUnregistered(t *testing.T) {
	ownErr := errors.New("own catalogue unreachable")
	referencesErr := errors.New("shipment request store unreachable")
	closuresErr := errors.New("resolution store unreachable")
	contentsErr := errors.New("content store unreachable")

	cases := map[string]struct {
		arrange func(t *testing.T, fixture *fixture)
		want    error
	}{
		"VE 自己的册报错": {
			arrange: func(_ *testing.T, fixture *fixture) { fixture.own.err = ownErr },
			want:    ownErr,
		},
		"PS 回指读口报错": {
			arrange: func(_ *testing.T, fixture *fixture) { fixture.references.err = referencesErr },
			want:    referencesErr,
		},
		"PS 答在场却没有回指": {
			arrange: func(_ *testing.T, fixture *fixture) {
				fixture.references.reference = psdomain.CommercialResolutionID{}
			},
			want: adapter.ErrUntranslatableAnswer,
		},
		"闭包不在场": {
			arrange: func(_ *testing.T, fixture *fixture) {
				fixture.closures.found = false
				fixture.closures.closure = pcdomain.CommercialClosure{}
			},
			want: adapter.ErrCommercialClosureAbsent,
		},
		"PC 闭包读口报错": {
			arrange: func(_ *testing.T, fixture *fixture) { fixture.closures.err = closuresErr },
			want:    closuresErr,
		},
		"闭包未采用客户合同版本": {
			arrange: func(t *testing.T, fixture *fixture) {
				fixture.hold(t, fixture.closureFor(t, "tenant-1", pcdomain.CustomerServiceRuleObject))
			},
			want: adapter.ErrCustomerContractNotAdopted,
		},
		"PC 交回别的租户的闭包": {
			arrange: func(t *testing.T, fixture *fixture) {
				effectiveIn(t, fixture.registry, "tenant-2", pcdomain.CustomerContractObject, "contract-1", "v1", "sha256:contract1", "scope-a")
				effectiveIn(t, fixture.registry, "tenant-2", pcdomain.CustomerServiceRuleObject, "csr-1", "v1", "sha256:csr1", "scope-a")
				fixture.hold(t, fixture.closureFor(t, "tenant-2", pcdomain.CustomerContractObject, pcdomain.CustomerServiceRuleObject))
			},
			want: adapter.ErrUntranslatableAnswer,
		},
		"PC 交回别的回指的闭包": {
			arrange: func(t *testing.T, fixture *fixture) {
				fixture.references.reference = value(t, psdomain.NewCommercialResolutionID, "CLO-other")
			},
			want: adapter.ErrUntranslatableAnswer,
		},
		"闭包在客户服务规则那一格采用了合同版本冒名": {
			arrange: func(t *testing.T, fixture *fixture) {
				impostor, err := pcdomain.RehydrateCommercialClosure(pcdomain.RehydrateCommercialClosureSpec{
					Outcome:      pcdomain.UniquelyResolved,
					ResolutionID: value(t, pcdomain.NewResolutionID, "CLO-impostor"),
					Key:          fixture.closure.ResolutionKey(),
					Anchor:       fixture.anchor,
					ViewRevision: value(t, pcdomain.NewAuthorityViewRevision, "view-1"),
					Adopted: []pcdomain.RehydrateAdoptedBasisSpec{
						{Kind: pcdomain.CustomerContractObject, Version: fixture.contract},
						{Kind: pcdomain.CustomerServiceRuleObject, Version: fixture.contract},
					},
				})
				if err != nil {
					t.Fatalf("rehydrate impostor closure: %v", err)
				}
				fixture.hold(t, impostor)
			},
			want: adapter.ErrUntranslatableAnswer,
		},
		"正文读口报错": {
			arrange: func(_ *testing.T, fixture *fixture) { fixture.contents.err = contentsErr },
			want:    contentsErr,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t)
			fixture.firstDeadlineContent(t)
			tc.arrange(t, fixture)

			rules, declared, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if declared || rules.FilingDeadline.Registered || rules.Materials.Registered {
				t.Fatalf("报错的同时还交了答案：declared=%v rules=%#v", declared, rules)
			}
		})
	}
}

// Covers: 装配缺陷在构造期就拒（ADR-0079 决定八）——协作方缺一即 ErrNilDependency，没有可选的一半：本适配器
// 一旦装上就是要真去问两个提供方的，缺一半而静默答未登记会让装配疏漏与租户没登记长得一样。装配方按 errors.Is
// 认哨兵、按文本认缺的是哪一口；错误值为哨兵时不交出一只会在运行期 panic 的适配器。
func TestClaimServiceRulesRefuseToBeBuiltWithoutTheirCollaborators(t *testing.T) {
	fixture := newFixture(t)
	cases := map[string]struct {
		drop  func(deps *adapter.ClaimServiceRulesDeps)
		named string
	}{
		"缺 VE 自己的册":  {func(deps *adapter.ClaimServiceRulesDeps) { deps.Rules = nil }, "eligibility rule view"},
		"缺 PS 回指读口":  {func(deps *adapter.ClaimServiceRulesDeps) { deps.References = nil }, "commercial resolution reference source"},
		"缺 PC 闭包读口":  {func(deps *adapter.ClaimServiceRulesDeps) { deps.Closures = nil }, "commercial resolution view"},
		"缺 PC 正文点读口": {func(deps *adapter.ClaimServiceRulesDeps) { deps.Contents = nil }, "customer service rule content view"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			deps := fixture.deps()
			tc.drop(&deps)
			rules, err := adapter.NewClaimServiceRules(deps)
			if !errors.Is(err, adapter.ErrNilDependency) {
				t.Fatalf("err = %v, want ErrNilDependency——装配方按 errors.Is 认不出这是装配漏了", err)
			}
			if !strings.Contains(err.Error(), tc.named) {
				t.Fatalf("错误文本 %q 没点名缺的是 %q", err.Error(), tc.named)
			}
			if rules != nil {
				t.Fatal("缺协作方却交出了一只会在运行期 panic 的适配器")
			}
		})
	}
	if _, err := adapter.NewClaimServiceRules(fixture.deps()); err != nil {
		t.Fatalf("协作方齐全却拒绝装配：%v", err)
	}
}
