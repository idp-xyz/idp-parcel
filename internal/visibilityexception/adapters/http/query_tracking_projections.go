package visibilityhttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// OperationsTrackingQuery 是一次已授权的运营追踪查阅(ADR-0076、CONTEXT「运营追踪
// 查阅」)。作用域来自认证与授权结果,授权边界只有租户——没有客户维;页大小由接入面
// 按渠道契约裁决——两样都不采信调用方自报。
type OperationsTrackingQuery struct {
	Scope domain.OperationsQueryScope
	Limit int
}

// OperationsTrackingIntake 把一次已认证的运营查阅请求翻译成查询。
//
// 它是接口而非解析代码,理由与 QueryIntake 相同:请求方身份与租户「必须同时核对」
// (CONTEXT 投影规则),认证方式属 `PAR-INT-01` 待提供;采信自报租户会穿透 ADR-0003
// 的隔离边界。未决期间本包不带任何实现,包括「开发用」的采信头部版本。
type OperationsTrackingIntake interface {
	IntakeOperationsQuery(ctx context.Context, request *http.Request) (OperationsTrackingQuery, error)
}

// OperationsProjectionReader 是本端点消费的读口。查阅不形成新投影版本,也不产生
// 披露决定、通知或任何业务事实(CONTEXT 生命周期「授权的运营追踪查阅」行)——所以
// 这里接存储读面,不接派生编排,与 /shipment-request-views 同一条分界。
type OperationsProjectionReader interface {
	ListCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]domain.TrackingProjection, error)
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.TrackedParcelReference,
	) (domain.TrackingProjection, bool, error)
	FindByVersion(
		ctx context.Context,
		tenant domain.TenantID,
		version domain.ProjectionVersionID,
	) (domain.TrackingProjection, bool, error)
}

// 编译期锁缝:读口形状与端口保持一致——本适配器不新造查询语义,只消费 ADR-0076
// 钉住的那一个读面。
var _ OperationsProjectionReader = ports.OperationsProjectionRead(nil)

// 业务结果的封闭集合(ADR-0076 第四条:按运营语义分格,不承袭 VIEW_NOT_FOUND 的三义
// 合并)。「投影未形成」与「版本未留存」对租户内已授权的运营查阅是如实的业务答案,
// 各占一格、走 2xx——ADR-0029 的探针同答保护的是对外部账户的存在性作答,不约束
// 租户内的运营查阅;对外部客户面的合并义务也不因此松动(那在 /customer-tracking-view
// 一侧照旧)。
const (
	outcomeProjectionsListed   = "PROJECTIONS_LISTED"
	outcomeCurrentProjection   = "CURRENT_PROJECTION"
	outcomeProjectionNotFormed = "PROJECTION_NOT_FORMED"
	outcomeProjectionVersion   = "PROJECTION_VERSION"
	outcomeVersionNotFound     = "VERSION_NOT_FOUND"
)

// NewQueryTrackingProjectionsEndpoint 交回运营追踪查阅的 HTTP 入口
// (GET /tracking-projections,ADR-0076)。
//
// 列表、单件当前版与按版本读回共用一个端点,按 `parcel` / `version` 查询参数分派:
// 参数是否在场属传输形状(与方法检查同级,先于 Intake),读它不构成读业务内容——
// 未配置 Intake 对全部分支同答 403,分支选择不泄露任何东西。两个定位参数同时在场
// 按坏请求拒:选哪个都是替调用方猜。
func NewQueryTrackingProjectionsEndpoint(
	intake OperationsTrackingIntake,
	reader OperationsProjectionReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		hasParcel := request.URL.Query().Has("parcel")
		hasVersion := request.URL.Query().Has("version")
		if hasParcel && hasVersion {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}

		query, err := intake.IntakeOperationsQuery(request.Context(), request)
		if err != nil {
			writeOperationsIntakeProblem(response, err)
			return
		}
		tenant := query.Scope.Tenant()

		switch {
		case hasParcel:
			serveCurrentProjection(response, request, reader, tenant)
		case hasVersion:
			serveProjectionVersion(response, request, reader, tenant)
		default:
			serveProjectionList(response, request, reader, tenant, query.Limit)
		}
	})
}

