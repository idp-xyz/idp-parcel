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

// Covers: VE CONTEXT 硬句 102「每次 ETA 必须明确预测对象、目标里程碑、预测时点、
// 输入事实、规则或模型版本、预计时间范围和可信程度。信息不足时允许不形成 ETA，不得
// 以计划时间或客户承诺填充」——七件缺一立不起（缺模型版本即拒，没有可填充的旁路）；
// 区间倒置拒；来源口径封闭二值。
func TestAnETADemandsItsSevenParts(t *testing.T) {
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

// Covers: VE CONTEXT 硬句 103「新的 ETA 形成新版本，不覆盖历史预测」——刷新换版本
// 指回原版、原预测不可变；跨包裹或跨里程碑的刷新不是同一条预测线。
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
}

// Covers: VE CONTEXT 硬句 105「只有适用产品或履约段明确预期某项观察，且版本化观察
// 窗口已经届满时，才能形成可见性缺口信号。无扫描不能直接形成延误、停止移动或遗失
// 结论」——窗口未届满独立哨兵拒（提前的缺口与无扫描即延误没有区别）；预期与窗口规则
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
