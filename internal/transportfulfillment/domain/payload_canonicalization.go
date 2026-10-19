package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// payloadCanonicalizationVersion 是运输履约命令口的现行形状。摘要带版本前缀，
// 已保存的旧摘要不在原处改写（ADR-0014），比对时按它自己的形状重算本次命令，见
// CompareStoredDigest。哪些字段进摘要，沿用各口原先的内容判据；没有内容判据的口
// 不在这里发明形状。
const payloadCanonicalizationVersion = "TFC-1"

func canonicalPayloadDigest(document []byte) string {
	sum := sha256.Sum256(document)
	return payloadCanonicalizationVersion + ":" + hex.EncodeToString(sum[:])
}

// StoredDigestComparison 是已存摘要与本次命令的比对结论。摘要只在同一形状版本内可比
// （ADR-0014），所以先按已存摘要的形状重算本次命令，再比。
type StoredDigestComparison int

const (
	// SamePayload：同一形状下同串，是重放。
	SamePayload StoredDigestComparison = iota + 1
	// DifferentPayload：同一形状下异串，是内容冲突。
	DifferentPayload
	// UnknownPayloadShape：已存摘要的版本前缀本口认不出，无从按它重算——不是冲突，也证明不了是重放。
	// 调用方按该处读失败的既有答复作答：读不懂这份记录与读不出来，同样是此刻判断不了。
	UnknownPayloadShape
)

// CompareStoredDigest 按已存摘要选形状：带 TFC-1 前缀的与 current（本次命令的 TFC-1 摘要）比；
// 不带前缀的是 TFC-1 之前写下的无版本摘要，与 unversioned（本次命令按无版本那一版算出的摘要）比；
// 其余前缀答 UnknownPayloadShape。存量不回写，所以无版本那一版要一直能算。
func CompareStoredDigest(stored, current, unversioned string) StoredDigestComparison {
	shape, _, versioned := strings.Cut(stored, ":")
	switch {
	case !versioned:
		return payloadComparison(stored == unversioned)
	case shape == payloadCanonicalizationVersion:
		return payloadComparison(stored == current)
	default:
		return UnknownPayloadShape
	}
}

func payloadComparison(same bool) StoredDigestComparison {
	if same {
		return SamePayload
	}
	return DifferentPayload
}

func marshalPayload(document any) ([]byte, string, error) {
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, "", fmt.Errorf("transport fulfillment: canonicalize command payload: %w", err)
	}
	return encoded, canonicalPayloadDigest(encoded), nil
}

// canonicalInstant 按纳秒成串，不折到登记册精度：经 CompareStoredDigest 比对的口，摘要是落库前由命令原值算出的，
// 原样重放必然同串；已存摘要不在原处改写（ADR-0014），所以它不能原地折到微秒，要折就得换形状版本。由此留下一处
// 有意的不对称：存摘要的口按纳秒比，两侧现算的口按微秒比（AtRegistryPrecision）。原样重放在两类口上都认作重放，
// 只有亚微秒不同的两份命令答法不同——存摘要的口答冲突，现算的口答重放。立形时就没有存量摘要的口可以一开始就折
// （CanonicalizeTransportSchedulePayload 即是，理由在它那里）。
func canonicalInstant(at time.Time) string {
	return at.UTC().Format(time.RFC3339Nano)
}

func sortedCopy(values []string) []string {
	copied := append([]string(nil), values...)
	slices.Sort(copied)
	if copied == nil {
		return []string{}
	}
	return copied
}