func serveProjectionList(
	response http.ResponseWriter,
	request *http.Request,
	reader OperationsProjectionReader,
	tenant domain.TenantID,
	limit int,
) {
	projections, err := reader.ListCurrent(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	// 空列表交回空数组而不是 null:调用方判「没有行」不该先判「有没有字段」。
	bodies := make([]projectionBody, 0, len(projections))
	for _, projection := range projections {
		bodies = append(bodies, projectionBodyOf(projection))
	}
	writeJSON(response, http.StatusOK, projectionListResponse{
		Outcome:     outcomeProjectionsListed,
		Projections: bodies,
	})
}

func serveCurrentProjection(
	response http.ResponseWriter,
	request *http.Request,
	reader OperationsProjectionReader,
	tenant domain.TenantID,
) {
	parcel, err := domain.NewTrackedParcelReference(request.URL.Query().Get("parcel"))
	if err != nil {
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	}
	projection, found, err := reader.FindCurrent(request.Context(), tenant, parcel)
	if err != nil {
		// 读不回是答案未形成,不是「无投影」——伪装成后者会让一次该重试的故障变成终局。
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	if !found {
		// 「投影未形成」如实作答(CONTEXT 投影规则):租户内无跨账户存在性可泄。
		writeJSON(response, http.StatusOK, projectionDetailResponse{Outcome: outcomeProjectionNotFormed})
		return
	}
	body := projectionBodyOf(projection)
	writeJSON(response, http.StatusOK, projectionDetailResponse{
		Outcome:    outcomeCurrentProjection,
		Projection: &body,
	})
}

func serveProjectionVersion(
	response http.ResponseWriter,
	request *http.Request,
	reader OperationsProjectionReader,
	tenant domain.TenantID,
) {
	version, err := domain.NewProjectionVersionID(request.URL.Query().Get("version"))
	if err != nil {
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	}
	projection, found, err := reader.FindByVersion(request.Context(), tenant, version)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	if !found {
		writeJSON(response, http.StatusOK, projectionDetailResponse{Outcome: outcomeVersionNotFound})
		return
	}
	body := projectionBodyOf(projection)
	writeJSON(response, http.StatusOK, projectionDetailResponse{
		Outcome:    outcomeProjectionVersion,
		Projection: &body,
	})
}

func writeOperationsIntakeProblem(response http.ResponseWriter, err error) {
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

type projectionListResponse struct {
	Outcome     string           `json:"outcome"`
	Projections []projectionBody `json:"projections"`
}

type projectionDetailResponse struct {
	Outcome    string          `json:"outcome"`
	Projection *projectionBody `json:"projection,omitempty"`
}

// projectionBody 逐字段透出投影版本:条目含来源上下文、事实类型、三个时间与替代
// 关系,不经客户披露删减(CONTEXT:运营查阅「面向投影的全部并行维度、冲突关系与
// 被替代条目」)——它不是 trackingViewBody 的放宽版,是另一个对象的如实转写。
// 并行维度(物流/关务/交付退运等)由读侧按条目的来源与类型派生,传输层只转写原词。
type projectionBody struct {
	Version      string                `json:"version"`
	Parcel       string                `json:"parcel"`
	DerivedAt    string                `json:"derivedAt"`
	PriorVersion string                `json:"priorVersion,omitempty"`
	Entries      []projectionEntryBody `json:"entries"`
}

// projectionEntryBody 是一条已接受事实引用与其里程碑归类。milestone 缺席即「按该
// 映射版本无法可靠归类」——未归类是真话不是缺陷,不得为时间线完整补一个宽泛值;
// supersedes 缺席即首登事实(替代关系由源上下文指名,本上下文只登记)。
type projectionEntryBody struct {
	Source         string `json:"source"`
	Fact           string `json:"fact"`
	Kind           string `json:"kind"`
	FactVersion    string `json:"factVersion"`
	Supersedes     string `json:"supersedes,omitempty"`
	OccurredAt     string `json:"occurredAt"`
	EffectiveAt    string `json:"effectiveAt"`
	ReceivedAt     string `json:"receivedAt"`
	MappingVersion string `json:"mappingVersion"`
	Milestone      string `json:"milestone,omitempty"`
}

func projectionBodyOf(projection domain.TrackingProjection) projectionBody {
	entries := projection.Entries()
	body := projectionBody{
		Version:   projection.Version().String(),
		Parcel:    projection.Parcel().String(),
		DerivedAt: projection.DerivedAt().UTC().Format(time.RFC3339Nano),
		Entries:   make([]projectionEntryBody, 0, len(entries)),
	}
	if prior, superseding := projection.PriorVersion(); superseding {
		body.PriorVersion = prior.String()
	}
	for _, entry := range entries {
		fact := entry.Fact()
		entryBody := projectionEntryBody{
			Source:         fact.Source().String(),
			Fact:           fact.Fact().String(),
			Kind:           fact.Kind().String(),
			FactVersion:    fact.Version().String(),
			OccurredAt:     fact.OccurredAt().UTC().Format(time.RFC3339Nano),
			EffectiveAt:    fact.EffectiveAt().UTC().Format(time.RFC3339Nano),
			ReceivedAt:     fact.ReceivedAt().UTC().Format(time.RFC3339Nano),
			MappingVersion: entry.MappingVersion().String(),
		}
		if supersedes, superseding := fact.Supersedes(); superseding {
			entryBody.Supersedes = supersedes.String()
		}
		if milestone, classified := entry.Milestone(); classified {
			entryBody.Milestone = milestone.String()
		}
		body.Entries = append(body.Entries, entryBody)
	}
	return body
}
