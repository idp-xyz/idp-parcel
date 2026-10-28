package settlementhttp

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// 供应商账单接收口与审核口（ADR-0340 决定一、二）：操作者渠道「运营决定」能力面上的两种账单决定，进的是 parcel-api 已装配的那一只
// 接收编排（审核是它的 Audit，ADR-0170 决定一）。提交操作者记成操作证据，只进审计（决定三）。

const (
	// OperatorDecisionSupplierBillReception 是接收一份供应商账单并裁决逐行匹配。
	OperatorDecisionSupplierBillReception OperatorDecision = "SUPPLIER_BILL_RECEPTION"
	// OperatorDecisionSupplierBillAudit 是对账单的一行形成审核应付。
	OperatorDecisionSupplierBillAudit OperatorDecision = "SUPPLIER_BILL_AUDIT"
)

// SupplierBillReceptionIntake 是账单接收口的 Intake。
type SupplierBillReceptionIntake interface {
	IntakeSupplierBillReception(ctx context.Context, request *http.Request) (application.ReceiveSupplierBillCommand, error)
}

// SupplierBillAuditIntake 是审核口的 Intake。
type SupplierBillAuditIntake interface {
	IntakeSupplierBillAudit(ctx context.Context, request *http.Request) (application.AuditBillLineCommand, error)
}

// SupplierBillReceiver 是接收口转交的那一口（ReceiveSupplierBillHandler.Handle）。
type SupplierBillReceiver interface {
	Handle(ctx context.Context, command application.ReceiveSupplierBillCommand) (application.ReceiveSupplierBillResult, error)
}

// SupplierBillAuditor 是审核口转交的那一口（ReceiveSupplierBillHandler.Audit）。
type SupplierBillAuditor interface {
	Audit(ctx context.Context, command application.AuditBillLineCommand) (application.AuditResult, error)
}

var (
	_ SupplierBillReceptionIntake = (*OperatorDecisionIntake)(nil)
	_ SupplierBillAuditIntake     = (*OperatorDecisionIntake)(nil)
)

func (intake *OperatorDecisionIntake) IntakeSupplierBillReception(
	ctx context.Context,
	request *http.Request,
) (application.ReceiveSupplierBillCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionSupplierBillReception)
	if err != nil {
		return application.ReceiveSupplierBillCommand{}, err
	}
	payload, err := readSinglePayload(io.LimitReader(request.Body, maxManualRegistrationBytes+1))
	if err != nil {
		return application.ReceiveSupplierBillCommand{}, err
	}
	command, err := registrationjson.SupplierBillReceptionFromJSONForTenant(payload, operator.Tenant)
	if err != nil {
		return application.ReceiveSupplierBillCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	command.OperationEvidence = operatorEvidence(operator)
	return command, nil
}

func (intake *OperatorDecisionIntake) IntakeSupplierBillAudit(
	ctx context.Context,
	request *http.Request,
) (application.AuditBillLineCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionSupplierBillAudit)
	if err != nil {
		return application.AuditBillLineCommand{}, err
	}
	payload, err := readSinglePayload(io.LimitReader(request.Body, maxManualRegistrationBytes+1))
	if err != nil {
		return application.AuditBillLineCommand{}, err
	}
	command, err := registrationjson.SupplierBillAuditFromJSONForTenant(payload, operator.Tenant)
	if err != nil {
		return application.AuditBillLineCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	command.OperationEvidence = operatorEvidence(operator)
	return command, nil
}

// NewReceiveSupplierBillEndpoint 交回账单接收口。
func NewReceiveSupplierBillEndpoint(intake SupplierBillReceptionIntake, receiver SupplierBillReceiver) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeSupplierBillReception(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		result, err := receiver.Handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeReceptionAnswer(response, result)
	})
}

// NewAuditSupplierBillEndpoint 交回审核口。
func NewAuditSupplierBillEndpoint(intake SupplierBillAuditIntake, auditor SupplierBillAuditor) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeSupplierBillAudit(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		result, err := auditor.Audit(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeAuditAnswer(response, result)
	})
}

type receptionResponse struct {
	Outcome               string                  `json:"outcome"`
	UndecidedReason       string                  `json:"undecidedReason,omitempty"`
	ContinuationReference string                  `json:"continuationReference,omitempty"`
	AuditAuthority        string                  `json:"auditAuthority,omitempty"`
	NotCurrent            *expectedNotCurrentBody `json:"notCurrent,omitempty"`
}

type expectedNotCurrentBody struct {
	Line      string `json:"line"`
	Version   string `json:"version"`
	Withdrawn bool   `json:"withdrawn"`
	Successor string `json:"successor,omitempty"`
}

// writeReceptionAnswer 判据同本包其余写口（writeFundsAnswer 头注）。未决里只有跨币种没有换算依据是租户取值缺席，答 200；其余是册
// 读不动。账单接收了而审核授权没登，接收照样成立，答复多一格 auditAuthority 说审核那一步会停住。
func writeReceptionAnswer(response http.ResponseWriter, result application.ReceiveSupplierBillResult) {
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	if result.Outcome() == application.BillUndecided && result.UndecidedReason() != application.BillConversionUnconfigured {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	if result.BillHandoffReference() != "" {
		writeProblem(response, http.StatusInternalServerError, codeHandoffNotSent)
		return
	}
	body := receptionResponse{
		Outcome: name, UndecidedReason: result.UndecidedReason().String(), ContinuationReference: result.ContinuationReference(),
	}
	if result.AuditUndecided() {
		body.AuditAuthority = "UNCONFIGURED"
	}
	if notCurrent, has := result.NotCurrent(); has {
		body.NotCurrent = &expectedNotCurrentBody{
			Line: notCurrent.Line.String(), Version: notCurrent.Version.String(),
			Withdrawn: notCurrent.Withdrawn, Successor: notCurrent.Successor.String(),
		}
	}
	status := http.StatusOK
	if result.Outcome() == application.BillReceived {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

type auditResponse struct {
	Outcome               string `json:"outcome"`
	Payable               string `json:"payable,omitempty"`
	Amount                string `json:"amount,omitempty"`
	Currency              string `json:"currency,omitempty"`
	UndecidedReason       string `json:"undecidedReason,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
}

// writeAuditAnswer 判据同上。审核授权、应付归集账户、越权升级上限三册没登是租户取值缺席，答 200 带原因；超上限要升级、这一行不可
// 审核都是答案；其余未决是册读不动。
func writeAuditAnswer(response http.ResponseWriter, result application.AuditResult) {
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	if result.Outcome() == application.AuditUndecided && !businessUndecidedAudit(result.UndecidedReason()) {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	if result.PayableHandoffReference() != "" {
		writeProblem(response, http.StatusInternalServerError, codeHandoffNotSent)
		return
	}
	body := auditResponse{
		Outcome: name, UndecidedReason: result.UndecidedReason().String(), ContinuationReference: result.ContinuationReference(),
	}
	if record, has := result.Payable(); has {
		currency, minor := record.Payable.Amount()
		body.Payable, body.Currency, body.Amount = record.Payable.Payable().String(), currency.String(), minorAmount(minor)
	}
	status := http.StatusOK
	if result.Outcome() == application.PayableFormed {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

func businessUndecidedAudit(reason application.AuditUndecidedReason) bool {
	switch reason {
	case application.AuditAuthorityUnconfigured, application.PayableAccountUnconfigured, application.AuditEscalationUnconfigured:
		return true
	default:
		return false
	}
}
