package domain

import "errors"

// ErrInvalidMeasurementSelectionForm 是形态词不在产品内置的词表里。库里出现不认识的词只可能是写入方绕过了构造器，
// 译装口上出现是操作员写错了词；恢复动作是改用内置的那一种。
var ErrInvalidMeasurementSelectionForm = errors.New("node operations: invalid measurement selection form")

// MeasurementSelectionForm 是从多条仍有效实测里取出一条的内置形态。
// 租户没有采用任何形态时，多于一条就不取。
type MeasurementSelectionForm uint8

const (
	MeasurementSelectionFormInvalid MeasurementSelectionForm = iota
	LatestStillValidByOccurredAt
)

func (form MeasurementSelectionForm) String() string {
	switch form {
	case LatestStillValidByOccurredAt:
		return "LATEST_STILL_VALID_BY_OCCURRED_AT"
	case MeasurementSelectionFormInvalid:
		return ""
	default:
		return ""
	}
}

// MeasurementSelectionFormFromName 把册上的词译回形态。词表外的拒。
func MeasurementSelectionFormFromName(name string) (MeasurementSelectionForm, error) {
	switch name {
	case LatestStillValidByOccurredAt.String():
		return LatestStillValidByOccurredAt, nil
	default:
		return MeasurementSelectionFormInvalid, ErrInvalidMeasurementSelectionForm
	}
}

// CurrentMeasurementAnswer 是当前有效实测的三种答案。
type CurrentMeasurementAnswer uint8

const (
	CurrentMeasurementAnswerInvalid CurrentMeasurementAnswer = iota
	CurrentMeasurementNone
	CurrentMeasurementOne
	CurrentMeasurementUndetermined
)

func (answer CurrentMeasurementAnswer) String() string {
	switch answer {
	case CurrentMeasurementNone:
		return "NONE"
	case CurrentMeasurementOne:
		return "ONE"
	case CurrentMeasurementUndetermined:
		return "UNDETERMINED"
	case CurrentMeasurementAnswerInvalid:
		return ""
	default:
		return ""
	}
}

// CurrentMeasurementCause 只在定不下时有值。多于一条且租户没采用形态，与采用了形态但发生时间相同，是两件不同的事。
type CurrentMeasurementCause uint8

const (
	CurrentMeasurementCauseInvalid CurrentMeasurementCause = iota
	SeveralMeasurementsWithoutAdoption
	SameOccurredAt
)

func (cause CurrentMeasurementCause) String() string {
	switch cause {
	case SeveralMeasurementsWithoutAdoption:
		return "SEVERAL_WITHOUT_ADOPTION"
	case SameOccurredAt:
		return "SAME_OCCURRED_AT"
	case CurrentMeasurementCauseInvalid:
		return ""
	default:
		return ""
	}
}

// SelectCurrentMeasurement 从仍有效的测量里取出当前有效实测。一条测量同时带重量与尺寸，取出来的那一条两样都用。
//
// 一条都没有，答没有。恰一条，就用它，不要求租户采用形态。多于一条时——复测就是再登一条——只有租户采用了
// 「发生时间最近的一条仍有效」才取最近的那一条；没采用就定不下。最近的发生时间有两条并列，也不取其中一条。
func SelectCurrentMeasurement(
	matched []ActualMeasurement,
	adopted bool,
) (ActualMeasurement, CurrentMeasurementAnswer, CurrentMeasurementCause) {
	switch len(matched) {
	case 0:
		return ActualMeasurement{}, CurrentMeasurementNone, CurrentMeasurementCauseInvalid
	case 1:
		return matched[0], CurrentMeasurementOne, CurrentMeasurementCauseInvalid
	}
	if !adopted {
		return ActualMeasurement{}, CurrentMeasurementUndetermined, SeveralMeasurementsWithoutAdoption
	}
	latest := matched[0]
	ties := 1
	for _, measurement := range matched[1:] {
		occurred := measurement.Source().OccurredAt()
		switch {
		case occurred.After(latest.Source().OccurredAt()):
			latest = measurement
			ties = 1
		case occurred.Equal(latest.Source().OccurredAt()):
			ties++
		}
	}
	if ties > 1 {
		return ActualMeasurement{}, CurrentMeasurementUndetermined, SameOccurredAt
	}
	return latest, CurrentMeasurementOne, CurrentMeasurementCauseInvalid
}
