package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: CONTEXT「客户服务规则必须按服务产品和客户合同明确适用范围、有效期间及责任方」，
// 以及 ADR-0093 的归族判据在构造门上的落点。
//
// 类别那一格最有后果：挂错类别的规则版本仍是一个合法的商业版本，入册、被解析选中都不会报错，
// 只是解析按错的类别去找——那正是 ADR-0093 否决复用 AuthorizationRuleObject 的理由。构造门若
// 不判类别，这个错要到下游取不到规则依据时才显形，而那时它长得像「这个客户没配规则」。
func TestCustomerServiceRuleVersionRefusesAVersionOfAnotherKind(t *testing.T) {
	responsible := commercialValue(t, domain.NewPartyID, "operator-1")
	scope := commercialValue(t, domain.NewCommercialScopeReference, "scope-a")
	product := commercialValue(t, domain.NewCommercialObjectID, "product-1")
	deadlines := []domain.ClaimDeadlineRule{firstClaimDeadline(t)}

	t.Run("a customer service rule version is accepted", func(t *testing.T) {
		rule, err := domain.NewCustomerServiceRuleVersion(
			effectiveVersionOfKind(t, domain.CustomerServiceRuleObject, "csr-1"),
			domain.CustomerServiceRuleAppliesToServiceProduct(product),
			responsible, scope, deadlines, nil,
		)
		if err != nil {
			t.Fatalf("new customer service rule version: %v", err)
		}
		if rule.ResponsibleParty() != responsible {
			t.Fatal("责任方没有原样留在规则版本上")
		}
	})

	t.Run("a version of another kind is refused", func(t *testing.T) {
		// 授权规则也是集内成员、也能生效，两者在版本壳上唯一的差别就是类别。
		_, err := domain.NewCustomerServiceRuleVersion(
			effectiveVersionOfKind(t, domain.AuthorizationRuleObject, "auth-1"),
			domain.CustomerServiceRuleAppliesToServiceProduct(product),
			responsible, scope, deadlines, nil,
		)
		if !errors.Is(err, domain.ErrInvalidCustomerServiceRuleVersion) {
			t.Fatalf("error = %v，别的类别的版本挂成了客户服务规则", err)
		}
	})

	t.Run("an applicability that names nothing is refused", func(t *testing.T) {
		// 零值适用对象过不了门：CONTEXT 要的是「明确适用范围」，而「没说挂在哪」与
		// 「挂在一个尚未指明的对象上」在零值里长得一样。
		_, err := domain.NewCustomerServiceRuleVersion(
			effectiveVersionOfKind(t, domain.CustomerServiceRuleObject, "csr-2"),
			domain.CustomerServiceRuleApplicability{},
			responsible, scope, deadlines, nil,
		)
		if !errors.Is(err, domain.ErrInvalidCustomerServiceRuleVersion) {
			t.Fatalf("error = %v，没有适用对象的规则版本立住了", err)
		}
	})
}

