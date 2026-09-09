package domain_test

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件钉词表读口与各册构造门同词（票 admin-write-faces/20「要做的」双向：集合里的每个词都被该册的构造门
// 接受，构造门接受的每个词都在集合里）。
//
// 预言机是导出的构造门本身，不是词表的实现：把整个 uint8 值域逐个喂给构造门，被接受的那些值的 String()
// 按值升序就是集合该有的样子——词表若改成一份手写常量表、枚举加一格而表没改，这里就红；词表若把「未声明」
// 那种缺格读法列成一格取值，构造门拒它，这里同样红。

// codesAccepted 扫整个 uint8 值域，交回构造门接受的那些值的原词（按值升序，即领域枚举的声明顺序）。
func codesAccepted(accepts func(raw uint8) bool, name func(raw uint8) string) []string {
	codes := []string{}
	for raw := 0; raw <= math.MaxUint8; raw++ {
		if accepts(uint8(raw)) {
			codes = append(codes, name(uint8(raw)))
		}
	}
	return codes
}

func vocabularyOf(t *testing.T, kind domain.CommercialObjectKind) []domain.VocabularySet {
	t.Helper()
	sets, err := domain.PublicationVocabulary(kind)
	if err != nil {
		t.Fatalf("PublicationVocabulary(%s): %v", kind, err)
	}
	return sets
}

// assertSetsMatchGates 逐集合比：名字按给定顺序、码与构造门接受的原词逐字逐序相等（一次比较同时钉住两个方向
// 与顺序），且没有一个码是空串——空串说明某个被接受的值没有原词，那是枚举漏了 String() 的一格。
func assertSetsMatchGates(t *testing.T, sets []domain.VocabularySet, order []string, expected map[string][]string) {
	t.Helper()
	if len(sets) != len(order) {
		t.Fatalf("sets = %v, want the %d sets %v", names(sets), len(order), order)
	}
	for index, set := range sets {
		if set.Name != order[index] {
			t.Fatalf("set[%d] = %q, want %q (order = %v)", index, set.Name, order[index], names(sets))
		}
		want := expected[set.Name]
		if strings.Join(set.Codes, ",") != strings.Join(want, ",") {
			t.Fatalf("set %q codes = %v, constructor gates accept %v", set.Name, set.Codes, want)
		}
		for _, code := range set.Codes {
			if code == "" {
				t.Fatalf("set %q lists an unnamed code: %v", set.Name, set.Codes)
			}
		}
	}
}

func names(sets []domain.VocabularySet) []string {
	found := make([]string, 0, len(sets))
	for _, set := range sets {
		found = append(found, set.Name)
	}
	return found
}

