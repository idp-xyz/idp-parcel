package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var predictedAt = time.Date(2026, 8, 11, 18, 0, 0, 0, time.UTC)

func etaSpec(t *testing.T, version string) domain.ETAPredictionSpec {
	t.Helper()
	return domain.ETAPredictionSpec{
		Version:     mustValue(t, domain.NewETAVersionID, version),
		Parcel:      mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Milestone:   mustValue(t, domain.NewMilestoneReference, "DELIVERED"),
		Source:      domain.OperatorDerivedETA,
		Inputs:      mustValue(t, domain.NewPredictionInputsReference, "inputs/route-plan+movement"),
		Model:       mustValue(t, domain.NewPredictionModelReference, "eta-model/v4"),
		RangeFrom:   predictedAt.Add(30 * time.Hour),
		RangeTo:     predictedAt.Add(42 * time.Hour),
		Confidence:  mustValue(t, domain.NewConfidenceReference, "MEDIUM/history-based"),
		PredictedAt: predictedAt,
	}
}

// Covers: VE CONTEXT「必须明确预测对象、目标里程碑、预测时点」「不得以计划时间或客户承诺填充」
// ——各件缺一立不起（缺模型版本即拒，没有可填充的旁路）；
// 区间倒置拒；来源口径封闭二值。
func TestAnETADemandsItsFullShape(t *testing.T) {
	eta, err := domain.FormETAPrediction(etaSpec(t, "eta-1/v1"))
	if err != nil {
		t.Fatalf("form ETA prediction: %v", err)
	}
	from, to := eta.Range()
	if !to.After(from) || eta.Source() != domain.OperatorDerivedETA {
		t.Fatalf("eta = %#v", eta)
	}

	missingModel := etaSpec(t, "eta-1/v1")
	missingModel.Model = domain.PredictionModelReference{}
	if _, err := domain.FormETAPrediction(missingModel); !errors.Is(err, domain.ErrInvalidETA) {
		t.Fatalf("err = %v; 缺模型版本的预测被收下了——信息不足就不形成，没有填充旁路", err)
	}

	inverted := etaSpec(t, "eta-1/v1")
	inverted.RangeTo = inverted.RangeFrom.Add(-time.Hour)
	if _, err := domain.FormETAPrediction(inverted); !errors.Is(err, domain.ErrInvalidETA) {
		t.Fatalf("err = %v; 倒置区间被收下了", err)
	}
}

// Covers: AT-VE-222 与 VE CONTEXT「新的 ETA 形成新版本，不覆盖历史预测」——刷新换版本
// 指回原版、原预测不可变；跨包裹、跨里程碑或跨来源的刷新不是同一条预测线。
func TestARefreshedETAKeepsItsHistory(t *testing.T) {
	first, err := domain.FormETAPrediction(etaSpec(t, "eta-1/v1"))
	if err != nil {
		t.Fatalf("form first ETA: %v", err)
	}

	refreshedSpec := etaSpec(t, "eta-1/v2")
	refreshedSpec.RangeFrom = predictedAt.Add(36 * time.Hour)
	refreshedSpec.RangeTo = predictedAt.Add(48 * time.Hour)
	refreshedSpec.PredictedAt = predictedAt.Add(6 * time.Hour)
	refreshed, err := first.Refresh(refreshedSpec)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	prior, present := refreshed.PriorVersion()
	if !present || prior.String() != "eta-1/v1" {
		t.Fatalf("prior = %s present = %v", prior, present)
	}
	firstFrom, _ := first.Range()
	if !firstFrom.Equal(predictedAt.Add(30 * time.Hour)) {
		t.Fatal("历史预测被覆盖了")
	}

	foreign := etaSpec(t, "eta-1/v3")
	foreign.Milestone = mustValue(t, domain.NewMilestoneReference, "PICKED_UP")
	if _, err := first.Refresh(foreign); !errors.Is(err, domain.ErrInvalidETA) {
		t.Fatalf("err = %v; 跨里程碑的刷新不是同一条预测线", err)
	}

	otherSource := etaSpec(t, "eta-1/v4")
	otherSource.Source = domain.CarrierProvidedETA
	if _, err := first.Refresh(otherSource); !errors.Is(err, domain.ErrInvalidETA) {
		t.Fatalf("err = %v; 换来源的刷新不是同一条预测线，两路来源各是一条线", err)
	}
}

