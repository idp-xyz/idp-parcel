package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

var (
	errCurrentParcelMeasurementUnwired     = errors.New("node operations: current parcel measurement query is unwired")
	errCurrentMeasurementCauseUnrecognized = errors.New("node operations: current measurement cause is outside the closed set")
)

// CurrentParcelMeasurementQuery 按包裹答当前有效实测。取哪一条、什么叫定不下只在这一处：
// 别的上下文要毛重、三边、来源与发生时间的原词，不该各抄一份，也不该为此依赖计价。
type CurrentParcelMeasurementQuery struct {
	units        parcelHandlingUnits
	measurements stillValidMeasurements
	adoptions    measurementSelectionAdoptions
}

type parcelHandlingUnits interface {
	ListHandlingUnits(context.Context, domain.TenantID, domain.ParcelAssociationReference) ([]domain.HandlingUnitID, error)
}

type stillValidMeasurements interface {
	ListStillValid(context.Context, domain.TenantID, domain.MeasurementObjectKind, string) ([]ports.ActualMeasurementRecord, error)
}

type measurementSelectionAdoptions interface {
	FindAdoption(context.Context, domain.TenantID) (ports.MeasurementSelectionAdoption, bool, error)
}

func NewCurrentParcelMeasurementQuery(
	units parcelHandlingUnits,
	measurements stillValidMeasurements,
	adoptions measurementSelectionAdoptions,
) (*CurrentParcelMeasurementQuery, error) {
	if units == nil || measurements == nil || adoptions == nil {
		return nil, errCurrentParcelMeasurementUnwired
	}
	return &CurrentParcelMeasurementQuery{units: units, measurements: measurements, adoptions: adoptions}, nil
}

// CurrentParcelMeasurementAnswer 是按包裹问当前有效实测的三种答案。
type CurrentParcelMeasurementAnswer uint8

const (
	CurrentParcelMeasurementAnswerInvalid CurrentParcelMeasurementAnswer = iota
	NoCurrentParcelMeasurement
	OneCurrentParcelMeasurement
	UndeterminedCurrentParcelMeasurement
)

func (answer CurrentParcelMeasurementAnswer) String() string {
	switch answer {
	case NoCurrentParcelMeasurement:
		return "NONE"
	case OneCurrentParcelMeasurement:
		return "ONE"
	case UndeterminedCurrentParcelMeasurement:
		return "UNDETERMINED"
	case CurrentParcelMeasurementAnswerInvalid:
		return ""
	default:
		return ""
	}
}

// CurrentParcelMeasurementCause 是定不下的封闭来由。多件作业实物发生在取用之前，不进
// SelectCurrentMeasurement 的来由；后两格与那只函数交回的 CurrentMeasurementCause 一一对应。
type CurrentParcelMeasurementCause uint8

const (
	CurrentParcelMeasurementCauseInvalid CurrentParcelMeasurementCause = iota
	SeveralAssociatedHandlingUnits
	SeveralStillValidWithoutAdoption
	OccurredAtTied
)

func (cause CurrentParcelMeasurementCause) String() string {
	switch cause {
	case SeveralAssociatedHandlingUnits:
		return "SEVERAL_ASSOCIATED_HANDLING_UNITS"
	case SeveralStillValidWithoutAdoption:
		return "SEVERAL_WITHOUT_ADOPTION"
	case OccurredAtTied:
		return "SAME_OCCURRED_AT"
	case CurrentParcelMeasurementCauseInvalid:
		return ""
	default:
		return ""
	}
}

// CurrentParcelMeasurement 是一次按包裹的答复。
type CurrentParcelMeasurement struct {
	answer      CurrentParcelMeasurementAnswer
	measurement domain.ActualMeasurement
	cause       CurrentParcelMeasurementCause
}

func (result CurrentParcelMeasurement) Answer() CurrentParcelMeasurementAnswer { return result.answer }

// Measurement 只在一条时是那条实测本体；其余两格是零值。
func (result CurrentParcelMeasurement) Measurement() domain.ActualMeasurement {
	return result.measurement
}

// Cause 只在定不下时是三种来由之一；其余两格是零值。
func (result CurrentParcelMeasurement) Cause() CurrentParcelMeasurementCause { return result.cause }

func (query *CurrentParcelMeasurementQuery) Current(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelAssociationReference,
) (CurrentParcelMeasurement, error) {
	return query.load(ctx, tenant, parcel, measurementBound{})
}