// CanonicalizeHandoverPayload 定形「登记运输交接」。范围、段与对象身份不进摘要。
func CanonicalizeHandoverPayload(verdict HandoverVerdict, releasedBy, receivedBy, releasingEvidence, receivingEvidence, rule, basis string, judgedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization  string `json:"canonicalization"`
		Face              string `json:"face"`
		Verdict           string `json:"verdict"`
		ReleasedBy        string `json:"released_by"`
		ReceivedBy        string `json:"received_by"`
		ReleasingEvidence string `json:"releasing_evidence"`
		ReceivingEvidence string `json:"receiving_evidence"`
		Rule              string `json:"rule"`
		Basis             string `json:"basis"`
		JudgedAt          string `json:"judged_at"`
	}{
		Canonicalization:  payloadCanonicalizationVersion,
		Face:              "REGISTER_TRANSPORT_HANDOVER",
		Verdict:           verdict.String(),
		ReleasedBy:        releasedBy,
		ReceivedBy:        receivedBy,
		ReleasingEvidence: releasingEvidence,
		ReceivingEvidence: receivingEvidence,
		Rule:              rule,
		Basis:             basis,
		JudgedAt:          canonicalInstant(judgedAt),
	})
}

// CanonicalizeHandoverCorrectionPayload 定形「更正运输交接」。前版引用在摘要里，
// 用来区分同一裁决内容落在不同前版上。
func CanonicalizeHandoverCorrectionPayload(verdict HandoverVerdict, predecessorVersion, releasingEvidence, receivingEvidence, rule, basis string, correctedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization   string `json:"canonicalization"`
		Face               string `json:"face"`
		Verdict            string `json:"verdict"`
		PredecessorVersion string `json:"predecessor_version"`
		ReleasingEvidence  string `json:"releasing_evidence"`
		ReceivingEvidence  string `json:"receiving_evidence"`
		Rule               string `json:"rule"`
		Basis              string `json:"basis"`
		CorrectedAt        string `json:"corrected_at"`
	}{
		Canonicalization:   payloadCanonicalizationVersion,
		Face:               "CORRECT_TRANSPORT_HANDOVER",
		Verdict:            verdict.String(),
		PredecessorVersion: predecessorVersion,
		ReleasingEvidence:  releasingEvidence,
		ReceivingEvidence:  receivingEvidence,
		Rule:               rule,
		Basis:              basis,
		CorrectedAt:        canonicalInstant(correctedAt),
	})
}

// CanonicalizeEffectiveDeliveryPayload 定形「登记有效交付」。方式、收件人与凭证
// 三者构成内容；对象与段身份不进摘要。
func CanonicalizeEffectiveDeliveryPayload(method, recipient, proof string) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Method           string `json:"method"`
		Recipient        string `json:"recipient"`
		Proof            string `json:"proof"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "REGISTER_EFFECTIVE_DELIVERY",
		Method:           method,
		Recipient:        recipient,
		Proof:            proof,
	})
}

// CanonicalizeDeliveryProofCorrectionPayload 定形「更正交付证明」（票 operator-channel/41 裁定 (a)）。指名的前版、
// 新 POD 与更正时刻构成内容；前版在内容里，是因为它是这次更正不从内容派生的锚——同一 POD 落在不同前版上是两次更正。
//
// 这一口不存摘要、两侧都从本体现算：在册行的 content_digest 是首登摘要，首登口拿它认首登的重放，更正改写它会让
// 首登原样重放被答成冲突。更正时刻先经 AtRegistryPrecision 折到登记册精度，再照 canonicalInstant 成串。
func CanonicalizeDeliveryProofCorrectionPayload(predecessorVersion, newProof string, correctedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization   string `json:"canonicalization"`
		Face               string `json:"face"`
		PredecessorVersion string `json:"predecessor_version"`
		NewProof           string `json:"new_proof"`
		CorrectedAt        string `json:"corrected_at"`
	}{
		Canonicalization:   payloadCanonicalizationVersion,
		Face:               "CORRECT_DELIVERY_PROOF",
		PredecessorVersion: predecessorVersion,
		NewProof:           newProof,
		CorrectedAt:        canonicalInstant(AtRegistryPrecision(correctedAt)),
	})
}

// CanonicalizeOffsitePickupRegistrationPayload 定形场外揽收的首次登记与更正共用的
// 内容判据：任务、地点、控制、执行人、发生时刻。
func CanonicalizeOffsitePickupRegistrationPayload(task, place, control, executedBy string, occurredAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Task             string `json:"task"`
		Place            string `json:"place"`
		Control          string `json:"control"`
		ExecutedBy       string `json:"executed_by"`
		OccurredAt       string `json:"occurred_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "REGISTER_OFFSITE_PICKUP",
		Task:             task,
		Place:            place,
		Control:          control,
		ExecutedBy:       executedBy,
		OccurredAt:       canonicalInstant(occurredAt),
	})
}

