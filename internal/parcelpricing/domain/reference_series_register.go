package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidReferenceSeriesRegistration 表示序列登记立不住：来源标识、登记责任方
	// 或生效区间缺位（CONTEXT 硬句四件），期次乱序/重叠，汇率缺口径，或更正关系不成对。
	ErrInvalidReferenceSeriesRegistration = errors.New("parcel pricing: invalid reference series registration")
	// ErrReferenceSeriesRegistrationSnapshotInvalid 表示序列快照解不出一版立得住的
	// 登记——坏写入或被改过的取值在重建门上暴露，而不是变成一个看起来合法的序列。
	ErrReferenceSeriesRegistrationSnapshotInvalid = errors.New("parcel pricing: invalid reference series registration snapshot")
)

// seriesCanonicalization 标识序列登记快照与其内容摘要所依据的文档形状。与 ADR-0014
// 同一条纪律：摘要只在同一形状版本内可比，拓宽形状必须递增这个值，不就地改写既有形状。
// 序列快照与价卡的 PPC 形状族各自演进，故各持一号。
const seriesCanonicalization = "PRS-1"

// SeriesPeriodValue 是一期序列取值：生效区间 [起, 止)，止点零值表示无上界（只许在
// 最后一期）。取值凭证指可再次复核的公布记录或牌价记录本身（CONTEXT）；缺凭证的期次
// 只有断言强度，可用于隔离验证，不得支撑生产金额——这个等级由登记结构自己表达。
type SeriesPeriodValue struct {
	startsAt time.Time
	endsAt   time.Time
	value    Decimal
	evidence string
}

func NewSeriesPeriodValue(startsAt, endsAt time.Time, value Decimal, evidenceRef string) (SeriesPeriodValue, error) {
	period := SeriesPeriodValue{
		startsAt: startsAt.UTC(),
		endsAt:   endsAt,
		value:    value,
		evidence: evidenceRef,
	}
	if !endsAt.IsZero() {
		period.endsAt = endsAt.UTC()
	}
	if !period.valid() {
		return SeriesPeriodValue{}, ErrInvalidReferenceSeriesRegistration
	}
	return period, nil
}

func (period SeriesPeriodValue) StartsAt() time.Time { return period.startsAt }

// EndsAt 只在有界期次上给出。
func (period SeriesPeriodValue) EndsAt() (time.Time, bool) {
	return period.endsAt, !period.endsAt.IsZero()
}

func (period SeriesPeriodValue) Value() Decimal { return period.value }

// Evidence 只在携带取值凭证的期次上给出。
func (period SeriesPeriodValue) Evidence() (string, bool) {
	return period.evidence, period.evidence != ""
}

// Verifiable 报告该期取值是否可再次复核；否则只有断言强度。
func (period SeriesPeriodValue) Verifiable() bool { return period.evidence != "" }

func (period SeriesPeriodValue) valid() bool {
	if period.startsAt.IsZero() {
		return false
	}
	if !period.endsAt.IsZero() && !period.endsAt.After(period.startsAt) {
		return false
	}
	if !period.value.valid() || period.value.IsNegative() {
		return false
	}
	// 凭证引用允许为空（断言强度），但不允许带边空白混形状。
	if period.evidence != "" && !trimmed(period.evidence) {
		return false
	}
	return true
}

func (period SeriesPeriodValue) covers(asOf time.Time) bool {
	if asOf.Before(period.startsAt) {
		return false
	}
	return period.endsAt.IsZero() || asOf.Before(period.endsAt)
}

// ReferenceSeriesRegistrationSpec 是登记一版计价参考序列所需的全部输入。数值不归
// 计价生产（ADR-0013）：这里收的是登记责任方对承运商公布记录或财务侧牌价的转录，
// 连同指回可复核记录的凭证。
type ReferenceSeriesRegistrationSpec struct {
	Tenant           TenantID
	Kind             ReferenceSeriesKind
	Reference        VersionReference
	SourceIdentifier string
	Registrant       string
	// QuoteBasis 是声明取值口径的商业价格政策版本（party-commercial 拥有）。汇率
	// 必备——不接受未声明口径的裸汇率；燃油的折扣系数写在卡上，不需要口径。
	QuoteBasis VersionReference
	Periods    []SeriesPeriodValue
	// PriorVersion 与 CorrectionBasis 成对声明更正关系：取值发现错误时形成新序列
	// 版本，原版本一字不动（CONTEXT）。
	PriorVersion    VersionReference
	CorrectionBasis string
}

