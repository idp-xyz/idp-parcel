package warehousehttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

func (intake *OperatorWaveIntake) IntakeWaveAssignment(ctx context.Context, request *http.Request) (application.AssignWaveCommand, error) {
	identity, err := intake.authenticator.AuthenticateWaveRelease(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.AssignWaveCommand{}, err
	}
	command, err := registrationjson.AssignWaveCommand(requestBody(request))
	if err != nil {
		return application.AssignWaveCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

func (UnconfiguredWaveIntake) IntakeWaveAssignment(context.Context, *http.Request) (application.AssignWaveCommand, error) {
	return application.AssignWaveCommand{}, ErrAccessChannelNotConfigured
}

// IntakeClaim 先认证再读领波次的载荷。领波次挂拣货这一种作业事实：领的就是去拣它，能拣货的人就能领。
// 领的人是认证交回的操作者，与管理台各册上记操作者的是同一个串。
func (intake *PickIntake) IntakeClaim(ctx context.Context, request *http.Request) (application.ClaimWaveCommand, error) {
	tenant, operator, reference, err := admitWarehouseWorker(ctx, request, intake.authenticator)
	if err != nil {
		return application.ClaimWaveCommand{}, err
	}
	command, err := registrationjson.ClaimWaveCommand(requestBody(request))
	if err != nil {
		return application.ClaimWaveCommand{}, asMalformed(err)
	}
	if reference != command.Warehouse.String() {
		return application.ClaimWaveCommand{}, &malformedRequest{reason: httpapi.HeaderWorkScopeReference + " must equal warehouse"}
	}
	command.Tenant = tenant
	command.Operator = operator
	return command, nil
}

func (UnconfiguredPickIntake) IntakeClaim(context.Context, *http.Request) (application.ClaimWaveCommand, error) {
	return application.ClaimWaveCommand{}, ErrAccessChannelNotConfigured
}

// NewAssignWaveEndpoint 交回指派拣货人的入口（POST /warehouse-fulfillment-wave-assignments）。
// 与放波次挂同一种运营决定：能放波次的人，也能定它由谁拣。
func NewAssignWaveEndpoint(intake WaveReleaseIntake, waves WaveService) http.Handler {
	return waveAssignmentEndpoint(func(request *http.Request) (application.WaveResult, error) {
		command, err := intake.IntakeWaveAssignment(request.Context(), request)
		if err != nil {
			return application.WaveResult{}, &toteIntakeError{err}
		}
		return waves.AssignWave(request.Context(), command)
	})
}

// NewClaimWaveEndpoint 交回拣货的人领波次的入口（POST /warehouse-fulfillment-wave-claims）。
func NewClaimWaveEndpoint(intake interface {
	IntakeClaim(context.Context, *http.Request) (application.ClaimWaveCommand, error)
}, waves WaveService) http.Handler {
	return waveAssignmentEndpoint(func(request *http.Request) (application.WaveResult, error) {
		command, err := intake.IntakeClaim(request.Context(), request)
		if err != nil {
			return application.WaveResult{}, &toteIntakeError{err}
		}
		return waves.ClaimWave(request.Context(), command)
	})
}

// waveAssignmentEndpoint 的答复带着这个波次此刻派给了谁：领不成时人要知道它在谁手里。
func waveAssignmentEndpoint(run func(*http.Request) (application.WaveResult, error)) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		result, err := run(request)
		if intake, ok := err.(*toteIntakeError); ok {
			writeReceiptProblem(response, request, intake.err)
			return
		}
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		name := result.Outcome().String()
		if name == "" {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		status := http.StatusOK
		if result.Outcome() == application.WaveAssigned || result.Outcome() == application.WaveUnassigned {
			status = http.StatusCreated
		}
		writeJSON(response, status, waveAssignmentAnswer{Outcome: name, Assignee: result.Assignee()})
	})
}

type waveAssignmentAnswer struct {
	Outcome string `json:"outcome"`
	// Assignee 是这个波次此刻派给了谁；没派不带。
	Assignee string `json:"assignee,omitempty"`
}