// OffsitePickupObjectContent 是一次场外揽收尝试里单个对象的内容。提交顺序不进摘要。
type OffsitePickupObjectContent struct {
	Object     string
	Outcome    AttemptObjectOutcome
	Basis      string
	Control    string
	OccurredAt time.Time
}

// CanonicalizeOffsitePickupPayload 定形「执行场外揽收」。对象行按对象、结果、依据、
// 控制与时刻排序，使提交顺序不构成另一份内容。
func CanonicalizeOffsitePickupPayload(attempt, task string, arrivedAt time.Time, objects []OffsitePickupObjectContent) ([]byte, string, error) {
	type objectDocument struct {
		Object     string `json:"object"`
		Outcome    string `json:"outcome"`
		Basis      string `json:"basis"`
		Control    string `json:"control"`
		OccurredAt string `json:"occurred_at"`
	}
	lines := make([]objectDocument, len(objects))
	for i, object := range objects {
		lines[i] = objectDocument{
			Object:     object.Object,
			Outcome:    object.Outcome.String(),
			Basis:      object.Basis,
			Control:    object.Control,
			OccurredAt: canonicalInstant(object.OccurredAt),
		}
	}
	slices.SortFunc(lines, func(left, right objectDocument) int {
		if left.Object != right.Object {
			if left.Object < right.Object {
				return -1
			}
			return 1
		}
		if left.Outcome != right.Outcome {
			if left.Outcome < right.Outcome {
				return -1
			}
			return 1
		}
		if left.Basis != right.Basis {
			if left.Basis < right.Basis {
				return -1
			}
			return 1
		}
		if left.Control != right.Control {
			if left.Control < right.Control {
				return -1
			}
			return 1
		}
		if left.OccurredAt < right.OccurredAt {
			return -1
		}
		if left.OccurredAt > right.OccurredAt {
			return 1
		}
		return 0
	})
	return marshalPayload(struct {
		Canonicalization string           `json:"canonicalization"`
		Face             string           `json:"face"`
		Attempt          string           `json:"attempt"`
		Task             string           `json:"task"`
		ArrivedAt        string           `json:"arrived_at"`
		Objects          []objectDocument `json:"objects"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "PERFORM_OFFSITE_PICKUP",
		Attempt:          attempt,
		Task:             task,
		ArrivedAt:        canonicalInstant(arrivedAt),
		Objects:          lines,
	})
}

// CanonicalizeCommissionPayload 定形「提交运输委托」。成员集合排序后进入摘要。
func CanonicalizeCommissionPayload(provider, agreement, conditions, role, responsibility string, submittedAt time.Time, members []string) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string   `json:"canonicalization"`
		Face             string   `json:"face"`
		Provider         string   `json:"provider"`
		Agreement        string   `json:"agreement"`
		Conditions       string   `json:"conditions"`
		Role             string   `json:"role"`
		Responsibility   string   `json:"responsibility"`
		SubmittedAt      string   `json:"submitted_at"`
		Members          []string `json:"members"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "SUBMIT_COMMISSION",
		Provider:         provider,
		Agreement:        agreement,
		Conditions:       conditions,
		Role:             role,
		Responsibility:   responsibility,
		SubmittedAt:      canonicalInstant(submittedAt),
		Members:          sortedCopy(members),
	})
}