// Covers: 票 20 完成判据「kind=ACCEPTANCE_RULE_PACKAGE 答票 12 点名的那几个集合、码序同领域枚举」——正文与各声明
// 通道里凡是领域枚举的格各一集合，码与该格的构造门同词；开放引用（规则引用、时点语义、资料组、终局类型）不成集合。
func TestAcceptanceRulePackageVocabularyMatchesItsConstructorGates(t *testing.T) {
	owner := effectiveVersionOfKind(t, domain.AcceptanceRulePackageObject, "rule-package-vocab")
	rule := commercialValue(t, domain.NewRuleReference, "rule-1")
	semantics := commercialValue(t, domain.NewAsOfSemanticsReference, "semantics-1")
	policyVersion := commercialValue(t, domain.NewAsOfPolicyVersion, "policy-v1")
	group := commercialValue(t, domain.NewSourceDataGroupReference, "consignee.address")

	amendmentRule := func(stage domain.DeclaredAmendmentStage, intent domain.DeclaredAmendmentIntent, allowance domain.AmendmentAllowance) bool {
		_, err := domain.NewSourceDataAmendmentAllowanceContent(owner, false, []domain.SourceDataAmendmentRule{
			{DataGroup: group, Stage: stage, Intent: intent, Allowance: allowance},
		})
		return err == nil
	}

	assertSetsMatchGates(t, vocabularyOf(t, domain.AcceptanceRulePackageObject),
		[]string{"category", "judgment", "applicableGroups", "manualReview", "sources", "outcome", "anchor", "stage", "intent", "allowance"},
		map[string][]string{
			"category": codesAccepted(func(raw uint8) bool {
				_, err := domain.NewAssembledRule(domain.RuleCategory(raw), rule)
				return err == nil
			}, func(raw uint8) string { return domain.RuleCategory(raw).String() }),
			"judgment": codesAccepted(func(raw uint8) bool {
				_, err := domain.NewAsOfPolicy(domain.JudgmentType(raw), semantics, policyVersion)
				return err == nil
			}, func(raw uint8) string { return domain.JudgmentType(raw).String() }),
			"applicableGroups": codesAccepted(func(raw uint8) bool {
				_, err := domain.DeclareAcceptanceRuleContent(owner, []domain.AcceptanceCheckGroupType{domain.AcceptanceCheckGroupType(raw)}, domain.ManualReviewNotRequired)
				return err == nil
			}, func(raw uint8) string { return domain.AcceptanceCheckGroupType(raw).String() }),
			"manualReview": codesAccepted(func(raw uint8) bool {
				_, err := domain.DeclareAcceptanceRuleContent(owner, []domain.AcceptanceCheckGroupType{domain.CustomerRelationshipCheckGroup}, domain.ManualReviewDirective(raw))
				return err == nil
			}, func(raw uint8) string { return domain.ManualReviewDirective(raw).String() }),
			"sources": codesAccepted(func(raw uint8) bool {
				_, err := domain.NewIntakeQualificationContent(owner, []domain.DeclaredIntakeSource{domain.DeclaredIntakeSource(raw)}, nil)
				return err == nil
			}, func(raw uint8) string { return domain.DeclaredIntakeSource(raw).String() }),
			"outcome": codesAccepted(func(raw uint8) bool {
				_, err := domain.NewFinalRuleContent(owner, []domain.FinalizationDeclaration{{Outcome: domain.DeclaredResponsibilityOutcome(raw), FinalKind: rule}})
				return err == nil
			}, func(raw uint8) string { return domain.DeclaredResponsibilityOutcome(raw).String() }),
			"anchor": codesAccepted(func(raw uint8) bool {
				_, err := domain.NewLabelValidityDeclaration(domain.ValidityAnchorKind(raw), time.Hour)
				return err == nil
			}, func(raw uint8) string { return domain.ValidityAnchorKind(raw).String() }),
			"stage": codesAccepted(func(raw uint8) bool {
				return amendmentRule(domain.DeclaredAmendmentStage(raw), domain.DeclaredSupplementIntent, domain.AmendmentAllowed)
			}, func(raw uint8) string { return domain.DeclaredAmendmentStage(raw).String() }),
			"intent": codesAccepted(func(raw uint8) bool {
				return amendmentRule(domain.DeclaredAcceptedNotYetReceived, domain.DeclaredAmendmentIntent(raw), domain.AmendmentAllowed)
			}, func(raw uint8) string { return domain.DeclaredAmendmentIntent(raw).String() }),
			"allowance": codesAccepted(func(raw uint8) bool {
				return amendmentRule(domain.DeclaredAcceptedNotYetReceived, domain.DeclaredSupplementIntent, domain.AmendmentAllowance(raw))
			}, func(raw uint8) string { return domain.AmendmentAllowance(raw).String() }),
		})
}