// ReferenceSeriesRegistration 是一版已立得住的序列登记（票 08 的登记聚合）。登记一期
// 取值是一次来源事实断言，不是无责任的转抄：转抄与登记错误由登记责任方承担，口径错误
// 由声明该口径的商业价格政策承担，两者不互相顶替。
type ReferenceSeriesRegistration struct {
	tenant           TenantID
	kind             ReferenceSeriesKind
	reference        VersionReference
	sourceIdentifier string
	registrant       string
	quoteBasis       *VersionReference
	periods          []SeriesPeriodValue
	priorVersion     *VersionReference
	correctionBasis  string
}

func NewReferenceSeriesRegistration(spec ReferenceSeriesRegistrationSpec) (ReferenceSeriesRegistration, error) {
	registration := ReferenceSeriesRegistration{
		tenant:           spec.Tenant,
		kind:             spec.Kind,
		reference:        spec.Reference,
		sourceIdentifier: spec.SourceIdentifier,
		registrant:       spec.Registrant,
		periods:          append([]SeriesPeriodValue(nil), spec.Periods...),
		correctionBasis:  spec.CorrectionBasis,
	}
	if spec.QuoteBasis != (VersionReference{}) {
		basis := spec.QuoteBasis
		registration.quoteBasis = &basis
	}
	if spec.PriorVersion != (VersionReference{}) {
		prior := spec.PriorVersion
		registration.priorVersion = &prior
	}
	if !registration.valid() {
		return ReferenceSeriesRegistration{}, ErrInvalidReferenceSeriesRegistration
	}
	return registration, nil
}

func (registration ReferenceSeriesRegistration) Tenant() TenantID          { return registration.tenant }
func (registration ReferenceSeriesRegistration) Kind() ReferenceSeriesKind { return registration.kind }
func (registration ReferenceSeriesRegistration) Reference() VersionReference {
	return registration.reference
}
func (registration ReferenceSeriesRegistration) SourceIdentifier() string {
	return registration.sourceIdentifier
}
func (registration ReferenceSeriesRegistration) Registrant() string { return registration.registrant }

// QuoteBasis 只在声明了取值口径的序列上给出（汇率必有，燃油可无）。
func (registration ReferenceSeriesRegistration) QuoteBasis() (VersionReference, bool) {
	if registration.quoteBasis == nil {
		return VersionReference{}, false
	}
	return *registration.quoteBasis, true
}

func (registration ReferenceSeriesRegistration) Periods() []SeriesPeriodValue {
	return append([]SeriesPeriodValue(nil), registration.periods...)
}

// Correction 只在更正版本上给出：指回被更正的版本与更正依据。
func (registration ReferenceSeriesRegistration) Correction() (VersionReference, string, bool) {
	if registration.priorVersion == nil {
		return VersionReference{}, "", false
	}
	return *registration.priorVersion, registration.correctionBasis, true
}

// EffectiveFrom 是首期起点；期次有序，首期即最早。
func (registration ReferenceSeriesRegistration) EffectiveFrom() time.Time {
	return registration.periods[0].startsAt
}

// EffectiveTo 只在末期有界时给出。
func (registration ReferenceSeriesRegistration) EffectiveTo() (time.Time, bool) {
	last := registration.periods[len(registration.periods)-1]
	return last.EndsAt()
}

// Verifiable 报告整版登记是否每一期都携带可复核凭证；任何一期缺凭证，整版只有断言
// 强度可言。
func (registration ReferenceSeriesRegistration) Verifiable() bool {
	for _, period := range registration.periods {
		if !period.Verifiable() {
			return false
		}
	}
	return true
}

