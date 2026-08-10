package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func ruleRef(t *testing.T, category domain.RuleCategory, reference string) domain.AssembledRule {
	t.Helper()
	assembled, err := domain.NewAssembledRule(category, commercialValue(t, domain.NewRuleReference, reference))
	if err != nil {
		t.Fatalf("new assembled rule: %v", err)
	}
	return assembled
}

func rulePackageVersion(t *testing.T, objectID string) domain.CommercialVersion {
	t.Helper()
	live, err := registerable(t, domain.AcceptanceRulePackageObject, objectID, "v1", "sha256:"+objectID).
		TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}

func rulePackageApplicability(t *testing.T) domain.RulePackageApplicability {
	t.Helper()
	built, err := domain.NewRulePackageApplicability(
		commercialValue(t, domain.NewCommercialObjectID, "product-1"),
		commercialValue(t, domain.NewCommercialObjectID, "contract-1"),
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
		mustInterval(t),
	)
	if err != nil {
		t.Fatalf("new rule package applicability: %v", err)
	}
	return built
}

func acceptanceRulePackage(t *testing.T, rules ...domain.AssembledRule) domain.AcceptanceRulePackage {
	t.Helper()
	built, err := domain.NewAcceptanceRulePackage(rulePackageVersion(t, "rules-1"), rulePackageApplicability(t), rules)
	if err != nil {
		t.Fatalf("new acceptance rule package: %v", err)
	}
	return built
}

// Covers: CONTEXT「区分接入最小身份、委托通用不变量、产品与合同资料、监管原始资料及
// 跨字段条件」— 五类分区各自独立，一条规则恰属一类。
func TestRulesAreAssembledIntoFiveIndependentCategories(t *testing.T) {
	categories := []domain.RuleCategory{
		domain.MinimumIngressIdentityRules,
		domain.ShipmentInvariantRules,
		domain.ProductAndContractDocumentRules,
		domain.RegulatorySourceDocumentRules,
		domain.CrossFieldConditionRules,
	}

	rules := make([]domain.AssembledRule, 0, len(categories))
	for _, category := range categories {
		rules = append(rules, ruleRef(t, category, "rule-"+category.String()))
	}
	pack := acceptanceRulePackage(t, rules...)

	seen := map[string]struct{}{}
	for _, category := range categories {
		label := category.String()
		if label == "" {
			t.Fatalf("category %d has no label", category)
		}
		if _, clash := seen[label]; clash {
			t.Fatalf("two categories share the label %q", label)
		}
		seen[label] = struct{}{}

		assembled := pack.RulesIn(category)
		if len(assembled) != 1 || assembled[0].Category() != category {
			t.Fatalf("category %q holds %v", category, assembled)
		}
	}
	if domain.RuleCategory(len(categories)+1).String() != "" {
		t.Fatal("a sixth rule category carries a label")
	}
}

// Covers: CONTEXT「规则包只装配各权威上下文的规则引用，不得替具体委托选择判断值」—
// 装配项是引用，结构上无处安放取值。
func TestAssembledRulesCarryReferencesNotValues(t *testing.T) {
	assembledType := reflect.TypeOf(domain.AssembledRule{})
	forbidden := []string{"value", "threshold", "amount", "limit", "min", "max", "date", "deadline"}
	for index := 0; index < assembledType.NumField(); index++ {
		name := strings.ToLower(assembledType.Field(index).Name)
		for _, word := range forbidden {
			if strings.Contains(name, word) {
				t.Fatalf("AssembledRule carries %s, so the package could choose a judgement value",
					assembledType.Field(index).Name)
			}
		}
	}

	if _, err := domain.NewAssembledRule(domain.ShipmentInvariantRules, domain.RuleReference{}); !errors.Is(err, domain.ErrInvalidAssembledRule) {
		t.Fatal("a rule without a reference was assembled")
	}
}

// Covers: CONTEXT「不得把正式关务判断前移到商业上下文」— 监管一类装的是原始资料要求的
// 引用，规则包上不存在任何关务裁决或监管结果。
func TestRegulatoryCategoryHoldsDocumentRulesNotCustomsVerdicts(t *testing.T) {
	pack := acceptanceRulePackage(t, ruleRef(t, domain.RegulatorySourceDocumentRules, "rule-declaration-fields"))

	if len(pack.RulesIn(domain.RegulatorySourceDocumentRules)) != 1 {
		t.Fatal("the regulatory category lost its document rule")
	}

	packType := reflect.TypeOf(domain.AcceptanceRulePackage{})
	forbidden := []string{"customs", "declaration", "verdict", "clearance", "regulatorydecision", "permit"}
	for index := 0; index < packType.NumField(); index++ {
		name := strings.ToLower(packType.Field(index).Name)
		for _, word := range forbidden {
			if strings.Contains(name, word) {
				t.Fatalf("AcceptanceRulePackage carries %s, which pulls a customs determination into the commercial context",
					packType.Field(index).Name)
			}
		}
	}
}

