package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidActualCarrierJudgment = errors.New("transport fulfillment: invalid actual carrier judgment")
	ErrInvalidCarrierEvidence       = errors.New("transport fulfillment: invalid carrier evidence")
	// ErrCarrierEvidencePrecedesSegment：业务时间早于段成立的证据说的是段成立之前的事——它要么属于
	// 前一段，要么与段成立事实矛盾，两种都不是本段承运主体的依据（CONTEXT「实际承运商判断」规则节）。
	// 与形状错误分格：恢复动作不是改输入，而是去看这条证据该归哪一段。
	ErrCarrierEvidencePrecedesSegment = errors.New("transport fulfillment: carrier evidence precedes the segment")
	// ErrCarrierEvidenceAlreadyConsidered：同一份依据不形成第二个版本。它是业务答案不是故障——重放
	// 一条已经在依据里的证据，判断不变，也不该多出一版让「未知期间」凭空断成两截。
	ErrCarrierEvidenceAlreadyConsidered = errors.New("transport fulfillment: carrier evidence already considered")
	// ErrCarrierEvidenceNotConsidered：要撤回或补认身份的那条依据不在当前版本里。
	ErrCarrierEvidenceNotConsidered = errors.New("transport fulfillment: carrier evidence is not among the current bases")
	// ErrCarrierIdentityAlreadyRecognised：那条依据已经指向在册身份，没有「未登记 → 已识别」可走。
	ErrCarrierIdentityAlreadyRecognised = errors.New("transport fulfillment: carrier identity on that evidence is already recognised")
)

// CarrierEvidenceSource 是合格证据来源的封闭四格（CONTEXT「实际承运商判断」规则节第三条；ADR-0103
// 决定四）：承运商直接收寄扫描、承运商收货凭证、接收方为承运主体的`已交接`权威运输交接结果、保留
// 原始承运来源的可信渠道回传（外部承运轨迹事实与之同格）。
//
// 封闭而不是字符串字段，是因为开放集合守不住「品牌或单号不能推断承运商」那句硬话——运输委托、承运
// 接受、订舱、舱单、面单、渠道品牌与单号都不是来源，新增一种来源先改 CONTEXT 再改这里。
type CarrierEvidenceSource uint8

const (
	CarrierEvidenceSourceInvalid CarrierEvidenceSource = iota
	CarrierDirectPickupScan
	CarrierReceiptVoucher
	HandedOverToCarrier
	TrustedChannelCallback
)

func (source CarrierEvidenceSource) String() string {
	switch source {
	case CarrierDirectPickupScan:
		return "CARRIER_PICKUP_SCAN"
	case CarrierReceiptVoucher:
		return "CARRIER_RECEIPT_VOUCHER"
	case HandedOverToCarrier:
		return "TRANSPORT_HANDOVER_TO_CARRIER"
	case TrustedChannelCallback:
		return "TRUSTED_CHANNEL_CALLBACK"
	default:
		return ""
	}
}

func (source CarrierEvidenceSource) valid() bool {
	return source >= CarrierDirectPickupScan && source <= TrustedChannelCallback
}

// ParseCarrierEvidenceSource 把库面或登记输入里的来源词认回封闭集合；词不在集合内即拒，不猜。
func ParseCarrierEvidenceSource(raw string) (CarrierEvidenceSource, error) {
	for source := CarrierDirectPickupScan; source <= TrustedChannelCallback; source++ {
		if source.String() == raw {
			return source, nil
		}
	}
	return CarrierEvidenceSourceInvalid, fmt.Errorf("%w: unknown carrier evidence source %q", ErrInvalidCarrierEvidence, raw)
}

// CarrierEvidenceReference 指名依据的来源事实（收寄、凭证、交接判断或外部承运轨迹事实）。
type CarrierEvidenceReference struct{ requiredValue }

func NewCarrierEvidenceReference(value string) (CarrierEvidenceReference, error) {
	required, err := newRequiredValue("carrier evidence reference", value)
	return CarrierEvidenceReference{required}, err
}