// Covers: 票 12「册与载荷」点名的几个字面——`anchor` 首发只有 CHANNEL_RESULT_OBSERVED；`allowance` 只有 ALLOWED /
// DISALLOWED，「未声明」是缺格的读法不是一行能选的值；`stage` 是 parcel-shipment 的原词、按 PS 阶段顺序。
// 字面钉在这里是为了让上一条「与构造门同词」不至于两边一起错。
func TestAcceptanceRulePackageVocabularyCarriesTheWordsTheFormTicketNames(t *testing.T) {
	byName := map[string][]string{}
	for _, set := range vocabularyOf(t, domain.AcceptanceRulePackageObject) {
		byName[set.Name] = set.Codes
	}
	if got := strings.Join(byName["anchor"], ","); got != "CHANNEL_RESULT_OBSERVED" {
		t.Fatalf("anchor = %q", got)
	}
	if got := strings.Join(byName["allowance"], ","); got != "ALLOWED,DISALLOWED" {
		t.Fatalf("allowance = %q; NOT_DECLARED must not be offered as a value", got)
	}
	wantStages := "ACCEPTED_NOT_YET_RECEIVED,RECEIVED_OR_MEASURED,LABELLED_OR_BAGGED,CUSTOMS_DATA_FORMING_NOT_SUBMITTED,CUSTOMS_SUBMITTED,CASE_CLOSED_OR_SERVICE_COMPLETED"
	if got := strings.Join(byName["stage"], ","); got != wantStages {
		t.Fatalf("stage = %q, want %q", got, wantStages)
	}
	if got := strings.Join(byName["intent"], ","); got != "SUPPLEMENT,CORRECTION,EXPLICIT_CLEAR" {
		t.Fatalf("intent = %q", got)
	}
	if got := strings.Join(byName["manualReview"], ","); got != "NOT_REQUIRED,REQUIRED" {
		t.Fatalf("manualReview = %q; the undeclared zero value must not be offered", got)
	}
}

// Covers: 票 13「三个封闭集（控制种类、失败处置、共同通过条件）由服务端词表读口供下拉」——键名照批文
// （jointPassCondition / control / onFailure），码与控制项及策略正文的构造门同词；控制种类里没有「无控制」
// 那一格（ADR-0115 Decision 一），构造门不接受它，集合里自然没有。
func TestPreAcceptanceFinancialControlPolicyVocabularyMatchesItsConstructorGates(t *testing.T) {
	owner := effectiveVersionOfKind(t, domain.PreAcceptanceFinancialControlPolicyObject, "control-policy-vocab")
	scope := commercialValue(t, domain.NewChargeScopeReference, "freight")
	responsibility := commercialValue(t, domain.NewControlResponsibilityReference, "customer")
	item, err := domain.NewPreAcceptanceControlItem(domain.PrepaidFreezeControl, scope, 1, domain.RejectOnControlFailure, responsibility)
	if err != nil {
		t.Fatalf("control item: %v", err)
	}

	assertSetsMatchGates(t, vocabularyOf(t, domain.PreAcceptanceFinancialControlPolicyObject),
		[]string{"jointPassCondition", "control", "onFailure"},
		map[string][]string{
			"jointPassCondition": codesAccepted(func(raw uint8) bool {
				_, err := domain.NewPreAcceptanceFinancialControlPolicy(owner, domain.JointPassCondition(raw), []domain.PreAcceptanceControlItem{item})
				return err == nil
			}, func(raw uint8) string { return domain.JointPassCondition(raw).String() }),
			"control": codesAccepted(func(raw uint8) bool {
				_, err := domain.NewPreAcceptanceControlItem(domain.PreAcceptanceControlKind(raw), scope, 1, domain.RejectOnControlFailure, responsibility)
				return err == nil
			}, func(raw uint8) string { return domain.PreAcceptanceControlKind(raw).String() }),
			"onFailure": codesAccepted(func(raw uint8) bool {
				_, err := domain.NewPreAcceptanceControlItem(domain.PrepaidFreezeControl, scope, 1, domain.ControlFailureDisposition(raw), responsibility)
				return err == nil
			}, func(raw uint8) string { return domain.ControlFailureDisposition(raw).String() }),
		})
}

