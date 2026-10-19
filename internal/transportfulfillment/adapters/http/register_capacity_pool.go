package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 容量池的在线登记口（ADR-0201 决定一）。登记册配置写：建池不搬包裹、不形成生产事实，挂操作者渠道的登记写面
// （ADR-0100、ADR-0151 定族判据）；受控 CLI parcel-transport-register 的 capacity-pool 命令收的是同一份载荷再加
// 租户，调的是同一个编排（ADR-0085）。载荷里没有池身份——池身份由班次与维度派生（ADR-0201 补记「池身份」）。

// CapacityPoolIntake 把已认证的接入请求翻译成建池命令。
type CapacityPoolIntake interface {
	IntakeCapacityPoolRegistration(ctx context.Context, request *http.Request) (application.EstablishPoolCommand, error)
}

// CapacityPoolRegistrar 是本适配器转交的应用编排。只有建池一口：预占、释放与消耗是运营决定，不在登记写面上。
type CapacityPoolRegistrar interface {
	EstablishPool(ctx context.Context, command application.EstablishPoolCommand) (application.OpportunityResult, error)
}

// CapacityPoolRegistrationPayload 是建池载荷的线格式，字段与应用命令一一对应、只少租户一格。维度取原词交编排：
// 不在封闭词里由编排答`未受理`（200），不在这里折成坏报文。
type CapacityPoolRegistrationPayload struct {
	Schedule  string `json:"schedule"`
	Dimension string `json:"dimension"`
	Unit      string `json:"unit"`
	Capacity  int64  `json:"capacity"`
}

func (payload CapacityPoolRegistrationPayload) Command(tenant domain.TenantID) (application.EstablishPoolCommand, error) {
	if tenant.String() == "" {
		return application.EstablishPoolCommand{}, ErrOperatorIdentityMissing
	}
	return application.EstablishPoolCommand{
		TenantID:  tenant,
		Schedule:  payload.Schedule,
		Dimension: payload.Dimension,
		Unit:      payload.Unit,
		Capacity:  payload.Capacity,
	}, nil
}

var _ CapacityPoolIntake = (*OperatorRegistryIntake)(nil)

func (intake *OperatorRegistryIntake) IntakeCapacityPoolRegistration(ctx context.Context, request *http.Request) (application.EstablishPoolCommand, error) {
	return registryCommand(ctx, intake, request, CapacityPoolRegistrationPayload.Command)
}

// NewRegisterCapacityPoolEndpoint 交回建池的 HTTP 入口。
func NewRegisterCapacityPoolEndpoint(intake CapacityPoolIntake, registrar CapacityPoolRegistrar) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.OpportunityResult, error, bool) {
		command, err := intake.IntakeCapacityPoolRegistration(request.Context(), request)
		if err != nil {
			return application.OpportunityResult{}, err, false
		}
		result, err := registrar.EstablishPool(request.Context(), command)
		return result, err, true
	}, writeCapacityPoolRegistrationOutcome)
}

// capacityPoolRegistrationResponse 是建池口的封闭响应形状：带池的答案（`已建立`、`已存在`、`池冲突`）透出 pool
// 一组——冲突时那是库里已建的那一个；`未决`只带原因与续办引用。
type capacityPoolRegistrationResponse struct {
	Outcome               string              `json:"outcome"`
	UndecidedReason       string              `json:"undecidedReason,omitempty"`
	ContinuationReference string              `json:"continuationReference,omitempty"`
	Pool                  *registeredPoolBody `json:"pool,omitempty"`
}

// registeredPoolBody 是登记答案里的一个池：身份、所在班次、维度、单位与硬上限。不带预占三量——刚建的池没有预占，
// 已建的池现有多少预占归查阅口 `capacity-pool` 那一册答。
type registeredPoolBody struct {
	PoolID     string `json:"poolId"`
	Schedule   string `json:"schedule"`
	Dimension  string `json:"dimension"`
	Unit       string `json:"unit"`
	Capacity   string `json:"capacity"`
	RecordedAt string `json:"recordedAt"`
}

func registeredPool(record ports.CapacityPoolRecord) registeredPoolBody {
	return registeredPoolBody{
		PoolID:     record.Pool.Pool().String(),
		Schedule:   record.Pool.Schedule().String(),
		Dimension:  record.Pool.Dimension().String(),
		Unit:       record.Pool.Unit().String(),
		Capacity:   quantity(record.Pool.Capacity()),
		RecordedAt: rfc3339(record.RecordedAt),
	}
}

func writeCapacityPoolRegistrationOutcome(response http.ResponseWriter, result application.OpportunityResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := capacityPoolRegistrationResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
	}
	if record, present := result.Pool(); present {
		pool := registeredPool(record)
		body.Pool = &pool
	}

	// ADR-0022：只有新建了池用 201；`已存在`、`池冲突`、班次不在册、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.PoolEstablished {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
