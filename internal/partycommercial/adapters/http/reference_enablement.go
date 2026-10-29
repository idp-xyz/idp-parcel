package commercialhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 启用范围登记册的在线口（ADR-0345）：某租户启用哪些国家 / 地区与币种由租户登记，没登记就是“未配置”。两个端点——
// 登记（POST /reference-data/enablement-registrations）与读回（GET /reference-data/enablement?kind=）。
// 启用与停用同是往修订链上追加一笔，所以只有一个登记口、没有 PATCH / DELETE。
//
// 本族没有受控 CLI，载荷只有在线这一份。登记责任方取认证信封里的操作者（ADR-0303 同写法“发行方#sub”），载荷里不许带。

// ReferenceEnablementRegistrationIntake 把一次已认证的接入请求翻译成启用范围登记命令。租户与登记责任方只能来自认证
// 结果——采信请求自称的会穿透 ADR-0003 的隔离边界，也会让审计事实变成自报。真实现是操作者渠道的 OperatorRegistryIntake。
type ReferenceEnablementRegistrationIntake interface {
	IntakeReferenceEnablementRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterReferenceEnablementCommand, error)
}

var _ ReferenceEnablementRegistrationIntake = (*OperatorRegistryIntake)(nil)

// referenceEnablementDocument 是登记口的在线载荷。tenantId 一格只为拒而存在（理由在 partyIdentityBatchDocument.TenantID）。
// 修订与生效时点用指针，缺席与零值分得开：缺了就是请求畸形，而不是替登记方补一个 0 或零时刻。
type referenceEnablementDocument struct {
	TenantID      json.RawMessage `json:"tenantId"`
	Kind          string          `json:"kind"`
	Code          string          `json:"code"`
	Revision      *int            `json:"revision"`
	Status        string          `json:"status"`
	EffectiveFrom *time.Time      `json:"effectiveFrom"`
	Source        string          `json:"source"`
}

func referenceEnablementTenant(document referenceEnablementDocument) []byte { return document.TenantID }

func (document referenceEnablementDocument) command(
	tenant domain.TenantID,
	registrant domain.OperatorSubjectReference,
) (application.RegisterReferenceEnablementCommand, error) {
	var none application.RegisterReferenceEnablementCommand
	kind, known := domain.ReferenceKindNamed(document.Kind)
	if !known {
		return none, fmt.Errorf("unknown reference kind %q", document.Kind)
	}
	code, err := domain.NewReferenceCode(document.Code)
	if err != nil {
		return none, err
	}
	status, known := domain.ReferenceEnablementStatusNamed(document.Status)
	if !known {
		return none, fmt.Errorf("unknown enablement status %q", document.Status)
	}
	if document.Revision == nil {
		return none, fmt.Errorf("revision is required")
	}
	if document.EffectiveFrom == nil {
		return none, fmt.Errorf("effectiveFrom is required")
	}
	source, err := domain.NewReferenceEnablementSource(document.Source)
	if err != nil {
		return none, err
	}
	return application.RegisterReferenceEnablementCommand{
		Tenant:        tenant,
		Kind:          kind,
		Code:          code,
		Revision:      *document.Revision,
		Status:        status,
		EffectiveFrom: *document.EffectiveFrom,
		Source:        source,
		Registrant:    registrant,
	}, nil
}

// IntakeReferenceEnablementRegistration 先认证、后解载荷：令牌不过的请求，请求体一个字节都不读。租户与登记责任方都取认证出
// 的身份。
func (intake *OperatorRegistryIntake) IntakeReferenceEnablementRegistration(ctx context.Context, request *http.Request) (application.RegisterReferenceEnablementCommand, error) {
	var none application.RegisterReferenceEnablementCommand
	operator, err := intake.authenticate(ctx, request)
	if err != nil {
		return none, err
	}
	document, err := decodeClosedDocument[referenceEnablementDocument](request)
	if err != nil {
		return none, err
	}
	if err := refuseSelfReportedTenant(referenceEnablementTenant(document)); err != nil {
		return none, err
	}
	return malformedUnless(document.command(operator.Tenant, operator.Operator.Reference()))
}

// ReferenceEnablementRegistrar 是登记端点转交的编排。事务边界在编排侧给出（登记写口无环境事务即拒），适配器只转交与映射，
// 不判断任何业务结果。
type ReferenceEnablementRegistrar interface {
	Register(
		ctx context.Context,
		command application.RegisterReferenceEnablementCommand,
	) (application.ReferenceEnablementResult, error)
}

// 编译期锁缝：本端点不新造登记语义，只消费 application 里那一个登记用例。
var _ ReferenceEnablementRegistrar = (*application.RegisterReferenceEnablementHandler)(nil)

// NewRegisterReferenceEnablementEndpoint 交回启用范围登记的 HTTP 入口：修订 1 是首笔，其后启用与停用都是下一修订。
func NewRegisterReferenceEnablementEndpoint(
	intake ReferenceEnablementRegistrationIntake,
	registrar ReferenceEnablementRegistrar,
) http.Handler {
	return newRegistrationEndpoint(
		intake.IntakeReferenceEnablementRegistration,
		registrar.Register,
		writeReferenceEnablementAnswer,
	)
}

// referenceEnablementAnswer 是登记口的封闭响应形状。`outcome` 与 `refusalReason` 都取应用结果枚举原名；`refusalReason` 只在
// 未受理时在场，是判别子；`cause` 是散文，不据此分支（纪律同 registrationAnswer.Cause）。
type referenceEnablementAnswer struct {
	Outcome       string `json:"outcome"`
	RefusalReason string `json:"refusalReason,omitempty"`
	Cause         string `json:"cause,omitempty"`
}