func (registration ReferenceSeriesRegistration) valid() bool {
	if !registration.tenant.valid() ||
		!registration.kind.valid() ||
		registration.reference.kind != ArtifactReferenceSeries ||
		!registration.reference.valid() ||
		!trimmed(registration.sourceIdentifier) ||
		!trimmed(registration.registrant) {
		return false
	}
	if registration.quoteBasis != nil {
		if registration.quoteBasis.kind != ArtifactCommercialPolicy || !registration.quoteBasis.valid() {
			return false
		}
	}
	// 不接受未声明口径的裸汇率（CONTEXT）；燃油的折扣系数写在卡上，无需口径。
	if registration.kind == ReferenceSeriesExchangeRate && registration.quoteBasis == nil {
		return false
	}
	if len(registration.periods) == 0 {
		return false
	}
	for index, period := range registration.periods {
		if !period.valid() {
			return false
		}
		// 无上界期次只许在最后：它之后的期次没有可判定的起点归属。
		if period.endsAt.IsZero() && index != len(registration.periods)-1 {
			return false
		}
		if index > 0 {
			previous := registration.periods[index-1]
			if !period.startsAt.After(previous.startsAt) || period.startsAt.Before(previous.endsAt) {
				return false
			}
		}
	}
	// 更正两件成对且不自指、不换序列身份：换身份就是另一条序列，只能另行登记。
	if registration.priorVersion == nil {
		return registration.correctionBasis == ""
	}
	if !trimmed(registration.correctionBasis) {
		return false
	}
	prior := *registration.priorVersion
	if prior.kind != ArtifactReferenceSeries || !prior.valid() {
		return false
	}
	if prior.id != registration.reference.id || prior.version == registration.reference.version {
		return false
	}
	return true
}

// ResolvedSeriesReading 是一次按计价基准时点的解析结果：可直接冻结进评价输入的序列
// 取值，连同该期取值的证据等级——缺凭证的期次只有断言强度，只准隔离验证的门在消费侧，
// 这里如实带出。
type ResolvedSeriesReading struct {
	value  ReferenceSeriesValue
	period SeriesPeriodValue
}

func (reading ResolvedSeriesReading) Value() ReferenceSeriesValue { return reading.value }
func (reading ResolvedSeriesReading) Verifiable() bool            { return reading.period.Verifiable() }
func (reading ResolvedSeriesReading) Evidence() (string, bool)    { return reading.period.Evidence() }

// ResolveAt 按计价基准时点解析一期取值（ADR-0013：解析按基准时点，重放用原取值不读
// 当前值）。时点不落在任何期次内时不给答案——缺口是证据不足，评价侧据以挂起，不编数。
func (registration ReferenceSeriesRegistration) ResolveAt(asOf time.Time) (ResolvedSeriesReading, bool) {
	if asOf.IsZero() {
		return ResolvedSeriesReading{}, false
	}
	for _, period := range registration.periods {
		if !period.covers(asOf.UTC()) {
			continue
		}
		var value ReferenceSeriesValue
		var err error
		if registration.quoteBasis != nil {
			value, err = NewQuotedReferenceSeriesValue(
				registration.kind, registration.reference, period.value, *registration.quoteBasis)
		} else {
			value, err = NewReferenceSeriesValue(registration.kind, registration.reference, period.value)
		}
		if err != nil {
			return ResolvedSeriesReading{}, false
		}
		return ResolvedSeriesReading{value: value, period: period}, true
	}
	return ResolvedSeriesReading{}, false
}

type seriesPeriodSnapshot struct {
	StartsAt time.Time       `json:"startsAt"`
	EndsAt   *time.Time      `json:"endsAt,omitempty"`
	Value    decimalSnapshot `json:"value"`
	Evidence string          `json:"evidence,omitempty"`
}

type referenceSeriesRegistrationSnapshot struct {
	Canonicalization string                    `json:"canonicalization"`
	Tenant           string                    `json:"tenant"`
	Kind             string                    `json:"kind"`
	Reference        versionReferenceSnapshot  `json:"reference"`
	SourceIdentifier string                    `json:"sourceIdentifier"`
	Registrant       string                    `json:"registrant"`
	QuoteBasis       *versionReferenceSnapshot `json:"quoteBasis,omitempty"`
	Periods          []seriesPeriodSnapshot    `json:"periods"`
	PriorVersion     *versionReferenceSnapshot `json:"priorVersion,omitempty"`
	CorrectionBasis  string                    `json:"correctionBasis,omitempty"`
	ContentDigest    string                    `json:"contentDigest"`
}

// Canonicalization 报出本登记按哪套形状做规范化与摘要（PRS 族）。
func (registration ReferenceSeriesRegistration) Canonicalization() string {
	return seriesCanonicalization
}

