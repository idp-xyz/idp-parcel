package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var deliveredOutcomeAt = time.Date(2026, 8, 10, 15, 0, 0, 0, time.UTC)

func responsibilitySpec(t *testing.T, kind domain.ResponsibilityOutcomeKind) domain.ResponsibilityOutcomeSpec {
	t.Helper()
	return domain.ResponsibilityOutcomeSpec{
		Kind:       kind,
		Parcel:     mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		Decision:   mustValue(t, domain.NewResponsibilityDecisionReference, "DELIVERY-JUDGMENT/TF-11"),
		Execution:  mustValue(t, domain.NewExecutionEvidenceReference, "EFFECTIVE-DELIVERY/TF-11/POD-3"),
		Version:    mustValue(t, domain.NewResponsibilityOutcomeVersion, "delivery-result/v1"),
		OccurredAt: deliveredOutcomeAt,
	}
}

func finalSpec(t *testing.T) domain.ParcelFinalOutcomeSpec {
	t.Helper()
	source, err := domain.NewResponsibilityOutcome(responsibilitySpec(t, domain.EffectiveDeliveryOutcome))
	if err != nil {
		t.Fatalf("new responsibility outcome: %v", err)
	}
	return domain.ParcelFinalOutcomeSpec{
		Version:     mustValue(t, domain.NewFinalOutcomeVersionID, "final-1/v1"),
		Parcel:      mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		Kind:        mustValue(t, domain.NewFinalKindReference, "NETWORK_SERVICE_DELIVERED"),
		Source:      source,
		RuleVersion: mustValue(t, domain.NewFinalRuleVersionReference, "final-rules/v1"),
	}
}

// Covers: `AT-PS-053`「合格有效交付满足网络服务终局规则——形成终局，保留交付来源引用」
// 的领域面——规则版本必备（没有合同规则支撑的终局与「有效交付即所有产品终局」的默认
// 值分不开，红线），生效时间恒等于责任结果的业务时间，终局类型是规则给出的开放引用
// 而不是本域私定的枚举。
func TestAFinalOutcomeDemandsItsRuleVersionAndKeepsItsSource(t *testing.T) {
	final, err := domain.FormParcelFinalOutcome(finalSpec(t))
	if err != nil {
		t.Fatalf("form parcel final outcome: %v", err)
	}
	if final.Source().Execution().String() != "EFFECTIVE-DELIVERY/TF-11/POD-3" {
		t.Fatal("交付来源引用没有随终局保留")
	}
	if !final.EffectiveAt().Equal(deliveredOutcomeAt) {
		t.Fatalf("effective at = %s; 生效时间必须来自责任结果的业务时间", final.EffectiveAt())
	}
	if _, rederived := final.PriorVersion(); rederived {
		t.Fatal("首个版本凭空长出了前版")
	}

	missingRule := finalSpec(t)
	missingRule.RuleVersion = domain.FinalRuleVersionReference{}
	if _, err := domain.FormParcelFinalOutcome(missingRule); !errors.Is(err, domain.ErrInvalidParcelFinalOutcome) {
		t.Fatalf("err = %v; 没有规则版本的终局被收下了", err)
	}

	wrongParcel := finalSpec(t)
	wrongParcel.Parcel = mustValue(t, domain.NewDeclaredParcelID, "parcel-9")
	if _, err := domain.FormParcelFinalOutcome(wrongParcel); !errors.Is(err, domain.ErrInvalidParcelFinalOutcome) {
		t.Fatalf("err = %v; 终局对象与来源对象不一致被收下了", err)
	}
}

// Covers: `AT-PS-091`「关务已接收销毁决定，但节点尚无执行事实——不形成执行完成或包裹
// 终局」与 `AT-PS-057/058` 的类型面——决定与执行双引用缺一立不起责任结果（「决定不
// 等于执行完成」构造期落地）；班次完成、POD 文件、异常案件在封闭四值外没有格可落
// （`AT-PS-055/056/093` 的结果联合面）。
func TestAResponsibilityOutcomeDemandsBothDecisionAndExecution(t *testing.T) {
	missingExecution := responsibilitySpec(t, domain.RegulatoryDispositionExecuted)
	missingExecution.Execution = domain.ExecutionEvidenceReference{}
	if _, err := domain.NewResponsibilityOutcome(missingExecution); !errors.Is(err, domain.ErrInvalidResponsibilityOutcome) {
		t.Fatalf("err = %v; 只有决定没有执行的结果被收下了", err)
	}

	missingDecision := responsibilitySpec(t, domain.ReturnCompletedOutcome)
	missingDecision.Decision = domain.ResponsibilityDecisionReference{}
	if _, err := domain.NewResponsibilityOutcome(missingDecision); !errors.Is(err, domain.ErrInvalidResponsibilityOutcome) {
		t.Fatalf("err = %v; 只有执行没有决定的结果被收下了", err)
	}

	if _, err := domain.NewResponsibilityOutcome(
		responsibilitySpec(t, domain.ResponsibilityOutcomeKindInvalid)); !errors.Is(err, domain.ErrInvalidResponsibilityOutcome) {
		t.Fatalf("err = %v; 封闭四值之外立起了责任结果", err)
	}
}

