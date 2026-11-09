package governancehttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

const (
	codeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	codeMalformedRequest = "MALFORMED_REQUEST"
	codeIntakeFailed     = "INTAKE_FAILED"
	codeNoAnswerFormed   = "NO_ANSWER_FORMED"
)

// GovernanceRegistryReader 是治理登记册端点消费的读口。上列的是检索列面：恢复决定
// 与接管记录的盘点 jsonb 不透出——见 ports.GovernanceRegistryRead 的口面纪律。读口
// 不收租户，与端点同句（ADR-0083 Decision 二）。
type GovernanceRegistryReader interface {
	ListAuthorityIntervals(ctx context.Context, limit int, query cataloguepage.Query,
	) (cataloguepage.Result[ports.AuthorityIntervalRegistryRow], error)
	ListSuspensions(ctx context.Context, limit int, query cataloguepage.Query,
	) (cataloguepage.Result[ports.SuspensionRegistryRow], error)
	ListResumptions(ctx context.Context, limit int, query cataloguepage.Query,
	) (cataloguepage.Result[ports.ResumptionRegistryRow], error)
	ListTakeovers(ctx context.Context, limit int, query cataloguepage.Query,
	) (cataloguepage.Result[ports.TakeoverRegistryRow], error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ GovernanceRegistryReader = ports.GovernanceRegistryRead(nil)

// 业务结果的封闭集合：四册各占一格。空册如实答空列表走 2xx 成格，不折成未配置
// （ADR-0077 Decision 四原文适用：空册的续办是运营方去登记口登记，未配置的续办是
// 接入方去配置渠道——恢复动作不同）。
const (
	outcomeAuthorityIntervalsListed = "AUTHORITY_INTERVALS_LISTED"
	outcomeSuspensionsListed        = "SUSPENSIONS_LISTED"
	outcomeResumptionsListed        = "RESUMPTIONS_LISTED"
	outcomeTakeoversListed          = "TAKEOVERS_LISTED"
)

// register 查询参数的封闭四册。词取登记 CLI 子命令与库表的既有转写——登记口与
// 查阅口对同一册用同一个词，页面不必维护第二套对照表。接管册由 ADR-0155 打开。
// 阶段评审查阅仍未开，不在封闭集内。
const (
	registerAuthorityInterval = "authority-interval"
	registerSuspension        = "suspension"
	registerResumption        = "resumption"
	registerTakeover          = "takeover"
)

// registerCatalogues 是四册各自的查询声明（ADR-0144）：register 选中哪一册，就按哪一册的声明解码其余参数。
var registerCatalogues = map[string]*cataloguepage.Catalogue{
	registerAuthorityInterval: ports.AuthorityIntervalRegistryPages,
	registerSuspension:        ports.SuspensionRegistryPages,
	registerResumption:        ports.ResumptionRegistryPages,
	registerTakeover:          ports.TakeoverRegistryPages,
}

// NewQueryGovernanceRegistersEndpoint 交回治理登记册查阅的 HTTP 入口
// （GET /governance-registers；最终路径归装配票，本批为 closure-batch/07）。
//
// 四册共用一个端点，按 `register` 查询参数分派（先例：networkrouting 目录端点也是
// 一口分派多册）。register 与其余参数都在 Intake 之后读：未配置的渠道对一切请求同答 403，
// register 缺席或写错也一样，答复不随请求内容变（ADR-0055 决定三）。配置了的渠道上
// 缺席按坏请求拒：替调用方默认一册就是替它猜。本入口只查阅，不登记接管。其余参数
// （after、sort 与 q）按选中那一册的声明解码（writePageQueryProblem 头注）。
func NewQueryGovernanceRegistersEndpoint(
	intake RegistryQueryIntake,
	reader GovernanceRegistryReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		query, err := intake.IntakeRegistryQuery(request.Context(), request)
		if err != nil {
			writeRegistryIntakeProblem(response, err)
			return
		}
		values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		register := values.Get("register")
		catalogue, known := registerCatalogues[register]
		if !known {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		pageQuery, err := catalogue.Decode(values)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		ctx := request.Context()
		limit := query.Limit

		switch register {
		case registerAuthorityInterval:
			serveRegistryList(response, outcomeAuthorityIntervalsListed, "intervals", limit,
				authorityIntervalBodyOf,
				func() (cataloguepage.Result[ports.AuthorityIntervalRegistryRow], error) {
					return reader.ListAuthorityIntervals(ctx, limit, pageQuery)
				})
		case registerSuspension:
			serveRegistryList(response, outcomeSuspensionsListed, "suspensions", limit,
				suspensionBodyOf,
				func() (cataloguepage.Result[ports.SuspensionRegistryRow], error) {
					return reader.ListSuspensions(ctx, limit, pageQuery)
				})
		case registerResumption:
			serveRegistryList(response, outcomeResumptionsListed, "resumptions", limit,
				resumptionBodyOf,
				func() (cataloguepage.Result[ports.ResumptionRegistryRow], error) {
					return reader.ListResumptions(ctx, limit, pageQuery)
				})
		case registerTakeover:
			serveRegistryList(response, outcomeTakeoversListed, "takeovers", limit,
				takeoverBodyOf,
				func() (cataloguepage.Result[ports.TakeoverRegistryRow], error) {
					return reader.ListTakeovers(ctx, limit, pageQuery)
				})
		}
	})
}

// serveRegistryList 是四册共用的转写：读回、逐行转体、带上 page、2xx 成格。读不回是
// 答案未形成（5xx），不伪装成空册——前者该重试，后者是终局答案。
func serveRegistryList[Entry any, Body any](
	response http.ResponseWriter,
	outcome string,
	field string,
	limit int,
	bodyOf func(Entry) Body,
	list func() (cataloguepage.Result[Entry], error),
) {
	page, err := list()
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	// 空册交回空数组而不是 null：调用方判「没有行」不该先判「有没有字段」。
	bodies := make([]Body, 0, len(page.Rows))
	for _, entry := range page.Rows {
		bodies = append(bodies, bodyOf(entry))
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"outcome": outcome,
		field:     bodies,
		"page":    page.Page(limit),
	})
}

