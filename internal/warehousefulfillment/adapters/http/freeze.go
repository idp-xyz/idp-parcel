package warehousehttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// StockFreezeAuthenticator 认证库存冻结与解冻。
type StockFreezeAuthenticator interface {
	AuthenticateStockFreeze(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

// StockFreezeIntake 先认证再读冻结或解冻的载荷。
type StockFreezeIntake interface {
	IntakeFreeze(ctx context.Context, request *http.Request) (application.FreezeCommand, error)
	IntakeUnfreeze(ctx context.Context, request *http.Request) (application.UnfreezeCommand, error)
}

// OperatorStockFreezeIntake 用运营决定面认证。
type OperatorStockFreezeIntake struct {
	authenticator StockFreezeAuthenticator
}

func NewOperatorStockFreezeIntake(authenticator StockFreezeAuthenticator) (*OperatorStockFreezeIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: stock freeze intake needs an authenticator")
	}
	return &OperatorStockFreezeIntake{authenticator: authenticator}, nil
}

func (intake *OperatorStockFreezeIntake) IntakeFreeze(ctx context.Context, request *http.Request) (application.FreezeCommand, error) {
	identity, err := intake.authenticator.AuthenticateStockFreeze(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.FreezeCommand{}, err
	}
	command, err := registrationjson.FreezeCommand(requestBody(request))
	if err != nil {
		return application.FreezeCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

func (intake *OperatorStockFreezeIntake) IntakeUnfreeze(ctx context.Context, request *http.Request) (application.UnfreezeCommand, error) {
	identity, err := intake.authenticator.AuthenticateStockFreeze(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.UnfreezeCommand{}, err
	}
	command, err := registrationjson.UnfreezeCommand(requestBody(request))
	if err != nil {
		return application.UnfreezeCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

// UnconfiguredStockFreezeIntake 在运营决定渠道没配时答接入渠道未配置。
type UnconfiguredStockFreezeIntake struct{}

func (UnconfiguredStockFreezeIntake) IntakeFreeze(context.Context, *http.Request) (application.FreezeCommand, error) {
	return application.FreezeCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredStockFreezeIntake) IntakeUnfreeze(context.Context, *http.Request) (application.UnfreezeCommand, error) {
	return application.UnfreezeCommand{}, ErrAccessChannelNotConfigured
}

// StockFreezer 是冻结与解冻转交的编排。
type StockFreezer interface {
	Freeze(ctx context.Context, command application.FreezeCommand) (application.FreezeResult, error)
	Unfreeze(ctx context.Context, command application.UnfreezeCommand) (application.FreezeResult, error)
}

func NewFreezeEndpoint(intake StockFreezeIntake, freezer StockFreezer) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeFreeze(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := freezer.Freeze(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeFreezeAnswer(response, request, result)
	})
}

func NewUnfreezeEndpoint(intake StockFreezeIntake, freezer StockFreezer) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeUnfreeze(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := freezer.Unfreeze(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeFreezeAnswer(response, request, result)
	})
}

func writeFreezeAnswer(response http.ResponseWriter, request *http.Request, result application.FreezeResult) {
	name := result.Outcome().String()
	if name == "" {
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := freezeAnswer{Outcome: name, Refusal: result.Refusal().String()}
	if freeze, ok := result.Freeze(); ok {
		answer.FreezeID = freeze.ID().String()
	}
	for _, issue := range result.Issues() {
		answer.Issues = append(answer.Issues, freezeIssueBody{Kind: issue.Kind().String()})
	}
	status := http.StatusOK
	if result.Outcome() == application.FreezeFrozen || result.Outcome() == application.FreezeUnfrozen {
		status = http.StatusCreated
	}
	writeJSON(response, status, answer)
}

type freezeAnswer struct {
	Outcome  string            `json:"outcome"`
	FreezeID string            `json:"freezeId,omitempty"`
	Refusal  string            `json:"refusal,omitempty"`
	Issues   []freezeIssueBody `json:"issues,omitempty"`
}

type freezeIssueBody struct {
	Kind string `json:"kind"`
}
