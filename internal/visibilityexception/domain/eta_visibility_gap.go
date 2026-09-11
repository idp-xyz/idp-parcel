package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidETA           = errors.New("visibility exception: invalid ETA prediction")
	ErrInvalidVisibilityGap = errors.New("visibility exception: invalid visibility gap")
	ErrWindowNotElapsed     = errors.New("visibility exception: the observation window has not elapsed")
)

// ETASourceKind 是预测来源口径的封闭二值：承运商提供与运营企业派生分别保留来源和
// 口径（CONTEXT「分别保留来源和口径」），不混在一个「预测」里。
type ETASourceKind uint8

const (
	ETASourceKindInvalid ETASourceKind = iota
	CarrierProvidedETA
	OperatorDerivedETA
)

func (kind ETASourceKind) valid() bool {
	return kind == CarrierProvidedETA || kind == OperatorDerivedETA
}

func (kind ETASourceKind) String() string {
	switch kind {
	case CarrierProvidedETA:
		return "CARRIER_PROVIDED"
	case OperatorDerivedETA:
		return "OPERATOR_DERIVED"
	default:
		return ""
	}
}

// ETAVersionID 是预测版本标识。新的 ETA 形成新版本，不覆盖历史预测。
type ETAVersionID struct{ requiredValue }

func NewETAVersionID(value string) (ETAVersionID, error) {
	required, err := newRequiredValue("ETA version ID", value)
	return ETAVersionID{required}, err
}

// PredictionModelReference 指名规则或模型版本。
type PredictionModelReference struct{ requiredValue }

func NewPredictionModelReference(value string) (PredictionModelReference, error) {
	required, err := newRequiredValue("prediction model reference", value)
	return PredictionModelReference{required}, err
}

// PredictionInputsReference 指名输入事实集。
type PredictionInputsReference struct{ requiredValue }

func NewPredictionInputsReference(value string) (PredictionInputsReference, error) {
	required, err := newRequiredValue("prediction inputs reference", value)
	return PredictionInputsReference{required}, err
}

// ETAPredictionSpec 是形成一次预测所需的全部输入（CONTEXT「必须明确预测对象、目标里程碑、预测时点」那一句的七件）。
type ETAPredictionSpec struct {
	Version     ETAVersionID
	Parcel      TrackedParcelReference
	Milestone   MilestoneReference
	Source      ETASourceKind
	Inputs      PredictionInputsReference
	Model       PredictionModelReference
	RangeFrom   time.Time
	RangeTo     time.Time
	Confidence  ConfidenceReference
	PredictedAt time.Time
}

// ETAPrediction 是版本化时间预测：不是客户承诺、路由计划或实际时间——类型上没有
// 那些字段，也没有任何东西能把计划时间填进来充当预测（七件缺一立不起，信息不足
// 允许不形成）。
type ETAPrediction struct {
	version      ETAVersionID
	parcel       TrackedParcelReference
	milestone    MilestoneReference
	source       ETASourceKind
	inputs       PredictionInputsReference
	model        PredictionModelReference
	rangeFrom    time.Time
	rangeTo      time.Time
	confidence   ConfidenceReference
	predictedAt  time.Time
	priorVersion ETAVersionID
}

func FormETAPrediction(spec ETAPredictionSpec) (ETAPrediction, error) {
	if !spec.Version.valid() ||
		!spec.Parcel.valid() ||
		!spec.Milestone.valid() ||
		!spec.Source.valid() ||
		!spec.Inputs.valid() ||
		!spec.Model.valid() ||
		!spec.Confidence.valid() ||
		spec.RangeFrom.IsZero() ||
		spec.RangeTo.IsZero() ||
		!spec.RangeTo.After(spec.RangeFrom) ||
		spec.PredictedAt.IsZero() {
		return ETAPrediction{}, ErrInvalidETA
	}
	return ETAPrediction{
		version:     spec.Version,
		parcel:      spec.Parcel,
		milestone:   spec.Milestone,
		source:      spec.Source,
		inputs:      spec.Inputs,
		model:       spec.Model,
		rangeFrom:   spec.RangeFrom.UTC(),
		rangeTo:     spec.RangeTo.UTC(),
		confidence:  spec.Confidence,
		predictedAt: spec.PredictedAt.UTC(),
	}, nil
}

func (eta ETAPrediction) Version() ETAVersionID {
	return eta.version
}

func (eta ETAPrediction) Parcel() TrackedParcelReference {
	return eta.parcel
}

func (eta ETAPrediction) Milestone() MilestoneReference {
	return eta.milestone
}

func (eta ETAPrediction) Source() ETASourceKind {
	return eta.source
}

