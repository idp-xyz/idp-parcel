package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func amendmentRule(t *testing.T, group string, stage domain.DeclaredAmendmentStage, intent domain.DeclaredAmendmentIntent, allowance domain.AmendmentAllowance) domain.SourceDataAmendmentRule {
	t.Helper()
	return domain.SourceDataAmendmentRule{
		DataGroup: commercialValue(t, domain.NewSourceDataGroupReference, group),
		Stage:     stage,
		Intent:    intent,
		Allowance: allowance,
	}
}

// Covers: ADR-0120 Decision 五——阶段六格与意图三格取 parcel-shipment 原词只镜像不另定；这里按字面钉住
// PC 这一侧，跨侧逐字相等由 PS 消费适配器旁的测试钉（ps-port-remainder/02）。允许性两值 + 零值「未声明」
// 的名字一并钉住：批文与库上 CHECK 都按这些字面说话。
func TestAmendmentClosedSetsMirrorParcelShipmentWordsVerbatim(t *testing.T) {
	stages := map[domain.DeclaredAmendmentStage]string{
		domain.DeclaredAcceptedNotYetReceived:         "ACCEPTED_NOT_YET_RECEIVED",
		domain.DeclaredReceivedOrMeasured:             "RECEIVED_OR_MEASURED",
		domain.DeclaredLabelledOrBagged:               "LABELLED_OR_BAGGED",
		domain.DeclaredCustomsDataFormingNotSubmitted: "CUSTOMS_DATA_FORMING_NOT_SUBMITTED",
		domain.DeclaredCustomsSubmitted:               "CUSTOMS_SUBMITTED",
		domain.DeclaredCaseClosedOrServiceCompleted:   "CASE_CLOSED_OR_SERVICE_COMPLETED",
	}
	for stage, want := range stages {
		if got := stage.String(); got != want {
			t.Fatalf("stage %d = %q, want %q", stage, got, want)
		}
	}
	if domain.DeclaredAmendmentStageInvalid.String() != "" || domain.DeclaredCaseClosedOrServiceCompleted+1 != 7 {
		t.Fatal("阶段封闭集不是恰好六格加一个零值")
	}

	intents := map[domain.DeclaredAmendmentIntent]string{
		domain.DeclaredSupplementIntent:    "SUPPLEMENT",
		domain.DeclaredCorrectionIntent:    "CORRECTION",
		domain.DeclaredExplicitClearIntent: "EXPLICIT_CLEAR",
	}
	for intent, want := range intents {
		if got := intent.String(); got != want {
			t.Fatalf("intent %d = %q, want %q", intent, got, want)
		}
	}
	if domain.DeclaredAmendmentIntentInvalid.String() != "" || domain.DeclaredExplicitClearIntent+1 != 4 {
		t.Fatal("意图封闭集不是恰好三格加一个零值")
	}

	allowances := map[domain.AmendmentAllowance]string{
		domain.AmendmentAllowanceNotDeclared: "NOT_DECLARED",
		domain.AmendmentAllowed:              "ALLOWED",
		domain.AmendmentDisallowed:           "DISALLOWED",
	}
	for allowance, want := range allowances {
		if got := allowance.String(); got != want {
			t.Fatalf("allowance %d = %q, want %q", allowance, got, want)
		}
	}
	var zero domain.AmendmentAllowance
	if zero != domain.AmendmentAllowanceNotDeclared {
		t.Fatal("允许性的零值不是「未声明」——零值必须落在最保守的那一格")
	}
}

