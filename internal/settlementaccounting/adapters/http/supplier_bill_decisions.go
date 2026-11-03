package settlementhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 供应商账单接收口与审核口（ADR-0340 决定一、二）：操作者渠道「运营决定」能力面上的两种账单决定，进的是 parcel-api 已装配的那一只
// 接收编排（审核是它的 Audit，ADR-0170 决定一）。提交操作者记成操作证据，只进审计（决定三）。

const (
	// OperatorDecisionSupplierBillReception 是接收一份供应商账单并裁决逐行匹配。
	OperatorDecisionSupplierBillReception OperatorDecision = "SUPPLIER_BILL_RECEPTION"
	// OperatorDecisionSupplierBillAudit 是对账单一行上的一段给出结论：通过（形成审核应付）或拒绝，含争议再审给出的那两种。
	OperatorDecisionSupplierBillAudit OperatorDecision = "SUPPLIER_BILL_AUDIT"
	// OperatorDecisionSupplierBillDisputeDecision 是把一行上的一段挂成争议，与对争议再审（ADR-0357 决定二）。再审给出通过或拒绝，
	// 所以再审两种都要。
	OperatorDecisionSupplierBillDisputeDecision OperatorDecision = "SUPPLIER_BILL_DISPUTE_DECISION"
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

// IntakeSupplierBillAudit 先按载荷要的授予逐一认证，再判载荷形状：争议决定与审核应付分别授权（UC-SA-004「权限」，ADR-0357
// 决定二），要哪几种得看这一次给的是什么结果、是不是再审，所以先粗读一遍结果与所解争议。认证排在形状之前——请求体读不出、
// 形状不对的，没有授予的人照旧先答未授予，不借 400 探出口的形状。
func (intake *OperatorDecisionIntake) IntakeSupplierBillAudit(
	ctx context.Context,
	request *http.Request,
) (application.AuditBillLineCommand, error) {
	payload, readErr := readSinglePayload(io.LimitReader(request.Body, maxManualRegistrationBytes+1))
	var operator OperatorIdentity
	for index, decision := range supplierBillAuditDecisions(payload) {
		authenticated, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), decision)
		if err != nil {
			return application.AuditBillLineCommand{}, err
		}
		if index == 0 {
			operator = authenticated
		}
	}
	if readErr != nil {
		return application.AuditBillLineCommand{}, readErr
	}
	command, err := registrationjson.SupplierBillAuditFromJSONForTenant(payload, operator.Tenant)
	if err != nil {
		return application.AuditBillLineCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	command.OperationEvidence = operatorEvidence(operator)
	return command, nil
}

