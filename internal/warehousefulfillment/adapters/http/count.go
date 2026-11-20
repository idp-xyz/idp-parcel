package warehousehttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// StockCountAuthenticator 认证盘点的四种运营决定。
type StockCountAuthenticator interface {
	AuthenticateStockCount(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

// StockCountIntake 先认证再读创建、开始、完成或作废。
type StockCountIntake interface {
	IntakeCreate(ctx context.Context, request *http.Request) (application.CreateCountCommand, error)
	IntakeCount(ctx context.Context, request *http.Request) (application.CountCommand, error)
}

// OperatorStockCountIntake 用运营决定面认证。
type OperatorStockCountIntake struct {
	authenticator StockCountAuthenticator
}

func NewOperatorStockCountIntake(authenticator StockCountAuthenticator) (*OperatorStockCountIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: stock count intake needs an authenticator")
	}
	return &OperatorStockCountIntake{authenticator: authenticator}, nil
}

func (intake *OperatorStockCountIntake) IntakeCreate(ctx context.Context, request *http.Request) (application.CreateCountCommand, error) {
	identity, err := intake.authenticator.AuthenticateStockCount(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.CreateCountCommand{}, err
	}
	command, err := registrationjson.CreateCountCommand(requestBody(request))
	if err != nil {
		return application.CreateCountCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

func (intake *OperatorStockCountIntake) IntakeCount(ctx context.Context, request *http.Request) (application.CountCommand, error) {
	identity, err := intake.authenticator.AuthenticateStockCount(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.CountCommand{}, err
	}
	command, err := registrationjson.CountCommand(requestBody(request))
	if err != nil {
		return application.CountCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

// UnconfiguredStockCountIntake 在运营决定渠道没配时答接入渠道未配置。
type UnconfiguredStockCountIntake struct{}

func (UnconfiguredStockCountIntake) IntakeCreate(context.Context, *http.Request) (application.CreateCountCommand, error) {
	return application.CreateCountCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredStockCountIntake) IntakeCount(context.Context, *http.Request) (application.CountCommand, error) {
	return application.CountCommand{}, ErrAccessChannelNotConfigured
}

// StockCounter 是盘点转交的编排。
type StockCounter interface {
	Create(ctx context.Context, command application.CreateCountCommand) (application.CountResult, error)
	Start(ctx context.Context, command application.CountCommand) (application.CountResult, error)
	Record(ctx context.Context, command application.RecordCountCommand) (application.CountResult, error)
	Complete(ctx context.Context, command application.CountCommand) (application.CountResult, error)
	Void(ctx context.Context, command application.CountCommand) (application.CountResult, error)
}

func NewCreateCountEndpoint(intake StockCountIntake, counter StockCounter) http.Handler {
	return countEndpoint(func(ctx context.Context, request *http.Request) (application.CountResult, error) {
		command, err := intake.IntakeCreate(ctx, request)
		if err != nil {
			return application.CountResult{}, err
		}
		return counter.Create(ctx, command)
	})
}

func NewStartCountEndpoint(intake StockCountIntake, counter StockCounter) http.Handler {
	return countStep(intake, counter, func(ctx context.Context, command application.CountCommand) (application.CountResult, error) {
		return counter.Start(ctx, command)
	})
}

func NewCompleteCountEndpoint(intake StockCountIntake, counter StockCounter) http.Handler {
	return countStep(intake, counter, func(ctx context.Context, command application.CountCommand) (application.CountResult, error) {
		return counter.Complete(ctx, command)
	})
}

func NewVoidCountEndpoint(intake StockCountIntake, counter StockCounter) http.Handler {
	return countStep(intake, counter, func(ctx context.Context, command application.CountCommand) (application.CountResult, error) {
		return counter.Void(ctx, command)
	})
}

func countStep(intake StockCountIntake, _ StockCounter, run func(context.Context, application.CountCommand) (application.CountResult, error)) http.Handler {
	return countEndpoint(func(ctx context.Context, request *http.Request) (application.CountResult, error) {
		command, err := intake.IntakeCount(ctx, request)
		if err != nil {
			return application.CountResult{}, err
		}
		return run(ctx, command)
	})
}

func countEndpoint(run func(context.Context, *http.Request) (application.CountResult, error)) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		result, err := run(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		writeCountAnswer(response, request, result)
	})
}

func writeCountAnswer(response http.ResponseWriter, request *http.Request, result application.CountResult) {
	name := result.Outcome().String()
	if name == "" {
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := countAnswer{Outcome: name, Block: result.Block().String(), Location: result.Location(), Status: result.Status(), Missing: result.Missing()}
	if count, ok := result.Count(); ok {
		answer.CountID = count.ID().String()
	}
	for _, issue := range result.Issues() {
		answer.Issues = append(answer.Issues, freezeIssueBody{Kind: issue.Kind().String()})
	}
	status := http.StatusOK
	if result.Outcome() == application.CountCreated || result.Outcome() == application.CountStarted || result.Outcome() == application.CountRecorded || result.Outcome() == application.CountCounted || result.Outcome() == application.CountVoided {
		status = http.StatusCreated
	}
	writeJSON(response, status, answer)
}

// CountEntryIntake 先认证再读清点结果。事实种类由装配绑成 COUNT_ENTRY。
type CountEntryIntake struct {
	authenticator InternalJobAuthenticator
}

func NewCountEntryIntake(authenticator InternalJobAuthenticator) (*CountEntryIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: count entry intake needs an authenticator")
	}
	return &CountEntryIntake{authenticator: authenticator}, nil
}

func (intake *CountEntryIntake) Intake(ctx context.Context, request *http.Request) (application.RecordCountCommand, error) {
	device, scopeKind, reference, err := warehouseWorkScope(request)
	if err != nil {
		return application.RecordCountCommand{}, err
	}
	tenant, operator, recordedDevice, err := intake.authenticator.AuthenticateInternalJob(ctx, httpapi.BearerToken(request), device, scopeKind, reference)
	if err != nil {
		return application.RecordCountCommand{}, err
	}
	command, err := registrationjson.RecordCountCommand(requestBody(request))
	if err != nil {
		return application.RecordCountCommand{}, asMalformed(err)
	}
	if reference != command.Warehouse.String() {
		return application.RecordCountCommand{}, &malformedRequest{reason: httpapi.HeaderWorkScopeReference + " must equal warehouse"}
	}
	command.Tenant = tenant
	command.Operator = operator
	command.Device = recordedDevice
	return command, nil
}

type UnconfiguredCountEntryIntake struct{}

func (UnconfiguredCountEntryIntake) Intake(context.Context, *http.Request) (application.RecordCountCommand, error) {
	return application.RecordCountCommand{}, ErrAccessChannelNotConfigured
}

func NewRecordCountEndpoint(intake interface {
	Intake(context.Context, *http.Request) (application.RecordCountCommand, error)
}, counter StockCounter) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.Intake(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := counter.Record(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeCountAnswer(response, request, result)
	})
}

// CountApprover 是批准与驳回盘点差额、以及登记批准职责规则转交的编排。
type CountApprover interface {
	Approve(ctx context.Context, command application.ApproveCountCommand) (application.ApprovalResult, error)
	Reject(ctx context.Context, command application.RejectCountCommand) (application.ApprovalResult, error)
	RegisterDuty(ctx context.Context, command application.RegisterApprovalDutyCommand) (application.ApprovalResult, error)
}

// StockApprovalAuthenticator 认证库存调整批准。
type StockApprovalAuthenticator interface {
	AuthenticateStockApproval(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

// StockApprovalIntake 先认证再读批准或驳回。
type StockApprovalIntake interface {
	IntakeApprove(ctx context.Context, request *http.Request) (application.ApproveCountCommand, error)
	IntakeReject(ctx context.Context, request *http.Request) (application.RejectCountCommand, error)
	IntakeApproveAdjustment(ctx context.Context, request *http.Request) (application.ApproveAdjustmentCommand, error)
	IntakeRejectAdjustment(ctx context.Context, request *http.Request) (application.RejectAdjustmentCommand, error)
	IntakeApproveJobDiscrepancy(ctx context.Context, request *http.Request) (application.ApproveCountCommand, error)
	IntakeRejectJobDiscrepancy(ctx context.Context, request *http.Request) (application.RejectCountCommand, error)
}

type OperatorStockApprovalIntake struct {
	authenticator StockApprovalAuthenticator
}

func NewOperatorStockApprovalIntake(authenticator StockApprovalAuthenticator) (*OperatorStockApprovalIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: stock approval intake needs an authenticator")
	}
	return &OperatorStockApprovalIntake{authenticator: authenticator}, nil
}

func (intake *OperatorStockApprovalIntake) IntakeApprove(ctx context.Context, request *http.Request) (application.ApproveCountCommand, error) {
	identity, err := intake.authenticator.AuthenticateStockApproval(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.ApproveCountCommand{}, err
	}
	command, err := registrationjson.ApproveCountCommand(requestBody(request))
	if err != nil {
		return application.ApproveCountCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

func (intake *OperatorStockApprovalIntake) IntakeApproveAdjustment(ctx context.Context, request *http.Request) (application.ApproveAdjustmentCommand, error) {
	identity, err := intake.authenticator.AuthenticateStockApproval(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.ApproveAdjustmentCommand{}, err
	}
	command, err := registrationjson.ApproveAdjustmentCommand(requestBody(request))
	if err != nil {
		return application.ApproveAdjustmentCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

func (intake *OperatorStockApprovalIntake) IntakeRejectAdjustment(ctx context.Context, request *http.Request) (application.RejectAdjustmentCommand, error) {
	identity, err := intake.authenticator.AuthenticateStockApproval(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.RejectAdjustmentCommand{}, err
	}
	command, err := registrationjson.RejectAdjustmentCommand(requestBody(request))
	if err != nil {
		return application.RejectAdjustmentCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

func (intake *OperatorStockApprovalIntake) IntakeApproveJobDiscrepancy(ctx context.Context, request *http.Request) (application.ApproveCountCommand, error) {
	identity, err := intake.authenticator.AuthenticateStockApproval(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.ApproveCountCommand{}, err
	}
	command, err := registrationjson.ApproveJobDiscrepancyCommand(requestBody(request))
	if err != nil {
		return application.ApproveCountCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

func (intake *OperatorStockApprovalIntake) IntakeRejectJobDiscrepancy(ctx context.Context, request *http.Request) (application.RejectCountCommand, error) {
	identity, err := intake.authenticator.AuthenticateStockApproval(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.RejectCountCommand{}, err
	}
	command, err := registrationjson.RejectJobDiscrepancyCommand(requestBody(request))
	if err != nil {
		return application.RejectCountCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

func (intake *OperatorStockApprovalIntake) IntakeReject(ctx context.Context, request *http.Request) (application.RejectCountCommand, error) {
	identity, err := intake.authenticator.AuthenticateStockApproval(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.RejectCountCommand{}, err
	}
	command, err := registrationjson.RejectCountCommand(requestBody(request))
	if err != nil {
		return application.RejectCountCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

type UnconfiguredStockApprovalIntake struct{}

func (UnconfiguredStockApprovalIntake) IntakeApprove(context.Context, *http.Request) (application.ApproveCountCommand, error) {
	return application.ApproveCountCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredStockApprovalIntake) IntakeReject(context.Context, *http.Request) (application.RejectCountCommand, error) {
	return application.RejectCountCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredStockApprovalIntake) IntakeApproveAdjustment(context.Context, *http.Request) (application.ApproveAdjustmentCommand, error) {
	return application.ApproveAdjustmentCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredStockApprovalIntake) IntakeRejectAdjustment(context.Context, *http.Request) (application.RejectAdjustmentCommand, error) {
	return application.RejectAdjustmentCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredStockApprovalIntake) IntakeApproveJobDiscrepancy(context.Context, *http.Request) (application.ApproveCountCommand, error) {
	return application.ApproveCountCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredStockApprovalIntake) IntakeRejectJobDiscrepancy(context.Context, *http.Request) (application.RejectCountCommand, error) {
	return application.RejectCountCommand{}, ErrAccessChannelNotConfigured
}

func NewRegisterApprovalDutyEndpoint(intake ApprovalDutyIntake, approver CountApprover) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeApprovalDuty(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := approver.RegisterDuty(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeApprovalAnswer(response, request, result)
	})
}

func NewApproveCountEndpoint(intake StockApprovalIntake, approver CountApprover) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeApprove(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := approver.Approve(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeApprovalAnswer(response, request, result)
	})
}

// JobDiscrepancyApprover 是批准与驳回库内作业差异转交的编排。
type JobDiscrepancyApprover interface {
	Approve(ctx context.Context, command application.ApproveCountCommand) (application.ApprovalResult, error)
	Reject(ctx context.Context, command application.RejectCountCommand) (application.ApprovalResult, error)
}

func NewApproveJobDiscrepancyEndpoint(intake StockApprovalIntake, approver JobDiscrepancyApprover) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeApproveJobDiscrepancy(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := approver.Approve(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeApprovalAnswer(response, request, result)
	})
}

func NewRejectJobDiscrepancyEndpoint(intake StockApprovalIntake, approver JobDiscrepancyApprover) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeRejectJobDiscrepancy(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := approver.Reject(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeApprovalAnswer(response, request, result)
	})
}

func NewRejectCountEndpoint(intake StockApprovalIntake, approver CountApprover) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeReject(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := approver.Reject(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeApprovalAnswer(response, request, result)
	})
}

// StockAdjustmentAuthenticator 认证库存调整的提交。
type StockAdjustmentAuthenticator interface {
	AuthenticateStockAdjustment(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

// StockAdjustmentIntake 先认证再读提交。
type StockAdjustmentIntake interface {
	IntakeSubmit(ctx context.Context, request *http.Request) (application.SubmitAdjustmentCommand, error)
}

type OperatorStockAdjustmentIntake struct {
	authenticator StockAdjustmentAuthenticator
}

func NewOperatorStockAdjustmentIntake(authenticator StockAdjustmentAuthenticator) (*OperatorStockAdjustmentIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: stock adjustment intake needs an authenticator")
	}
	return &OperatorStockAdjustmentIntake{authenticator: authenticator}, nil
}

func (intake *OperatorStockAdjustmentIntake) IntakeSubmit(ctx context.Context, request *http.Request) (application.SubmitAdjustmentCommand, error) {
	identity, err := intake.authenticator.AuthenticateStockAdjustment(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.SubmitAdjustmentCommand{}, err
	}
	command, err := registrationjson.SubmitAdjustmentCommand(requestBody(request))
	if err != nil {
		return application.SubmitAdjustmentCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

type UnconfiguredStockAdjustmentIntake struct{}

func (UnconfiguredStockAdjustmentIntake) IntakeSubmit(context.Context, *http.Request) (application.SubmitAdjustmentCommand, error) {
	return application.SubmitAdjustmentCommand{}, ErrAccessChannelNotConfigured
}

// StockAdjuster 是库存调整转交的编排。
type StockAdjuster interface {
	Submit(ctx context.Context, command application.SubmitAdjustmentCommand) (application.AdjustmentResult, error)
	Approve(ctx context.Context, command application.ApproveAdjustmentCommand) (application.AdjustmentResult, error)
	Reject(ctx context.Context, command application.RejectAdjustmentCommand) (application.AdjustmentResult, error)
}

func NewSubmitAdjustmentEndpoint(intake StockAdjustmentIntake, adjuster StockAdjuster) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeSubmit(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := adjuster.Submit(request.Context(), command)
		if errors.Is(err, application.ErrOwnerAccountNotInTenant) {
			writeProblem(response, http.StatusNotFound, codeOwnerAccountNotVisible)
			return
		}
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeAdjustmentAnswer(response, request, result)
	})
}

func NewApproveAdjustmentEndpoint(intake StockApprovalIntake, adjuster StockAdjuster) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeApproveAdjustment(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := adjuster.Approve(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeAdjustmentAnswer(response, request, result)
	})
}

func NewRejectAdjustmentEndpoint(intake StockApprovalIntake, adjuster StockAdjuster) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeRejectAdjustment(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := adjuster.Reject(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeAdjustmentAnswer(response, request, result)
	})
}

func writeAdjustmentAnswer(response http.ResponseWriter, request *http.Request, result application.AdjustmentResult) {
	name := result.Outcome().String()
	if name == "" {
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	status := http.StatusOK
	if result.Outcome() == application.ApprovalSubmitted || result.Outcome() == application.ApprovalPosted || result.Outcome() == application.ApprovalRejected {
		status = http.StatusCreated
	}
	body := countAnswer{Outcome: name, Status: result.Status()}
	for _, issue := range result.Issues() {
		body.Issues = append(body.Issues, freezeIssueBody{Kind: issue.Kind().String()})
	}
	writeJSON(response, status, body)
}

func writeApprovalAnswer(response http.ResponseWriter, request *http.Request, result application.ApprovalResult) {
	name := result.Outcome().String()
	if name == "" {
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	status := http.StatusOK
	if result.Outcome() == application.ApprovalPosted || result.Outcome() == application.ApprovalRejected || result.Outcome() == application.ApprovalDutyRegistered {
		status = http.StatusCreated
	}
	writeJSON(response, status, countAnswer{Outcome: name, Status: result.Status()})
}

type countAnswer struct {
	Outcome  string            `json:"outcome"`
	CountID  string            `json:"countId,omitempty"`
	Block    string            `json:"block,omitempty"`
	Location string            `json:"location,omitempty"`
	Status   string            `json:"status,omitempty"`
	Missing  []string          `json:"missing,omitempty"`
	Issues   []freezeIssueBody `json:"issues,omitempty"`
}
