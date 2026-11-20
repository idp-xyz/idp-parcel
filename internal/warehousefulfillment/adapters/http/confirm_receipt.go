package warehousehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// ReceiptConfirmationAuthenticator 铸收货确认的作业事实信封。种类与引用由 Intake 先核对，这里只铸造。
type ReceiptConfirmationAuthenticator interface {
	AuthenticateReceiptConfirmation(
		ctx context.Context,
		bearerToken, deviceID, scopeKind, scopeReference string,
	) (domain.TenantID, domain.ReceiptOperatorID, domain.ReceiptDeviceID, error)
}

// ReceiptConfirmationIntake 是收货确认口的 Intake。
type ReceiptConfirmationIntake interface {
	IntakeReceiptConfirmation(ctx context.Context, request *http.Request) (application.ConfirmReceiptCommand, error)
}

// ReceiptConfirmer 是收货确认转交的编排。事务边界在编排侧给出。判定次序在 ConfirmReceiptHandler，这里不重写。
type ReceiptConfirmer interface {
	Confirm(ctx context.Context, command application.ConfirmReceiptCommand) (application.ConfirmReceiptResult, error)
}

var _ ReceiptConfirmer = (*application.ConfirmReceiptHandler)(nil)

// ReceiptWorkFactIntake 读作业事实请求头与载荷，认证之后把租户、操作者与设备填进命令。
type ReceiptWorkFactIntake struct {
	authenticator ReceiptConfirmationAuthenticator
}

func NewReceiptWorkFactIntake(authenticator ReceiptConfirmationAuthenticator) (*ReceiptWorkFactIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: receipt confirmation intake needs an authenticator")
	}
	return &ReceiptWorkFactIntake{authenticator: authenticator}, nil
}

var _ ReceiptConfirmationIntake = (*ReceiptWorkFactIntake)(nil)

func (intake *ReceiptWorkFactIntake) IntakeReceiptConfirmation(
	ctx context.Context,
	request *http.Request,
) (application.ConfirmReceiptCommand, error) {
	none := application.ConfirmReceiptCommand{}
	device, kind, reference, err := warehouseWorkScope(request)
	if err != nil {
		return none, err
	}
	body := request.Body
	if body == nil {
		body = http.NoBody
	}
	command, err := registrationjson.ReceiptConfirmationCommand(body)
	if err != nil {
		var malformed *registrationjson.MalformedDocumentError
		if errors.As(err, &malformed) {
			return none, &malformedRequest{reason: malformed.Reason}
		}
		return none, err
	}
	// 作业范围引用必须等于载荷里的仓。不等是坏载荷，不读册：读了就会让一份对不上的请求探出这台设备登没登。
	if reference != command.Warehouse.String() {
		return none, &malformedRequest{reason: httpapi.HeaderWorkScopeReference + " must equal warehouse"}
	}
	tenant, operator, recordedDevice, err := intake.authenticator.AuthenticateReceiptConfirmation(
		ctx, httpapi.BearerToken(request), device, kind, reference,
	)
	if err != nil {
		return none, err
	}
	command.Tenant = tenant
	command.Operator = operator
	command.Device = recordedDevice
	return command, nil
}

// warehouseWorkScope 读作业事实面的三个头。缺一个，或种类不是仓，都是坏载荷，不读册。
// 头名与读法在 httpapi.ReadWorkFactHeaders，这里只认仓（ADR-0384 决定三的 WAREHOUSE）。
func warehouseWorkScope(request *http.Request) (device, kind, reference string, err error) {
	device, kind, reference, complete := httpapi.ReadWorkFactHeaders(request)
	if !complete {
		return "", "", "", &malformedRequest{reason: httpapi.HeaderWorkDevice + ", " + httpapi.HeaderWorkScopeKind + " and " + httpapi.HeaderWorkScopeReference + " are required"}
	}
	if kind != "WAREHOUSE" {
		return "", "", "", &malformedRequest{reason: httpapi.HeaderWorkScopeKind + " must be WAREHOUSE"}
	}
	return device, kind, reference, nil
}

