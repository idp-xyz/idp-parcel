package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// MeasuredWeightAnswer 是按一件成员问当前有效实测的三格。定不下的来由用节点查询交回的稳定字面，本包不另起一套词。
type MeasuredWeightAnswer uint8

const (
	MeasuredWeightAnswerInvalid MeasuredWeightAnswer = iota
	MeasuredWeightNone
	MeasuredWeightOne
	MeasuredWeightUndetermined
)

func (answer MeasuredWeightAnswer) String() string {
	switch answer {
	case MeasuredWeightNone:
		return "NONE"
	case MeasuredWeightOne:
		return "ONE"
	case MeasuredWeightUndetermined:
		return "UNDETERMINED"
	case MeasuredWeightAnswerInvalid:
		return ""
	default:
		return ""
	}
}

// MeasuredWeightReading 是一件成员此刻的实测。有一条时数值、单位、来源身份与发生时间原样在；定不下时只有 Cause。
type MeasuredWeightReading struct {
	Answer            MeasuredWeightAnswer
	GrossWeight       string
	GrossWeightUnit   string
	MeasurementSource string
	MeasuredAt        time.Time
	Cause             string
}

// MemberMeasuredWeightView 按（租户，成员包裹）答当前有效实测。取哪一条不在关务里再写一份。
type MemberMeasuredWeightView interface {
	LoadMemberMeasuredWeight(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelReference,
	) (MeasuredWeightReading, error)
}