// Covers: ADR-0104 Decision 三「子行至少一项，不允许显式空版本」——一版客户服务规则存在的全部
// 理由就是承载差异，「对首发两项都无客户差异」不是一版规则，是不登记；以及 Decision 二两项正文
// 的形状：期限按种类成行、每种至多一行，材料按索赔类型成行、每类至多一行。
func TestCustomerServiceRuleVersionCarriesAtLeastOneItemAndAtMostOneRowPerKey(t *testing.T) {
	responsible := commercialValue(t, domain.NewPartyID, "operator-1")
	scope := commercialValue(t, domain.NewCommercialScopeReference, "scope-a")
	contract := commercialValue(t, domain.NewCommercialObjectID, "contract-1")
	applicability := domain.CustomerServiceRuleAppliesToCustomerContract(contract)
	version := effectiveVersionOfKind(t, domain.CustomerServiceRuleObject, "csr-3")

	t.Run("no item at all is refused", func(t *testing.T) {
		_, err := domain.NewCustomerServiceRuleVersion(version, applicability, responsible, scope, nil, nil)
		if !errors.Is(err, domain.ErrInvalidCustomerServiceRuleVersion) {
			t.Fatalf("error = %v，一项正文都没有的规则版本立住了", err)
		}
	})

	t.Run("materials alone are enough", func(t *testing.T) {
		rule, err := domain.NewCustomerServiceRuleVersion(
			version, applicability, responsible, scope,
			nil, []domain.MinimumMaterialsRule{minimumMaterials(t, "claim-loss", "material-photo", "material-invoice")},
		)
		if err != nil {
			t.Fatalf("只带材料的规则版本：%v", err)
		}
		materials, found := rule.MinimumMaterialsFor(commercialValue(t, domain.NewClaimKindReference, "claim-loss"))
		if !found || len(materials.Materials()) != 2 {
			t.Fatalf("材料清单没有原样留下：found=%v %#v", found, materials)
		}
		if _, found := rule.ClaimDeadline(domain.FirstClaimDeadline); found {
			t.Fatal("没登记期限却读出了一条")
		}
	})

	t.Run("two rows for one deadline kind are refused", func(t *testing.T) {
		first := firstClaimDeadline(t)
		again := claimDeadline(t, domain.FirstClaimDeadline, "event-delivered", 45, "calendar-cn")
		_, err := domain.NewCustomerServiceRuleVersion(
			version, applicability, responsible, scope,
			[]domain.ClaimDeadlineRule{first, again}, nil,
		)
		if !errors.Is(err, domain.ErrDuplicateCustomerServiceRuleItem) {
			t.Fatalf("error = %v，同一种期限两行立住了", err)
		}
	})

	t.Run("two rows for one claim kind are refused", func(t *testing.T) {
		_, err := domain.NewCustomerServiceRuleVersion(
			version, applicability, responsible, scope, nil,
			[]domain.MinimumMaterialsRule{
				minimumMaterials(t, "claim-loss", "material-photo"),
				minimumMaterials(t, "claim-loss", "material-invoice"),
			},
		)
		if !errors.Is(err, domain.ErrDuplicateCustomerServiceRuleItem) {
			t.Fatalf("error = %v，同一索赔类型两行材料立住了", err)
		}
	})

	t.Run("deadlines come back keyed by kind and ordered", func(t *testing.T) {
		review := claimDeadline(t, domain.ConclusionReviewDeadline, "event-conclusion-delivered", 15, "calendar-cn")
		rule, err := domain.NewCustomerServiceRuleVersion(
			version, applicability, responsible, scope,
			[]domain.ClaimDeadlineRule{review, firstClaimDeadline(t)}, nil,
		)
		if err != nil {
			t.Fatalf("两种期限的规则版本：%v", err)
		}
		deadlines := rule.ClaimDeadlines()
		if len(deadlines) != 2 || deadlines[0].Kind() != domain.FirstClaimDeadline || deadlines[1].Kind() != domain.ConclusionReviewDeadline {
			t.Fatalf("期限顺序 = %#v，want 首次索赔在前、结论复核在后", deadlines)
		}
		got, found := rule.ClaimDeadline(domain.ConclusionReviewDeadline)
		if !found || got.DurationDays() != 15 || got.StartEvent().String() != "event-conclusion-delivered" {
			t.Fatalf("结论复核期限 = %#v found=%v", got, found)
		}
	})
}

