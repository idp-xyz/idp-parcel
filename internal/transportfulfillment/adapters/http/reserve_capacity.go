package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 容量预占口（UC-TF-004 步 5；ADR-0203 决定三至六）。预占是运营人员在管理台上为一个业务范围占住一个池的一段容量，
// 是生产决定，挂操作者渠道的「运营决定」能力面、按自己的决定种类认证（ADR-0151 定族判据）；建池是登记册配置写，
// 不在这一口。路径取读面册名前缀 `transport-fulfillment-`，理由同关段口。

// CapacityReservationIntake 把已认证的运营写请求翻译成预占命令。
type CapacityReservationIntake interface {
	IntakeCapacityReservation(ctx context.Context, request *http.Request) (application.ReserveCapacityCommand, error)
}

// CapacityReserver 是本适配器转交的应用编排。
type CapacityReserver interface {
	Reserve(ctx context.Context, command application.ReserveCapacityCommand) (application.OpportunityResult, error)
}

// CapacityDemandSourcePayload 是需求来源的线格式：种类取封闭词原样交编排，不在封闭词里由编排答`未受理`（200），不在这里
// 折成坏报文。
type CapacityDemandSourcePayload struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
}

// CapacityReservationPayload 是预占口的线格式，字段与应用命令一一对应、只少租户一格。没有任何越过硬上限的键：封闭
// 解码下带 `approval`、`override` 一类键的载荷是坏报文（400），审批越不过硬上限（AT-TF-031）。判断时点照 ADR-0023
// 由声明方给。
type CapacityReservationPayload struct {
	Pool        string                      `json:"pool"`
	Reservation string                      `json:"reservation"`
	Quantity    int64                       `json:"quantity"`
	Unit        string                      `json:"unit"`
	Source      CapacityDemandSourcePayload `json:"source"`
	ValidUntil  string                      `json:"validUntil"`
	ReservedAt  string                      `json:"reservedAt"`
}

// Command 把载荷连同信封给的租户翻成预占命令。时刻解不出是坏报文（400）；各格成不成立由编排答。
func (payload CapacityReservationPayload) Command(tenant domain.TenantID) (application.ReserveCapacityCommand, error) {
	if tenant.String() == "" {
		return application.ReserveCapacityCommand{}, ErrOperatorIdentityMissing
	}
	validUntil, err := parseOptionalInstant("validUntil", payload.ValidUntil)
	if err != nil {
		return application.ReserveCapacityCommand{}, err
	}
	reservedAt, err := parseOptionalInstant("reservedAt", payload.ReservedAt)
	if err != nil {
		return application.ReserveCapacityCommand{}, err
	}
	return application.ReserveCapacityCommand{
		TenantID:        tenant,
		Pool:            payload.Pool,
		Reservation:     payload.Reservation,
		Quantity:        payload.Quantity,
		Unit:            payload.Unit,
		SourceKind:      payload.Source.Kind,
		SourceReference: payload.Source.Reference,
		ValidUntil:      validUntil,
		At:              reservedAt,
	}, nil
}

// IntakeCapacityReservation 译容量预占（`/transport-fulfillment-capacity-reservations`）。
func (intake *OperatorDecisionIntake) IntakeCapacityReservation(ctx context.Context, request *http.Request) (application.ReserveCapacityCommand, error) {
	tenant, err := intake.tenantFor(ctx, request, OperatorDecisionCapacityReservation)
	if err != nil {
		return application.ReserveCapacityCommand{}, err
	}
	var payload CapacityReservationPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.ReserveCapacityCommand{}, err
	}
	return payload.Command(tenant)
}

// NewReserveCapacityEndpoint 交回容量预占的 HTTP 入口。
func NewReserveCapacityEndpoint(intake CapacityReservationIntake, reserver CapacityReserver) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.OpportunityResult, error, bool) {
		command, err := intake.IntakeCapacityReservation(request.Context(), request)
		if err != nil {
			return application.OpportunityResult{}, err, false
		}
		result, err := reserver.Reserve(request.Context(), command)
		return result, err, true
	}, writeCapacityDecisionOutcome(application.CapacityReserved))
}