// Covers: 票 15「`method` 下拉由服务端词表读口供」——预付 / 账期一族，与 NewSettlementPolicy 同词；没有第三格
// 「客户级默认」，那正是本上下文禁的。
func TestSettlementPolicyVocabularyMatchesItsConstructorGate(t *testing.T) {
	owner := effectiveVersionOfKind(t, domain.SettlementPolicyObject, "settlement-vocab")
	where := applicability(t, "customer-1", "contract-1/v1", "freight", "SYN")

	assertSetsMatchGates(t, vocabularyOf(t, domain.SettlementPolicyObject),
		[]string{"method"},
		map[string][]string{
			"method": codesAccepted(func(raw uint8) bool {
				_, err := domain.NewSettlementPolicy(owner, domain.SettlementMethod(raw), where)
				return err == nil
			}, func(raw uint8) string { return domain.SettlementMethod(raw).String() }),
		})
	if got := strings.Join(vocabularyOf(t, domain.SettlementPolicyObject)[0].Codes, ","); got != "PREPAID,TERMS" {
		t.Fatalf("method = %q", got)
	}
}

// Covers: 票 17「请求方下拉由服务端词表读口供（封闭二值）」——键名 `party`，与 NewCancellationAuthorityContent 同词；
// 授权授予册的 AuthorizedAction 不在其中（票 17：那是授予册的事，与请求方无关）。
func TestAuthorizationRuleVocabularyMatchesItsConstructorGate(t *testing.T) {
	owner := effectiveVersionOfKind(t, domain.AuthorizationRuleObject, "authorization-vocab")
	rule := commercialValue(t, domain.NewRuleReference, "cancel-rule-1")

	sets := vocabularyOf(t, domain.AuthorizationRuleObject)
	assertSetsMatchGates(t, sets,
		[]string{"party"},
		map[string][]string{
			"party": codesAccepted(func(raw uint8) bool {
				_, err := domain.NewCancellationAuthorityContent(owner, []domain.CancellationAuthorityDeclaration{{Party: domain.DeclaredCancellationParty(raw), Rule: rule}})
				return err == nil
			}, func(raw uint8) string { return domain.DeclaredCancellationParty(raw).String() }),
		})
	if got := strings.Join(sets[0].Codes, ","); got != "CUSTOMER,OPERATIONS" {
		t.Fatalf("party = %q", got)
	}
}

// Covers: 票 18「期限种类的封闭集由服务端词表读口供下拉」——键名照批文 claimDeadlines[].kind，码与 NewClaimDeadlineRule 同词、
// 按 visibility-exception CONTEXT 三种期限的声明顺序；起算事件、日历、索赔类型与材料是开放引用，不成集合。
func TestCustomerServiceRuleVocabularyMatchesItsConstructorGate(t *testing.T) {
	startEvent := commercialValue(t, domain.NewDeadlineStartEventReference, "event-delivered")
	calendar := commercialValue(t, domain.NewBusinessCalendarReference, "calendar-cn")

	sets := vocabularyOf(t, domain.CustomerServiceRuleObject)
	assertSetsMatchGates(t, sets,
		[]string{"kind"},
		map[string][]string{
			"kind": codesAccepted(func(raw uint8) bool {
				_, err := domain.NewClaimDeadlineRule(domain.ClaimDeadlineKind(raw), startEvent, 30, calendar)
				return err == nil
			}, func(raw uint8) string { return domain.ClaimDeadlineKind(raw).String() }),
		})
	if got := strings.Join(sets[0].Codes, ","); got != "FIRST_CLAIM,MATERIAL_SUPPLEMENT,CONCLUSION_REVIEW" {
		t.Fatalf("kind = %q", got)
	}
}