// Covers: ADR-0104 Decision 二「只校形状不校值」——形状即：种类在封闭三格内、起算事件与日历引用
// 非空、时长为正整数天；材料清单至少一项且不重复。任何具体天数、日历或材料取值本文件只当夹具用，
// 不作断言的依据。
func TestClaimDeadlineAndMinimumMaterialsRulesCheckShapeOnly(t *testing.T) {
	startEvent := commercialValue(t, domain.NewDeadlineStartEventReference, "event-delivered")
	calendar := commercialValue(t, domain.NewBusinessCalendarReference, "calendar-cn")

	t.Run("a non-positive duration is refused", func(t *testing.T) {
		for _, days := range []int{0, -1} {
			if _, err := domain.NewClaimDeadlineRule(domain.FirstClaimDeadline, startEvent, days, calendar); !errors.Is(err, domain.ErrInvalidClaimDeadlineRule) {
				t.Fatalf("days=%d error = %v，非正时长立住了", days, err)
			}
		}
	})

	t.Run("an undeclared kind or a blank reference is refused", func(t *testing.T) {
		if _, err := domain.NewClaimDeadlineRule(domain.ClaimDeadlineKindInvalid, startEvent, 30, calendar); !errors.Is(err, domain.ErrInvalidClaimDeadlineRule) {
			t.Fatalf("error = %v，集外的期限种类立住了", err)
		}
		if _, err := domain.NewClaimDeadlineRule(domain.FirstClaimDeadline, domain.DeadlineStartEventReference{}, 30, calendar); !errors.Is(err, domain.ErrInvalidClaimDeadlineRule) {
			t.Fatalf("error = %v，没有起算事件的期限立住了", err)
		}
		if _, err := domain.NewClaimDeadlineRule(domain.FirstClaimDeadline, startEvent, 30, domain.BusinessCalendarReference{}); !errors.Is(err, domain.ErrInvalidClaimDeadlineRule) {
			t.Fatalf("error = %v，没有日历引用的期限立住了", err)
		}
	})

	t.Run("materials must list at least one distinct entry", func(t *testing.T) {
		claimKind := commercialValue(t, domain.NewClaimKindReference, "claim-loss")
		photo := commercialValue(t, domain.NewMaterialRequirementReference, "material-photo")
		if _, err := domain.NewMinimumMaterialsRule(claimKind, nil); !errors.Is(err, domain.ErrInvalidMinimumMaterialsRule) {
			t.Fatalf("error = %v，空清单立住了", err)
		}
		if _, err := domain.NewMinimumMaterialsRule(claimKind, []domain.MaterialRequirementReference{photo, photo}); !errors.Is(err, domain.ErrInvalidMinimumMaterialsRule) {
			t.Fatalf("error = %v，重复条目立住了", err)
		}
		if _, err := domain.NewMinimumMaterialsRule(domain.ClaimKindReference{}, []domain.MaterialRequirementReference{photo}); !errors.Is(err, domain.ErrInvalidMinimumMaterialsRule) {
			t.Fatalf("error = %v，没有索赔类型的材料规则立住了", err)
		}
	})

	t.Run("the deadline kinds are the three VE names", func(t *testing.T) {
		for kind, name := range map[domain.ClaimDeadlineKind]string{
			domain.FirstClaimDeadline:         "FIRST_CLAIM",
			domain.MaterialSupplementDeadline: "MATERIAL_SUPPLEMENT",
			domain.ConclusionReviewDeadline:   "CONCLUSION_REVIEW",
		} {
			if kind.String() != name {
				t.Fatalf("%d.String() = %q, want %q", kind, kind.String(), name)
			}
		}
		if domain.ClaimDeadlineKindInvalid.String() != "" {
			t.Fatal("零值种类有了名字")
		}
	})
}

