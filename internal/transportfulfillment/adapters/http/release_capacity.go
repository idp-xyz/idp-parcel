package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 容量释放口（UC-TF-004 步 5；ADR-0203 决定三、五、六）。显式释放一份预占上未消耗的范围，与预占分成两种决定：能占的
// 不因此能放，授予的粒度等于权力的粒度（ADR-0151）。族、路径前缀与响应形状同预占口。

// CapacityReleaseIntake 把已认证的运营写请求翻译成释放命令。
type CapacityReleaseIntake interface {
	IntakeCapacityRelease(ctx context.Context, request *http.Request) (application.ReleaseCapacityCommand, error)
}

// CapacityReleaser 是本适配器转交的应用编排。
type CapacityReleaser interface {
	Release(ctx context.Context, command application.ReleaseCapacityCommand) (application.OpportunityResult, error)
}

// CapacityReleasePayload 是释放口的线格式。`release` 是这一次释放的身份，原样重交靠它认出重放；来源说的是为什么不再
// 需要这些量。释放时点照 ADR-0023 由声明方给。
type CapacityReleasePayload struct {
	Pool        string                      `json:"pool"`
	Reservation string                      `json:"reservation"`
	Release     string                      `json:"release"`
	Quantity    int64                       `json:"quantity"`
	Unit        string                      `json:"unit"`
	Source      CapacityDemandSourcePayload `json:"source"`
	ReleasedAt  string                      `json:"releasedAt"`
}

// Command 把载荷连同信封给的租户翻成释放命令。时刻解不出是坏报文（400）；各格成不成立由编排答。
func (payload CapacityReleasePayload) Command(tenant domain.TenantID) (application.ReleaseCapacityCommand, error) {
	if tenant.String() == "" {
		return application.ReleaseCapacityCommand{}, ErrOperatorIdentityMissing
	}
	releasedAt, err := parseOptionalInstant("releasedAt", payload.ReleasedAt)
	if err != nil {
		return application.ReleaseCapacityCommand{}, err
	}
	return application.ReleaseCapacityCommand{
		TenantID:        tenant,
		Pool:            payload.Pool,
		Reservation:     payload.Reservation,
		Release:         payload.Release,
		Quantity:        payload.Quantity,
		Unit:            payload.Unit,
		SourceKind:      payload.Source.Kind,
		SourceReference: payload.Source.Reference,
		At:              releasedAt,
	}, nil
}

// IntakeCapacityRelease 译容量释放（`/transport-fulfillment-capacity-releases`）。
func (intake *OperatorDecisionIntake) IntakeCapacityRelease(ctx context.Context, request *http.Request) (application.ReleaseCapacityCommand, error) {
	tenant, err := intake.tenantFor(ctx, request, OperatorDecisionCapacityRelease)
	if err != nil {
		return application.ReleaseCapacityCommand{}, err
	}
	var payload CapacityReleasePayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.ReleaseCapacityCommand{}, err
	}
	return payload.Command(tenant)
}

// NewReleaseCapacityEndpoint 交回容量释放的 HTTP 入口。
func NewReleaseCapacityEndpoint(intake CapacityReleaseIntake, releaser CapacityReleaser) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.OpportunityResult, error, bool) {
		command, err := intake.IntakeCapacityRelease(request.Context(), request)
		if err != nil {
			return application.OpportunityResult{}, err, false
		}
		result, err := releaser.Release(request.Context(), command)
		return result, err, true
	}, writeCapacityDecisionOutcome(application.CapacityReleased))
}
