package commercialhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 签署证据的在线口。写面不收 tenantId 与 recordedBy：租户与登记人取认证出的身份。
// 读面按（类别、对象、版本）交回要求状态与全部历史。只挂客户合同与供应商协议。

type SigningEvidenceRegistrationIntake interface {
	IntakeSigningEvidenceRegistration(ctx context.Context, request *http.Request) (application.RegisterSigningEvidenceCommand, error)
}

type SigningEvidenceRequirementIntake interface {
	IntakeSigningEvidenceRequirement(ctx context.Context, request *http.Request) (application.RegisterSigningEvidenceRequirementCommand, error)
}

var (
	_ SigningEvidenceRegistrationIntake = (*OperatorRegistryIntake)(nil)
	_ SigningEvidenceRequirementIntake  = (*OperatorRegistryIntake)(nil)
)

type signingEvidenceDocument struct {
	TenantID              json.RawMessage `json:"tenantId"`
	RecordedBy            json.RawMessage `json:"recordedBy"`
	EvidenceID            string          `json:"evidenceId"`
	Kind                  string          `json:"kind"`
	ObjectID              string          `json:"objectId"`
	Version               string          `json:"version"`
	SignedOn              string          `json:"signedOn"`
	CounterpartySignatory string          `json:"counterpartySignatory"`
	OurSignatory          string          `json:"ourSignatory"`
	FileName              string          `json:"fileName"`
	SHA256                string          `json:"sha256"`
	LibraryIndex          string          `json:"libraryIndex"`
	Source                string          `json:"source"`
	Corrects              string          `json:"corrects"`
	RecordedAt            *time.Time      `json:"recordedAt"`
}

type signingEvidenceRequirementDocument struct {
	TenantID                  json.RawMessage `json:"tenantId"`
	RequiresCustomerContract  *bool           `json:"requiresCustomerContract"`
	RequiresSupplierAgreement *bool           `json:"requiresSupplierAgreement"`
}

func (intake *OperatorRegistryIntake) IntakeSigningEvidenceRegistration(ctx context.Context, request *http.Request) (application.RegisterSigningEvidenceCommand, error) {
	var none application.RegisterSigningEvidenceCommand
	operator, err := intake.authenticate(ctx, request)
	if err != nil {
		return none, err
	}
	document, err := decodeClosedDocument[signingEvidenceDocument](request)
	if err != nil {
		return none, err
	}
	if err := refuseSelfReportedTenant(document.TenantID); err != nil {
		return none, err
	}
	if err := refuseSelfReportedRecorder(document.RecordedBy); err != nil {
		return none, err
	}
	evidence, err := signingEvidenceFromDocument(document, operator.Tenant, operator.Operator.Reference())
	if err != nil {
		return none, err
	}
	return application.RegisterSigningEvidenceCommand{Evidence: evidence}, nil
}

func (intake *OperatorRegistryIntake) IntakeSigningEvidenceRequirement(ctx context.Context, request *http.Request) (application.RegisterSigningEvidenceRequirementCommand, error) {
	var none application.RegisterSigningEvidenceRequirementCommand
	operator, err := intake.authenticate(ctx, request)
	if err != nil {
		return none, err
	}
	document, err := decodeClosedDocument[signingEvidenceRequirementDocument](request)
	if err != nil {
		return none, err
	}
	if err := refuseSelfReportedTenant(document.TenantID); err != nil {
		return none, err
	}
	rule, err := signingEvidenceRequirementFromDocument(document, operator.Tenant)
	if err != nil {
		return none, err
	}
	return application.RegisterSigningEvidenceRequirementCommand{Rule: rule}, nil
}

func refuseSelfReportedRecorder(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	return fmt.Errorf("%w: recordedBy must not be carried in the payload; the recorder is the authenticated operator", ErrMalformedRequest)
}

