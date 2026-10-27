package shipmenthttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 取消先后冲突册的运营查阅端点（ADR-0295 决定一）。
//
// 本端点只有读。解开冲突（认定取消依据不成立、交来源方更正）是「运营决定」能力面上另一个端点的命令，不在查阅面里长出动作。查阅给的是此刻未解开的冲突与全部未解开的
// 计数；告警阈值是产品策略、本产品没有告警引擎，这里只透出计数与清单是否被页大小截断。

// CancellationOrderConflictQuery 是一次已授权的冲突册查阅：作用域只有租户（册行按租户切，不按客户账户），页大小由接入面按渠道契约裁决，两样都不采信调用方自报。
type CancellationOrderConflictQuery struct {
	Tenant domain.TenantID
	Limit  int
}

// CancellationOrderConflictQueryIntake 把一次已认证的运营查阅请求翻译成查询。它是接口而非解析代码，理由同本包其余 Intake：作用域整组只能来自认证与授权结果
// （操作者渠道 ADR-0100，真实现是 OperatorOperationsReadIntake），采信自报租户会穿透 ADR-0003 的隔离边界。
type CancellationOrderConflictQueryIntake interface {
	IntakeCancellationOrderConflictQuery(ctx context.Context, request *http.Request) (CancellationOrderConflictQuery, error)
}

// CancellationOrderConflictCatalogueReader 是本端点消费的读口：查阅不触发决定，所以接存储读面。
type CancellationOrderConflictCatalogueReader interface {
	ListUnresolvedCancellationOrderConflicts(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) (ports.UnresolvedCancellationOrderConflicts, error)
}

var (
	// 编译期锁缝：读口形状与端口保持一致，本适配器不新造查询语义。
	_ CancellationOrderConflictCatalogueReader = ports.CancellationOrderConflictCatalogue(nil)
	_ CancellationOrderConflictQueryIntake     = UnconfiguredIntake{}
)

// IntakeCancellationOrderConflictQuery 不读请求（参数匿名）：渠道未配置就无从铸造作用域。
func (UnconfiguredIntake) IntakeCancellationOrderConflictQuery(context.Context, *http.Request) (CancellationOrderConflictQuery, error) {
	return CancellationOrderConflictQuery{}, ErrAccessChannelNotConfigured
}

// 业务结果的封闭集合。空册仍是 LISTED：空清单是「此刻没有未解开的冲突」这个如实答案，与「接入渠道未配置」（403）分得开。
const outcomeCancellationOrderConflictsListed = "CANCELLATION_ORDER_CONFLICTS_LISTED"

// NewQueryCancellationOrderConflictsEndpoint 交回取消先后冲突册查阅的 HTTP 入口（GET /cancellation-order-conflicts）。
func NewQueryCancellationOrderConflictsEndpoint(
	intake CancellationOrderConflictQueryIntake,
	reader CancellationOrderConflictCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		query, err := intake.IntakeCancellationOrderConflictQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		listed, err := reader.ListUnresolvedCancellationOrderConflicts(request.Context(), query.Tenant, query.Limit)
		if err != nil {
			// 读不回是答案未形成，不伪装成空册：把一次该重试的故障读成「没有未解开的冲突」，卡住的包裹就更没人看见了。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		// 空列表交回空数组而不是 null：调用方判「没有积压」不该先判「有没有字段」。
		bodies := make([]cancellationOrderConflictBody, 0, len(listed.Conflicts))
		for _, entry := range listed.Conflicts {
			bodies = append(bodies, cancellationOrderConflictBodyOf(entry))
		}
		writeJSON(response, http.StatusOK, cancellationOrderConflictsResponse{
			Outcome:         outcomeCancellationOrderConflictsListed,
			UnresolvedCount: listed.Count,
			Truncated:       listed.Count > len(listed.Conflicts),
			Conflicts:       bodies,
		})
	})
}

type cancellationOrderConflictsResponse struct {
	Outcome         string                          `json:"outcome"`
	UnresolvedCount int                             `json:"unresolvedCount"`
	Truncated       bool                            `json:"truncated"`
	Conflicts       []cancellationOrderConflictBody `json:"conflicts"`
}

// cancellationOrderConflictBody 逐字段透出册上一行：决定命令要的几段（包裹、边界种类、边界那一代、取消）原样给出，两边的业务时间与发现时间一并给出，让决定人
// 看得到谁在先、差多少。
type cancellationOrderConflictBody struct {
	ParcelID     string `json:"parcelId"`
	BoundaryKind string `json:"boundaryKind"`
	BoundaryRef  string `json:"boundaryRef"`
	Cancellation string `json:"cancellation"`
	CancelledAt  string `json:"cancelledAt"`
	BoundaryAt   string `json:"boundaryAt"`
	DetectedAt   string `json:"detectedAt"`
}

func cancellationOrderConflictBodyOf(entry ports.CancellationOrderConflictEntry) cancellationOrderConflictBody {
	return cancellationOrderConflictBody{
		ParcelID:     entry.Parcel.String(),
		BoundaryKind: entry.Boundary.Kind.String(),
		BoundaryRef:  entry.Boundary.Reference.String(),
		Cancellation: entry.Cancellation.String(),
		CancelledAt:  entry.CancelledAt.UTC().Format(time.RFC3339Nano),
		BoundaryAt:   entry.Boundary.OccurredAt.UTC().Format(time.RFC3339Nano),
		DetectedAt:   entry.DetectedAt.UTC().Format(time.RFC3339Nano),
	}
}
