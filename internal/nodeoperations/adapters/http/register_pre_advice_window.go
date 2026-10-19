package nodeopshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件是预报有效时窗册的在线登记写面（ADR-0212 决定二末项；ADR-0085 决定一；参数登记册 PAR-NET-18）。它与受控批量口
// parcel-node-operations-register 调同一个编排；挂操作者渠道的登记册配置写能力面（ADR-0100 决定四），写开关换不了它（ADR-0150
// 决定三）。

// PreAdviceWindowRegistrationIntake 把一次已认证的时窗登记请求译成命令。租户只取认证结果。
type PreAdviceWindowRegistrationIntake interface {
	IntakePreAdviceWindowRegistration(ctx context.Context, request *http.Request) (application.RegisterPreAdviceWindowCommand, error)
}

// PreAdviceWindowRegistrar 是本口转交的登记编排，装配负责把它包进一笔事务。
type PreAdviceWindowRegistrar interface {
	Handle(ctx context.Context, command application.RegisterPreAdviceWindowCommand) (application.PreAdviceWindowRegistrationResult, error)
}

// PreAdviceWindowRegistrationPayload 是时窗登记的线格式。两个时长以分钟计、必给：缺席不是零，是没说——租户取值不替它补默认
// （ADR-0212 决定九）。effectiveUntil 可以不给，即没有终点。批量口把它原样嵌进自己的输入、另加租户。
type PreAdviceWindowRegistrationPayload struct {
	LeadMinutes    *int64     `json:"leadMinutes"`
	GraceMinutes   *int64     `json:"graceMinutes"`
	EffectiveFrom  time.Time  `json:"effectiveFrom"`
	EffectiveUntil *time.Time `json:"effectiveUntil"`
	Basis          string     `json:"basis"`
}

// maxWindowMinutes 只挡溢出：分钟数乘成 time.Duration 不能越界。它不是业务上限——多长的时窗合理是租户的事。
const maxWindowMinutes = int64(1<<63-1) / int64(time.Minute)

// Command 把载荷连同租户译成命令。缺格、时长越界、依据带 NUL 都答坏载荷；时长为负、终点不晚于起点一类交给编排里的形成门判，两个
// 入口同一道。
func (payload PreAdviceWindowRegistrationPayload) Command(tenant domain.TenantID) (application.RegisterPreAdviceWindowCommand, error) {
	none := application.RegisterPreAdviceWindowCommand{}
	malformed := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrMalformedRequest, fmt.Sprintf(format, args...))
	}
	minutes := func(name string, value *int64) (time.Duration, error) {
		if value == nil {
			return 0, malformed("%s is required; it has no default", name)
		}
		if *value > maxWindowMinutes || *value < -maxWindowMinutes {
			return 0, malformed("%s is out of range", name)
		}
		return time.Duration(*value) * time.Minute, nil
	}
	lead, err := minutes("leadMinutes", payload.LeadMinutes)
	if err != nil {
		return none, err
	}
	grace, err := minutes("graceMinutes", payload.GraceMinutes)
	if err != nil {
		return none, err
	}
	if payload.EffectiveFrom.IsZero() {
		return none, malformed("effectiveFrom is required; it has no default")
	}
	if strings.ContainsRune(payload.Basis, 0) {
		return none, malformed("basis carries NUL")
	}
	command := application.RegisterPreAdviceWindowCommand{
		TenantID:      tenant,
		Lead:          lead,
		Grace:         grace,
		EffectiveFrom: payload.EffectiveFrom,
		Basis:         payload.Basis,
	}
	if payload.EffectiveUntil != nil {
		if payload.EffectiveUntil.IsZero() {
			return none, malformed("effectiveUntil is the zero instant; leave it out for an open end")
		}
		command.EffectiveUntil = *payload.EffectiveUntil
	}
	return command, nil
}

// NewRegisterPreAdviceWindowEndpoint 交回时窗登记的 HTTP 入口。按 ADR-0022 映射：首登答 201；已有结果与区间重叠都是形成了的答案，答
// 200 带 outcome 并回显那一行。编排拒形答坏载荷；册读写不动答没形成答案，册上什么也没落。
func NewRegisterPreAdviceWindowEndpoint(intake PreAdviceWindowRegistrationIntake, registrar PreAdviceWindowRegistrar) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakePreAdviceWindowRegistration(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := registrar.Handle(request.Context(), command)
		switch {
		case errors.Is(err, application.ErrPreAdviceWindowRejected):
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		case err != nil:
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		outcome := result.Outcome().String()
		if outcome == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		status := http.StatusOK
		if result.Outcome() == application.PreAdviceWindowRegistered {
			status = http.StatusCreated
		}
		writeJSON(response, status, preAdviceWindowResponse{Outcome: outcome, Window: windowBody(result.Window())})
	})
}

// preAdviceWindowResponse 是本口的封闭响应形状：outcome 取编排结果的原名，window 是答案所指的那一行（新落的、已在册的或撞上的）。
type preAdviceWindowResponse struct {
	Outcome string                 `json:"outcome"`
	Window  preAdviceWindowDetails `json:"window"`
}

type preAdviceWindowDetails struct {
	LeadMinutes    int64      `json:"leadMinutes"`
	GraceMinutes   int64      `json:"graceMinutes"`
	EffectiveFrom  time.Time  `json:"effectiveFrom"`
	EffectiveUntil *time.Time `json:"effectiveUntil,omitempty"`
	Basis          string     `json:"basis"`
	RegisteredAt   time.Time  `json:"registeredAt"`
}

func windowBody(window domain.PreAdviceValidityWindow) preAdviceWindowDetails {
	details := preAdviceWindowDetails{
		LeadMinutes:   int64(window.Lead() / time.Minute),
		GraceMinutes:  int64(window.Grace() / time.Minute),
		EffectiveFrom: window.EffectiveFrom(),
		Basis:         window.Basis(),
		RegisteredAt:  window.RegisteredAt(),
	}
	if until, bounded := window.EffectiveUntil(); bounded {
		details.EffectiveUntil = &until
	}
	return details
}