// CurrentAsOf 与 Current 走同一段实现，只声明发生时间上界。零值也是一条上界，不是不带时点；不带时点走 Current。
func (query *CurrentParcelMeasurementQuery) CurrentAsOf(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelAssociationReference,
	asOf time.Time,
) (CurrentParcelMeasurement, error) {
	return query.load(ctx, tenant, parcel, measurementBound{limited: true, at: asOf})
}

// measurementBound 是发生时间上界。limited 为假时与不带时点的读法同一条路径，一点不多滤。
type measurementBound struct {
	limited bool
	at      time.Time
}

func (query *CurrentParcelMeasurementQuery) load(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelAssociationReference,
	bound measurementBound,
) (CurrentParcelMeasurement, error) {
	// 空白标识构不成一次关联。与 NewParcelAssociationReference 拒空白同一谓词，答没有实测，不去问读口。
	if strings.TrimSpace(parcel.String()) == "" {
		return CurrentParcelMeasurement{answer: NoCurrentParcelMeasurement}, nil
	}
	units, err := query.units.ListHandlingUnits(ctx, tenant, parcel)
	if err != nil {
		return CurrentParcelMeasurement{}, err
	}
	switch len(units) {
	case 0:
		return CurrentParcelMeasurement{answer: NoCurrentParcelMeasurement}, nil
	case 1:
	default:
		// 多件作业实物时不取其中一件的实测：包裹对不上恰一件实物，哪一条都不是这件包裹的当前有效实测。
		return CurrentParcelMeasurement{
			answer: UndeterminedCurrentParcelMeasurement,
			cause:  SeveralAssociatedHandlingUnits,
		}, nil
	}
	records, err := query.measurements.ListStillValid(ctx, tenant, domain.HandlingUnitObject, units[0].String())
	if err != nil {
		return CurrentParcelMeasurement{}, err
	}
	adoption, adopted, err := query.adoptions.FindAdoption(ctx, tenant)
	if err != nil {
		return CurrentParcelMeasurement{}, err
	}
	useForm := adopted && adoption.Form == domain.LatestStillValidByOccurredAt
	measurements := make([]domain.ActualMeasurement, 0, len(records))
	for _, record := range records {
		measurements = append(measurements, record.Measurement)
	}
	// 时点只截发生时间，而且截在取当前有效实测之前。作废与采用形态仍按此刻读：后来作废的是登错的，
	// 首次收寄复核也不该用；采用形态是租户此刻的取用规则，与不带时点的读法同一口径。
	// 发生时间不晚于 asOf 才算截至那时已经取得，恰等于的算已取得。截在取用之后的话，多条无采用会先被定成不可得，
	// 滤到只剩一条也救不回来。
	if bound.limited {
		still := make([]domain.ActualMeasurement, 0, len(measurements))
		for _, measurement := range measurements {
			if !measurement.Source().OccurredAt().After(bound.at) {
				still = append(still, measurement)
			}
		}
		measurements = still
	}
	current, answer, cause := domain.SelectCurrentMeasurement(measurements, useForm)
	switch answer {
	case domain.CurrentMeasurementUndetermined:
		mapped, err := parcelMeasurementCause(cause)
		if err != nil {
			return CurrentParcelMeasurement{}, err
		}
		return CurrentParcelMeasurement{answer: UndeterminedCurrentParcelMeasurement, cause: mapped}, nil
	case domain.CurrentMeasurementOne:
		return CurrentParcelMeasurement{answer: OneCurrentParcelMeasurement, measurement: current}, nil
	case domain.CurrentMeasurementNone:
		return CurrentParcelMeasurement{answer: NoCurrentParcelMeasurement}, nil
	default:
		return CurrentParcelMeasurement{}, fmt.Errorf("node operations: current measurement answer %s", answer)
	}
}

// 取用交回的来由对不上下面的封闭集时，不能挑一格冒充。
func parcelMeasurementCause(cause domain.CurrentMeasurementCause) (CurrentParcelMeasurementCause, error) {
	switch cause {
	case domain.SeveralMeasurementsWithoutAdoption:
		return SeveralStillValidWithoutAdoption, nil
	case domain.SameOccurredAt:
		return OccurredAtTied, nil
	default:
		return CurrentParcelMeasurementCauseInvalid, fmt.Errorf("%w: %s", errCurrentMeasurementCauseUnrecognized, cause)
	}
}
