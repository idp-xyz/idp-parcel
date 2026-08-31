package governancehttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
)

const (
	codeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	codeMalformedRequest = "MALFORMED_REQUEST"
	codeIntakeFailed     = "INTAKE_FAILED"
	codeNoAnswerFormed   = "NO_ANSWER_FORMED"
)

// GovernanceRegistryReader 是治理登记册端点消费的读口。上列的是检索列面：恢复决定
// 的盘点 jsonb 不透出——见 ports.GovernanceRegistryRead 的口面纪律。读口不收租户，
// 与端点同句（ADR-0083 Decision 二）。
type GovernanceRegistryReader interface {
	ListAuthorityIntervals(ctx context.Context, limit int) ([]ports.AuthorityIntervalRegistryRow, error)
	ListSuspensions(ctx context.Context, limit int) ([]ports.SuspensionRegistryRow, error)
	ListResumptions(ctx context.Context, limit int) ([]ports.ResumptionRegistryRow, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ GovernanceRegistryReader = ports.GovernanceRegistryRead(nil)

// 业务结果的封闭集合：三册各占一格。空册如实答空列表走 2xx 成格，不折成未配置
// （ADR-0077 Decision 四原文适用：空册的续办是运营方去登记口登记，未配置的续办是
// 接入方去配置渠道——恢复动作不同）。
const (
	outcomeAuthorityIntervalsListed = "AUTHORITY_INTERVALS_LISTED"
	outcomeSuspensionsListed        = "SUSPENSIONS_LISTED"
	outcomeResumptionsListed        = "RESUMPTIONS_LISTED"
)

// register 查询参数的封闭三册。词取登记 CLI 子命令与库表的既有转写——登记口与
// 查阅口对同一册用同一个词，页面不必维护第二套对照表。阶段评审与接管两册第二批
// 未开（票 12 首批三类裁决），不在封闭集内——加词属新裁决，不属装配。
const (
	registerAuthorityInterval = "authority-interval"
	registerSuspension        = "suspension"
	registerResumption        = "resumption"
)

// NewQueryGovernanceRegistersEndpoint 交回治理登记册查阅的 HTTP 入口
// （GET /governance-registers；最终路径归装配票，本批为 closure-batch/07）。
//
// 三册共用一个端点，按 `register` 查询参数分派（先例：networkrouting 目录端点一口
// 七分派）：参数在场与否、取值在不在封闭集内属传输形状，先于 Intake；未配置 Intake
// 对全部分支同答 403，分支选择不泄露任何东西。缺席按坏请求拒：替调用方默认一册就是
// 替它猜。
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

		register := request.URL.Query().Get("register")
		if register != registerAuthorityInterval && register != registerSuspension &&
			register != registerResumption {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}

		query, err := intake.IntakeRegistryQuery(request.Context(), request)
		if err != nil {
			writeRegistryIntakeProblem(response, err)
			return
		}
		ctx := request.Context()
		limit := query.Limit

		switch register {
		case registerAuthorityInterval:
			serveRegistryList(response, outcomeAuthorityIntervalsListed, "intervals",
				authorityIntervalBodyOf,
				func() ([]ports.AuthorityIntervalRegistryRow, error) {
					return reader.ListAuthorityIntervals(ctx, limit)
				})
		case registerSuspension:
			serveRegistryList(response, outcomeSuspensionsListed, "suspensions",
				suspensionBodyOf,
				func() ([]ports.SuspensionRegistryRow, error) {
					return reader.ListSuspensions(ctx, limit)
				})
		case registerResumption:
			serveRegistryList(response, outcomeResumptionsListed, "resumptions",
				resumptionBodyOf,
				func() ([]ports.ResumptionRegistryRow, error) {
					return reader.ListResumptions(ctx, limit)
				})
		}
	})
}

// serveRegistryList 是三册共用的转写：读回、逐行转体、2xx 成格。读不回是答案未形成
// （5xx），不伪装成空册——前者该重试，后者是终局答案。
func serveRegistryList[Entry any, Body any](
	response http.ResponseWriter,
	outcome string,
	field string,
	bodyOf func(Entry) Body,
	list func() ([]Entry, error),
) {
	entries, err := list()
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	// 空册交回空数组而不是 null：调用方判「没有行」不该先判「有没有字段」。
	bodies := make([]Body, 0, len(entries))
	for _, entry := range entries {
		bodies = append(bodies, bodyOf(entry))
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"outcome": outcome,
		field:     bodies,
	})
}

func writeRegistryIntakeProblem(response http.ResponseWriter, err error) {
	if errors.Is(err, ErrAccessChannelNotConfigured) {
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
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