// capacityDecisionResponse 是预占口与释放口共用的封闭响应形状。`pool` 在读到了池的各格上出现（池不在册、未受理、未决
// 没有它）；`reservation` 是这一格说的那一份预占——新立的、重放认出的、冲突里挡路的、释放所落的；`release` 只在释放口
// 出现。`availableQuantity` 只随`容量超出`出现，是预占整段里最紧那一刻的余量，单位同 `pool.unit`。
type capacityDecisionResponse struct {
	Outcome               string                   `json:"outcome"`
	UndecidedReason       string                   `json:"undecidedReason,omitempty"`
	ContinuationReference string                   `json:"continuationReference,omitempty"`
	AvailableQuantity     string                   `json:"availableQuantity,omitempty"`
	Pool                  *registeredPoolBody      `json:"pool,omitempty"`
	Reservation           *capacityReservationBody `json:"reservation,omitempty"`
	Release               *capacityReleaseBody     `json:"release,omitempty"`
}

// capacityReservationBody 逐字段透出在册的一份预占：三量与已分配各自照实，不代算余量；单位取池的（预占与池同一单位）。
// 时间一律 RFC 3339 UTC。已分配是各装载分配当前那一版分走的量（ADR-0203 决定二）：释放撞守恒时，声明方要看得见挡住它的
// 是分配，不只是释放与消耗。
type capacityReservationBody struct {
	Reservation string             `json:"reservation"`
	Quantity    string             `json:"quantity"`
	Released    string             `json:"released"`
	Consumed    string             `json:"consumed"`
	Allocated   string             `json:"allocated"`
	Unit        string             `json:"unit"`
	Source      capacitySourceBody `json:"source"`
	ReservedAt  string             `json:"reservedAt"`
	ValidUntil  string             `json:"validUntil"`
}

func capacityReservation(reservation domain.CapacityReservation, unit domain.QuantityUnitReference) capacityReservationBody {
	reserved, released, consumed := reservation.Quantities()
	return capacityReservationBody{
		Reservation: reservation.Reference().String(),
		Quantity:    quantity(reserved),
		Released:    quantity(released),
		Consumed:    quantity(consumed),
		Allocated:   quantity(reservation.Allocated()),
		Unit:        unit.String(),
		Source:      capacitySource(reservation.Source()),
		ReservedAt:  rfc3339(reservation.ReservedAt()),
		ValidUntil:  rfc3339(reservation.ValidUntil()),
	}
}

type capacitySourceBody struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
}

// capacityReleaseBody 逐字段透出在册的一次释放与它所落的预占。
type capacityReleaseBody struct {
	Release     string             `json:"release"`
	Reservation string             `json:"reservation"`
	Quantity    string             `json:"quantity"`
	Unit        string             `json:"unit"`
	Source      capacitySourceBody `json:"source"`
	ReleasedAt  string             `json:"releasedAt"`
}

func capacitySource(source domain.CapacityDemandSource) capacitySourceBody {
	return capacitySourceBody{Kind: source.Kind().String(), Reference: source.Reference().String()}
}

// writeCapacityDecisionOutcome 交回预占口或释放口的写法：只有 formed 那一格（新预占了、新释放了）用 201；`已有`、冲突、
// 各否定格、未受理、未决都是形成了的答案（200，ADR-0022）。
func writeCapacityDecisionOutcome(formed application.OpportunityOutcome) func(http.ResponseWriter, application.OpportunityResult) {
	return func(response http.ResponseWriter, result application.OpportunityResult) {
		outcome := result.Outcome().String()
		if outcome == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		body := capacityDecisionResponse{
			Outcome:               outcome,
			UndecidedReason:       result.UndecidedReason().String(),
			ContinuationReference: result.ContinuationReference(),
		}
		if result.Outcome() == application.CapacityExceededOutcome {
			body.AvailableQuantity = quantity(result.AvailableQuantity())
		}
		if record, present := result.Pool(); present {
			pool := registeredPool(record)
			body.Pool = &pool
			unit := record.Pool.Unit().String()
			if reservation, found := result.Reservation(); found {
				held := capacityReservation(reservation, record.Pool.Unit())
				body.Reservation = &held
			}
			if on, release, found := result.Release(); found {
				body.Release = &capacityReleaseBody{
					Release:     release.Reference().String(),
					Reservation: on.String(),
					Quantity:    quantity(release.Quantity()),
					Unit:        unit,
					Source:      capacitySource(release.Source()),
					ReleasedAt:  rfc3339(release.ReleasedAt()),
				}
			}
		}
		status := http.StatusOK
		if result.Outcome() == formed {
			status = http.StatusCreated
		}
		writeJSON(response, status, body)
	}
}
