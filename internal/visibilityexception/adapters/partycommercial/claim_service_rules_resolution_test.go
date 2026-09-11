package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/partycommercial"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件是接手方（通道 3）在读封存实现之前先写下的第一片 red（parallel-sessions.md「镜像测试」那节的接收
// 条件）：只对着票 ve-claims/04「要做什么」2 与 ADR-0136 决定二第三段的结果代数逐格钉判据，不照任何一份
// 代码的形状长。三个协作方全用替身——PS 回指读口、PC 闭包读口、PC 正文读口——闭包本身经领域重建门造出，
// 因为这里证的是翻译：回指 → 闭包 → 已采用的客户服务规则版本 → 正文，每一格落在端口的哪个取值上。

type resolutionReferenceDouble struct {
	reference psdomain.CommercialResolutionID
	present   bool
	err       error
	asked     []string
}

func (double *resolutionReferenceDouble) LoadCommercialResolutionReference(
	_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.CommercialResolutionID, bool, error) {
	double.asked = append(double.asked, tenant.String()+"/"+parcel.String())
	if double.err != nil {
		return psdomain.CommercialResolutionID{}, false, double.err
	}
	return double.reference, double.present, nil
}

var _ adapter.CommercialResolutionReferenceSource = (*resolutionReferenceDouble)(nil)

type heldClosureDouble struct {
	closure pcdomain.CommercialClosure
	found   bool
	err     error
	asked   []string
}

func (double *heldClosureDouble) LoadResolution(
	_ context.Context, tenant pcdomain.TenantID, resolution pcdomain.ResolutionID,
) (pcdomain.CommercialClosure, bool, error) {
	double.asked = append(double.asked, tenant.String()+"/"+resolution.String())
	if double.err != nil {
		return pcdomain.CommercialClosure{}, false, double.err
	}
	return double.closure, double.found, nil
}

type resolutionFixture struct {
	own         *ownRulesDouble
	resolutions *resolutionReferenceDouble
	closures    *heldClosureDouble
	contents    *contentsDouble
	contract    pcdomain.CommercialVersion
	rule        pcdomain.CommercialVersion
}

// redEffectiveVersion 走草稿 → 发布 → 生效造一版商业版本，不进任何登记册：闭包重建门只看版本本身。
func redEffectiveVersion(t *testing.T, tenant string, kind pcdomain.CommercialObjectKind, objectID, version string) pcdomain.CommercialVersion {
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
		Scope:         value(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		ContentDigest: value(t, pcdomain.NewCommercialContentDigest, "sha256:"+objectID),
		Effective:     interval,
	})
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	basis, err := pcdomain.NewApprovalBasis(
		value(t, pcdomain.NewApprovalReference, "approval-"+objectID),
		value(t, pcdomain.NewCommercialSourceReference, "source-"+objectID),
		approvedAt)
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