// CanonicalizeBookingPayload 定形「提交订舱」。
func CanonicalizeBookingPayload(commission string, quantity int64, unit string, requestedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Commission       string `json:"commission"`
		Quantity         int64  `json:"quantity"`
		Unit             string `json:"unit"`
		RequestedAt      string `json:"requested_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "SUBMIT_BOOKING",
		Commission:       commission,
		Quantity:         quantity,
		Unit:             unit,
		RequestedAt:      canonicalInstant(requestedAt),
	})
}

// CanonicalizeBookingAnswerPayload 定形「应答订舱」。
func CanonicalizeBookingAnswerPayload(outcome CarrierAcceptanceOutcome, acceptance string, quantity int64, basis string, decidedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Outcome          string `json:"outcome"`
		Acceptance       string `json:"acceptance"`
		Quantity         int64  `json:"quantity"`
		Basis            string `json:"basis"`
		DecidedAt        string `json:"decided_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "ANSWER_BOOKING",
		Outcome:          outcome.String(),
		Acceptance:       acceptance,
		Quantity:         quantity,
		Basis:            basis,
		DecidedAt:        canonicalInstant(decidedAt),
	})
}

// CanonicalizeAlternateJourneyPayload 定形「启动替代旅程」。成员集合排序后进入摘要。
func CanonicalizeAlternateJourneyPayload(journey string, basisKind DispositionBasisKind, startedAt time.Time, members []string) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string   `json:"canonicalization"`
		Face             string   `json:"face"`
		Journey          string   `json:"journey"`
		BasisKind        string   `json:"basis_kind"`
		StartedAt        string   `json:"started_at"`
		Members          []string `json:"members"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "START_ALTERNATE_JOURNEY",
		Journey:          journey,
		BasisKind:        basisKind.String(),
		StartedAt:        canonicalInstant(startedAt),
		Members:          sortedCopy(members),
	})
}

// CanonicalizeRegulatoryDispositionPayload 定形「承接监管处置」。移动动作与授权
// 去空白后进入摘要；承接对象排序，使提交顺序不构成另一份内容。
func CanonicalizeRegulatoryDispositionPayload(basis string, movementAction string, decision DispositionAcceptanceKind, acceptedObjects []string, declineBasis, movementAuthority string, decidedAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization  string   `json:"canonicalization"`
		Face              string   `json:"face"`
		Basis             string   `json:"basis"`
		MovementAction    string   `json:"movement_action"`
		Decision          string   `json:"decision"`
		AcceptedObjects   []string `json:"accepted_objects"`
		DeclineBasis      string   `json:"decline_basis"`
		MovementAuthority string   `json:"movement_authority"`
		DecidedAt         string   `json:"decided_at"`
	}{
		Canonicalization:  payloadCanonicalizationVersion,
		Face:              "ACCEPT_REGULATORY_DISPOSITION",
		Basis:             basis,
		MovementAction:    strings.TrimSpace(movementAction),
		Decision:          decision.String(),
		AcceptedObjects:   sortedCopy(acceptedObjects),
		DeclineBasis:      declineBasis,
		MovementAuthority: strings.TrimSpace(movementAuthority),
		DecidedAt:         canonicalInstant(decidedAt),
	})
}

// CanonicalizeCarrierFirstEffectivePickupPayload 定形「判断实际承运商首次有效收寄」（票 operator-channel/20 的裁定）。
// 读法表达取得控制时，承运主体两键与业务时间连同读法构成内容；不表达时只有读法——失效版本上没有主体与时间，
// 命令里给了也不构成内容。租户、对象与依据是身份；命令上的进段引用（JudgeCarrierFirstEffectivePickupCommand 的 Segment、
// PlannedSegment、SegmentServiceAction）同 CanonicalizeHandoverPayload 不进摘要；名称素材也不进：它只在待确认版本上，
// 而重放比对只发生在非待确认链尾上，已形成版本上没有它可算。
//
// 这一口不存摘要、两侧都从本体现算，业务时间先经 AtRegistryPrecision 折到登记册精度，再照 canonicalInstant 成串。
func CanonicalizeCarrierFirstEffectivePickupPayload(expressesControl bool, subject CarrierSubject, occurredAt time.Time) ([]byte, string, error) {
	if !expressesControl {
		return marshalPayload(struct {
			Canonicalization string `json:"canonicalization"`
			Face             string `json:"face"`
			ExpressesControl bool   `json:"expresses_control"`
		}{
			Canonicalization: payloadCanonicalizationVersion,
			Face:             "JUDGE_CARRIER_FIRST_EFFECTIVE_PICKUP",
		})
	}
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		ExpressesControl bool   `json:"expresses_control"`
		SubjectKind      string `json:"subject_kind"`
		SubjectReference string `json:"subject_reference"`
		OccurredAt       string `json:"occurred_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "JUDGE_CARRIER_FIRST_EFFECTIVE_PICKUP",
		ExpressesControl: true,
		SubjectKind:      subject.Kind().String(),
		SubjectReference: subject.Reference(),
		OccurredAt:       canonicalInstant(AtRegistryPrecision(occurredAt)),
	})
}