// Covers: ADR-0129 决定五——信用政策册的 `ratioBase` 一集从 NewCreditRatioLimit 的接受判据逐值列出，一字不多不少：
// 「未声明」不在（它是重建门的读法，不是一行能选的取值），票面候选名 DEPOSIT_BALANCE 不在。
func TestCreditPolicyVocabularyMatchesTheRatioLimitGate(t *testing.T) {
	assertSetsMatchGates(t, vocabularyOf(t, domain.CreditPolicyObject),
		[]string{"ratioBase"},
		map[string][]string{
			"ratioBase": codesAccepted(func(raw uint8) bool {
				_, err := domain.NewCreditRatioLimit(1, domain.CreditRatioBase(raw))
				return err == nil
			}, func(raw uint8) string { return domain.CreditRatioBase(raw).String() }),
		})
	if got := strings.Join(vocabularyOf(t, domain.CreditPolicyObject)[0].Codes, ","); got != "POSTED_BALANCE,PRIOR_PERIOD_CONFIRMED_CHARGES" {
		t.Fatalf("ratioBase = %q", got)
	}
}

// Covers: 票 20 裁决三「某册没有封闭集就答空集合列表，不答 404」——每个合法 kind 都答得出来；正文里没有封闭集的册
// 一律是空列表（非 nil：线上是 `[]` 不是 `null`），服务产品与客户合同点名在内。
func TestRegistersWithoutClosedSetsAnswerAnEmptyList(t *testing.T) {
	withSets := map[domain.CommercialObjectKind]bool{
		domain.AcceptanceRulePackageObject:               true,
		domain.PreAcceptanceFinancialControlPolicyObject: true,
		domain.SettlementPolicyObject:                    true,
		domain.AuthorizationRuleObject:                   true,
		domain.CustomerServiceRuleObject:                 true,
		// ADR-0129：信用政策册自此有一集（比例基数）。
		domain.CreditPolicyObject: true,
	}
	answered := 0
	for raw := 0; raw <= math.MaxUint8; raw++ {
		kind := domain.CommercialObjectKind(raw)
		if kind.String() == "" {
			continue
		}
		answered++
		sets, err := domain.PublicationVocabulary(kind)
		if err != nil {
			t.Fatalf("%s: err = %v, want nil (a legal kind without words is an empty list, not a refusal)", kind, err)
		}
		if withSets[kind] {
			if len(sets) == 0 {
				t.Fatalf("%s: no sets, want the form ticket's closed sets", kind)
			}
			continue
		}
		if sets == nil || len(sets) != 0 {
			t.Fatalf("%s: sets = %#v, want an empty non-nil list", kind, sets)
		}
	}
	if answered == 0 {
		t.Fatal("no CommercialObjectKind has a name; the enumeration probe is broken")
	}
	if sets := vocabularyOf(t, domain.ServiceProductObject); len(sets) != 0 {
		t.Fatalf("SERVICE_PRODUCT sets = %v, want none", names(sets))
	}
	if sets := vocabularyOf(t, domain.CustomerContractObject); len(sets) != 0 {
		t.Fatalf("CUSTOMER_CONTRACT sets = %v, want none", names(sets))
	}
}

// Covers: 票 20 裁决四「未知 kind 答问题不答空」的领域半边——集合外的类别答 ErrInvalidCommercialVersion，与
// CanonicalizePublicationContent 对坏 kind 的答复同一格；不答一份空列表冒充「合法但没词」。
func TestUnknownKindIsRefusedNotAnsweredEmpty(t *testing.T) {
	for _, kind := range []domain.CommercialObjectKind{domain.CommercialObjectKindInvalid, domain.CommercialObjectKind(math.MaxUint8)} {
		sets, err := domain.PublicationVocabulary(kind)
		if !errors.Is(err, domain.ErrInvalidCommercialVersion) || sets != nil {
			t.Fatalf("PublicationVocabulary(%d) = %v, %v; want nil, ErrInvalidCommercialVersion", kind, sets, err)
		}
	}
}