// redClosure 经重建门造一份「唯一已解析」闭包，采用依据由调用方给——这正是 PC 闭包快照读回时的形状。
func redClosure(t *testing.T, tenant string, adopted ...pcdomain.CommercialVersion) pcdomain.CommercialClosure {
	t.Helper()
	anchor, err := pcdomain.NewSelectionAnchor(anchorAt, value(t, pcdomain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("new selection anchor: %v", err)
	}
	specs := make([]pcdomain.RehydrateAdoptedBasisSpec, 0, len(adopted))
	bases := make([]pcdomain.CommercialObjectKind, 0, len(adopted))
	for _, version := range adopted {
		specs = append(specs, pcdomain.RehydrateAdoptedBasisSpec{Kind: version.Kind(), Version: version})
		bases = append(bases, version.Kind())
	}
	closure, err := pcdomain.RehydrateCommercialClosure(pcdomain.RehydrateCommercialClosureSpec{
		Outcome:      pcdomain.UniquelyResolved,
		ResolutionID: value(t, pcdomain.NewResolutionID, "SYN-RES-1"),
		Key: pcdomain.ClosureResolutionKey{
			TenantID:             value(t, pcdomain.NewTenantID, tenant),
			CustomerAccountID:    value(t, pcdomain.NewCustomerAccountID, "customer-1"),
			LegalEntityCandidate: value(t, pcdomain.NewLegalEntityReference, "legal-1"),
			Scope:                value(t, pcdomain.NewCommercialScopeReference, "scope-a"),
			Purpose:              pcdomain.AcceptanceControlPurpose,
			Anchor:               anchor,
			RequiredBases:        bases,
		},
		Anchor:       anchor,
		ViewRevision: value(t, pcdomain.NewAuthorityViewRevision, "view-1"),
		Adopted:      specs,
	})
	if err != nil {
		t.Fatalf("rehydrate closure: %v", err)
	}
	return closure
}

// newResolutionFixture 造「VE 自己的册在场且类型在保、PS 对目标包裹答回指、PC 闭包同时采用了客户合同与
// 客户服务规则版本、正文两维都登了」的世界；各用例从这里往回拆一格。
func newResolutionFixture(t *testing.T) *resolutionFixture {
	t.Helper()
	contract := redEffectiveVersion(t, "tenant-1", pcdomain.CustomerContractObject, "contract-1", "v1")
	rule := redEffectiveVersion(t, "tenant-1", pcdomain.CustomerServiceRuleObject, "csr-1", "v1")
	fixture := &resolutionFixture{
		own: &ownRulesDouble{declared: true, rules: veports.EligibilityRules{
			RuleVersion: "SYN-CLAIM-RULES-1", KindCovered: true,
			Authorization: veports.AuthorizationCatalogue{Registered: true, RuleVersion: "SYN-AUTH-1",
				AuthorizedApplicants: []vedomain.ApplicantReference{value(t, vedomain.NewApplicantReference, "applicant-1")}},
		}},
		resolutions: &resolutionReferenceDouble{reference: value(t, psdomain.NewCommercialResolutionID, "SYN-RES-1"), present: true},
		closures:    &heldClosureDouble{closure: redClosure(t, "tenant-1", contract, rule), found: true},
		contents:    &contentsDouble{},
		contract:    contract,
		rule:        rule,
	}
	deadline, err := pcdomain.NewClaimDeadlineRule(pcdomain.FirstClaimDeadline,
		value(t, pcdomain.NewDeadlineStartEventReference, "event-delivered"), 30,
		value(t, pcdomain.NewBusinessCalendarReference, "calendar-cn"))
	if err != nil {
		t.Fatalf("new claim deadline rule: %v", err)
	}
	materials, err := pcdomain.NewMinimumMaterialsRule(value(t, pcdomain.NewClaimKindReference, "claim-loss"),
		[]pcdomain.MaterialRequirementReference{value(t, pcdomain.NewMaterialRequirementReference, "material-photo")})
	if err != nil {
		t.Fatalf("new minimum materials rule: %v", err)
	}
	body, err := pcdomain.NewCustomerServiceRuleVersion(rule,
		pcdomain.CustomerServiceRuleAppliesToServiceProduct(value(t, pcdomain.NewCommercialObjectID, "product-1")),
		value(t, pcdomain.NewPartyID, "operator-1"), value(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		[]pcdomain.ClaimDeadlineRule{deadline}, []pcdomain.MinimumMaterialsRule{materials})
	if err != nil {
		t.Fatalf("new customer service rule version: %v", err)
	}
	fixture.contents.rule, fixture.contents.found = body, true
	return fixture
}

func (fixture *resolutionFixture) adapter(t *testing.T) *adapter.ClaimServiceRules {
	t.Helper()
	built, err := adapter.NewClaimServiceRules(adapter.ClaimServiceRulesDeps{
		Rules:      fixture.own,
		References: fixture.resolutions,
		Closures:   fixture.closures,
		Contents:   fixture.contents,
	})
	if err != nil {
		t.Fatalf("new claim service rules: %v", err)
	}
	return built
}

func (fixture *resolutionFixture) query(t *testing.T) veports.EligibilityQuery {
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

// Covers: ADR-0136 决定二「已登记」一格——目标包裹原样作 PS 声明包裹身份问回指（不猜它是不是包裹）；按
// （查询租户，回指）取闭包；按闭包采用的客户服务规则版本、以查询租户点读正文；两维 Registered 为真且
// RuleVersion 冻三段引用。VE 自己的册交出的三维一字不改。
func TestClaimServiceRulesReadTheRuleAdoptedAtAcceptanceThroughTheBackReference(t *testing.T) {
	fixture := newResolutionFixture(t)

	rules, declared, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
	if err != nil || !declared {
		t.Fatalf("declared=%v err=%v", declared, err)
	}
	if !rules.FilingDeadline.Registered || rules.FilingDeadline.RuleVersion != "tenant-1/csr-1/v1" {
		t.Fatalf("首次索赔期限维 = %#v，要按闭包采用的 csr-1/v1 答已登记", rules.FilingDeadline)
	}
	if !rules.Materials.Registered || rules.Materials.RuleVersion != "tenant-1/csr-1/v1" {
		t.Fatalf("最低材料维 = %#v，要按闭包采用的 csr-1/v1 答已登记", rules.Materials)
	}
	if len(fixture.resolutions.asked) != 1 || fixture.resolutions.asked[0] != "tenant-1/parcel-1" {
		t.Fatalf("问 PS = %v，要按（查询租户，目标范围引用原样）问恰一次", fixture.resolutions.asked)
	}
	if len(fixture.closures.asked) != 1 || fixture.closures.asked[0] != "tenant-1/SYN-RES-1" {
		t.Fatalf("问 PC 闭包 = %v，要按（查询租户，PS 交回的回指）问恰一次", fixture.closures.asked)
	}
	if fixture.contents.askedTenant.String() != "tenant-1" ||
		fixture.contents.askedRule.ObjectID().String() != "csr-1" || fixture.contents.askedRule.Version().String() != "v1" {
		t.Fatalf("点读 = %s/%s/%s，要是查询租户与闭包采用的那一版",
			fixture.contents.askedTenant, fixture.contents.askedRule.ObjectID(), fixture.contents.askedRule.Version())
	}
	if rules.RuleVersion != "SYN-CLAIM-RULES-1" || !rules.KindCovered || !rules.Authorization.Registered {
		t.Fatalf("VE 自己的册交出的三维被改动了：%#v", rules)
	}
}

// Covers: ADR-0136 决定二第三段「PS 答没有 → 两维未登记」——目标不属任何已接受委托（含目标是明确服务范围）
// 没有回指可问，不进 PC；行为与此前 Keys == nil 那一格相同：未登记不是 error，VE 自己的三维照旧。
func TestClaimServiceRulesAnswerUnregisteredWhenParcelShipmentHasNoBackReference(t *testing.T) {
	fixture := newResolutionFixture(t)
	fixture.resolutions.present = false

	rules, declared, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
	if err != nil || !declared {
		t.Fatalf("declared=%v err=%v", declared, err)
	}
	if rules.FilingDeadline.Registered || rules.Materials.Registered {
		t.Fatalf("PS 没有回指却答了登记：%#v", rules)
	}
	if len(fixture.closures.asked) != 0 {
		t.Fatal("没有回指还去问了 PC 闭包")
	}
	if rules.RuleVersion != "SYN-CLAIM-RULES-1" || !rules.KindCovered || !rules.Authorization.Registered {
		t.Fatalf("VE 自己的册交出的三维被改动了：%#v", rules)
	}
}

// Covers: ADR-0136 决定二第三段「闭包不在场 → error」——对一份已接受委托的回指 LoadResolution 答 found=false
// 是提供方缺数据，不是「没登规则」，不得折成未登记。
func TestClaimServiceRulesRefuseWhenTheClosureBehindTheBackReferenceIsAbsent(t *testing.T) {
	fixture := newResolutionFixture(t)
	fixture.closures.found = false

	_, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
	if !errors.Is(err, adapter.ErrCommercialClosureAbsent) {
		t.Fatalf("err = %v, want ErrCommercialClosureAbsent", err)
	}
}

// Covers: ADR-0136 决定三「闭包在场却未采用客户合同版本 → error」——PC CONTEXT 说合同恒在，缺它是接受流的
// 装配缺陷；与 ADR-0133 决定二同格。
func TestClaimServiceRulesRefuseWhenTheClosureAdoptedNoCustomerContract(t *testing.T) {
	fixture := newResolutionFixture(t)
	fixture.closures.closure = redClosure(t, "tenant-1", fixture.rule)

	_, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
	if !errors.Is(err, adapter.ErrCustomerContractNotAdopted) {
		t.Fatalf("err = %v, want ErrCustomerContractNotAdopted", err)
	}
}

// Covers: ADR-0136 决定二第三段「闭包在场却未采用客户服务规则版本 → 两维未登记」——租户没把
// CustomerServiceRuleObject 列进 PS 解析键的必需依据，恢复动作是去登记不是修代码；不进正文点读。
func TestClaimServiceRulesAnswerUnregisteredWhenTheClosureAdoptedNoCustomerServiceRule(t *testing.T) {
	fixture := newResolutionFixture(t)
	fixture.closures.closure = redClosure(t, "tenant-1", fixture.contract)

	rules, declared, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
	if err != nil || !declared {
		t.Fatalf("declared=%v err=%v", declared, err)
	}
	if rules.FilingDeadline.Registered || rules.Materials.Registered {
		t.Fatalf("闭包没采用客户服务规则却答了登记：%#v", rules)
	}
	if fixture.contents.askedTenant.String() != "" {
		t.Fatal("没有采用的规则版本还去点读了正文")
	}
}

// Covers: 票 03 原格「规则版本在场而正文未登 → 两维未登记」在新三段下不变。
func TestClaimServiceRulesAnswerUnregisteredWhenTheRuleBodyIsNotRegistered(t *testing.T) {
	fixture := newResolutionFixture(t)
	fixture.contents.found = false

	rules, declared, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
	if err != nil || !declared {
		t.Fatalf("declared=%v err=%v", declared, err)
	}
	if rules.FilingDeadline.Registered || rules.Materials.Registered {
		t.Fatalf("正文未登却答了登记：%#v", rules)
	}
}

// Covers: 判据 2 末句「租户不符仍报 ErrUntranslatableAnswer」——按查询租户取回的闭包声称属于另一户：
// 租户是身份不是过滤器（ADR-0003），不纠正、不沉默。
func TestClaimServiceRulesRefuseAClosureOfAnotherTenant(t *testing.T) {
	fixture := newResolutionFixture(t)
	contract := redEffectiveVersion(t, "tenant-2", pcdomain.CustomerContractObject, "contract-1", "v1")
	rule := redEffectiveVersion(t, "tenant-2", pcdomain.CustomerServiceRuleObject, "csr-1", "v1")
	fixture.closures.closure = redClosure(t, "tenant-2", contract, rule)

	_, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
	if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
	}
}

// Covers: 两个提供方的 error 原样上抛，不折成未登记（ADR-0029：依赖调不通的恢复动作是修依赖）。
func TestClaimServiceRulesPropagateProviderErrors(t *testing.T) {
	psDown := errors.New("parcel shipment read face unavailable")
	pcDown := errors.New("party commercial closure store unavailable")
	for name, arrange := range map[string]func(*resolutionFixture) error{
		"PS 读口不可用":   func(fixture *resolutionFixture) error { fixture.resolutions.err = psDown; return psDown },
		"PC 闭包读口不可用": func(fixture *resolutionFixture) error { fixture.closures.err = pcDown; return pcDown },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newResolutionFixture(t)
			want := arrange(fixture)
			_, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
			if !errors.Is(err, want) {
				t.Fatalf("err = %v, want %v 原样可辨", err, want)
			}
		})
	}
}

// Covers: 票面「要做什么」3 / ADR-0079 决定八——两个提供方半边都不许 nil：缺一半静默答未登记会让装配疏漏
// 与租户没登记长得一样。
func TestNewClaimServiceRulesRefuseNilProviderHalves(t *testing.T) {
	fixture := newResolutionFixture(t)
	for name, deps := range map[string]adapter.ClaimServiceRulesDeps{
		"缺 PS 回指读口": {Rules: fixture.own, Closures: fixture.closures, Contents: fixture.contents},
		"缺 PC 闭包读口": {Rules: fixture.own, References: fixture.resolutions, Contents: fixture.contents},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := adapter.NewClaimServiceRules(deps); err == nil {
				t.Fatal("nil 半边被收下了")
			}
		})
	}
}