func carrierETASpec(t *testing.T, version string) domain.ETAPredictionSpec {
	t.Helper()
	spec := etaSpec(t, version)
	spec.Source = domain.CarrierProvidedETA
	spec.Inputs = mustValue(t, domain.NewPredictionInputsReference, "inputs/carrier-feed")
	spec.Model = mustValue(t, domain.NewPredictionModelReference, "carrier-estimate/v1")
	spec.Confidence = mustValue(t, domain.NewConfidenceReference, "HIGH/carrier-confirmed")
	spec.PredictedAt = predictedAt.Add(12 * time.Hour)
	return spec
}

func formedETA(t *testing.T, spec domain.ETAPredictionSpec) domain.ETAPrediction {
	t.Helper()
	eta, err := domain.FormETAPrediction(spec)
	if err != nil {
		t.Fatalf("form ETA prediction: %v", err)
	}
	return eta
}

// Covers: AT-VE-224 与 AT-VE-051「两个来源都提供 ETA」的选取半边与 VE CONTEXT「ETA 客户可见性判断……候选内部 ETA」——两路来源各有
// 当前版时，候选取运营企业派生的那一版；只有承运商提供的一路时才取它。选取只看来源，不看预测时点也不看可信度：
// 承运商那一版更新、可信度更高也不顶替，与入参次序无关（选取不是「谁后到谁赢」，也不是替门槛挑一个能过门槛的）。
func TestTheCandidateETAIsTheOperatorDerivedCurrentVersionElseTheCarrierProvidedOne(t *testing.T) {
	operator := formedETA(t, etaSpec(t, "eta-op/v1"))
	carrier := formedETA(t, carrierETASpec(t, "eta-carrier/v1"))
	if !carrier.PredictedAt().After(operator.PredictedAt()) {
		t.Fatal("夹具应让承运商那一版预测时点更新")
	}

	for name, currents := range map[string][]domain.ETAPrediction{
		"运营企业在前": {operator, carrier},
		"承运商在前":  {carrier, operator},
	} {
		chosen, found := domain.CandidateETAFrom(currents)
		if !found || chosen.Version() != operator.Version() || chosen.Source() != domain.OperatorDerivedETA {
			t.Fatalf("%s：候选 = %q（%v）found=%v，want 运营企业派生的 %q", name, chosen.Version(), chosen.Source(), found, operator.Version())
		}
	}

	only, found := domain.CandidateETAFrom([]domain.ETAPrediction{carrier})
	if !found || only.Version() != carrier.Version() {
		t.Fatalf("只有承运商一路时候选 = %q found=%v，want %q", only.Version(), found, carrier.Version())
	}
	if _, found := domain.CandidateETAFrom(nil); found {
		t.Fatal("没有任何当前版却选出了候选")
	}
}

// Covers: VE CONTEXT「且版本化观察窗口已经届满时」「不能直接形成延误、停止移动或遗失结论」
// ——窗口未届满独立哨兵拒（提前的缺口与无扫描即延误没有区别）；预期与窗口规则
// 必备；类型上没有延误/遗失字段。
func TestAGapFormsOnlyAfterTheWindowElapses(t *testing.T) {
	windowEnd := predictedAt.Add(24 * time.Hour)

	gap, err := domain.FormVisibilityGap(
		mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		mustValue(t, domain.NewExpectedObservationReference, "LINEHAUL_ARRIVAL_SCAN"),
		mustValue(t, domain.NewObservationWindowReference, "window-rules/v2"),
		windowEnd,
		windowEnd.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("form visibility gap: %v", err)
	}
	if gap.Expectation().String() != "LINEHAUL_ARRIVAL_SCAN" || gap.WindowRule().String() != "window-rules/v2" {
		t.Fatalf("gap = %#v", gap)
	}

	if _, err := domain.FormVisibilityGap(
		mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		mustValue(t, domain.NewExpectedObservationReference, "LINEHAUL_ARRIVAL_SCAN"),
		mustValue(t, domain.NewObservationWindowReference, "window-rules/v2"),
		windowEnd,
		windowEnd.Add(-time.Hour),
	); !errors.Is(err, domain.ErrWindowNotElapsed) {
		t.Fatalf("err = %v; 窗口未届满形成了缺口", err)
	}

	if _, err := domain.FormVisibilityGap(
		mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		domain.ExpectedObservationReference{},
		mustValue(t, domain.NewObservationWindowReference, "window-rules/v2"),
		windowEnd,
		windowEnd.Add(time.Minute),
	); !errors.Is(err, domain.ErrInvalidVisibilityGap) {
		t.Fatalf("err = %v; 说不出预期什么观察的缺口被收下了", err)
	}
}