// CanonicalizeScheduleTemplatePayload 定形「登记周期安排」（ADR-0201）。正文各格全进摘要：同一代码再登时内容同即
// `已存在`，异即下一版，没有不进摘要的正文。星期按周一到周日写词，生效区间不设终点写 null——零值日期与「不设
// 终点」在摘要里分得开。代码与版本是身份，不进。
func CanonicalizeScheduleTemplatePayload(content ScheduleTemplateContent) ([]byte, string, error) {
	weekdays, err := normalizedWeekdays(content.Weekdays)
	if err != nil {
		return nil, "", err
	}
	words := make([]string, 0, len(weekdays))
	for _, weekday := range weekdays {
		words = append(words, ScheduleWeekdayWord(weekday))
	}
	var effectiveUntil *string
	if content.EffectiveUntil.valid() {
		until := content.EffectiveUntil.String()
		effectiveUntil = &until
	}
	return marshalPayload(struct {
		Canonicalization     string   `json:"canonicalization"`
		Face                 string   `json:"face"`
		Connection           string   `json:"connection"`
		FromNode             string   `json:"from_node"`
		ToNode               string   `json:"to_node"`
		Weekdays             []string `json:"weekdays"`
		DepartureLocalMinute int      `json:"departure_local_minute"`
		DurationMinutes      int64    `json:"duration_minutes"`
		TimeZone             string   `json:"time_zone"`
		ServiceScope         string   `json:"service_scope"`
		EffectiveFrom        string   `json:"effective_from"`
		EffectiveUntil       *string  `json:"effective_until"`
	}{
		Canonicalization:     payloadCanonicalizationVersion,
		Face:                 "REGISTER_SCHEDULE_TEMPLATE",
		Connection:           content.Direction.Connection().String(),
		FromNode:             content.Direction.From().String(),
		ToNode:               content.Direction.To().String(),
		Weekdays:             words,
		DepartureLocalMinute: content.DepartureLocalMinute,
		DurationMinutes:      int64(content.Duration / time.Minute),
		TimeZone:             content.TimeZone,
		ServiceScope:         content.ServiceScope.String(),
		EffectiveFrom:        content.EffectiveFrom.String(),
		EffectiveUntil:       effectiveUntil,
	})
}

