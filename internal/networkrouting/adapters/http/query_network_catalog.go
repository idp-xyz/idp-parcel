// Package networkhttp 是 network-routing 的 HTTP 入站适配器，按 ADR-0022 把结果映射
// 成响应：状态码只回答服务端有没有形成答案，业务判别一律进响应体的 `outcome`。
//
// 包名与目录名不一致与 shipmenthttp 同理：目录按 ADR-0018 叫 `adapters/http`，包名
// 叫 `http` 会遮住标准库。
package networkhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// ErrMalformedRequest 表示这次请求构造不出查询，且重发同样的内容不会改变结果。
// 4xx/5xx 的分法决定调用侧的动作——4xx 出队交给人，5xx 留队重试。
var ErrMalformedRequest = errors.New("network routing http: malformed request")

const (
	codeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	codeMalformedRequest = "MALFORMED_REQUEST"
	codeIntakeFailed     = "INTAKE_FAILED"
	codeNoAnswerFormed   = "NO_ANSWER_FORMED"
)

// NetworkCatalogQuery 是一次已授权的网络目录运营查阅（ADR-0077）。作用域来自认证与
// 授权结果，授权边界只有租户——没有客户维；页大小由接入面按渠道契约裁决——两样都
// 不采信调用方自报。
type NetworkCatalogQuery struct {
	Scope domain.OperationsQueryScope
	Limit int
}

// CatalogueQueryIntake 把一次已认证的运营查阅请求翻译成查询。
//
// 它是接口而非解析代码：请求方身份与租户必须同时核对，运营接入面的认证方式属
// `PAR-INT-01` 待提供；采信自报租户会穿透 ADR-0003 的隔离边界。未决期间本包不带
// 任何实现，包括「开发用」的采信头部版本。
type CatalogueQueryIntake interface {
	IntakeCatalogueQuery(ctx context.Context, request *http.Request) (NetworkCatalogQuery, error)
}

