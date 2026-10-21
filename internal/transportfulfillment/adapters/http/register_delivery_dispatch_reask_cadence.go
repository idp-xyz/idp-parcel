package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 派送重问间隔的登记口（ADR-0230 决定五）：登记册配置写，租户登自己的重问间隔。产品不给默认，没登过的租户到期重问答未配置。

// ReaskCadenceIntake 把已认证的一次登记翻成命令：租户取认证结果。
type ReaskCadenceIntake interface {
	IntakeReaskCadenceRegistration(ctx context.Context, request *http.Request) (application.RegisterReaskCadenceCommand, error)
}

// ReaskCadenceRegistrationPayload 是线格式：登记方指名的版本与按整分钟的间隔。间隔不是正的整分钟由编排答`未受理`（200）。
type ReaskCadenceRegistrationPayload struct {
	Version         string `json:"version"`
	IntervalMinutes int    `json:"intervalMinutes"`
}

func (payload ReaskCadenceRegistrationPayload) Command(tenant domain.TenantID) (application.RegisterReaskCadenceCommand, error) {
	if tenant.String() == "" {
		return application.RegisterReaskCadenceCommand{}, ErrOperatorIdentityMissing
	}
	return application.RegisterReaskCadenceCommand{TenantID: tenant, Version: payload.Version, IntervalMinutes: payload.IntervalMinutes}, nil
}

// ReaskCadenceRegistrar 是本口转交的编排。
type ReaskCadenceRegistrar interface {
	Register(ctx context.Context, command application.RegisterReaskCadenceCommand) (application.RegisterReaskCadenceResult, error)
}

var _ ReaskCadenceIntake = UnconfiguredIntake{}

func (UnconfiguredIntake) IntakeReaskCadenceRegistration(context.Context, *http.Request) (application.RegisterReaskCadenceCommand, error) {
	return application.RegisterReaskCadenceCommand{}, ErrAccessChannelNotConfigured
}

// NewRegisterReaskCadenceEndpoint 交回重问间隔登记的 HTTP 入口。
func NewRegisterReaskCadenceEndpoint(intake ReaskCadenceIntake, registrar ReaskCadenceRegistrar) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.RegisterReaskCadenceResult, error, bool) {
		command, err := intake.IntakeReaskCadenceRegistration(request.Context(), request)
		if err != nil {
			return application.RegisterReaskCadenceResult{}, err, false
		}
		result, err := registrar.Register(request.Context(), command)
		return result, err, true
	}, writeReaskCadenceOutcome)
}

// reaskCadenceResponse 是封闭响应形状。带记录的答案透出那一版；`内容冲突`带回的是在册的那一版。
type reaskCadenceResponse struct {
	Outcome         string `json:"outcome"`
	Version         string `json:"version,omitempty"`
	IntervalMinutes int64  `json:"intervalMinutes,omitempty"`
	RecordedAt      string `json:"recordedAt,omitempty"`
}

func writeReaskCadenceOutcome(response http.ResponseWriter, result application.RegisterReaskCadenceResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := reaskCadenceResponse{Outcome: outcome}
	if record, present := result.Record(); present {
		body.Version = record.Cadence.Version().String()
		body.IntervalMinutes = int64(record.Cadence.Interval().Minutes())
		body.RecordedAt = rfc3339(record.RecordedAt)
	}
	// ADR-0022：只有新落了一版用 201；重放、冲突、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.ReaskCadenceRegistered {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