// CanonicalizeTransportSchedulePayload 定形「登记具体班次」（ADR-0201）。进摘要的是班次本身：方向、出发与到达、
// 服务范围与来历（物化来的再加周期安排代码与运行日期，临时班次那两格写 null）。版本是登记册给的，不进；物化
// 时那一版周期安排的版本号也不进——按更新的一版再物化出一模一样的班次，班次没有变，不该多出一版。
//
// 出发与到达先经 AtRegistryPrecision 折到登记册精度，再照 canonicalInstant 成串：这本册子上同键异内容不是冲突而是
// 下一版，按纳秒算摘要，一次带亚微秒时刻的临时班次登记之后照读回值原样重登，就会多出一版与上一版库面完全相同的班次。
func CanonicalizeTransportSchedulePayload(schedule TransportSchedule) ([]byte, string, error) {
	var template, runningDate *string
	if code, _, date, materialized := schedule.origin.Template(); materialized {
		codeText, dateText := code.String(), date.String()
		template, runningDate = &codeText, &dateText
	}
	return marshalPayload(struct {
		Canonicalization string  `json:"canonicalization"`
		Face             string  `json:"face"`
		Connection       string  `json:"connection"`
		FromNode         string  `json:"from_node"`
		ToNode           string  `json:"to_node"`
		DepartsAt        string  `json:"departs_at"`
		ArrivesAt        string  `json:"arrives_at"`
		ServiceScope     string  `json:"service_scope"`
		Origin           string  `json:"origin"`
		Template         *string `json:"template"`
		RunningDate      *string `json:"running_date"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "REGISTER_TRANSPORT_SCHEDULE",
		Connection:       schedule.direction.Connection().String(),
		FromNode:         schedule.direction.From().String(),
		ToNode:           schedule.direction.To().String(),
		DepartsAt:        canonicalInstant(AtRegistryPrecision(schedule.departsAt)),
		ArrivesAt:        canonicalInstant(AtRegistryPrecision(schedule.arrivesAt)),
		ServiceScope:     schedule.serviceScope.String(),
		Origin:           schedule.origin.kind.String(),
		Template:         template,
		RunningDate:      runningDate,
	})
}

// CanonicalizeEstablishPoolPayload 定形「建立运力池」。
func CanonicalizeEstablishPoolPayload(schedule, unit string, capacity int64) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Schedule         string `json:"schedule"`
		Unit             string `json:"unit"`
		Capacity         int64  `json:"capacity"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "ESTABLISH_POOL",
		Schedule:         schedule,
		Unit:             unit,
		Capacity:         capacity,
	})
}

// CanonicalizeMovementFactPayload 定形「登记实际移动事实」（票 tf-face-algebra/08 的裁定）：班次、种类、地点与放行引用构成内容。
// 租户、事实与版本是身份；来源不进，它由这一面注入而不是调用方报的，隔离写时期的存量行带的是合成来源；GateRequired 不进，
// 它是门禁判断的输入、不是事实的属性（见 RehydrateMovementFactSpec），在册本体上也没有它可算；登记时刻不进。
// 业务时间照 ADR-0023「发生时间与接收时间……两者都不进入内容摘要」也不进：只差发生时间的同身份重投读作重放。
//
// 放行缺席写 null：零值引用就是「没有门禁依据」，与 RecordMovementFact 判在场的口径同一。这一口不存摘要、两侧都从本体现算。
func CanonicalizeMovementFactPayload(schedule ScheduleReference, kind MovementFactKind, location MovementLocationReference, gateClearance GateClearanceReference) ([]byte, string, error) {
	var clearance *string
	if gateClearance.valid() {
		reference := gateClearance.String()
		clearance = &reference
	}
	return marshalPayload(struct {
		Canonicalization string  `json:"canonicalization"`
		Face             string  `json:"face"`
		Schedule         string  `json:"schedule"`
		Kind             string  `json:"kind"`
		Location         string  `json:"location"`
		GateClearance    *string `json:"gate_clearance"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "RECORD_MOVEMENT_FACT",
		Schedule:         schedule.String(),
		Kind:             kind.String(),
		Location:         location.String(),
		GateClearance:    clearance,
	})
}