// CarrierSubjectKind 是承运主体引用的两支（ADR-0103 决定三）：外部参与方，或自营运营法人。走哪一支
// 由证据决定——证据是自营执行方的作业事实则为自营法人，是外部承运方的收寄、凭证、交接或回传则为
// 外部参与方——不由运输委托的有无推断。
type CarrierSubjectKind uint8

const (
	CarrierSubjectKindInvalid CarrierSubjectKind = iota
	ExternalCarrierParty
	OwnOperatingLegalEntity
)

func (kind CarrierSubjectKind) String() string {
	switch kind {
	case ExternalCarrierParty:
		return "EXTERNAL_PARTY"
	case OwnOperatingLegalEntity:
		return "OPERATING_LEGAL_ENTITY"
	default:
		return ""
	}
}

func (kind CarrierSubjectKind) valid() bool {
	return kind == ExternalCarrierParty || kind == OwnOperatingLegalEntity
}

// ParseCarrierSubjectKind 把库面或登记输入里的分支词认回封闭集合。
func ParseCarrierSubjectKind(raw string) (CarrierSubjectKind, error) {
	for kind := ExternalCarrierParty; kind <= OwnOperatingLegalEntity; kind++ {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return CarrierSubjectKindInvalid, fmt.Errorf("%w: unknown carrier subject kind %q", ErrInvalidActualCarrierJudgment, raw)
}

// CarrierSubject 是承运主体身份的引用：外部参与方引用 party-commercial 的参与方身份，自营段引用真实
// 执行运输的运营法人。**本上下文不为承运方铸身份**（CONTEXT Boundaries）——这里只有一个引用，
// 名称不在此处；证据里的名称在册上找不到身份时不进这里，作为素材随依据保留。
type CarrierSubject struct {
	kind      CarrierSubjectKind
	reference requiredValue
}

func NewCarrierSubject(kind CarrierSubjectKind, reference string) (CarrierSubject, error) {
	if !kind.valid() {
		return CarrierSubject{}, fmt.Errorf("%w: carrier subject kind", ErrInvalidActualCarrierJudgment)
	}
	required, err := newRequiredValue("carrier subject reference", reference)
	if err != nil {
		return CarrierSubject{}, fmt.Errorf("%w: %v", ErrInvalidActualCarrierJudgment, err)
	}
	return CarrierSubject{kind: kind, reference: required}, nil
}

func (subject CarrierSubject) Kind() CarrierSubjectKind { return subject.kind }
func (subject CarrierSubject) Reference() string        { return subject.reference.String() }

func (subject CarrierSubject) valid() bool {
	return subject.kind.valid() && subject.reference.valid()
}

// CarrierEvidenceSpec 是形成一条依据所需的全部输入。Subject 与 Material 恰有一个在场：身份已在册
// 就引用它，不在册就只留素材——两者都给，等于既说「找到了身份」又说「没找到」。
type CarrierEvidenceSpec struct {
	Source     CarrierEvidenceSource
	Reference  CarrierEvidenceReference
	OccurredAt time.Time
	Subject    CarrierSubject
	Material   string
}

// CarrierEvidence 是判断版本的一条依据：来源种类、来源事实引用、业务时间（取自来源事实，那是源的），
// 以及它指名的承运主体——已在册则为身份引用，未在册则只有名称素材（CONTEXT：「不据名称铸身份，
// 形成待确认（承运主体身份未登记），名称作为素材随依据保留」）。
type CarrierEvidence struct {
	source     CarrierEvidenceSource
	reference  CarrierEvidenceReference
	occurredAt time.Time
	subject    CarrierSubject
	material   string
}

func NewCarrierEvidence(spec CarrierEvidenceSpec) (CarrierEvidence, error) {
	if !spec.Source.valid() || !spec.Reference.valid() || spec.OccurredAt.IsZero() {
		return CarrierEvidence{}, ErrInvalidCarrierEvidence
	}
	material := strings.TrimSpace(spec.Material)
	if spec.Subject.valid() == (material != "") {
		return CarrierEvidence{}, fmt.Errorf("%w: exactly one of a registered subject or name material", ErrInvalidCarrierEvidence)
	}
	return CarrierEvidence{
		source:     spec.Source,
		reference:  spec.Reference,
		occurredAt: spec.OccurredAt.UTC(),
		subject:    spec.Subject,
		material:   material,
	}, nil
}

func (evidence CarrierEvidence) Source() CarrierEvidenceSource       { return evidence.source }
func (evidence CarrierEvidence) Reference() CarrierEvidenceReference { return evidence.reference }
func (evidence CarrierEvidence) OccurredAt() time.Time               { return evidence.occurredAt }

// Subject 交回这条依据指名的在册承运主体；身份未登记时第二个返回值为 false，名称在 Material 上。
func (evidence CarrierEvidence) Subject() (CarrierSubject, bool) {
	return evidence.subject, evidence.subject.valid()
}

// Material 是承运主体的名称素材，只在身份未登记时携带。
func (evidence CarrierEvidence) Material() string { return evidence.material }

func (evidence CarrierEvidence) valid() bool {
	return evidence.source.valid() && evidence.reference.valid() && !evidence.occurredAt.IsZero() &&
		evidence.subject.valid() != (evidence.material != "")
}

// PendingCarrierReason 是待确认的封闭三原因（ADR-0103 决定二，按 ADR-0029 的判据分格——三者的恢复
// 动作各不相同：等证据 / 人裁 / 去 party-commercial 登记）。
type PendingCarrierReason uint8

const (
	PendingCarrierReasonNone PendingCarrierReason = iota
	NoQualifiedCarrierEvidence
	CarrierEvidenceSourceConflict
	CarrierIdentityNotRegistered
)

func (reason PendingCarrierReason) String() string {
	switch reason {
	case NoQualifiedCarrierEvidence:
		return "NO_QUALIFIED_EVIDENCE"
	case CarrierEvidenceSourceConflict:
		return "SOURCE_CONFLICT"
	case CarrierIdentityNotRegistered:
		return "IDENTITY_NOT_REGISTERED"
	default:
		return ""
	}
}

func (reason PendingCarrierReason) valid() bool {
	return reason >= NoQualifiedCarrierEvidence && reason <= CarrierIdentityNotRegistered
}

// ParsePendingCarrierReason 把库面里的原因词认回封闭集合。
func ParsePendingCarrierReason(raw string) (PendingCarrierReason, error) {
	for reason := NoQualifiedCarrierEvidence; reason <= CarrierIdentityNotRegistered; reason++ {
		if reason.String() == raw {
			return reason, nil
		}
	}
	return PendingCarrierReasonNone, fmt.Errorf("%w: unknown pending carrier reason %q", ErrInvalidActualCarrierJudgment, raw)
}

// CarrierVerdict 是判断值：已识别的承运主体，或带原因的待确认。恰居其一——「待确认」是一个取值不是
// 缺失（ADR-0103 标题句），所以这里没有「既不识别也无原因」的第三态。
type CarrierVerdict struct {
	subject CarrierSubject
	pending PendingCarrierReason
}

// Identified 在判断值为已识别时交回承运主体。
func (verdict CarrierVerdict) Identified() (CarrierSubject, bool) {
	return verdict.subject, verdict.subject.valid()
}

// Pending 在判断值为待确认时交回原因。
func (verdict CarrierVerdict) Pending() (PendingCarrierReason, bool) {
	return verdict.pending, verdict.pending.valid()
}

func (verdict CarrierVerdict) valid() bool {
	return verdict.subject.valid() != verdict.pending.valid()
}

// ActualCarrierJudgmentVersion 是判断历史里的一版：判断值、业务时间、判断形成时间、依据的来源事实
// 引用（ADR-0103 决定二）。业务时间取自依据（源的），形成时间由本上下文铸（本仓的）——两者分开，
// 「不追溯覆盖未知期间」才量得出来：未知期间按形成时间说的是知识史，承运主体自何时起承运按业务
// 时间说的是事实史（决定五）。
type ActualCarrierJudgmentVersion struct {
	sequence     int
	verdict      CarrierVerdict
	businessTime time.Time
	formedAt     time.Time
	bases        []CarrierEvidence
}

func (version ActualCarrierJudgmentVersion) Sequence() int           { return version.sequence }
func (version ActualCarrierJudgmentVersion) Verdict() CarrierVerdict { return version.verdict }
func (version ActualCarrierJudgmentVersion) BusinessTime() time.Time { return version.businessTime }
func (version ActualCarrierJudgmentVersion) FormedAt() time.Time     { return version.formedAt }
func (version ActualCarrierJudgmentVersion) Bases() []CarrierEvidence {
	return append([]CarrierEvidence(nil), version.bases...)
}

// ActualCarrierJudgment 是本上下文就一个实际履约段是谁在承运所形成的带版本的判断记录（CONTEXT
// 「实际承运商判断」词条）。键是（租户，实际履约段）；一段一份判断历史，逐对象的收寄、交接、履约
// 事实是输入不是主体（ADR-0103 决定一）。
//
// 它不是段或参与关系上的一格，不修改 ActualFulfillmentSegment 与 FulfillmentParticipation；值语义，
// 每次转换交回新值、原版本一字不动——类型上没有任何改写既有版本的方法。
type ActualCarrierJudgment struct {
	tenantID      TenantID
	segment       FulfillmentSegmentReference
	establishedAt time.Time
	versions      []ActualCarrierJudgmentVersion
}

// OpenActualCarrierJudgmentSpec 是段成立时铸首版所需的输入。EstablishedAt 是段成立的业务时刻（首个
// 对象的控制起点），FormedAt 是本上下文铸首版的时钟。
type OpenActualCarrierJudgmentSpec struct {
	TenantID      TenantID
	Segment       FulfillmentSegmentReference
	EstablishedAt time.Time
	FormedAt      time.Time
}

// OpenActualCarrierJudgment 在实际履约段成立那一刻形成首个判断版本：待确认（无合格证据），业务时间取
// 段成立时刻——未知期间从这一版起算（CONTEXT 生命周期「实际承运商判断」首条）。
func OpenActualCarrierJudgment(spec OpenActualCarrierJudgmentSpec) (ActualCarrierJudgment, error) {
	if !spec.TenantID.valid() || !spec.Segment.valid() || spec.EstablishedAt.IsZero() || spec.FormedAt.IsZero() {
		return ActualCarrierJudgment{}, ErrInvalidActualCarrierJudgment
	}
	return ActualCarrierJudgment{
		tenantID:      spec.TenantID,
		segment:       spec.Segment,
		establishedAt: spec.EstablishedAt.UTC(),
		versions: []ActualCarrierJudgmentVersion{{
			sequence:     1,
			verdict:      CarrierVerdict{pending: NoQualifiedCarrierEvidence},
			businessTime: spec.EstablishedAt.UTC(),
			formedAt:     spec.FormedAt.UTC(),
		}},
	}, nil
}

func (judgment ActualCarrierJudgment) TenantID() TenantID                   { return judgment.tenantID }
func (judgment ActualCarrierJudgment) Segment() FulfillmentSegmentReference { return judgment.segment }
func (judgment ActualCarrierJudgment) SegmentEstablishedAt() time.Time      { return judgment.establishedAt }

// Versions 按序号交回全部版本，首版在前。
func (judgment ActualCarrierJudgment) Versions() []ActualCarrierJudgmentVersion {
	return append([]ActualCarrierJudgmentVersion(nil), judgment.versions...)
}

// Current 是当前版本——序号最大的那一版。判断永远至少有一版，所以这里不需要第二个返回值。
func (judgment ActualCarrierJudgment) Current() ActualCarrierJudgmentVersion {
	return judgment.versions[len(judgment.versions)-1]
}
