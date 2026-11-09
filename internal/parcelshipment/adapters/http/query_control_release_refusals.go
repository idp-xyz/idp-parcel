package shipmenthttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 释放未受理册的运营查阅端点（ADR-0293 决定一）。
//
// 本端点只有读。处理（核对后在结算侧释放、标记已处理）是「运营决定」能力面上另一个端点的命令，不在查阅面里长出动作。查阅给的是
// 此刻未处理的行与全部未处理的计数；告警阈值是产品策略、本产品没有告警引擎，这里只透出计数与清单是否被页大小截断。

// ControlReleaseRefusalQuery 是一次已授权的释放未受理册查阅：作用域只有租户（册行按租户切，不按客户账户），页大小由接入面按渠道契约裁决，
// 两样都不采信调用方自报。与面单交易查阅同理，它刻意不收必带账户维的 AuthorizedQueryScope。
type ControlReleaseRefusalQuery struct {
	Tenant domain.TenantID
	Limit  int
}

// ControlReleaseRefusalQueryIntake 把一次已认证的运营查阅请求翻译成查询。它是接口而非解析代码，理由同本包其余 Intake：作用域整组只能来自
// 认证与授权结果（操作者渠道 ADR-0100，真实现是 OperatorOperationsReadIntake），采信自报租户会穿透 ADR-0003 的隔离边界。
type ControlReleaseRefusalQueryIntake interface {
	IntakeControlReleaseRefusalQuery(ctx context.Context, request *http.Request) (ControlReleaseRefusalQuery, error)
}

// ControlReleaseRefusalCatalogueReader 是本端点消费的读口：查阅不触发处理，所以接存储读面。
type ControlReleaseRefusalCatalogueReader interface {
	ListUnhandledControlReleaseRefusals(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (ports.UnhandledControlReleaseRefusals, error)
}

var (
	// 编译期锁缝：读口形状与端口保持一致，本适配器不新造查询语义。
	_ ControlReleaseRefusalCatalogueReader = ports.ControlReleaseRefusalCatalogue(nil)
	_ ControlReleaseRefusalQueryIntake     = UnconfiguredIntake{}
)

// IntakeControlReleaseRefusalQuery 不读请求（参数匿名）：渠道未配置就无从铸造作用域。
func (UnconfiguredIntake) IntakeControlReleaseRefusalQuery(context.Context, *http.Request) (ControlReleaseRefusalQuery, error) {
	return ControlReleaseRefusalQuery{}, ErrAccessChannelNotConfigured
}

// 业务结果的封闭集合。空册仍是 LISTED：空清单是「此刻没有积压」这个如实答案，与「接入渠道未配置」（403）分得开。
const outcomeControlReleaseRefusalsListed = "CONTROL_RELEASE_REFUSALS_LISTED"

// NewQueryControlReleaseRefusalsEndpoint 交回释放未受理册查阅的 HTTP 入口（GET /control-release-refusals）。查询参数按
// ports.ControlReleaseRefusalPages 解码（ADR-0144），在 Intake 之后；unhandledCount 仍是整册积压，truncated 即本页之后还有。
func NewQueryControlReleaseRefusalsEndpoint(
	intake ControlReleaseRefusalQueryIntake,
	reader ControlReleaseRefusalCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		query, err := intake.IntakeControlReleaseRefusalQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		pageQuery, err := ports.ControlReleaseRefusalPages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		listed, err := reader.ListUnhandledControlReleaseRefusals(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			// 读不回是答案未形成，不伪装成空册：把一次该重试的故障读成「没有积压」，积压就更没人看见了。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		// 空列表交回空数组而不是 null：调用方判「没有积压」不该先判「有没有字段」。
		bodies := make([]controlReleaseRefusalBody, 0, len(listed.Refusals))
		for _, refusal := range listed.Refusals {
			body := controlReleaseRefusalBodyOf(refusal)
			if numbering, found := listed.DocumentNumbers[refusal.ShipmentRequestID]; found {
				if number, numbered := numbering.Number(); numbered {
					body.DocumentNumber = number.String()
				}
				if absence, absent := numbering.Absence(); absent {
					body.DocumentNumberAbsence = absence.String()
				}
			}
			bodies = append(bodies, body)
		}
		writeJSON(response, http.StatusOK, controlReleaseRefusalsResponse{
			Outcome:        outcomeControlReleaseRefusalsListed,
			UnhandledCount: listed.Count,
			Truncated:      listed.Next != "",
			Refusals:       bodies,
			Page:           cataloguepage.NewPage(query.Limit, listed.Next, listed.Total),
		})
	})
}

type controlReleaseRefusalsResponse struct {
	Outcome        string                      `json:"outcome"`
	UnhandledCount int                         `json:"unhandledCount"`
	Truncated      bool                        `json:"truncated"`
	Refusals       []controlReleaseRefusalBody `json:"refusals"`
	Page           cataloguepage.Page          `json:"page"`
}

// controlReleaseRefusalBody 逐字段透出册上一行。occupationScope 只在存下了作用域时在场：缺席即施加时还不存作用域的存量行，scopeRecorded
// 把这一点显式交代，页面不必靠「字段不在」去猜。reason 是提供方给的（或本上下文译不动答复时自有的）稳定原因引用，原样透出。
// documentNumber 是行所指委托的单据号（管理台叫运单号，ADR-0355 决定十一），没有号时整键不出现、改带 documentNumberAbsence；
// 委托读不到时两格都不出现。
type controlReleaseRefusalBody struct {
	ShipmentRequestID     string               `json:"shipmentRequestId"`
	DocumentNumber        string               `json:"documentNumber,omitempty"`
	DocumentNumberAbsence string               `json:"documentNumberAbsence,omitempty"`
	SubmissionVersionID   string               `json:"submissionVersionId"`
	ControlResultID       string               `json:"controlResultId"`
	CustomerAccountID     string               `json:"customerAccountId"`
	ScopeRecorded         bool                 `json:"scopeRecorded"`
	OccupationScope       *occupationScopeBody `json:"occupationScope,omitempty"`
	Reason                string               `json:"reason"`
	RecordedAt            string               `json:"recordedAt"`
}

type occupationScopeBody struct {
	LegalEntity       string `json:"legalEntity"`
	SettlementAccount string `json:"settlementAccount"`
	Currency          string `json:"currency"`
}

func controlReleaseRefusalBodyOf(refusal ports.ControlReleaseRefusal) controlReleaseRefusalBody {
	body := controlReleaseRefusalBody{
		ShipmentRequestID:   refusal.ShipmentRequestID.String(),
		SubmissionVersionID: refusal.SubmissionVersion.String(),
		ControlResultID:     refusal.ControlResultID.String(),
		CustomerAccountID:   refusal.CustomerAccountID.String(),
		ScopeRecorded:       refusal.Scope.Recorded(),
		Reason:              refusal.Reason.String(),
		RecordedAt:          refusal.RecordedAt.UTC().Format(time.RFC3339Nano),
	}
	if refusal.Scope.Recorded() {
		body.OccupationScope = &occupationScopeBody{
			LegalEntity:       refusal.Scope.LegalEntity(),
			SettlementAccount: refusal.Scope.Account(),
			Currency:          refusal.Scope.Currency(),
		}
	}
	return body
}
