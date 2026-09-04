package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidSourceFeedTranscription 表示转录的输入立不住：绑定、记录或观测是零值，或前一版
	// 不是同租户同序列同种类的登记。
	ErrInvalidSourceFeedTranscription = errors.New("parcel pricing: invalid source feed transcription")
	// ErrSourceFeedOutOfOrder 表示新期次起点不晚于前一版末期起点、或落进前一版有界末期之内。
	// 连接器不补数、不改数、不沿用旧值——这一格如实拒，缺口留给评价去挂起（ADR-0099 决定六）。
	ErrSourceFeedOutOfOrder = errors.New("parcel pricing: source feed publication is not after the prior version's last period")
)

// publishedVersionLayout 是连接器登记的版本号写法：来源声明的公布日期。同一来源一天公布一次
// 是常态；同一天再来一份内容不同的公布，版本号撞上而摘要不同，登记册答内容冲突——那正是
// 「取值更正必须形成新版本」要人来办的那一格，连接器不替人办。
const publishedVersionLayout = "2006-01-02"

// SeriesObservation 是连接器从公布记录原文里读出的一期取值：从哪一刻起生效、值多少。连接器
// 只转录，不算数、不改数、不选口径——口径来自绑定，这里只有来源说了什么。
type SeriesObservation struct {
	effectiveFrom time.Time
	value         Decimal
}

func NewSeriesObservation(effectiveFrom time.Time, value Decimal) (SeriesObservation, error) {
	observed := SeriesObservation{effectiveFrom: effectiveFrom.UTC(), value: value}
	if !observed.valid() {
		return SeriesObservation{}, ErrInvalidSourceFeedTranscription
	}
	return observed, nil
}

func (observed SeriesObservation) EffectiveFrom() time.Time { return observed.effectiveFrom }
func (observed SeriesObservation) Value() Decimal           { return observed.value }

func (observed SeriesObservation) valid() bool {
	return !observed.effectiveFrom.IsZero() && observed.value.valid() && !observed.value.IsNegative()
}

// SourceFeedTranscription 是一次转录的全部输入。Prior 可缺（首版）；StorageLocator 可缺（本体
// 存放未配置，或原地引用）。
type SourceFeedTranscription struct {
	Binding        SourceConnectorBinding
	Record         PublishedRecord
	StorageLocator string
	Prior          *ReferenceSeriesRegistration
	Observation    SeriesObservation
}

// TranscribeSourceFeed 把一次抓取转录成一版整版重述的登记 spec（ADR-0099 决定五、六）。
//
// 三条规则，各自只在这里定一次，连接器逐家实现时不重写：
//
//   - 同一份工件再来一次，交回前一版逐字相同的 spec：认的是工件摘要（连接器登记的版本引用
//     digest 就是它），登记册据以答幂等重放。若按抓取时刻重铸凭证，同一份文件第二次抓就会
//     被读成同版本内容冲突。
//   - 否则是延展：前一版全部期次原样在，开放末期在新期次起点闭合，新期次接上并开放；不带
//     更正关系——更正是人的事。
//   - 版本号取来源声明的公布日期，引用 digest 取工件摘要，凭证为工件引用——连接器登记的
//     期次因此天生 VERIFIABLE。
func TranscribeSourceFeed(input SourceFeedTranscription) (ReferenceSeriesRegistrationSpec, error) {
	binding, record := input.Binding, input.Record
	if !binding.valid() || !record.valid() || !input.Observation.valid() {
		return ReferenceSeriesRegistrationSpec{}, ErrInvalidSourceFeedTranscription
	}

	var priorPeriods []SeriesPeriodValue
	if input.Prior != nil {
		prior := *input.Prior
		if !prior.valid() || prior.tenant != binding.tenant ||
			prior.reference.id != binding.seriesID || prior.kind != binding.seriesKind {
			return ReferenceSeriesRegistrationSpec{}, fmt.Errorf(
				"%w: prior version does not belong to binding %s/%s", ErrInvalidSourceFeedTranscription, binding.tenant, binding.seriesID)
		}
		if prior.reference.digest == record.contentDigest {
			return prior.Spec(), nil
		}
		priorPeriods = prior.periods
	}

	next, err := NewSeriesPeriodValue(
		input.Observation.effectiveFrom, time.Time{}, input.Observation.value, record.EvidenceReference(input.StorageLocator))
	if err != nil {
		return ReferenceSeriesRegistrationSpec{}, fmt.Errorf("%w: %v", ErrInvalidSourceFeedTranscription, err)
	}
	periods, err := restateSeriesPeriods(priorPeriods, next)
	if err != nil {
		return ReferenceSeriesRegistrationSpec{}, err
	}
	reference, err := NewVersionReference(
		ArtifactReferenceSeries, binding.seriesID, record.publishedOn.Format(publishedVersionLayout), record.contentDigest)
	if err != nil {
		return ReferenceSeriesRegistrationSpec{}, fmt.Errorf("%w: %v", ErrInvalidSourceFeedTranscription, err)
	}

	spec := ReferenceSeriesRegistrationSpec{
		Tenant:           binding.tenant,
		Kind:             binding.seriesKind,
		Reference:        reference,
		SourceIdentifier: binding.sourceIdentifier,
		Registrant:       binding.registrant,
		Periods:          periods,
	}
	if binding.quoteBasis != nil {
		spec.QuoteBasis = *binding.quoteBasis
	}
	return spec, nil
}

// restateSeriesPeriods 把新期次接到前一版期次之后：开放末期在新起点闭合，有界末期之后允许
// 缺口，落进末期之内或不晚于末期起点都拒。只闭合开放末期——有界末期是前一版自己声明的终点，
// 连接器改它就是改数。
func restateSeriesPeriods(prior []SeriesPeriodValue, next SeriesPeriodValue) ([]SeriesPeriodValue, error) {
	if len(prior) == 0 {
		return []SeriesPeriodValue{next}, nil
	}
	restated := append([]SeriesPeriodValue(nil), prior...)
	last := restated[len(restated)-1]
	if !next.startsAt.After(last.startsAt) {
		return nil, fmt.Errorf("%w: %s is not after %s", ErrSourceFeedOutOfOrder,
			next.startsAt.Format(time.RFC3339), last.startsAt.Format(time.RFC3339))
	}
	if last.endsAt.IsZero() {
		last.endsAt = next.startsAt
		restated[len(restated)-1] = last
	} else if next.startsAt.Before(last.endsAt) {
		return nil, fmt.Errorf("%w: %s falls inside the bounded last period ending %s", ErrSourceFeedOutOfOrder,
			next.startsAt.Format(time.RFC3339), last.endsAt.Format(time.RFC3339))
	}
	return append(restated, next), nil
}

// Spec 是构造器的逆：交回能原样重建本登记的输入。整版重述从前一版取期次、幂等重放要交回前一版
// 逐字相同的登记，都靠它。
func (registration ReferenceSeriesRegistration) Spec() ReferenceSeriesRegistrationSpec {
	spec := ReferenceSeriesRegistrationSpec{
		Tenant:           registration.tenant,
		Kind:             registration.kind,
		Reference:        registration.reference,
		SourceIdentifier: registration.sourceIdentifier,
		Registrant:       registration.registrant,
		Periods:          registration.Periods(),
		CorrectionBasis:  registration.correctionBasis,
	}
	if registration.quoteBasis != nil {
		spec.QuoteBasis = *registration.quoteBasis
	}
	if registration.priorVersion != nil {
		spec.PriorVersion = *registration.priorVersion
	}
	return spec
}