// OperationsCatalogReader 是本端点消费的读口。查阅不触发判断、决定或披露——所以这里
// 接存储读面，不接应用编排（ADR-0077 Decision 一）；三个证据视图与解析层的护栏
// （ADR-0068 Decision 六）不因本端点松动——上列按族透版本行原文，不选版不折叠，
// 形不成判断依据。
type OperationsCatalogReader interface {
	ListNodeVersions(ctx context.Context, tenant domain.TenantID, limit int) ([]ports.NodeDefinitionVersion, error)
	ListConnectionVersions(ctx context.Context, tenant domain.TenantID, limit int) ([]ports.ConnectionDefinitionVersion, error)
	ListLineVersions(ctx context.Context, tenant domain.TenantID, limit int) ([]ports.LineDefinitionVersion, error)
	ListServiceAreaVersions(ctx context.Context, tenant domain.TenantID, limit int) ([]ports.ServiceAreaDefinitionVersion, error)
	ListServiceCalendarVersions(ctx context.Context, tenant domain.TenantID, limit int) ([]ports.ServiceCalendarDefinitionVersion, error)
	ListAvailabilityAdjustments(ctx context.Context, tenant domain.TenantID, limit int) ([]ports.AvailabilityAdjustmentStatement, error)
	ListRouteStrategyVersions(ctx context.Context, tenant domain.TenantID, limit int) ([]ports.RouteStrategyDefinitionVersion, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义，只消费 ADR-0077
// 钉住的那一个读面。
var _ OperationsCatalogReader = ports.OperationsCatalogRead(nil)

// 业务结果的封闭集合：七族各占一格。空族如实答空列表走 2xx 成格，不折成未配置
// （ADR-0077 Decision 四：空目录的续办是操作员去登记口登记，未配置的续办是接入方去
// 配置渠道——恢复动作不同，判据同 ADR-0029）。
const (
	outcomeNodeVersionsListed            = "NODE_VERSIONS_LISTED"
	outcomeConnectionVersionsListed      = "CONNECTION_VERSIONS_LISTED"
	outcomeLineVersionsListed            = "LINE_VERSIONS_LISTED"
	outcomeServiceAreaVersionsListed     = "SERVICE_AREA_VERSIONS_LISTED"
	outcomeServiceCalendarVersionsListed = "SERVICE_CALENDAR_VERSIONS_LISTED"
	outcomeAvailabilityAdjustmentsListed = "AVAILABILITY_ADJUSTMENTS_LISTED"
	outcomeRouteStrategyVersionsListed   = "ROUTE_STRATEGY_VERSIONS_LISTED"
)

// family 查询参数的封闭七族，词取 `cmd/parcel-network-register` 的 kind 封闭集原文
// ——登记口与查阅口对同一族用同一个词，页面与种子脚本不必维护第二套对照表。
const (
	familyNode                   = "node"
	familyConnection             = "connection"
	familyLine                   = "line"
	familyServiceArea            = "service-area"
	familyServiceCalendar        = "service-calendar"
	familyAvailabilityAdjustment = "availability-adjustment"
	familyRouteStrategy          = "route-strategy"
)

// NewQueryNetworkCatalogEndpoint 交回网络目录运营查阅的 HTTP 入口
// （GET /network-catalog，ADR-0077）。
//
// 七族共用一个端点，按 `family` 查询参数分派（先例：tracking 端点一口三分派）：参数
// 在场与否、取值在不在封闭集内属传输形状（与方法检查同级，先于 Intake），读它不构成
// 读业务内容——未配置 Intake 对全部分支同答 403，分支选择不泄露任何东西。缺席按坏
// 请求拒：替调用方默认一族就是替它猜。
func NewQueryNetworkCatalogEndpoint(
	intake CatalogueQueryIntake,
	reader OperationsCatalogReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		family := request.URL.Query().Get("family")
		if !knownFamily(family) {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		ctx := request.Context()
		tenant := query.Scope.Tenant()
		limit := query.Limit

		switch family {
		case familyNode:
			serveVersionList(response, outcomeNodeVersionsListed, nodeVersionBodyOf,
				func() ([]ports.NodeDefinitionVersion, error) {
					return reader.ListNodeVersions(ctx, tenant, limit)
				})
		case familyConnection:
			serveVersionList(response, outcomeConnectionVersionsListed, connectionVersionBodyOf,
				func() ([]ports.ConnectionDefinitionVersion, error) {
					return reader.ListConnectionVersions(ctx, tenant, limit)
				})
		case familyLine:
			serveVersionList(response, outcomeLineVersionsListed, lineVersionBodyOf,
				func() ([]ports.LineDefinitionVersion, error) {
					return reader.ListLineVersions(ctx, tenant, limit)
				})
		case familyServiceArea:
			serveVersionList(response, outcomeServiceAreaVersionsListed, serviceAreaVersionBodyOf,
				func() ([]ports.ServiceAreaDefinitionVersion, error) {
					return reader.ListServiceAreaVersions(ctx, tenant, limit)
				})
		case familyServiceCalendar:
			serveVersionList(response, outcomeServiceCalendarVersionsListed, serviceCalendarVersionBodyOf,
				func() ([]ports.ServiceCalendarDefinitionVersion, error) {
					return reader.ListServiceCalendarVersions(ctx, tenant, limit)
				})
		case familyAvailabilityAdjustment:
			serveVersionList(response, outcomeAvailabilityAdjustmentsListed, adjustmentBodyOf,
				func() ([]ports.AvailabilityAdjustmentStatement, error) {
					return reader.ListAvailabilityAdjustments(ctx, tenant, limit)
				})
		case familyRouteStrategy:
			serveVersionList(response, outcomeRouteStrategyVersionsListed, routeStrategyVersionBodyOf,
				func() ([]ports.RouteStrategyDefinitionVersion, error) {
					return reader.ListRouteStrategyVersions(ctx, tenant, limit)
				})
		}
	})
}

func knownFamily(family string) bool {
	switch family {
	case familyNode, familyConnection, familyLine, familyServiceArea,
		familyServiceCalendar, familyAvailabilityAdjustment, familyRouteStrategy:
		return true
	default:
		return false
	}
}

// serveVersionList 是七族共用的转写：读回、逐行转体、2xx 成格。读不回是答案未形成
// （5xx），不伪装成空族——前者该重试，后者是终局答案。
func serveVersionList[Entry any, Body any](
	response http.ResponseWriter,
	outcome string,
	bodyOf func(Entry) Body,
	list func() ([]Entry, error),
) {
	entries, err := list()
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	// 空列表交回空数组而不是 null：调用方判「没有行」不该先判「有没有字段」。
	bodies := make([]Body, 0, len(entries))
	for _, entry := range entries {
		bodies = append(bodies, bodyOf(entry))
	}
	writeJSON(response, http.StatusOK, versionListResponse[Body]{
		Outcome:  outcome,
		Versions: bodies,
	})
}

func writeCatalogueIntakeProblem(response http.ResponseWriter, err error) {
	if errors.Is(err, ErrAccessChannelNotConfigured) {
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
		return
	}
	if errors.Is(err, ErrMalformedRequest) {
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	}
	writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
}

type versionListResponse[Body any] struct {
	Outcome  string `json:"outcome"`
	Versions []Body `json:"versions"`
}

// 各族行体逐字段透出版本行原文，不经披露删减（运营查阅在租户内，无跨账户存在性可泄）。
// effectiveTo / liftedAt 缺席即未闭区间（当前版）或未解除——缺席是真话不是缺陷，
// 不为区间完整补一个编造的「无限远」时刻。字段词取 CONTEXT 原词的既有英文转写
// （ports 行类型同名字段），内容列（地理覆盖、日历、策略正文）今天不存在（PAR-NET-14），
// 不发明。
type nodeVersionBody struct {
	Code             string `json:"code"`
	Version          int32  `json:"version"`
	BusinessTimezone string `json:"businessTimezone"`
	EffectiveFrom    string `json:"effectiveFrom"`
	EffectiveTo      string `json:"effectiveTo,omitempty"`
}

func nodeVersionBodyOf(row ports.NodeDefinitionVersion) nodeVersionBody {
	return nodeVersionBody{
		Code:             row.Code,
		Version:          row.Version,
		BusinessTimezone: row.BusinessTimezone,
		EffectiveFrom:    utcText(row.EffectiveFrom),
		EffectiveTo:      optionalUTCText(row.EffectiveTo, row.HasEffectiveTo),
	}
}

type connectionVersionBody struct {
	Code             string `json:"code"`
	Version          int32  `json:"version"`
	FromNode         string `json:"fromNode"`
	ToNode           string `json:"toNode"`
	BusinessTimezone string `json:"businessTimezone"`
	EffectiveFrom    string `json:"effectiveFrom"`
	EffectiveTo      string `json:"effectiveTo,omitempty"`
}

func connectionVersionBodyOf(row ports.ConnectionDefinitionVersion) connectionVersionBody {
	return connectionVersionBody{
		Code:             row.Code,
		Version:          row.Version,
		FromNode:         row.FromNode,
		ToNode:           row.ToNode,
		BusinessTimezone: row.BusinessTimezone,
		EffectiveFrom:    utcText(row.EffectiveFrom),
		EffectiveTo:      optionalUTCText(row.EffectiveTo, row.HasEffectiveTo),
	}
}

type lineVersionBody struct {
	Code             string   `json:"code"`
	Version          int32    `json:"version"`
	Segments         []string `json:"segments"`
	BusinessTimezone string   `json:"businessTimezone"`
	ApplicableScope  string   `json:"applicableScope"`
	EffectiveFrom    string   `json:"effectiveFrom"`
	EffectiveTo      string   `json:"effectiveTo,omitempty"`
}

func lineVersionBodyOf(row ports.LineDefinitionVersion) lineVersionBody {
	return lineVersionBody{
		Code:             row.Code,
		Version:          row.Version,
		Segments:         row.Segments,
		BusinessTimezone: row.BusinessTimezone,
		ApplicableScope:  row.ApplicableScope,
		EffectiveFrom:    utcText(row.EffectiveFrom),
		EffectiveTo:      optionalUTCText(row.EffectiveTo, row.HasEffectiveTo),
	}
}

type serviceAreaVersionBody struct {
	Code          string `json:"code"`
	Version       int32  `json:"version"`
	EffectiveFrom string `json:"effectiveFrom"`
	EffectiveTo   string `json:"effectiveTo,omitempty"`
}

func serviceAreaVersionBodyOf(row ports.ServiceAreaDefinitionVersion) serviceAreaVersionBody {
	return serviceAreaVersionBody{
		Code:          row.Code,
		Version:       row.Version,
		EffectiveFrom: utcText(row.EffectiveFrom),
		EffectiveTo:   optionalUTCText(row.EffectiveTo, row.HasEffectiveTo),
	}
}

type serviceCalendarVersionBody struct {
	TargetKind    string `json:"targetKind"`
	TargetCode    string `json:"targetCode"`
	Version       int32  `json:"version"`
	EffectiveFrom string `json:"effectiveFrom"`
	EffectiveTo   string `json:"effectiveTo,omitempty"`
}

func serviceCalendarVersionBodyOf(row ports.ServiceCalendarDefinitionVersion) serviceCalendarVersionBody {
	return serviceCalendarVersionBody{
		TargetKind:    row.TargetKind.String(),
		TargetCode:    row.TargetCode,
		Version:       row.Version,
		EffectiveFrom: utcText(row.EffectiveFrom),
		EffectiveTo:   optionalUTCText(row.EffectiveTo, row.HasEffectiveTo),
	}
}

type adjustmentBody struct {
	Code        string `json:"code"`
	Version     int32  `json:"version"`
	TargetKind  string `json:"targetKind"`
	TargetCode  string `json:"targetCode"`
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	EffectiveAt string `json:"effectiveAt"`
	LiftedAt    string `json:"liftedAt,omitempty"`
}

func adjustmentBodyOf(row ports.AvailabilityAdjustmentStatement) adjustmentBody {
	return adjustmentBody{
		Code:        row.Code,
		Version:     row.Version,
		TargetKind:  row.TargetKind.String(),
		TargetCode:  row.TargetCode,
		Kind:        row.Kind.String(),
		Source:      row.Source,
		EffectiveAt: utcText(row.EffectiveAt),
		LiftedAt:    optionalUTCText(row.LiftedAt, row.HasLiftedAt),
	}
}

type routeStrategyVersionBody struct {
	Code            string `json:"code"`
	Version         int32  `json:"version"`
	ApplicableScope string `json:"applicableScope"`
	EffectiveFrom   string `json:"effectiveFrom"`
	EffectiveTo     string `json:"effectiveTo,omitempty"`
}

func routeStrategyVersionBodyOf(row ports.RouteStrategyDefinitionVersion) routeStrategyVersionBody {
	return routeStrategyVersionBody{
		Code:            row.Code,
		Version:         row.Version,
		ApplicableScope: row.ApplicableScope,
		EffectiveFrom:   utcText(row.EffectiveFrom),
		EffectiveTo:     optionalUTCText(row.EffectiveTo, row.HasEffectiveTo),
	}
}

func utcText(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func optionalUTCText(value time.Time, present bool) string {
	if !present {
		return ""
	}
	return utcText(value)
}

type problemResponse struct {
	Error problemDetail `json:"error"`
}

type problemDetail struct {
	Code string `json:"code"`
}

func writeProblem(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code}})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