// writeReferenceEnablementAnswer 把应用答案逐名转写。已登记取 201，其余取 200：目录里没有这个码、修订错位是服务端形成的
// 业务答案（ADR-0022），折成 4xx 会让调用侧把一件“改内容再来”的事读成“请求本身不合法”。
func writeReferenceEnablementAnswer(response http.ResponseWriter, result application.ReferenceEnablementResult) {
	outcome := result.Outcome()
	name := outcome.String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := referenceEnablementAnswer{Outcome: name}
	if outcome == application.ReferenceEnablementNotAccepted {
		answer.RefusalReason = result.Refusal().String()
		if answer.RefusalReason == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		if cause := result.Cause(); cause != nil {
			answer.Cause = cause.Error()
		}
	}
	status := http.StatusOK
	if outcome == application.ReferenceEnablementRegistered {
		status = http.StatusCreated
	}
	writeJSON(response, status, answer)
}

// ReferenceEnablementReader 是读回端点消费的读口：此刻的状态由读侧编排导出，端点不自己判。
type ReferenceEnablementReader interface {
	ListReferenceEnablements(
		ctx context.Context,
		tenant domain.TenantID,
		kind domain.ReferenceKind,
	) (time.Time, []application.ReferenceEnablementEntry, error)
}

var _ ReferenceEnablementReader = (*application.QueryReferenceEnablementsHandler)(nil)

const (
	outcomeReferenceEnablementsListed = "REFERENCE_ENABLEMENTS_LISTED"
	// standingNotConfigured：此刻没有生效的行。它只出现在读回的 standing 上，不是可登记的状态。
	standingNotConfigured = "NOT_CONFIGURED"
)

// NewQueryReferenceEnablementsEndpoint 交回启用范围读回的 HTTP 入口（GET /reference-data/enablement?kind=，ADR-0077 模式）：
// 按种类读出本租户册上登过的每个码此刻的状态与全部修订。册上没有行的码不在结果里——它们是“未配置”，由缺席表达，端点不
// 替登记方铺满整份目录。租户只取自信封。
func NewQueryReferenceEnablementsEndpoint(
	intake CommercialCatalogueIntake,
	reader ReferenceEnablementReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}

		kindName := request.URL.Query().Get("kind")
		kind, known := domain.ReferenceKindNamed(kindName)
		if !known {
			writeProblemWithDetail(response, http.StatusBadRequest, codeMalformedRequest,
				fmt.Sprintf("kind must be COUNTRY or CURRENCY, got %q", kindName))
			return
		}

		asOf, entries, err := reader.ListReferenceEnablements(request.Context(), query.Scope.Tenant(), kind)
		if err != nil {
			// 读不回是答案未形成，不是“一个都没配”。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]referenceEnablementBody, 0, len(entries))
		for _, entry := range entries {
			bodies = append(bodies, referenceEnablementBodyOf(entry))
		}
		writeJSON(response, http.StatusOK, referenceEnablementListResponse{
			Outcome:     outcomeReferenceEnablementsListed,
			Kind:        kind.String(),
			AsOf:        rfc3339(asOf),
			Enablements: bodies,
		})
	})
}

type referenceEnablementListResponse struct {
	Outcome     string                    `json:"outcome"`
	Kind        string                    `json:"kind"`
	AsOf        string                    `json:"asOf"`
	Enablements []referenceEnablementBody `json:"enablements"`
}

// referenceEnablementBody 是一个码的读回：Standing 是 asOf 那一刻导出的状态，生效时点与来源单据说的是那一行；
// LatestRevision 是册上最新的修订号，登记方的下一笔登记它加一。History 带全部修订，含尚未到点的。
type referenceEnablementBody struct {
	Code           string                            `json:"code"`
	Standing       string                            `json:"standing"`
	EffectiveFrom  string                            `json:"effectiveFrom,omitempty"`
	LatestRevision int                               `json:"latestRevision"`
	History        []referenceEnablementRevisionBody `json:"history"`
}

type referenceEnablementRevisionBody struct {
	Revision      int    `json:"revision"`
	Status        string `json:"status"`
	EffectiveFrom string `json:"effectiveFrom"`
	Source        string `json:"source,omitempty"`
	RegisteredBy  string `json:"registeredBy"`
	RecordedAt    string `json:"recordedAt"`
}

func referenceEnablementBodyOf(entry application.ReferenceEnablementEntry) referenceEnablementBody {
	body := referenceEnablementBody{
		Code:     entry.Code.String(),
		Standing: standingNotConfigured,
		History:  make([]referenceEnablementRevisionBody, 0, len(entry.History)),
	}
	if entry.HasStanding {
		body.Standing = entry.Standing.Status().String()
		body.EffectiveFrom = rfc3339(entry.Standing.EffectiveFrom())
	}
	for _, row := range entry.History {
		body.History = append(body.History, referenceEnablementRevisionBody{
			Revision:      row.Revision(),
			Status:        row.Status().String(),
			EffectiveFrom: rfc3339(row.EffectiveFrom()),
			Source:        row.Source().String(),
			RegisteredBy:  row.Registrant().String(),
			RecordedAt:    rfc3339(row.RecordedAt()),
		})
		if row.Revision() > body.LatestRevision {
			body.LatestRevision = row.Revision()
		}
	}
	return body
}
