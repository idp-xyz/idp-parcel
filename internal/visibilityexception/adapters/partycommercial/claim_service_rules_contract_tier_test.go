package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/partycommercial"
)

// 本文件钉票 customer-service-rule-contract-applicability/04 的三族（spec 缝二）：闭包采纳挂合同版时经 PC 层次读口
// 取两份正文、按行拼；正文挂的合同 ≠ 闭包采用的客户合同答 ErrUntranslatableAnswer；挂产品版照旧单版点读。层次读口
// 用替身——它自己的选法与真库契约归 PC 缝三，这里证的是 VE 怎么用它交回的两层。

type layersDouble struct {
	layers pcports.CustomerServiceRuleLayers
	err    error

	asked        int
	askedTenant  pcdomain.TenantID
	askedScope   pcdomain.CommercialScopeReference
	askedRule    pcdomain.CommercialVersion
	askedAnchor  pcdomain.SelectionAnchor
	askedProduct pcdomain.CommercialObjectID
}

func (double *layersDouble) LoadCustomerServiceRuleLayers(
	_ context.Context,
	tenant pcdomain.TenantID,
	scope pcdomain.CommercialScopeReference,
	contractRule pcdomain.CommercialVersion,
	anchor pcdomain.SelectionAnchor,
	declaredProduct pcdomain.CommercialObjectID,
) (pcports.CustomerServiceRuleLayers, error) {
	double.asked++
	double.askedTenant, double.askedScope, double.askedRule = tenant, scope, contractRule
	double.askedAnchor, double.askedProduct = anchor, declaredProduct
	if double.err != nil {
		return pcports.CustomerServiceRuleLayers{}, double.err
	}
	return double.layers, nil
}

var _ pcports.CustomerServiceRuleLayerView = (*layersDouble)(nil)

