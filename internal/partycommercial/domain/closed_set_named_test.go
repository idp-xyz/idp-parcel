package domain_test

import (
	"fmt"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// closedSetNamedCase 是表里的一行：一个封闭集的反查怎么往返、拒什么。成员列表是那一集此刻的名单快照——名单本身只在
// String() 一处，这里列它是为了让「每个合法码都能往返」有东西可数；集外词各行自己钉，钉的是各票裁过的那几个词。
type closedSetNamedCase struct {
	set       string
	roundTrip func(t *testing.T)
	refuses   func(t *testing.T, name string)
	outside   []string
}

// closedSetNamed 把一集的反查与成员折成一行断言：往返 Named(String(c)) == c；集外与空串答 (零值, false)。落空时交出的
// 必须是零值而不是名单里的第一个——调用方拿到 false 之后不该还能从第一个返回值里读出一格取值。零值本身是合法成员的集
// （PlanBindingConversion 的 NONE）也在这条判据之内：落空交零值与「NONE 是一格」并不冲突，分辨靠的是第二个返回值。
func closedSetNamed[Code interface {
	~uint8
	fmt.Stringer
}](set string, named func(string) (Code, bool), members []Code, outside ...string) closedSetNamedCase {
	return closedSetNamedCase{
		set: set,
		roundTrip: func(t *testing.T) {
			t.Helper()
			for _, member := range members {
				if got, known := named(member.String()); !known || got != member {
					t.Fatalf("%s: Named(%q) = (%d, %v), want (%d, true)", set, member.String(), got, known, member)
				}
			}
		},
		refuses: func(t *testing.T, name string) {
			t.Helper()
			if got, known := named(name); known || got != 0 {
				t.Fatalf("%s: Named(%q) = (%d, %v), want (0, false)", set, name, got, known)
			}
		},
		outside: outside,
	}
}

// Covers: 各册封闭集的 *Named 反查与 String() 是同一份名单——每个合法码往返、空串与集外答 (零值, false)。集外词里钉着
// 各票各自裁过的：13 的 NO_CONTROL（「无控制」由客户合同声明、不是一种控制——ADR-0115 Decision 一）与共同通过条件的
// 空串（缺席不折成 ALL_CONTROLS_PASS——Decision 三）；15 的 CUSTOMER_DEFAULT（第三个结算取值「客户级默认」是本上下文
// 明禁的）；12 的 UNDECLARED / NOT_DECLARED（缺格的读法不是一格能写的取值——ADR-0120 Decision 三）；14 的空转换与空税务
// 口径（不代填 NONE、不落进不适用）。一张表盖全部集，各集的反查合一到 closedCodeNamed 之后语义就靠这张表钉
// （票 admin-write-faces/24）。
func TestClosedSetNamedLookupsRoundTripAndRefuseOutsiders(t *testing.T) {
	cases := []closedSetNamedCase{
		closedSetNamed("CommercialObjectKind", domain.CommercialObjectKindNamed,
			[]domain.CommercialObjectKind{
				domain.ServiceProductObject, domain.CustomerContractObject, domain.SupplierAgreementObject,
				domain.AcceptanceRulePackageObject, domain.PreAcceptanceFinancialControlPolicyObject, domain.PriceRuleObject,
				domain.SettlementPolicyObject, domain.CreditPolicyObject, domain.AuthorizationRuleObject,
				domain.CustomerServiceRuleObject,
			},
			"", "service_product", "SHIPPER_ACCOUNT", "SERVICE_PRODUCT "),
		closedSetNamed("RuleCategory", domain.RuleCategoryNamed,
			[]domain.RuleCategory{
				domain.MinimumIngressIdentityRules, domain.ShipmentInvariantRules, domain.ProductAndContractDocumentRules,
				domain.RegulatorySourceDocumentRules, domain.CrossFieldConditionRules,
			},
			"", "minimum_ingress_identity"),
		closedSetNamed("JudgmentType", domain.JudgmentTypeNamed,
			[]domain.JudgmentType{domain.NetworkReachabilityJudgment, domain.PreAcceptanceFinancialControlJudgment},
			"", "network_reachability"),
		closedSetNamed("AcceptanceCheckGroupType", domain.AcceptanceCheckGroupTypeNamed,
			[]domain.AcceptanceCheckGroupType{domain.CustomerRelationshipCheckGroup, domain.NetworkReachabilityCheckGroup},
			"", "customer_relationship"),
		closedSetNamed("ManualReviewDirective", domain.ManualReviewDirectiveNamed,
			[]domain.ManualReviewDirective{domain.ManualReviewNotRequired, domain.ManualReviewRequired},
			"", "UNDECLARED", "required"),
		closedSetNamed("DeclaredIntakeSource", domain.DeclaredIntakeSourceNamed,
			[]domain.DeclaredIntakeSource{domain.DeclaredNodeIntake, domain.DeclaredOffsitePickup},
			"", "node_intake"),
		// 集外词里 LABEL_CHANNEL_FINAL 是 pc-gaps/12 票面上被裁掉的占位（渠道是通道，终局是服务的），
		// LABEL_SERVICE_OUTCOME 是 parcel-shipment 那一侧的原词——两边同一个词根、不是同一个词，翻译在 PS 适配器。
		closedSetNamed("DeclaredResponsibilityOutcome", domain.DeclaredResponsibilityOutcomeNamed,
			[]domain.DeclaredResponsibilityOutcome{
				domain.DeclaredEffectiveDelivery, domain.DeclaredRegulatoryDisposition,
				domain.DeclaredLabelServiceCompleted, domain.DeclaredLabelServiceFailed,
			},
			"", "effective_delivery", "LABEL_CHANNEL_FINAL", "LABEL_SERVICE_OUTCOME", "label_service_completed"),
		closedSetNamed("ValidityAnchorKind", domain.ValidityAnchorKindNamed,
			[]domain.ValidityAnchorKind{domain.ChannelResultObservedAnchor},
			"", "LABEL_ISSUED"),
		closedSetNamed("DeclaredAmendmentStage", domain.DeclaredAmendmentStageNamed,
			[]domain.DeclaredAmendmentStage{domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCaseClosedOrServiceCompleted},
			"", "accepted_not_yet_received"),
		closedSetNamed("DeclaredAmendmentIntent", domain.DeclaredAmendmentIntentNamed,
			[]domain.DeclaredAmendmentIntent{domain.DeclaredSupplementIntent, domain.DeclaredExplicitClearIntent},
			"", "supplement"),
		closedSetNamed("AmendmentAllowance", domain.AmendmentAllowanceNamed,
			[]domain.AmendmentAllowance{domain.AmendmentAllowed, domain.AmendmentDisallowed},
			"", "NOT_DECLARED", "allowed"),
		closedSetNamed("PreAcceptanceControlKind", domain.PreAcceptanceControlKindNamed,
			[]domain.PreAcceptanceControlKind{domain.PrepaidFreezeControl, domain.CreditCheckControl},
			"", "NO_CONTROL", "prepaid_freeze", "CREDIT"),
		closedSetNamed("ControlFailureDisposition", domain.ControlFailureDispositionNamed,
			[]domain.ControlFailureDisposition{domain.RejectOnControlFailure, domain.AuthorizedDispositionOnControlFailure},
			"", "ALLOW", "reject"),
		closedSetNamed("JointPassCondition", domain.JointPassConditionNamed,
			[]domain.JointPassCondition{domain.AllControlsPass},
			"", "ANY_CONTROL_PASSES", "all_controls_pass"),
		closedSetNamed("PreAcceptanceControlRequirement", domain.PreAcceptanceControlRequirementNamed,
			[]domain.PreAcceptanceControlRequirement{domain.PreAcceptanceControlRequired, domain.PreAcceptanceControlNotApplicable},
			"", "UNDECLARED", "NO_CONTROL", "required"),
		closedSetNamed("SettlementMethod", domain.SettlementMethodNamed,
			[]domain.SettlementMethod{domain.PrepaidMethod, domain.TermsMethod},
			"", "CUSTOMER_DEFAULT", "CASH", "prepaid", "PREPAID "),
		closedSetNamed("PriceDirection", domain.PriceDirectionNamed,
			[]domain.PriceDirection{domain.BuyDirection, domain.SellDirection, domain.InternalDirection},
			"", "sell", "BOTH"),
		closedSetNamed("PlanBindingConversion", domain.PlanBindingConversionNamed,
			[]domain.PlanBindingConversion{domain.PlanBindingConversionNone, domain.PlanBindingFrozenBuyEvaluation},
			"", "none", "FROZEN"),
		closedSetNamed("TaxDisposition", domain.TaxDispositionNamed,
			[]domain.TaxDisposition{domain.TaxInclusive, domain.TaxExclusive, domain.TaxNotApplicable},
			"", "TAX_MAYBE", "tax_inclusive"),
		closedSetNamed("DeclaredCancellationParty", domain.DeclaredCancellationPartyNamed,
			[]domain.DeclaredCancellationParty{domain.DeclaredCustomerCancellation, domain.DeclaredOperationsCancellation},
			"", "customer", "CARRIER"),
	}
	for _, testCase := range cases {
		t.Run(testCase.set, func(t *testing.T) {
			testCase.roundTrip(t)
			for _, name := range testCase.outside {
				testCase.refuses(t, name)
			}
		})
	}
}
