package warehousehttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// WaveRuleIntake 是成波规则登记的 Intake。规则是租户按仓登记的取值，挂主数据登记，不挂运营决定。
type WaveRuleIntake interface {
	IntakeWaveRule(ctx context.Context, request *http.Request) (application.RegisterWaveRuleCommand, error)
}

func (intake *OperatorRegistryIntake) IntakeWaveRule(ctx context.Context, request *http.Request) (application.RegisterWaveRuleCommand, error) {
	return intakeRegistration(ctx, request, intake.authenticator, registrationjson.RegisterWaveRuleCommand)
}

func (UnconfiguredIntake) IntakeWaveRule(context.Context, *http.Request) (application.RegisterWaveRuleCommand, error) {
	return application.RegisterWaveRuleCommand{}, ErrAccessChannelNotConfigured
}

// NewRegisterWaveRuleEndpoint 交回登记成波规则的入口（POST /warehouse-fulfillment-wave-rules）。
func NewRegisterWaveRuleEndpoint(intake WaveRuleIntake, waves WaveService) http.Handler {
	return toteEndpoint(func(request *http.Request) (application.WaveResult, error) {
		command, err := intake.IntakeWaveRule(request.Context(), request)
		if err != nil {
			return application.WaveResult{}, &toteIntakeError{err}
		}
		return waves.RegisterWaveRule(request.Context(), command)
	})
}

func (intake *OperatorWaveIntake) IntakeWaveRuleRun(ctx context.Context, request *http.Request) (application.RunWaveRuleCommand, error) {
	identity, err := intake.authenticator.AuthenticateWaveRelease(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.RunWaveRuleCommand{}, err
	}
	command, err := registrationjson.RunWaveRuleCommand(requestBody(request))
	if err != nil {
		return application.RunWaveRuleCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

func (UnconfiguredWaveIntake) IntakeWaveRuleRun(context.Context, *http.Request) (application.RunWaveRuleCommand, error) {
	return application.RunWaveRuleCommand{}, ErrAccessChannelNotConfigured
}

// NewRunWaveRuleEndpoint 交回按规则成波的入口（POST /warehouse-fulfillment-wave-rule-runs）。
// 它就是放波次，只是单由规则圈：与放波次挂同一种运营决定。答复带着放出的波次与三个数——
// 放进波次的、圈到了而分配不出的、超过一次上限没轮到的——人要照它决定要不要再点一次、去看哪几张单。
func NewRunWaveRuleEndpoint(intake WaveReleaseIntake, waves WaveService) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeWaveRuleRun(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := waves.RunWaveRule(request.Context(), command)
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
		if result.Outcome() == application.WaveAllocated {
			status = http.StatusCreated
		}
		waveBodies := make([]waveRuleRunWaveBody, 0, len(result.Waves()))
		for _, wave := range result.Waves() {
			waveBodies = append(waveBodies, waveRuleRunWaveBody{WaveID: wave.Wave, Requests: wave.Requests})
		}
		writeJSON(response, status, waveRuleRunAnswer{
			Outcome: name, RuleRevision: result.Revision(), Waves: waveBodies,
			Released: result.Released(), Skipped: result.Skipped(), Remaining: result.Remaining(),
		})
	})
}

type waveRuleRunAnswer struct {
	Outcome string `json:"outcome"`
	// RuleRevision 是这一次照的是规则的哪一版；没走到读规则那一步时不带。
	RuleRevision int                   `json:"ruleRevision,omitempty"`
	Waves        []waveRuleRunWaveBody `json:"waves"`
	// Released 是放进波次的出库请求数，Skipped 是圈到了而分配不出、留在已预占的，
	// Remaining 是超过一次执行的上限、这一次没轮到的。
	Released  int `json:"released"`
	Skipped   int `json:"skipped"`
	Remaining int `json:"remaining"`
}

type waveRuleRunWaveBody struct {
	WaveID   string `json:"waveId"`
	Requests int    `json:"requests"`
}