func writeRegistryIntakeProblem(response http.ResponseWriter, err error) {
	if errors.Is(err, ErrAccessChannelNotConfigured) {
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
		return
	}
	if errors.Is(err, ErrOperatorCredentialRejected) {
		writeProblem(response, http.StatusUnauthorized, codeOperatorCredentialRejected)
		return
	}
	if errors.Is(err, ErrOperatorNotGranted) {
		writeProblem(response, http.StatusForbidden, codeOperatorNotGranted)
		return
	}
	if errors.Is(err, ErrIdentityDependencyUnavailable) {
		writeProblem(response, http.StatusServiceUnavailable, codeIdentityDependencyUnavailable)
		return
	}
	writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
}

// 行体逐字段透出检索列面，全部是脱敏引用（建表纪律与登记 CLI 纪律保证）。toAt 缺席
// 即开放区间——缺席是真话不是缺陷，不为区间齐整补一个编造的「无限远」时刻。
type authorityIntervalBody struct {
	ObjectScope string `json:"objectScope"`
	Capability  string `json:"capability"`
	FactKind    string `json:"factKind"`
	Authority   string `json:"authority"`
	FromAt      string `json:"fromAt"`
	ToAt        string `json:"toAt,omitempty"`
	InsertedAt  string `json:"insertedAt"`
}

func authorityIntervalBodyOf(row ports.AuthorityIntervalRegistryRow) authorityIntervalBody {
	body := authorityIntervalBody{
		ObjectScope: row.ObjectScope,
		Capability:  row.Capability,
		FactKind:    row.FactKind,
		Authority:   row.Authority,
		FromAt:      rfc3339(row.FromAt),
		InsertedAt:  rfc3339(row.InsertedAt),
	}
	if row.HasToAt {
		body.ToAt = rfc3339(row.ToAt)
	}
	return body
}