// UnconfiguredReceiptIntake 在作业事实渠道没配时答接入渠道未配置，不读载荷。
type UnconfiguredReceiptIntake struct{}

func (UnconfiguredReceiptIntake) IntakeReceiptConfirmation(context.Context, *http.Request) (application.ConfirmReceiptCommand, error) {
	return application.ConfirmReceiptCommand{}, ErrAccessChannelNotConfigured
}

var _ ReceiptConfirmationIntake = UnconfiguredReceiptIntake{}

type confirmReceiptAnswer struct {
	Outcome string       `json:"outcome"`
	Receipt *receiptBody `json:"receipt,omitempty"`
}

// NewConfirmReceiptEndpoint 交回收货确认的 HTTP 入口（POST /warehouse-fulfillment-receipt-confirmations）。
// 新建答 201，其余业务结果答 200，outcome 用核心的字面量。问题体用作业事实族的码，写成问题详情：
// 这一行不沿用运营口的 {"error":{"code"}}。
func NewConfirmReceiptEndpoint(intake ReceiptConfirmationIntake, confirmer ReceiptConfirmer) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeReceiptConfirmation(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := confirmer.Confirm(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "receipt confirmation did not form an answer",
				"warehouse", command.Warehouse.String(), "factId", command.FactID.String())
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeReceiptAnswer(response, request, result)
	})
}

func writeReceiptAnswer(response http.ResponseWriter, request *http.Request, result application.ConfirmReceiptResult) {
	name := result.Outcome().String()
	if name == "" {
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := confirmReceiptAnswer{Outcome: name}
	status := http.StatusOK
	if result.Outcome() == application.ConfirmReceiptConfirmed {
		status = http.StatusCreated
	}
	receipt, has := result.Receipt()
	switch result.Outcome() {
	case application.ConfirmReceiptConfirmed, application.ConfirmReceiptExistingResult, application.ConfirmReceiptDivertedToUnclaimed:
		if !has {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		body := receiptBodies([]domain.RecordedReceipt{receipt}, nil, false, true)[0]
		answer.Receipt = &body
	}
	writeJSON(response, status, answer)
}

// writeReceiptProblem 把 Intake 的错写成问题详情。码与状态对照节点作业的作业事实口：未授予、设备未登记或已停用、
// 不在准入范围、令牌无效、渠道未配置各是一格。400 可以带只谈这份载荷的 detail，403 与 5xx 不带。
func writeReceiptProblem(response http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, ErrAccessChannelNotConfigured):
		httpapi.WriteProblem(response, request, http.StatusForbidden, codeAccessChannelNotConfigured)
	case errors.Is(err, ErrOperatorCredentialRejected):
		httpapi.WriteProblem(response, request, http.StatusUnauthorized, codeOperatorCredentialRejected)
	case errors.Is(err, ErrOperatorNotGranted):
		httpapi.WriteProblem(response, request, http.StatusForbidden, codeOperatorNotGranted)
	case errors.Is(err, ErrDeviceNotInUse):
		httpapi.WriteProblem(response, request, http.StatusForbidden, codeDeviceNotInUse)
	case errors.Is(err, ErrOutsideAdmissionScope):
		httpapi.WriteProblem(response, request, http.StatusForbidden, codeOutsideAdmissionScope)
	case errors.Is(err, ErrIdentityDependencyUnavailable):
		httpapi.WriteProblem(response, request, http.StatusServiceUnavailable, codeIdentityDependencyUnavailable)
	case errors.Is(err, ErrMalformedRequest):
		var malformed *malformedRequest
		if errors.As(err, &malformed) {
			httpapi.WriteProblemWithDetail(response, request, http.StatusBadRequest, codeMalformedRequest, malformed.reason)
			return
		}
		httpapi.WriteProblem(response, request, http.StatusBadRequest, codeMalformedRequest)
	default:
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeIntakeFailed)
	}
}