func signingEvidenceFromDocument(
	document signingEvidenceDocument,
	tenant domain.TenantID,
	recorder domain.OperatorSubjectReference,
) (domain.SigningEvidence, error) {
	var none domain.SigningEvidence
	kind, known := domain.CommercialObjectKindNamed(document.Kind)
	if !known || (kind != domain.CustomerContractObject && kind != domain.SupplierAgreementObject) {
		return none, fmt.Errorf("%w: kind must be CUSTOMER_CONTRACT or SUPPLIER_AGREEMENT", ErrMalformedRequest)
	}
	id, err := domain.NewSigningEvidenceID(document.EvidenceID)
	if err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	objectID, err := domain.NewCommercialObjectID(document.ObjectID)
	if err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	version, err := domain.NewCommercialVersionLabel(document.Version)
	if err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	signedOn, err := time.ParseInLocation("2006-01-02", document.SignedOn, time.UTC)
	if err != nil {
		return none, fmt.Errorf("%w: signedOn must be a calendar date", ErrMalformedRequest)
	}
	file, err := domain.NewSigningFile(document.FileName, document.SHA256, document.LibraryIndex)
	if err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	source, known := domain.SigningEvidenceSourceNamed(document.Source)
	if !known {
		return none, fmt.Errorf("%w: source must be OFFLINE_SIGNED", ErrMalformedRequest)
	}
	var corrects domain.SigningEvidenceID
	if document.Corrects != "" {
		corrects, err = domain.NewSigningEvidenceID(document.Corrects)
		if err != nil {
			return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
		}
	}
	if document.RecordedAt == nil || document.RecordedAt.IsZero() {
		return none, fmt.Errorf("%w: recordedAt is required", ErrMalformedRequest)
	}
	evidence, err := domain.NewSigningEvidence(
		id, tenant, kind, objectID, version, signedOn,
		document.CounterpartySignatory, document.OurSignatory,
		file, source, corrects, recorder, document.RecordedAt.UTC(),
	)
	if err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return evidence, nil
}

func signingEvidenceRequirementFromDocument(document signingEvidenceRequirementDocument, tenant domain.TenantID) (domain.SigningEvidenceRequirement, error) {
	if document.RequiresCustomerContract == nil || document.RequiresSupplierAgreement == nil {
		return domain.SigningEvidenceRequirement{}, fmt.Errorf("%w: requiresCustomerContract and requiresSupplierAgreement are both required", ErrMalformedRequest)
	}
	rule, err := domain.NewSigningEvidenceRequirement(tenant, *document.RequiresCustomerContract, *document.RequiresSupplierAgreement)
	if err != nil {
		return domain.SigningEvidenceRequirement{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return rule, nil
}

type SigningEvidenceRegistrar interface {
	Handle(context.Context, application.RegisterSigningEvidenceCommand) (application.SigningEvidenceRegistrationResult, error)
}

type SigningEvidenceRequirementRegistrar interface {
	Handle(context.Context, application.RegisterSigningEvidenceRequirementCommand) (application.SigningEvidenceRequirementResult, error)
}

var (
	_ SigningEvidenceRegistrar            = (*application.RegisterSigningEvidenceHandler)(nil)
	_ SigningEvidenceRequirementRegistrar = (*application.RegisterSigningEvidenceRequirementHandler)(nil)
)

func NewRegisterSigningEvidenceEndpoint(intake SigningEvidenceRegistrationIntake, registrar SigningEvidenceRegistrar) http.Handler {
	return newRegistrationEndpoint(intake.IntakeSigningEvidenceRegistration, registrar.Handle, writeSigningEvidenceRegistrationAnswer)
}

func NewRegisterSigningEvidenceRequirementEndpoint(intake SigningEvidenceRequirementIntake, registrar SigningEvidenceRequirementRegistrar) http.Handler {
	return newRegistrationEndpoint(intake.IntakeSigningEvidenceRequirement, registrar.Handle, writeSigningEvidenceRequirementAnswer)
}

func writeSigningEvidenceRegistrationAnswer(response http.ResponseWriter, result application.SigningEvidenceRegistrationResult) {
	writeNamedOutcome(response, result.Outcome().String(), result.Outcome() == application.SigningEvidenceRegistered)
}

func writeSigningEvidenceRequirementAnswer(response http.ResponseWriter, result application.SigningEvidenceRequirementResult) {
	writeNamedOutcome(response, result.Outcome().String(), result.Outcome() == application.SigningEvidenceRequirementRegistered)
}

func writeNamedOutcome(response http.ResponseWriter, name string, created bool) {
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(response, status, struct {
		Outcome string `json:"outcome"`
	}{Outcome: name})
}

type SigningEvidenceReader interface {
	Handle(
		ctx context.Context,
		tenant domain.TenantID,
		kind domain.CommercialObjectKind,
		objectID domain.CommercialObjectID,
		version domain.CommercialVersionLabel,
	) (application.SigningEvidenceRead, error)
}

var _ SigningEvidenceReader = (*application.QuerySigningEvidenceHandler)(nil)

const outcomeSigningEvidencesRead = "SIGNING_EVIDENCES_READ"

func NewQuerySigningEvidencesEndpoint(intake CommercialCatalogueIntake, reader SigningEvidenceReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		kind, known := domain.CommercialObjectKindNamed(request.PathValue(versionKindPathValue))
		objectID, objectErr := domain.NewCommercialObjectID(request.PathValue(versionObjectIDPathValue))
		version, versionErr := domain.NewCommercialVersionLabel(request.PathValue(versionLabelPathValue))
		if !known || objectErr != nil || versionErr != nil ||
			(kind != domain.CustomerContractObject && kind != domain.SupplierAgreementObject) {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		read, err := reader.Handle(request.Context(), query.Scope.Tenant(), kind, objectID, version)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, signingEvidenceReadAnswerOf(read))
	})
}