// Covers: `AT-PS-063`「原有效交付因 POD 错误被失效——保留原终局历史，形成新判断版本
// 并重新派生当前结果」——重派生换版本、带原因、换新责任结果、指回原版；原终局不可变；
// 重号或跨包裹的重派生立不成。
func TestRederivationFormsANewVersionWithoutErasingHistory(t *testing.T) {
	final, err := domain.FormParcelFinalOutcome(finalSpec(t))
	if err != nil {
		t.Fatalf("form parcel final outcome: %v", err)
	}

	corrected := responsibilitySpec(t, domain.EffectiveDeliveryOutcome)
	corrected.Version = mustValue(t, domain.NewResponsibilityOutcomeVersion, "delivery-result/v2")
	corrected.OccurredAt = deliveredOutcomeAt.Add(time.Hour)
	correctedSource, err := domain.NewResponsibilityOutcome(corrected)
	if err != nil {
		t.Fatalf("new corrected outcome: %v", err)
	}

	rederived, err := final.Rederive(
		mustValue(t, domain.NewFinalOutcomeVersionID, "final-1/v2"),
		correctedSource,
		mustValue(t, domain.NewRederivationReason, "POD_INVALIDATED/pod-3"),
	)
	if err != nil {
		t.Fatalf("rederive: %v", err)
	}
	prior, present := rederived.PriorVersion()
	if !present || prior.String() != "final-1/v1" {
		t.Fatalf("prior = %s present = %v; 重派生必须指回原版", prior, present)
	}
	if !rederived.EffectiveAt().Equal(deliveredOutcomeAt.Add(time.Hour)) {
		t.Fatal("重派生没有换用新责任结果的业务时间")
	}
	if final.Version().String() != "final-1/v1" {
		t.Fatal("原终局被改写了")
	}
	if _, wasRederived := final.PriorVersion(); wasRederived {
		t.Fatal("原版本凭空长出了前版")
	}

	if _, err := final.Rederive(final.Version(), correctedSource,
		mustValue(t, domain.NewRederivationReason, "POD_INVALIDATED")); !errors.Is(err, domain.ErrInvalidParcelFinalOutcome) {
		t.Fatalf("err = %v; 重号的重派生分不出两版", err)
	}

	foreign := responsibilitySpec(t, domain.EffectiveDeliveryOutcome)
	foreign.Parcel = mustValue(t, domain.NewDeclaredParcelID, "parcel-9")
	foreignSource, err := domain.NewResponsibilityOutcome(foreign)
	if err != nil {
		t.Fatalf("new foreign outcome: %v", err)
	}
	if _, err := final.Rederive(
		mustValue(t, domain.NewFinalOutcomeVersionID, "final-1/v3"),
		foreignSource,
		mustValue(t, domain.NewRederivationReason, "POD_INVALIDATED"),
	); !errors.Is(err, domain.ErrInvalidParcelFinalOutcome) {
		t.Fatalf("err = %v; 跨包裹的重派生被收下了", err)
	}
}

// Covers: `AT-PS-054`「一个委托三个包裹，只有两个终局——两个包裹终局有效，委托派生
// 部分完成」——完成摘要是逐包裹结果的派生（取消终局与履约终局同格进汇总），单个成员
// 未决不回滚其他终局；成员重复或空集合立不出摘要。
func TestCompletionIsDerivedPerParcelNotEdited(t *testing.T) {
	parcel := func(id string) domain.DeclaredParcelID {
		return mustValue(t, domain.NewDeclaredParcelID, id)
	}

	partial, err := domain.DeriveShipmentCompletion([]domain.MemberFinalState{
		{Parcel: parcel("parcel-1"), Finalized: true},
		{Parcel: parcel("parcel-2"), Finalized: true},
		{Parcel: parcel("parcel-3"), Finalized: false},
	})
	if err != nil {
		t.Fatalf("derive partial: %v", err)
	}
	if partial.State() != domain.CompletionPartial || partial.Finalized() != 2 || partial.Total() != 3 {
		t.Fatalf("summary = %s %d/%d, want PARTIAL 2/3", partial.State(), partial.Finalized(), partial.Total())
	}

	complete, err := domain.DeriveShipmentCompletion([]domain.MemberFinalState{
		{Parcel: parcel("parcel-1"), Finalized: true},
	})
	if err != nil {
		t.Fatalf("derive complete: %v", err)
	}
	if complete.State() != domain.CompletionComplete {
		t.Fatalf("state = %s, want COMPLETE", complete.State())
	}

	if _, err := domain.DeriveShipmentCompletion(nil); !errors.Is(err, domain.ErrInvalidCompletionInput) {
		t.Fatalf("err = %v; 空集合立出了摘要", err)
	}
	if _, err := domain.DeriveShipmentCompletion([]domain.MemberFinalState{
		{Parcel: parcel("parcel-1"), Finalized: true},
		{Parcel: parcel("parcel-1"), Finalized: false},
	}); !errors.Is(err, domain.ErrInvalidCompletionInput) {
		t.Fatalf("err = %v; 重复成员被收下了——同一包裹两份答案分不出真假", err)
	}
	if _, err := domain.DeriveShipmentCompletion([]domain.MemberFinalState{
		{Parcel: parcel("parcel-1"), Cancelled: true},
	}); !errors.Is(err, domain.ErrInvalidCompletionInput) {
		t.Fatalf("err = %v; 标了取消却没标终局的矛盾输入被收下了", err)
	}
}