// ContentDigest 是整版登记（含每期取值与凭证引用）的内容指纹，按 PRS 形状规范化后
// 计算，只在同一形状版本内可比。
func (registration ReferenceSeriesRegistration) ContentDigest() string {
	document := registration.snapshotDocument()
	document.ContentDigest = ""
	return hashCanonical(document)
}

func (registration ReferenceSeriesRegistration) snapshotDocument() referenceSeriesRegistrationSnapshot {
	document := referenceSeriesRegistrationSnapshot{
		Canonicalization: seriesCanonicalization,
		Tenant:           registration.tenant.String(),
		Kind:             registration.kind.String(),
		Reference:        versionReferenceOf(registration.reference),
		SourceIdentifier: registration.sourceIdentifier,
		Registrant:       registration.registrant,
		CorrectionBasis:  registration.correctionBasis,
	}
	if registration.quoteBasis != nil {
		basis := versionReferenceOf(*registration.quoteBasis)
		document.QuoteBasis = &basis
	}
	if registration.priorVersion != nil {
		prior := versionReferenceOf(*registration.priorVersion)
		document.PriorVersion = &prior
	}
	for _, period := range registration.periods {
		row := seriesPeriodSnapshot{
			StartsAt: period.startsAt,
			Value:    decimalOf(period.value),
			Evidence: period.evidence,
		}
		if endsAt, bounded := period.EndsAt(); bounded {
			end := endsAt
			row.EndsAt = &end
		}
		document.Periods = append(document.Periods, row)
	}
	return document
}

// MarshalReferenceSeriesRegistration 把一版序列登记折成持久化快照。快照携带自己的
// 内容摘要，读回经同一道整版重验——列面只是比对，权威内容在快照。
func MarshalReferenceSeriesRegistration(registration ReferenceSeriesRegistration) ([]byte, error) {
	if !registration.valid() {
		return nil, ErrReferenceSeriesRegistrationSnapshotInvalid
	}
	document := registration.snapshotDocument()
	document.ContentDigest = registration.ContentDigest()
	return json.Marshal(document)
}

// RehydrateReferenceSeriesRegistration 从快照重建序列登记并整版重验：形状版本不被
// 当前构建支持时拒绝重建（按别的形状重算摘要在结构上不可能），登记后被改过的取值以
// 摘要自校暴露。
func RehydrateReferenceSeriesRegistration(raw []byte) (ReferenceSeriesRegistration, error) {
	var document referenceSeriesRegistrationSnapshot
	if err := json.Unmarshal(raw, &document); err != nil {
		return ReferenceSeriesRegistration{}, fmt.Errorf("%w: %v", ErrReferenceSeriesRegistrationSnapshotInvalid, err)
	}
	if document.Canonicalization != seriesCanonicalization {
		return ReferenceSeriesRegistration{}, fmt.Errorf("%w: snapshot records %q, this build canonicalizes %q",
			ErrCanonicalizationVersionUnsupported, document.Canonicalization, seriesCanonicalization)
	}
	registration := ReferenceSeriesRegistration{
		tenant:           TenantID{identifier{value: document.Tenant}},
		kind:             ReferenceSeriesKind(document.Kind),
		reference:        versionReferenceFrom(document.Reference),
		sourceIdentifier: document.SourceIdentifier,
		registrant:       document.Registrant,
		correctionBasis:  document.CorrectionBasis,
	}
	if document.QuoteBasis != nil {
		basis := versionReferenceFrom(*document.QuoteBasis)
		registration.quoteBasis = &basis
	}
	if document.PriorVersion != nil {
		prior := versionReferenceFrom(*document.PriorVersion)
		registration.priorVersion = &prior
	}
	for _, row := range document.Periods {
		period := SeriesPeriodValue{
			startsAt: row.StartsAt,
			value:    decimalFrom(row.Value),
			evidence: row.Evidence,
		}
		if row.EndsAt != nil {
			period.endsAt = *row.EndsAt
		}
		registration.periods = append(registration.periods, period)
	}
	if !registration.valid() {
		return ReferenceSeriesRegistration{}, ErrReferenceSeriesRegistrationSnapshotInvalid
	}
	if registration.ContentDigest() != document.ContentDigest {
		return ReferenceSeriesRegistration{}, fmt.Errorf(
			"%w: content digest disagrees with the snapshot body", ErrReferenceSeriesRegistrationSnapshotInvalid)
	}
	return registration, nil
}