type signingEvidenceReadAnswer struct {
	Outcome                   string                `json:"outcome"`
	Requirement               string                `json:"requirement"`
	RequiresCustomerContract  *bool                 `json:"requiresCustomerContract,omitempty"`
	RequiresSupplierAgreement *bool                 `json:"requiresSupplierAgreement,omitempty"`
	Evidences                 []signingEvidenceBody `json:"evidences"`
}

type signingEvidenceBody struct {
	EvidenceID            string `json:"evidenceId"`
	SignedOn              string `json:"signedOn"`
	CounterpartySignatory string `json:"counterpartySignatory,omitempty"`
	OurSignatory          string `json:"ourSignatory,omitempty"`
	FileName              string `json:"fileName"`
	SHA256                string `json:"sha256"`
	LibraryIndex          string `json:"libraryIndex"`
	Source                string `json:"source"`
	Corrects              string `json:"corrects,omitempty"`
	RecordedBy            string `json:"recordedBy"`
	RecordedAt            string `json:"recordedAt"`
}

func signingEvidenceReadAnswerOf(read application.SigningEvidenceRead) signingEvidenceReadAnswer {
	answer := signingEvidenceReadAnswer{
		Outcome:     outcomeSigningEvidencesRead,
		Requirement: "NOT_CONFIGURED",
		Evidences:   make([]signingEvidenceBody, 0, len(read.Evidence)),
	}
	if read.RequirementFound {
		answer.Requirement = "CONFIGURED"
		contract := read.Requirement.RequiresCustomerContract()
		agreement := read.Requirement.RequiresSupplierAgreement()
		answer.RequiresCustomerContract = &contract
		answer.RequiresSupplierAgreement = &agreement
	}
	for _, evidence := range read.Evidence {
		body := signingEvidenceBody{
			EvidenceID:            evidence.ID().String(),
			SignedOn:              evidence.SignedOn().Format("2006-01-02"),
			CounterpartySignatory: evidence.CounterpartySignatory(),
			OurSignatory:          evidence.OurSignatory(),
			FileName:              evidence.File().Name(),
			SHA256:                evidence.File().SHA256(),
			LibraryIndex:          evidence.File().LibraryIndex(),
			Source:                evidence.Source().String(),
			RecordedBy:            evidence.RecordedBy().String(),
			RecordedAt:            rfc3339(evidence.RecordedAt()),
		}
		if corrects, ok := evidence.Corrects(); ok {
			body.Corrects = corrects.String()
		}
		answer.Evidences = append(answer.Evidences, body)
	}
	return answer
}