// Covers: CONTEXT「规则或策略缺失不得被解释为允许接受」— 一条规则都没有的规则包等于
// 无条件接受，因此建不成。
func TestAnEmptyRulePackageCannotBeBuilt(t *testing.T) {
	if _, err := domain.NewAcceptanceRulePackage(rulePackageVersion(t, "rules-1"), rulePackageApplicability(t), nil); !errors.Is(err, domain.ErrInvalidAcceptanceRulePackage) {
		t.Fatalf("error = %v, want ErrInvalidAcceptanceRulePackage", err)
	}
}

// Covers: CONTEXT「接单规则包必须按服务产品、客户合同、责任法人、服务或关务范围和有效
// 期间选择」— 五维缺一即建不成适用范围。
func TestRulePackageApplicabilityNeedsAllFiveDimensions(t *testing.T) {
	complete := func() (domain.CommercialObjectID, domain.CommercialObjectID, domain.LegalEntityReference, domain.CommercialScopeReference) {
		return commercialValue(t, domain.NewCommercialObjectID, "product-1"),
			commercialValue(t, domain.NewCommercialObjectID, "contract-1"),
			commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
			commercialValue(t, domain.NewCommercialScopeReference, "scope-a")
	}

	t.Run("no product", func(t *testing.T) {
		_, contract, legal, scope := complete()
		if _, err := domain.NewRulePackageApplicability(domain.CommercialObjectID{}, contract, legal, scope, mustInterval(t)); !errors.Is(err, domain.ErrInvalidRulePackageApplicability) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("no contract", func(t *testing.T) {
		product, _, legal, scope := complete()
		if _, err := domain.NewRulePackageApplicability(product, domain.CommercialObjectID{}, legal, scope, mustInterval(t)); !errors.Is(err, domain.ErrInvalidRulePackageApplicability) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("no legal entity", func(t *testing.T) {
		product, contract, _, scope := complete()
		if _, err := domain.NewRulePackageApplicability(product, contract, domain.LegalEntityReference{}, scope, mustInterval(t)); !errors.Is(err, domain.ErrInvalidRulePackageApplicability) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("no scope", func(t *testing.T) {
		product, contract, legal, _ := complete()
		if _, err := domain.NewRulePackageApplicability(product, contract, legal, domain.CommercialScopeReference{}, mustInterval(t)); !errors.Is(err, domain.ErrInvalidRulePackageApplicability) {
			t.Fatalf("error = %v", err)
		}
	})
}

// Covers: CONTEXT 商业版本共同不变量 — 内容挂在一个当前可用的接单规则包版本上，且该
// 版本正是 `asOf` 策略声明所依附的那一个。
func TestRulePackageContentBindsToItsOwnUsableVersion(t *testing.T) {
	pack := acceptanceRulePackage(t, ruleRef(t, domain.ShipmentInvariantRules, "rule-1"))

	declaration, err := domain.DeclareAsOfPolicies(pack.Version(), []domain.AsOfPolicy{
		asOfPolicy(t, domain.NetworkReachabilityJudgment, "semantics-network", "asof-policy-v1"),
	})
	if err != nil {
		t.Fatalf("the package version could not declare as-of policies: %v", err)
	}
	if declaration.RulePackage().ObjectID() != pack.Version().ObjectID() {
		t.Fatal("the as-of declaration is bound to a different rule package")
	}

	t.Run("refuses another object kind", func(t *testing.T) {
		contract := contractVersion(t, "contract-9")
		if _, err := domain.NewAcceptanceRulePackage(contract, rulePackageApplicability(t), []domain.AssembledRule{ruleRef(t, domain.ShipmentInvariantRules, "rule-1")}); !errors.Is(err, domain.ErrInvalidAcceptanceRulePackage) {
			t.Fatalf("error = %v, want ErrInvalidAcceptanceRulePackage", err)
		}
	})
}