// Covers: UC-PS-006 一致性硬句「只有全部当前有效包裹均为取消终局且不存在非取消终局时，
// 委托才派生为`已取消`」——全员取消派生已取消；一件交付加一件取消是全部完成而不是
// 已取消（那件交付责任真实发生过）；有取消但未全终局什么都不派生。
func TestShipmentCancelledDerivesOnlyFromAllCancelledMembers(t *testing.T) {
	parcel := func(id string) domain.DeclaredParcelID {
		return mustValue(t, domain.NewDeclaredParcelID, id)
	}

	allCancelled, err := domain.DeriveShipmentCompletion([]domain.MemberFinalState{
		{Parcel: parcel("parcel-1"), Finalized: true, Cancelled: true},
		{Parcel: parcel("parcel-2"), Finalized: true, Cancelled: true},
	})
	if err != nil {
		t.Fatalf("derive all cancelled: %v", err)
	}
	if !allCancelled.DerivesShipmentCancelled() || allCancelled.State() != domain.CompletionComplete {
		t.Fatalf("summary = %s cancelled = %v", allCancelled.State(), allCancelled.DerivesShipmentCancelled())
	}

	mixed, err := domain.DeriveShipmentCompletion([]domain.MemberFinalState{
		{Parcel: parcel("parcel-1"), Finalized: true, Cancelled: true},
		{Parcel: parcel("parcel-2"), Finalized: true},
	})
	if err != nil {
		t.Fatalf("derive mixed: %v", err)
	}
	if mixed.DerivesShipmentCancelled() {
		t.Fatal("存在非取消终局的委托被派生成已取消——那件交付责任真实发生过")
	}
	if mixed.State() != domain.CompletionComplete {
		t.Fatalf("state = %s; 混合终局仍是全部完成", mixed.State())
	}

	partial, err := domain.DeriveShipmentCompletion([]domain.MemberFinalState{
		{Parcel: parcel("parcel-1"), Finalized: true, Cancelled: true},
		{Parcel: parcel("parcel-2")},
	})
	if err != nil {
		t.Fatalf("derive partial: %v", err)
	}
	if partial.DerivesShipmentCancelled() || partial.State() != domain.CompletionPartial {
		t.Fatalf("summary = %s cancelled = %v; 未全终局不派生已取消", partial.State(), partial.DerivesShipmentCancelled())
	}
}

// 反解析与 String() 必须互为逆：跨上下文消费方按信封里的字符串重建采用键，形式在
// 本包定义，反解析就得在本包给出——散一份拷贝到消费方，日后加一个责任结果种类时
// 编译器一处都不会提醒。
func TestResponsibilityOutcomeKindRoundTripsThroughItsString(t *testing.T) {
	for _, kind := range []domain.ResponsibilityOutcomeKind{
		domain.EffectiveDeliveryOutcome,
		domain.ReturnCompletedOutcome,
		domain.ServiceTerminatedOutcome,
		domain.RegulatoryDispositionExecuted,
	} {
		t.Run(kind.String(), func(t *testing.T) {
			got, err := domain.NewResponsibilityOutcomeKind(kind.String())
			if err != nil {
				t.Fatalf("反解析 %q：%v", kind.String(), err)
			}
			if got != kind {
				t.Fatalf("反解析 %q = %v, want %v", kind.String(), got, kind)
			}
		})
	}
}

func TestAnUnknownResponsibilityOutcomeKindIsRefused(t *testing.T) {
	for name, raw := range map[string]string{
		"空串":         "",
		"零值的 String": domain.ResponsibilityOutcomeKindInvalid.String(),
		"取消不是责任结果":   "PARCEL_CANCELLED",
		"大小写不宽容":     "effective_delivery",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := domain.NewResponsibilityOutcomeKind(raw)
			if err == nil {
				t.Fatalf("认不得的种类必须报错，却交回 %v", got)
			}
			if got != domain.ResponsibilityOutcomeKindInvalid {
				t.Fatalf("失败时必须交回零值，却是 %v", got)
			}
		})
	}
}
