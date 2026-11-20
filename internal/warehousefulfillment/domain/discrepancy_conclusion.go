package domain

import (
	"fmt"
	"time"
)

// ErrInvalidConclusion 是结论的字面量或途径形状不成立。错误正文不带回显值：依据引用与说明都不该进日志。
var ErrInvalidConclusion = fmt.Errorf("warehouse fulfillment: invalid discrepancy conclusion")

// ConclusionVerdict 是货主对一份收货差异的结论：确认或异议。结论不改库存账。
type ConclusionVerdict uint8

const (
	ConclusionVerdictInvalid ConclusionVerdict = iota
	ConclusionConfirm
	ConclusionObject
)

func (verdict ConclusionVerdict) String() string {
	switch verdict {
	case ConclusionConfirm:
		return "CONFIRM"
	case ConclusionObject:
		return "OBJECT"
	case ConclusionVerdictInvalid:
		return ""
	default:
		return ""
	}
}

// NewConclusionVerdict 只认 CONFIRM 与 OBJECT。大小写不折叠，错的字不进错误正文。
func NewConclusionVerdict(word string) (ConclusionVerdict, error) {
	switch word {
	case ConclusionConfirm.String():
		return ConclusionConfirm, nil
	case ConclusionObject.String():
		return ConclusionObject, nil
	default:
		return ConclusionVerdictInvalid, ErrInvalidConclusion
	}
}

// ConclusionNote 是结论说明，逐字保存。零值就是没带。
type ConclusionNote struct{ value string }

func NewConclusionNote(value string) (ConclusionNote, error) {
	required, err := newRequiredValue("conclusion note", value)
	if err != nil {
		return ConclusionNote{}, err
	}
	return ConclusionNote{value: required.String()}, nil
}

func (note ConclusionNote) String() string { return note.value }

func (note ConclusionNote) Given() bool { return note.value != "" }

// DiscrepancyConclusion 是一份差异上落下的一条结论。现行至多一条。被取代的代录结论 current 为假，照留。
// 货主本人的行带来源请求键，不带操作者与依据。代录的行带操作者与依据，不带来源请求键。
type DiscrepancyConclusion struct {
	tenant      TenantID
	discrepancy ReceiptDiscrepancyID
	verdict     ConclusionVerdict
	note        ConclusionNote
	path        SubmissionPath
	requestKey  RequestKey
	operator    Registrant
	evidence    EvidenceReference
	recordedAt  time.Time
	current     bool
}

func NewDiscrepancyConclusion(
	tenant TenantID,
	discrepancy ReceiptDiscrepancyID,
	verdict ConclusionVerdict,
	note ConclusionNote,
	path SubmissionPath,
	requestKey RequestKey,
	operator Registrant,
	evidence EvidenceReference,
	recordedAt time.Time,
	current bool,
) (DiscrepancyConclusion, error) {
	if !tenant.valid() || !discrepancy.valid() || recordedAt.IsZero() {
		return DiscrepancyConclusion{}, ErrInvalidConclusion
	}
	if verdict != ConclusionConfirm && verdict != ConclusionObject {
		return DiscrepancyConclusion{}, ErrInvalidConclusion
	}
	switch path {
	case SubmissionByOwner:
		if !requestKey.valid() || operator.valid() || evidence.valid() {
			return DiscrepancyConclusion{}, ErrInvalidConclusion
		}
	case SubmissionOnBehalf:
		if requestKey.valid() || !operator.valid() || !evidence.valid() {
			return DiscrepancyConclusion{}, ErrInvalidConclusion
		}
	default:
		return DiscrepancyConclusion{}, ErrInvalidConclusion
	}
	return DiscrepancyConclusion{
		tenant: tenant, discrepancy: discrepancy, verdict: verdict, note: note, path: path,
		requestKey: requestKey, operator: operator, evidence: evidence, recordedAt: recordedAt, current: current,
	}, nil
}

func (conclusion DiscrepancyConclusion) Tenant() TenantID { return conclusion.tenant }

func (conclusion DiscrepancyConclusion) Discrepancy() ReceiptDiscrepancyID {
	return conclusion.discrepancy
}

func (conclusion DiscrepancyConclusion) Verdict() ConclusionVerdict { return conclusion.verdict }

func (conclusion DiscrepancyConclusion) Note() ConclusionNote { return conclusion.note }

func (conclusion DiscrepancyConclusion) Path() SubmissionPath { return conclusion.path }

func (conclusion DiscrepancyConclusion) RequestKey() RequestKey { return conclusion.requestKey }

func (conclusion DiscrepancyConclusion) Operator() Registrant { return conclusion.operator }

func (conclusion DiscrepancyConclusion) Evidence() EvidenceReference { return conclusion.evidence }

func (conclusion DiscrepancyConclusion) RecordedAt() time.Time { return conclusion.recordedAt }

func (conclusion DiscrepancyConclusion) Current() bool { return conclusion.current }