// Covers: ADR-0120 Decision 一、三——拥有者必须是已生效接单规则包；未封闭零格是缺件、封闭零格是一句显式
// 的话；每一格的四样都要立得住，「未声明」登成一格是缺件；同一格两行是冲突，取值相同也是。
func TestSourceDataAmendmentAllowanceContentConstructionGate(t *testing.T) {
	owner := rulePackage(t)
	allowed := amendmentRule(t, "consignee.address", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentAllowed)

	t.Run("拥有者不是已生效接单规则包", func(t *testing.T) {
		if _, err := domain.NewSourceDataAmendmentAllowanceContent(effectiveServiceProduct(t), false, []domain.SourceDataAmendmentRule{allowed}); !errors.Is(err, domain.ErrUnusableRulePackage) {
			t.Fatalf("error = %v, want ErrUnusableRulePackage", err)
		}
	})

	t.Run("未封闭零格是缺件", func(t *testing.T) {
		if _, err := domain.NewSourceDataAmendmentAllowanceContent(owner, false, nil); !errors.Is(err, domain.ErrSourceDataAmendmentNotConfigured) {
			t.Fatalf("error = %v, want ErrSourceDataAmendmentNotConfigured", err)
		}
	})

	t.Run("封闭零格是一句显式的话", func(t *testing.T) {
		content, err := domain.NewSourceDataAmendmentAllowanceContent(owner, true, nil)
		if err != nil {
			t.Fatalf("封闭零格被拒了：%v", err)
		}
		if !content.Closed() || len(content.Rules()) != 0 || !content.Owner().SameVersionAs(owner) {
			t.Fatalf("声明变形：closed=%v rules=%d", content.Closed(), len(content.Rules()))
		}
	})

	for name, rule := range map[string]domain.SourceDataAmendmentRule{
		"资料组为空":   {Stage: domain.DeclaredAcceptedNotYetReceived, Intent: domain.DeclaredCorrectionIntent, Allowance: domain.AmendmentAllowed},
		"阶段集外":    amendmentRule(t, "consignee.address", domain.DeclaredAmendmentStageInvalid, domain.DeclaredCorrectionIntent, domain.AmendmentAllowed),
		"阶段越界":    amendmentRule(t, "consignee.address", domain.DeclaredCaseClosedOrServiceCompleted+1, domain.DeclaredCorrectionIntent, domain.AmendmentAllowed),
		"意图集外":    amendmentRule(t, "consignee.address", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredAmendmentIntentInvalid, domain.AmendmentAllowed),
		"未声明登成一格": amendmentRule(t, "consignee.address", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentAllowanceNotDeclared),
		"允许性越界":   amendmentRule(t, "consignee.address", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentDisallowed+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewSourceDataAmendmentAllowanceContent(owner, false, []domain.SourceDataAmendmentRule{rule}); !errors.Is(err, domain.ErrSourceDataAmendmentNotConfigured) {
				t.Fatalf("error = %v, want ErrSourceDataAmendmentNotConfigured", err)
			}
			// 封闭也救不了一格坏声明：closed 只管缺格的读法，不管登进来的格立不立得住。
			if _, err := domain.NewSourceDataAmendmentAllowanceContent(owner, true, []domain.SourceDataAmendmentRule{rule}); !errors.Is(err, domain.ErrSourceDataAmendmentNotConfigured) {
				t.Fatalf("closed=true error = %v, want ErrSourceDataAmendmentNotConfigured", err)
			}
		})
	}

	t.Run("同一格两行是冲突", func(t *testing.T) {
		same := amendmentRule(t, "consignee.address", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentAllowed)
		if _, err := domain.NewSourceDataAmendmentAllowanceContent(owner, false, []domain.SourceDataAmendmentRule{allowed, same}); !errors.Is(err, domain.ErrConflictingSourceDataAmendment) {
			t.Fatalf("同值同格 error = %v, want ErrConflictingSourceDataAmendment", err)
		}
		flipped := amendmentRule(t, "consignee.address", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentDisallowed)
		if _, err := domain.NewSourceDataAmendmentAllowanceContent(owner, false, []domain.SourceDataAmendmentRule{allowed, flipped}); !errors.Is(err, domain.ErrConflictingSourceDataAmendment) {
			t.Fatalf("异值同格 error = %v, want ErrConflictingSourceDataAmendment", err)
		}
	})

	t.Run("换一维就是另一格，不冲突", func(t *testing.T) {
		otherIntent := amendmentRule(t, "consignee.address", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredExplicitClearIntent, domain.AmendmentDisallowed)
		otherStage := amendmentRule(t, "consignee.address", domain.DeclaredCustomsSubmitted, domain.DeclaredCorrectionIntent, domain.AmendmentDisallowed)
		otherGroup := amendmentRule(t, "parcel.weight", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentAllowed)
		content, err := domain.NewSourceDataAmendmentAllowanceContent(owner, false, []domain.SourceDataAmendmentRule{allowed, otherIntent, otherStage, otherGroup})
		if err != nil {
			t.Fatalf("四格各不同键却被拒：%v", err)
		}
		if got := len(content.Rules()); got != 4 {
			t.Fatalf("格数 = %d, want 4", got)
		}
	})
}