// Inputs 是（包裹+里程碑+输入版本）幂等键的第三维，适配器持久化也要它——不导出，
// 编排与存储都立不起这一维。
func (eta ETAPrediction) Inputs() PredictionInputsReference {
	return eta.inputs
}

func (eta ETAPrediction) Model() PredictionModelReference {
	return eta.model
}

func (eta ETAPrediction) Range() (time.Time, time.Time) {
	return eta.rangeFrom, eta.rangeTo
}

func (eta ETAPrediction) Confidence() ConfidenceReference {
	return eta.confidence
}

func (eta ETAPrediction) PredictedAt() time.Time {
	return eta.predictedAt
}

// PriorVersion 只在更新版本上给出，指回被更新的那一版。
func (eta ETAPrediction) PriorVersion() (ETAVersionID, bool) {
	return eta.priorVersion, eta.priorVersion.valid()
}

// RehydrateETAPrediction 从持久化列重建预测。首版走 FormETAPrediction；带指回的
// 当前版把 prior 填回——库只管当前行，历史版本由指回关系承担，不是第二行。
func RehydrateETAPrediction(spec ETAPredictionSpec, prior ETAVersionID) (ETAPrediction, error) {
	eta, err := FormETAPrediction(spec)
	if err != nil {
		return ETAPrediction{}, err
	}
	if prior.valid() {
		if prior == spec.Version {
			return ETAPrediction{}, ErrInvalidETA
		}
		eta.priorVersion = prior
	}
	return eta, nil
}

// Refresh 形成新的预测版本：换版本、换输入与区间、指回原版；CONTEXT「新的 ETA 形成新版本，不覆盖历史预测」，
// 也不修改客户承诺、路由计划或实际事实——这里根本没有它们。
func (eta ETAPrediction) Refresh(spec ETAPredictionSpec) (ETAPrediction, error) {
	if spec.Version == eta.version || spec.Parcel != eta.parcel || spec.Milestone != eta.milestone {
		return ETAPrediction{}, ErrInvalidETA
	}
	refreshed, err := FormETAPrediction(spec)
	if err != nil {
		return ETAPrediction{}, err
	}
	refreshed.priorVersion = eta.version
	return refreshed, nil
}

// ExpectedObservationReference 指名适用产品或履约段明确预期的那项观察。
type ExpectedObservationReference struct{ requiredValue }

func NewExpectedObservationReference(value string) (ExpectedObservationReference, error) {
	required, err := newRequiredValue("expected observation reference", value)
	return ExpectedObservationReference{required}, err
}

// ObservationWindowReference 指名版本化的观察窗口规则。
type ObservationWindowReference struct{ requiredValue }

func NewObservationWindowReference(value string) (ObservationWindowReference, error) {
	required, err := newRequiredValue("observation window reference", value)
	return ObservationWindowReference{required}, err
}

// VisibilityGap 是可见性缺口：明确预期某项观察且版本化观察窗口届满仍未取得。它只
// 证明预期数据尚未获得——类型上没有延误、停止移动或遗失字段，无扫描直接推不出那些
// 结论（CONTEXT「不能直接形成延误、停止移动或遗失结论」）。
type VisibilityGap struct {
	parcel      TrackedParcelReference
	expectation ExpectedObservationReference
	windowRule  ObservationWindowReference
	windowEnd   time.Time
	formedAt    time.Time
}

// FormVisibilityGap 在观察窗口届满后形成缺口。窗口未届满即拒——「等一等」与「缺口」
// 是两回事，提前形成的缺口与无扫描即延误的推断没有区别。
func FormVisibilityGap(
	parcel TrackedParcelReference,
	expectation ExpectedObservationReference,
	windowRule ObservationWindowReference,
	windowEnd time.Time,
	formedAt time.Time,
) (VisibilityGap, error) {
	if !parcel.valid() || !expectation.valid() || !windowRule.valid() ||
		windowEnd.IsZero() || formedAt.IsZero() {
		return VisibilityGap{}, ErrInvalidVisibilityGap
	}
	if !formedAt.After(windowEnd) {
		return VisibilityGap{}, ErrWindowNotElapsed
	}
	return VisibilityGap{
		parcel:      parcel,
		expectation: expectation,
		windowRule:  windowRule,
		windowEnd:   windowEnd.UTC(),
		formedAt:    formedAt.UTC(),
	}, nil
}

func (gap VisibilityGap) Parcel() TrackedParcelReference {
	return gap.parcel
}

func (gap VisibilityGap) Expectation() ExpectedObservationReference {
	return gap.expectation
}

func (gap VisibilityGap) WindowRule() ObservationWindowReference {
	return gap.windowRule
}

func (gap VisibilityGap) WindowEnd() time.Time {
	return gap.windowEnd
}

func (gap VisibilityGap) FormedAt() time.Time {
	return gap.formedAt
}