// contractTierVersion 造一版壳上指名客户合同的客户服务规则——02 的解析按壳把它归合同层。
func contractTierVersion(t *testing.T, tenant, objectID, contract string) pcdomain.CommercialVersion {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	draft, err := pcdomain.NewCommercialDraft(pcdomain.CommercialVersionSpec{
		TenantID:      value(t, pcdomain.NewTenantID, tenant),
		Kind:          pcdomain.CustomerServiceRuleObject,
		ObjectID:      value(t, pcdomain.NewCommercialObjectID, objectID),
		Version:       value(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         value(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		ContentDigest: value(t, pcdomain.NewCommercialContentDigest, "sha256:"+objectID),
		Effective:     interval,
		References: map[pcdomain.CommercialObjectKind]pcdomain.CommercialObjectID{
			pcdomain.CustomerContractObject: value(t, pcdomain.NewCommercialObjectID, contract),
		},
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
	contractPublished := func(pcdomain.CommercialObjectKind, pcdomain.CommercialObjectID) pcdomain.NamedReferenceStanding {
		return pcdomain.NamedReferencePublished
	}
	published, err := draft.Publish(basis, pcdomain.ApprovalRoleConfirmed, approvedAt, contractPublished)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	live, err := published.TakeEffect(approvedAt)
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}

type ruleRows struct {
	deadlineStartEvent string // 空 = 不登首次索赔期限这一行
	materialsFor       string // 空 = 不登最低材料这一行
	material           string
}

func ruleBody(
	t *testing.T,
	version pcdomain.CommercialVersion,
	applicability pcdomain.CustomerServiceRuleApplicability,
	rows ruleRows,
) pcdomain.CustomerServiceRuleVersion {
	t.Helper()
	var deadlines []pcdomain.ClaimDeadlineRule
	if rows.deadlineStartEvent != "" {
		deadline, err := pcdomain.NewClaimDeadlineRule(pcdomain.FirstClaimDeadline,
			value(t, pcdomain.NewDeadlineStartEventReference, rows.deadlineStartEvent), 30,
			value(t, pcdomain.NewBusinessCalendarReference, "calendar-cn"))
		if err != nil {
			t.Fatalf("new claim deadline rule: %v", err)
		}
		deadlines = append(deadlines, deadline)
	}
	var materials []pcdomain.MinimumMaterialsRule
	if rows.materialsFor != "" {
		row, err := pcdomain.NewMinimumMaterialsRule(value(t, pcdomain.NewClaimKindReference, rows.materialsFor),
			[]pcdomain.MaterialRequirementReference{value(t, pcdomain.NewMaterialRequirementReference, rows.material)})
		if err != nil {
			t.Fatalf("new minimum materials rule: %v", err)
		}
		materials = append(materials, row)
	}
	body, err := pcdomain.NewCustomerServiceRuleVersion(version, applicability,
		value(t, pcdomain.NewPartyID, "operator-1"), value(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		deadlines, materials)
	if err != nil {
		t.Fatalf("new customer service rule version: %v", err)
	}
	return body
}

func appliesToContract(t *testing.T, contract string) pcdomain.CustomerServiceRuleApplicability {
	t.Helper()
	return pcdomain.CustomerServiceRuleAppliesToCustomerContract(value(t, pcdomain.NewCommercialObjectID, contract))
}

func appliesToProduct(t *testing.T, product string) pcdomain.CustomerServiceRuleApplicability {
	t.Helper()
	return pcdomain.CustomerServiceRuleAppliesToServiceProduct(value(t, pcdomain.NewCommercialObjectID, product))
}

// contractTierFixture 在 newResolutionFixture 的世界上换一格：闭包采纳的是挂 contract-1 的合同版 csr-c1，层次读口交回
// 合同版（只登首次索赔期限、起算事件定制）与底座 csr-1（两行都登）。各用例从这里再拆一格。
func contractTierFixture(t *testing.T) (*resolutionFixture, pcdomain.CommercialVersion) {
	t.Helper()
	fixture := newResolutionFixture(t)
	contractRule := contractTierVersion(t, "tenant-1", "csr-c1", "contract-1")
	fixture.closures.closure = redClosure(t, "tenant-1", fixture.contract, contractRule)
	fixture.layers.layers = pcports.CustomerServiceRuleLayers{
		ContractTier: ruleBody(t, contractRule, appliesToContract(t, "contract-1"),
			ruleRows{deadlineStartEvent: "event-custom"}),
		HasContractTier:    true,
		ProductBaseOutcome: pcdomain.UniquelyResolved,
		ProductBase: ruleBody(t, fixture.rule, appliesToProduct(t, "product-1"),
			ruleRows{deadlineStartEvent: "event-delivered", materialsFor: "claim-loss", material: "material-photo"}),
		HasProductBase: true,
	}
	return fixture, contractRule
}

// Covers: 票 04 做什么 1、2 / ADR-0176 决定二——采纳合同版时经层次读口按（租户、合同版自己的范围、闭包采纳的那一版、
// 闭包锚点、闭包键声明的产品）问恰一次，不走单版点读；合同版有行的那一维用合同版并冻它的版本引用，合同版无行的那一维
// 取底座行并冻底座的版本引用。
func TestAContractTierRuleTakesItsOwnRowsAndInheritsTheRestFromTheProductBase(t *testing.T) {
	fixture, contractRule := contractTierFixture(t)

	rules, declared, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
	if err != nil || !declared {
		t.Fatalf("declared=%v err=%v", declared, err)
	}
	if !rules.FilingDeadline.Registered || rules.FilingDeadline.RuleVersion != "tenant-1/csr-c1/v1" ||
		rules.FilingDeadline.StartEvent != "event-custom" {
		t.Fatalf("首次索赔期限维 = %#v，合同版有这一行，要答合同版的定制起算事件并冻 csr-c1", rules.FilingDeadline)
	}
	if !rules.Materials.Registered || rules.Materials.RuleVersion != "tenant-1/csr-1/v1" ||
		len(rules.Materials.Required) != 1 || rules.Materials.Required[0].String() != "material-photo" {
		t.Fatalf("最低材料维 = %#v，合同版无这一行，要继承底座 csr-1 那一行", rules.Materials)
	}

	if fixture.layers.asked != 1 {
		t.Fatalf("层次读口被问 %d 次，要恰一次", fixture.layers.asked)
	}
	closure := fixture.closures.closure
	if fixture.layers.askedTenant.String() != "tenant-1" ||
		fixture.layers.askedScope != contractRule.Scope() ||
		fixture.layers.askedRule.ObjectID().String() != "csr-c1" ||
		fixture.layers.askedAnchor != closure.Anchor() ||
		fixture.layers.askedProduct != closure.ResolutionKey().ServiceProduct {
		t.Fatalf("层次读口问法 = %s %s %s %v %q，要是查询租户、合同版自己的范围、闭包采纳的那一版、闭包锚点与闭包键上的产品",
			fixture.layers.askedTenant, fixture.layers.askedScope, fixture.layers.askedRule.ObjectID(),
			fixture.layers.askedAnchor, fixture.layers.askedProduct)
	}
	if fixture.contents.asked != 0 {
		t.Fatal("合同版另走了一次单版点读：两层要从层次读口一次取回")
	}
}

// Covers: 票 04 做什么 2「合同版有行用合同版」——两层都有某一行时取合同版那一行，不取底座。
func TestAContractTierRowWinsOverTheProductBaseRow(t *testing.T) {
	fixture, contractRule := contractTierFixture(t)
	fixture.layers.layers.ContractTier = ruleBody(t, contractRule, appliesToContract(t, "contract-1"),
		ruleRows{deadlineStartEvent: "event-custom", materialsFor: "claim-loss", material: "material-invoice"})

	rules, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if rules.Materials.RuleVersion != "tenant-1/csr-c1/v1" ||
		len(rules.Materials.Required) != 1 || rules.Materials.Required[0].String() != "material-invoice" {
		t.Fatalf("最低材料维 = %#v，合同版有这一行就用合同版", rules.Materials)
	}
}

// Covers: 票 04 做什么 2「两层皆无答既有『未登记』」——合同版与底座都没有这一类索赔的材料行时，那一维如实未登记；
// 底座选法答`无适用依据`时合同版没写的行同样未登记，合同版写了的照答。
func TestARowAbsentFromBothTiersIsUnregistered(t *testing.T) {
	t.Run("两层都无这一行", func(t *testing.T) {
		fixture, _ := contractTierFixture(t)
		fixture.layers.layers.ProductBase = ruleBody(t, fixture.rule, appliesToProduct(t, "product-1"),
			ruleRows{deadlineStartEvent: "event-delivered", materialsFor: "claim-damage", material: "material-photo"})

		rules, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if rules.Materials.Registered {
			t.Fatalf("最低材料维 = %#v，两层都没有 claim-loss 的材料行，要答未登记", rules.Materials)
		}
		if !rules.FilingDeadline.Registered || rules.FilingDeadline.RuleVersion != "tenant-1/csr-c1/v1" {
			t.Fatalf("首次索赔期限维 = %#v，合同版写了的那一维照答", rules.FilingDeadline)
		}
	})
	t.Run("没有底座", func(t *testing.T) {
		fixture, _ := contractTierFixture(t)
		fixture.layers.layers.ProductBaseOutcome = pcdomain.NoApplicableBasis
		fixture.layers.layers.ProductBase, fixture.layers.layers.HasProductBase = pcdomain.CustomerServiceRuleVersion{}, false

		rules, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if rules.Materials.Registered {
			t.Fatalf("最低材料维 = %#v，没有底座可继承，要答未登记", rules.Materials)
		}
		if !rules.FilingDeadline.Registered || rules.FilingDeadline.RuleVersion != "tenant-1/csr-c1/v1" {
			t.Fatalf("首次索赔期限维 = %#v，合同版写了的那一维照答", rules.FilingDeadline)
		}
	})
	t.Run("底座选出了但没登正文", func(t *testing.T) {
		fixture, _ := contractTierFixture(t)
		fixture.layers.layers.ProductBase, fixture.layers.layers.HasProductBase = pcdomain.CustomerServiceRuleVersion{}, false

		rules, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if rules.Materials.Registered {
			t.Fatalf("最低材料维 = %#v，底座没登正文，没有可继承的行", rules.Materials)
		}
	})
}

// Covers: 判断项「合同版没登正文不整版继承底座」——合同版壳在场而正文未登时两维都未登记，与单版点读「正文未登」同格；
// 拿底座顶上来会把合同里可能收紧的条款悄悄换成产品标准。
func TestAContractTierWithoutABodyDoesNotFallBackToTheProductBase(t *testing.T) {
	fixture, _ := contractTierFixture(t)
	fixture.layers.layers.ContractTier, fixture.layers.layers.HasContractTier = pcdomain.CustomerServiceRuleVersion{}, false

	rules, declared, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
	if err != nil || !declared {
		t.Fatalf("declared=%v err=%v", declared, err)
	}
	if rules.FilingDeadline.Registered || rules.Materials.Registered {
		t.Fatalf("合同版正文未登却答了登记：%#v", rules)
	}
}

// Covers: 票 04 做什么 1 / ADR-0176 决定四——正文挂的合同 ≠ 闭包采用的客户合同答 ErrUntranslatableAnswer。合同版、
// 底座与挂产品层那一版各有一格：哪一层的正文挂了别的合同，拼进来就是把别户的条款套给这一户（spec 用户故事 9）。
func TestABodyTiedToAnotherContractIsUntranslatable(t *testing.T) {
	t.Run("合同版正文挂别的合同", func(t *testing.T) {
		fixture, contractRule := contractTierFixture(t)
		fixture.layers.layers.ContractTier = ruleBody(t, contractRule, appliesToContract(t, "contract-2"),
			ruleRows{deadlineStartEvent: "event-custom"})

		_, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
		if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
			t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
		}
	})
	t.Run("底座正文挂别的合同", func(t *testing.T) {
		fixture, _ := contractTierFixture(t)
		fixture.layers.layers.ProductBase = ruleBody(t, fixture.rule, appliesToContract(t, "contract-2"),
			ruleRows{deadlineStartEvent: "event-delivered", materialsFor: "claim-loss", material: "material-photo"})

		_, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
		if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
			t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
		}
	})
	t.Run("产品层那一版正文挂别的合同", func(t *testing.T) {
		fixture := newResolutionFixture(t)
		fixture.contents.rule = ruleBody(t, fixture.rule, appliesToContract(t, "contract-2"),
			ruleRows{deadlineStartEvent: "event-delivered"})

		_, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
		if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
			t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
		}
	})
}

// Covers: 票 04 做什么 3——闭包采纳产品层那一版时照旧单版点读，不问层次读口；正文挂产品不核合同（ADR-0176 决定四），
// 正文挂的合同正是闭包采用的那一份时照答（壳空而正文挂合同的那一形态，02 判断项 1）。
func TestAProductTierRuleIsStillReadAloneAndOnlyAContractBodyIsChecked(t *testing.T) {
	t.Run("正文挂产品", func(t *testing.T) {
		fixture := newResolutionFixture(t)

		rules, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !rules.FilingDeadline.Registered || !rules.Materials.Registered {
			t.Fatalf("产品层那一版行为变了：%#v", rules)
		}
		if fixture.layers.asked != 0 {
			t.Fatal("产品层那一版去问了层次读口")
		}
	})
	t.Run("壳空而正文挂闭包采用的合同", func(t *testing.T) {
		fixture := newResolutionFixture(t)
		fixture.contents.rule = ruleBody(t, fixture.rule, appliesToContract(t, "contract-1"),
			ruleRows{deadlineStartEvent: "event-delivered"})

		rules, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !rules.FilingDeadline.Registered || rules.FilingDeadline.RuleVersion != "tenant-1/csr-1/v1" {
			t.Fatalf("首次索赔期限维 = %#v，正文挂的正是闭包采用的合同，照答", rules.FilingDeadline)
		}
		if fixture.layers.asked != 0 {
			t.Fatal("壳上没指名合同的那一版去问了层次读口")
		}
	})
}

// Covers: 判断项 Q5——底座多候选落与「闭包对客户服务规则答`适用冲突`」同一既有格（ErrUntranslatableAnswer，恢复动作
// 都是运营更正登记）；底座`解析未决`与层次读口的 error 不折成未登记。
func TestAnUnusableProductBaseIsNotFoldedIntoUnregistered(t *testing.T) {
	t.Run("底座适用冲突", func(t *testing.T) {
		fixture, _ := contractTierFixture(t)
		fixture.layers.layers.ProductBaseOutcome = pcdomain.ApplicabilityConflict
		fixture.layers.layers.ProductBase, fixture.layers.layers.HasProductBase = pcdomain.CustomerServiceRuleVersion{}, false

		_, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
		if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
			t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
		}
	})
	t.Run("底座解析未决", func(t *testing.T) {
		fixture, _ := contractTierFixture(t)
		fixture.layers.layers.ProductBaseOutcome = pcdomain.ResolutionPending
		fixture.layers.layers.ProductBase, fixture.layers.layers.HasProductBase = pcdomain.CustomerServiceRuleVersion{}, false

		_, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
		if err == nil {
			t.Fatal("底座解析未决被折成了业务答案")
		}
	})
	t.Run("层次读口不可用", func(t *testing.T) {
		fixture, _ := contractTierFixture(t)
		down := errors.New("party commercial layered read unavailable")
		fixture.layers.err = down

		_, _, err := fixture.adapter(t).RulesForClaim(context.Background(), fixture.query(t))
		if !errors.Is(err, down) {
			t.Fatalf("err = %v, want %v 原样可辨", err, down)
		}
	})
}