// supplierBillAuditDecisions 说这一份审核载荷要哪几种授予（ADR-0357 决定二）：给出结论（通过、拒绝）要审核应付，挂争议与再审要争议
// 决定，再审两种都要。只粗读结果与所解争议两格，读不出或没有结果（只点名应付标识的旧形）按通过整行算，只要审核应付——形状对不对
// 由 registrationjson 在认证之后判。
func supplierBillAuditDecisions(payload []byte) []OperatorDecision {
	var glance struct {
		Verdict         string `json:"verdict"`
		ResolvesDispute string `json:"resolvesDispute"`
	}
	_ = json.Unmarshal(payload, &glance)
	disputed := glance.Verdict == domain.AuditDisputed.String()
	var decisions []OperatorDecision
	if disputed || glance.ResolvesDispute != "" {
		decisions = append(decisions, OperatorDecisionSupplierBillDisputeDecision)
	}
	if !disputed {
		decisions = append(decisions, OperatorDecisionSupplierBillAudit)
	}
	return decisions
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

// receptionResponse 的 registeredClaim 与 registeredSupplierBillNumber 只在账单号冲突时出现、恰出其一：前者是这个号已标识的那份
// 主张，后者是这份主张已登记的那个号，连同它的分配方 registeredSupplier（ADR-0357 决定一）。expectedCostSupplier 只在供应商不符与
// 供应商未记两格出现（ADR-0358 决定二）：哪一行、点名的哪一版，不符时另带那一版记下的供应商，好让人知道它是谁的。
type receptionResponse struct {
	Outcome                      string                    `json:"outcome"`
	UndecidedReason              string                    `json:"undecidedReason,omitempty"`
	ContinuationReference        string                    `json:"continuationReference,omitempty"`
	AuditAuthority               string                    `json:"auditAuthority,omitempty"`
	NotCurrent                   *expectedNotCurrentBody   `json:"notCurrent,omitempty"`
	RegisteredClaim              string                    `json:"registeredClaim,omitempty"`
	RegisteredSupplierBillNumber string                    `json:"registeredSupplierBillNumber,omitempty"`
	RegisteredSupplier           string                    `json:"registeredSupplier,omitempty"`
	ExpectedCostSupplier         *expectedCostSupplierBody `json:"expectedCostSupplier,omitempty"`
}

type expectedCostSupplierBody struct {
	Line     string `json:"line"`
	Version  string `json:"version"`
	Supplier string `json:"supplier,omitempty"`
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
	if conflict, has := result.NumberConflict(); has {
		body.RegisteredClaim, body.RegisteredSupplierBillNumber = conflict.RegisteredClaim.String(), conflict.RegisteredNumber.String()
		body.RegisteredSupplier = conflict.RegisteredSupplier.String()
	}
	if refusal, has := result.SupplierRefusal(); has {
		body.ExpectedCostSupplier = &expectedCostSupplierBody{
			Line: refusal.Line.String(), Version: refusal.Version.String(), Supplier: refusal.Supplier.String(),
		}
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

// auditResponse 的 decision 是调用方生成的审核决定标识（幂等键），verdict 与 rangeFrom、rangeTo 是这份决定给的结果与那一段；
// 通过的那一份另带 payable（就是决定标识，ADR-0357 决定三）、金额与币种，documentNumber 是服务端形成应付时按编号规则取的应付
// 单据号（ADR-0355 决定十），没有时不出这一格，改出 documentNumberAbsence。documentType 只在撞号时出现。matchedAmount 与
// passedAmount 只在这一行不可审核时出现：这一行的已匹配金额与此前已通过的金额，好让人知道还能通过多少。
type auditResponse struct {
	Outcome               string `json:"outcome"`
	Decision              string `json:"decision,omitempty"`
	Verdict               string `json:"verdict,omitempty"`
	RangeFrom             string `json:"rangeFrom,omitempty"`
	RangeTo               string `json:"rangeTo,omitempty"`
	ResolvesDispute       string `json:"resolvesDispute,omitempty"`
	Payable               string `json:"payable,omitempty"`
	DocumentNumber        string `json:"documentNumber,omitempty"`
	DocumentNumberAbsence string `json:"documentNumberAbsence,omitempty"`
	DocumentType          string `json:"documentType,omitempty"`
	Amount                string `json:"amount,omitempty"`
	Currency              string `json:"currency,omitempty"`
	MatchedAmount         string `json:"matchedAmount,omitempty"`
	PassedAmount          string `json:"passedAmount,omitempty"`
	UndecidedReason       string `json:"undecidedReason,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
}

// writeAuditAnswer 判据同上。审核授权、应付归集账户、越权升级上限三册没登是租户取值缺席，答 200 带原因；超上限要升级、这一行不可
// 审核、范围已裁决、冲突都是答案；形成应付、记下争议或拒绝答 201；其余未决是册读不动。
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
	if result.Outcome() == application.PayableNumberingRuleCollision {
		body.DocumentType = string(domain.AuditedPayableDocumentNumberType.Code)
	}
	if decision, has := result.Decision(); has {
		span := decision.Decision.Span()
		body.Decision, body.Verdict = decision.Key.Decision.String(), decision.Decision.Verdict().String()
		body.RangeFrom, body.RangeTo = minorAmount(span.From()), minorAmount(span.To())
		if resolves, resolving := decision.Decision.Resolves(); resolving {
			body.ResolvesDispute = resolves.String()
		}
	}
	if record, has := result.Payable(); has {
		currency, minor := record.Payable.Amount()
		body.Payable, body.Currency, body.Amount = record.Payable.Payable().String(), currency.String(), minorAmount(minor)
		body.DocumentNumber, body.DocumentNumberAbsence = record.Numbering.Words()
	}
	if result.Outcome() == application.LineNotAuditable {
		matched, passed := result.MatchedAmounts()
		body.MatchedAmount, body.PassedAmount = minorAmount(matched), minorAmount(passed)
	}
	status := http.StatusOK
	switch result.Outcome() {
	case application.PayableFormed, application.AuditDisputeRecorded, application.AuditRejectionRecorded:
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
