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

func (intake *OperatorWaveIntake) IntakeDueWaveRules(ctx context.Context, request *http.Request) (application.RunDueWaveRulesCommand, error) {
	identity, err := intake.authenticator.AuthenticateWaveRelease(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.RunDueWaveRulesCommand{}, err
	}
	command, err := registrationjson.RunDueWaveRulesCommand(requestBody(request))
	if err != nil {
		return application.RunDueWaveRulesCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

func (UnconfiguredWaveIntake) IntakeDueWaveRules(context.Context, *http.Request) (application.RunDueWaveRulesCommand, error) {
	return application.RunDueWaveRulesCommand{}, ErrAccessChannelNotConfigured
}

// NewRunDueWaveRulesEndpoint 交回「放到点的规则」的入口（POST /warehouse-fulfillment-wave-rule-due-runs）。
// 产品里没有定时器：这一口由部署方的定时任务带着一个有放波次授予的操作者凭证按时来调，每次只放此刻已经到点、
// 还没办过的时点。它与放波次挂同一种运营决定——放出的波次记在来调的那个身份名下。
func NewRunDueWaveRulesEndpoint(intake WaveReleaseIntake, waves WaveService) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeDueWaveRules(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := waves.RunDueWaveRules(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		name := result.Outcome().String()
		if name == "" {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		answer := dueWaveRulesAnswer{Outcome: name, Runs: []dueWaveRuleRunBody{}, Unscheduled: []dueWaveRuleUnscheduledBody{}}
		for _, run := range result.Runs() {
			waveBodies := make([]waveRuleRunWaveBody, 0, len(run.Result.Waves()))
			for _, wave := range run.Result.Waves() {
				waveBodies = append(waveBodies, waveRuleRunWaveBody{WaveID: wave.Wave, Requests: wave.Requests})
			}
			answer.Runs = append(answer.Runs, dueWaveRuleRunBody{
				WarehouseCode: run.Warehouse, RuleCode: run.Rule, Date: run.Date, Slot: run.Slot, RunID: run.RunID,
				Outcome: run.Result.Outcome().String(), Waves: waveBodies,
				Released: run.Result.Released(), Skipped: run.Result.Skipped(), Remaining: run.Result.Remaining(),
			})
		}
		for _, rule := range result.Unscheduled() {
			answer.Unscheduled = append(answer.Unscheduled, dueWaveRuleUnscheduledBody{WarehouseCode: rule.Warehouse, RuleCode: rule.Rule})
		}
		status := http.StatusOK
		if result.Outcome() == application.WaveDueRulesRun {
			status = http.StatusCreated
		}
		writeJSON(response, status, answer)
	})
}

type dueWaveRulesAnswer struct {
	// Outcome 是 DUE_RULES_RUN（办了至少一个时点）或 NOTHING_DUE（此刻没有到点而没办过的）。
	Outcome string               `json:"outcome"`
	Runs    []dueWaveRuleRunBody `json:"runs"`
	// Unscheduled 是登了时点、但仓的业务时区没登记所以算不出到没到点的规则：未配置。
	Unscheduled []dueWaveRuleUnscheduledBody `json:"unscheduled"`
}

type dueWaveRuleRunBody struct {
	WarehouseCode string `json:"warehouseCode"`
	RuleCode      string `json:"ruleCode"`
	// Date 与 Slot 是这个时点所在的业务日（YYYY-MM-DD）与时点（HH:MM），按仓的业务时区。
	Date  string `json:"date"`
	Slot  string `json:"slot"`
	RunID string `json:"runId"`
	// Outcome 是这个时点成波的结果，与按规则成波那一口的词相同。
	Outcome   string                `json:"outcome"`
	Waves     []waveRuleRunWaveBody `json:"waves"`
	Released  int                   `json:"released"`
	Skipped   int                   `json:"skipped"`
	Remaining int                   `json:"remaining"`
}

type dueWaveRuleUnscheduledBody struct {
	WarehouseCode string `json:"warehouseCode"`
	RuleCode      string `json:"ruleCode"`
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
