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
// 已保存的旧摘要不在原处改写（ADR-0014）。哪些字段进摘要，沿用各口原先的内容
// 判据；没有内容判据的口不在这里发明形状。
const payloadCanonicalizationVersion = "TFC-1"

func canonicalPayloadDigest(document []byte) string {
	sum := sha256.Sum256(document)
	return payloadCanonicalizationVersion + ":" + hex.EncodeToString(sum[:])
}

func marshalPayload(document any) ([]byte, string, error) {
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, "", fmt.Errorf("transport fulfillment: canonicalize command payload: %w", err)
	}
	return encoded, canonicalPayloadDigest(encoded), nil
}

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

// CanonicalizeEstablishSchedulePayload 定形「建立班期」。
func CanonicalizeEstablishSchedulePayload(direction string, departsAt time.Time) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Direction        string `json:"direction"`
		DepartsAt        string `json:"departs_at"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "ESTABLISH_SCHEDULE",
		Direction:        direction,
		DepartsAt:        canonicalInstant(departsAt),
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