type suspensionBody struct {
	SuspensionID  string `json:"suspensionId"`
	TriggerSource string `json:"triggerSource"`
	Basis         string `json:"basis"`
	Evidence      string `json:"evidence"`
	Scope         string `json:"scope"`
	ExecutedBy    string `json:"executedBy"`
	OccurredAt    string `json:"occurredAt"`
	EffectiveAt   string `json:"effectiveAt"`
	InTransitNote string `json:"inTransitNote"`
}

func suspensionBodyOf(row ports.SuspensionRegistryRow) suspensionBody {
	return suspensionBody{
		SuspensionID:  row.SuspensionID,
		TriggerSource: row.TriggerSource,
		Basis:         row.Basis,
		Evidence:      row.Evidence,
		Scope:         row.Scope,
		ExecutedBy:    row.ExecutedBy,
		OccurredAt:    rfc3339(row.OccurredAt),
		EffectiveAt:   rfc3339(row.EffectiveAt),
		InTransitNote: row.InTransitNote,
	}
}

type resumptionBody struct {
	SuspensionID     string `json:"suspensionId"`
	ReleaseEvidence  string `json:"releaseEvidence"`
	ConsistencyCheck string `json:"consistencyCheck"`
	InventoryTakenAt string `json:"inventoryTakenAt"`
	DecidedBy        string `json:"decidedBy"`
	DecidedAt        string `json:"decidedAt"`
	EffectiveAt      string `json:"effectiveAt"`
}

type takeoverBody struct {
	ObjectScope      string `json:"objectScope"`
	Capability       string `json:"capability"`
	FactKind         string `json:"factKind"`
	Authority        string `json:"authority"`
	FromAt           string `json:"fromAt"`
	ToAt             string `json:"toAt,omitempty"`
	StopEvidence     string `json:"stopEvidence"`
	AcceptedFacts    string `json:"acceptedFacts"`
	PendingExternals string `json:"pendingExternals"`
	ActualControl    string `json:"actualControl"`
	Responsibilities string `json:"responsibilities"`
	NextAction       string `json:"nextAction"`
	InventoryTakenAt string `json:"inventoryTakenAt"`
	EffectiveAt      string `json:"effectiveAt"`
}

func takeoverBodyOf(row ports.TakeoverRegistryRow) takeoverBody {
	body := takeoverBody{
		ObjectScope:      row.ObjectScope,
		Capability:       row.Capability,
		FactKind:         row.FactKind,
		Authority:        row.Authority,
		FromAt:           rfc3339(row.FromAt),
		StopEvidence:     row.StopEvidence,
		AcceptedFacts:    row.AcceptedFacts,
		PendingExternals: row.PendingExternals,
		ActualControl:    row.ActualControl,
		Responsibilities: row.Responsibilities,
		NextAction:       row.NextAction,
		InventoryTakenAt: rfc3339(row.InventoryTakenAt),
		EffectiveAt:      rfc3339(row.EffectiveAt),
	}
	if row.HasToAt {
		body.ToAt = rfc3339(row.ToAt)
	}
	return body
}

func resumptionBodyOf(row ports.ResumptionRegistryRow) resumptionBody {
	return resumptionBody{
		SuspensionID:     row.SuspensionID,
		ReleaseEvidence:  row.ReleaseEvidence,
		ConsistencyCheck: row.ConsistencyCheck,
		InventoryTakenAt: rfc3339(row.InventoryTakenAt),
		DecidedBy:        row.DecidedBy,
		DecidedAt:        rfc3339(row.DecidedAt),
		EffectiveAt:      rfc3339(row.EffectiveAt),
	}
}

func rfc3339(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

type problemResponse struct {
	Error problemDetail `json:"error"`
}

// problemDetail 的 detail 只随「改请求才会好」的 4xx 在场，是给操作者看的理由散文，
// 调用方不据此分支；为空时那一格缺席，不写空串。
type problemDetail struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

func writeProblem(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code}})
}

// writeProblemWithDetail 是 writeProblem 带理由散文的兄弟。单立一个函数而不是给
// writeProblem 加变参：交不交 detail 在调用点要一眼读得出来——5xx 照旧只交 code，那是
// 依赖故障的内部原文，不外泄。
func writeProblemWithDetail(response http.ResponseWriter, status int, code string, detail string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code, Detail: detail}})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