// Covers: ADR-0104 Decision 四「消费方点读之后核『这一版挂的是不是我手上这份产品 / 合同』」——
// 与 ConsistentAcceptanceRulePackage 同一道核对：壳上指名了正文适用对象那一类的引用时必须相等；
// 壳上没指名、或指名的是另一类，放行。
func TestCustomerServiceRuleApplicabilityIsCheckedAgainstTheShellReference(t *testing.T) {
	productA := commercialValue(t, domain.NewCommercialObjectID, "product-a")
	productB := commercialValue(t, domain.NewCommercialObjectID, "product-b")
	contract := commercialValue(t, domain.NewCommercialObjectID, "contract-1")

	namesProductA := effectiveVersionReferencing(t, "csr-ref-1", map[domain.CommercialObjectKind]domain.CommercialObjectID{
		domain.ServiceProductObject: productA,
	})
	namesContract := effectiveVersionReferencing(t, "csr-ref-2", map[domain.CommercialObjectKind]domain.CommercialObjectID{
		domain.CustomerContractObject: contract,
	})
	namesNothing := effectiveVersionOfKind(t, domain.CustomerServiceRuleObject, "csr-ref-3")

	if err := domain.ConsistentCustomerServiceRuleApplicability(namesProductA, domain.CustomerServiceRuleAppliesToServiceProduct(productA)); err != nil {
		t.Fatalf("壳与正文指同一产品却被拒：%v", err)
	}
	if err := domain.ConsistentCustomerServiceRuleApplicability(namesProductA, domain.CustomerServiceRuleAppliesToServiceProduct(productB)); !errors.Is(err, domain.ErrCustomerServiceRuleApplicabilityMismatch) {
		t.Fatalf("error = %v，壳指产品 A、正文挂产品 B 放行了", err)
	}
	if err := domain.ConsistentCustomerServiceRuleApplicability(namesContract, domain.CustomerServiceRuleAppliesToServiceProduct(productB)); err != nil {
		t.Fatalf("壳只指名合同、正文挂产品，本该无从比对而放行：%v", err)
	}
	if err := domain.ConsistentCustomerServiceRuleApplicability(namesNothing, domain.CustomerServiceRuleAppliesToCustomerContract(contract)); err != nil {
		t.Fatalf("壳没指名任何引用却被拒：%v", err)
	}
}

func effectiveVersionOfKind(
	t *testing.T,
	kind domain.CommercialObjectKind,
	objectID string,
) domain.CommercialVersion {
	t.Helper()
	version := registerable(t, kind, objectID, "v1", "sha256:"+objectID)
	live, err := version.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}

// effectiveVersionReferencing 造一个已生效、壳上指名了给定引用的客户服务规则版本。被引对象
// 以「已发布」交给 Publish，否则悬空引用会让发布停在未决。
func effectiveVersionReferencing(
	t *testing.T,
	objectID string,
	references map[domain.CommercialObjectKind]domain.CommercialObjectID,
) domain.CommercialVersion {
	t.Helper()
	spec := commercialSpec(t, domain.CustomerServiceRuleObject, objectID, "v1", "sha256:"+objectID)
	spec.References = references
	draft, err := domain.NewCommercialDraft(spec)
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	allPublished := func(domain.CommercialObjectKind, domain.CommercialObjectID) domain.NamedReferenceStanding {
		return domain.NamedReferencePublished
	}
	published, err := draft.Publish(
		approval(t, "approval-"+objectID),
		domain.ApprovalRoleConfirmed,
		time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		allPublished,
	)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	live, err := published.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}

func firstClaimDeadline(t *testing.T) domain.ClaimDeadlineRule {
	t.Helper()
	return claimDeadline(t, domain.FirstClaimDeadline, "event-delivered", 30, "calendar-cn")
}

func claimDeadline(t *testing.T, kind domain.ClaimDeadlineKind, startEvent string, days int, calendar string) domain.ClaimDeadlineRule {
	t.Helper()
	rule, err := domain.NewClaimDeadlineRule(
		kind,
		commercialValue(t, domain.NewDeadlineStartEventReference, startEvent),
		days,
		commercialValue(t, domain.NewBusinessCalendarReference, calendar),
	)
	if err != nil {
		t.Fatalf("new claim deadline rule: %v", err)
	}
	return rule
}

func minimumMaterials(t *testing.T, claimKind string, materials ...string) domain.MinimumMaterialsRule {
	t.Helper()
	references := make([]domain.MaterialRequirementReference, 0, len(materials))
	for _, material := range materials {
		references = append(references, commercialValue(t, domain.NewMaterialRequirementReference, material))
	}
	rule, err := domain.NewMinimumMaterialsRule(commercialValue(t, domain.NewClaimKindReference, claimKind), references)
	if err != nil {
		t.Fatalf("new minimum materials rule: %v", err)
	}
	return rule
}