// Covers: ADR-0120 Decision 三、四——三值在本上下文算出：有格按格答；缺格未封闭「未声明」、封闭「不允许」；
// 封闭下显式登的 DISALLOWED 是冗余不是冲突；资料组为空或阶段 / 意图不在集内一律「未声明」、不看封闭。
func TestAllowanceForAnswersDeclaredCellsAndReadsGapsByTheClosedMarker(t *testing.T) {
	owner := rulePackage(t)
	address := commercialValue(t, domain.NewSourceDataGroupReference, "consignee.address")
	weight := commercialValue(t, domain.NewSourceDataGroupReference, "parcel.weight")
	rules := []domain.SourceDataAmendmentRule{
		amendmentRule(t, "consignee.address", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentAllowed),
		amendmentRule(t, "consignee.address", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredExplicitClearIntent, domain.AmendmentDisallowed),
		amendmentRule(t, "consignee.address", domain.DeclaredCustomsSubmitted, domain.DeclaredCorrectionIntent, domain.AmendmentDisallowed),
	}

	for name, tc := range map[string]struct {
		closed bool
		group  domain.SourceDataGroupReference
		stage  domain.DeclaredAmendmentStage
		intent domain.DeclaredAmendmentIntent
		want   domain.AmendmentAllowance
	}{
		"有格按格答·允许":         {false, address, domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentAllowed},
		"有格按格答·同阶段换意图即另一格": {false, address, domain.DeclaredAcceptedNotYetReceived, domain.DeclaredExplicitClearIntent, domain.AmendmentDisallowed},
		"有格按格答·同意图换阶段即另一格": {false, address, domain.DeclaredCustomsSubmitted, domain.DeclaredCorrectionIntent, domain.AmendmentDisallowed},
		"缺格未封闭是未声明":        {false, weight, domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentAllowanceNotDeclared},
		"缺格封闭是不允许":         {true, weight, domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentDisallowed},
		"封闭不改已登的格":         {true, address, domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentAllowed},
		"封闭下资料组为空仍是未声明":    {true, domain.SourceDataGroupReference{}, domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentAllowanceNotDeclared},
		"封闭下阶段判不出仍是未声明":    {true, address, domain.DeclaredAmendmentStageInvalid, domain.DeclaredCorrectionIntent, domain.AmendmentAllowanceNotDeclared},
		"封闭下意图集外仍是未声明":     {true, address, domain.DeclaredAcceptedNotYetReceived, domain.DeclaredAmendmentIntentInvalid, domain.AmendmentAllowanceNotDeclared},
	} {
		t.Run(name, func(t *testing.T) {
			content, err := domain.NewSourceDataAmendmentAllowanceContent(owner, tc.closed, rules)
			if err != nil {
				t.Fatalf("组声明：%v", err)
			}
			if got := content.AllowanceFor(tc.group, tc.stage, tc.intent); got != tc.want {
				t.Fatalf("AllowanceFor = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("封闭下显式登 DISALLOWED 是冗余不是冲突", func(t *testing.T) {
		content, err := domain.NewSourceDataAmendmentAllowanceContent(owner, true, []domain.SourceDataAmendmentRule{
			amendmentRule(t, "parcel.weight", domain.DeclaredReceivedOrMeasured, domain.DeclaredSupplementIntent, domain.AmendmentDisallowed),
		})
		if err != nil {
			t.Fatalf("显式重复一遍「不许」被拒了：%v", err)
		}
		if got := content.AllowanceFor(weight, domain.DeclaredReceivedOrMeasured, domain.DeclaredSupplementIntent); got != domain.AmendmentDisallowed {
			t.Fatalf("AllowanceFor = %v, want DISALLOWED", got)
		}
	})
}

// Covers: Rules 按（资料组, 阶段, 意图）稳定排序且交回副本——发布写入面按整份登记，顺序要稳；改副本不动声明。
func TestSourceDataAmendmentRulesAreStablyOrderedCopies(t *testing.T) {
	owner := rulePackage(t)
	content, err := domain.NewSourceDataAmendmentAllowanceContent(owner, false, []domain.SourceDataAmendmentRule{
		amendmentRule(t, "parcel.weight", domain.DeclaredReceivedOrMeasured, domain.DeclaredSupplementIntent, domain.AmendmentAllowed),
		amendmentRule(t, "consignee.address", domain.DeclaredCustomsSubmitted, domain.DeclaredCorrectionIntent, domain.AmendmentDisallowed),
		amendmentRule(t, "consignee.address", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredExplicitClearIntent, domain.AmendmentDisallowed),
		amendmentRule(t, "consignee.address", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentAllowed),
	})
	if err != nil {
		t.Fatalf("组声明：%v", err)
	}
	rules := content.Rules()
	want := []string{
		"consignee.address/ACCEPTED_NOT_YET_RECEIVED/CORRECTION",
		"consignee.address/ACCEPTED_NOT_YET_RECEIVED/EXPLICIT_CLEAR",
		"consignee.address/CUSTOMS_SUBMITTED/CORRECTION",
		"parcel.weight/RECEIVED_OR_MEASURED/SUPPLEMENT",
	}
	if len(rules) != len(want) {
		t.Fatalf("格数 = %d, want %d", len(rules), len(want))
	}
	for index, rule := range rules {
		got := rule.DataGroup.String() + "/" + rule.Stage.String() + "/" + rule.Intent.String()
		if got != want[index] {
			t.Fatalf("rules[%d] = %s, want %s", index, got, want[index])
		}
	}
	rules[0].Allowance = domain.AmendmentDisallowed
	if got := content.AllowanceFor(rules[0].DataGroup, rules[0].Stage, rules[0].Intent); got != domain.AmendmentAllowed {
		t.Fatal("改了 Rules 交回的副本，声明本身跟着变了")
	}
}
