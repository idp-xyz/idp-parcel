package registrationjson

import (
	"encoding/json"
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// OwnerDiscrepancyConclusionFields 是货主结论口解出来的业务内容。来源请求键不在这里：它由信封按接入请求标识推导。
// 货主取自信封。依据、决定标识与操作者再带即坏载荷。
type OwnerDiscrepancyConclusionFields struct {
	AccessRequestID string
	Discrepancy     domain.ReceiptDiscrepancyID
	Verdict         domain.ConclusionVerdict
	Note            domain.ConclusionNote
}

type ownerDiscrepancyConclusionDocument struct {
	carriedIdentity
	refusedOwner
	DecisionID        json.RawMessage `json:"decisionId"`
	EvidenceReference json.RawMessage `json:"evidenceReference"`
	Operator          json.RawMessage `json:"operator"`
	AccessRequestID   *string         `json:"accessRequestId"`
	DiscrepancyID     *string         `json:"discrepancyId"`
	Conclusion        *string         `json:"conclusion"`
	Note              *string         `json:"note"`
}

// ReadOwnerDiscrepancyConclusion 把货主结论口的载荷译成字段。认证回调读这一次，字段留给随后组命令。
func ReadOwnerDiscrepancyConclusion(reader io.Reader) (OwnerDiscrepancyConclusionFields, error) {
	var none OwnerDiscrepancyConclusionFields
	document, err := decodeDocument[ownerDiscrepancyConclusionDocument](reader, "receipt discrepancy conclusion")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if err := document.refuseOwner(); err != nil {
		return none, err
	}
	if len(document.DecisionID) > 0 {
		return none, malformed("decisionId must not be carried in the payload")
	}
	if len(document.EvidenceReference) > 0 {
		return none, malformed("evidenceReference must not be carried in the payload; the owner conclusion has no evidence reference")
	}
	if len(document.Operator) > 0 {
		return none, malformed("operator must not be carried in the payload")
	}
	fields, err := conclusionFields(document.DiscrepancyID, document.Conclusion, document.Note)
	if err != nil {
		return none, err
	}
	fields.AccessRequestID = rawAccessRequestID(document.AccessRequestID)
	return fields, nil
}

type onBehalfDiscrepancyConclusionDocument struct {
	carriedIdentity
	refusedOwner
	DecisionID        json.RawMessage `json:"decisionId"`
	AccessRequestID   json.RawMessage `json:"accessRequestId"`
	DiscrepancyID     *string         `json:"discrepancyId"`
	Conclusion        *string         `json:"conclusion"`
	Note              *string         `json:"note"`
	EvidenceReference *string         `json:"evidenceReference"`
}

// OnBehalfDiscrepancyConclusionCommand 把代录结论译成命令。不从载荷收货主、决定标识或接入请求标识。依据引用必带。
func OnBehalfDiscrepancyConclusionCommand(
	reader io.Reader,
	tenant domain.TenantID,
	operator domain.Registrant,
) (application.ConcludeDiscrepancyCommand, error) {
	var none application.ConcludeDiscrepancyCommand
	document, err := decodeDocument[onBehalfDiscrepancyConclusionDocument](reader, "receipt discrepancy conclusion recorded on behalf")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if err := document.refuseOwner(); err != nil {
		return none, err
	}
	if len(document.DecisionID) > 0 {
		return none, malformed("decisionId must not be carried in the payload; replay identity is the discrepancy")
	}
	if len(document.AccessRequestID) > 0 {
		return none, malformed("accessRequestId must not be carried in the payload")
	}
	fields, err := conclusionFields(document.DiscrepancyID, document.Conclusion, document.Note)
	if err != nil {
		return none, err
	}
	evidence, err := requiredText("evidenceReference", document.EvidenceReference, domain.NewEvidenceReference)
	if err != nil {
		return none, err
	}
	return application.ConcludeDiscrepancyCommand{
		Tenant: tenant, Discrepancy: fields.Discrepancy, Verdict: fields.Verdict, Note: fields.Note,
		Path: domain.SubmissionOnBehalf, Operator: operator, Evidence: evidence,
	}, nil
}

func conclusionFields(discrepancyID, conclusion, note *string) (OwnerDiscrepancyConclusionFields, error) {
	var none OwnerDiscrepancyConclusionFields
	id, err := requiredText("discrepancyId", discrepancyID, domain.NewReceiptDiscrepancyID)
	if err != nil {
		return none, err
	}
	verdict, err := conclusionVerdict(conclusion)
	if err != nil {
		return none, err
	}
	written, err := optionalConclusionNote(note)
	if err != nil {
		return none, err
	}
	return OwnerDiscrepancyConclusionFields{Discrepancy: id, Verdict: verdict, Note: written}, nil
}

func conclusionVerdict(raw *string) (domain.ConclusionVerdict, error) {
	if raw == nil {
		return domain.ConclusionVerdictInvalid, malformed("conclusion is required")
	}
	verdict, err := domain.NewConclusionVerdict(*raw)
	if err != nil {
		return domain.ConclusionVerdictInvalid, malformed("conclusion must be CONFIRM or OBJECT")
	}
	return verdict, nil
}

func optionalConclusionNote(raw *string) (domain.ConclusionNote, error) {
	if raw == nil {
		return domain.ConclusionNote{}, nil
	}
	note, err := textCell("note", *raw, domain.NewConclusionNote)
	if err != nil {
		return domain.ConclusionNote{}, err
	}
	return note, nil
}
