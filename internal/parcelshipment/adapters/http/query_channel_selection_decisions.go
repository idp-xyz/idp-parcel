package shipmenthttp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 渠道择优决定查阅端点（票 label-channel/23；票 14 裁决「谁读它：运营查阅面，沿 ADR-0077 读面通例另立读口
// 与票；本票只落登记」——本文件就是那个读面）。
//
// 本端点只有读。并列冲突的人工裁决**不在这里**：裁决人与裁决规则属 `PAR-NET-16` 待提供的实例半边，机制侧
// 「人工裁决」是什么形状（一条规则引用换成人工那一格的新决定记录，还是别的）要先过 /domain-modeling，
// 不在读面票里顺手定。读面只把冲突列出来，不给动作。
//
// 读面透出的是**引用与四格**：候选引用、评价引用、选中/落选/出局/并列、出局因由。没有金额、没有评价内容——
// 金额要看去 parcel-pricing 的评价（票 23 红线，与决定记录本身「只引用不拷贝」同一条）。

// ChannelSelectionDecisionQuery 是一次已授权的渠道择优决定查阅：作用域只有租户，页大小由接入面按渠道契约
// 裁决，两样都不采信调用方自报。
//
// 它刻意不收 AuthorizedQueryScope（理由同 LabelTransactionQuery）：择优对象是商业范围与产品—渠道映射，不是
// 客户账户，本册没有账户维可分；收一个必带账户维的作用域等于在类型上声称会按账户过滤而它不会。
type ChannelSelectionDecisionQuery struct {
	Tenant domain.TenantID
	Limit  int
}

// ChannelSelectionDecisionQueryIntake 把一次已认证的运营查阅请求翻译成查询。它是接口而非解析代码，理由同本包
// 其余 Intake：作用域整组只能来自认证与授权结果（操作者渠道 ADR-0100，真 Intake 未就位），采信自报租户会穿透 ADR-0003 的隔离
// 边界。未决期间本包不带任何采信实现，包括「开发用」的采信头部版本。
type ChannelSelectionDecisionQueryIntake interface {
	IntakeChannelSelectionDecisionQuery(ctx context.Context, request *http.Request) (ChannelSelectionDecisionQuery, error)
}

// ChannelSelectionDecisionsReader 是本端点消费的读口：查阅不触发判断、决定或披露，所以接存储读面，不接应用编排。
type ChannelSelectionDecisionsReader interface {
	ListTiedChannelSelectionDecisions(
		ctx context.Context,
		tenant domain.TenantID,
		filter ports.TiedChannelSelectionFilter,
		limit int,
	) ([]domain.ChannelSelectionDecision, error)
	FindChannelSelectionDecision(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.ChannelSelectionDecisionID,
	) (domain.ChannelSelectionDecision, bool, error)
}

// 编译期锁缝：读口形状与端口保持一致——本适配器不新造查询语义。
var _ ChannelSelectionDecisionsReader = ports.ChannelSelectionDecisionRead(nil)

// 本端点的两格准入实现。未配置一格同本包其余 Intake：不读请求，只答未配置。隔离读放行一格只交出注入作用域的
// 租户维与页大小（判据同 IntakeLabelTransactionQuery：少的那一维是本册压根没有的那一维，不是被丢掉的过滤条件）。
var (
	_ ChannelSelectionDecisionQueryIntake = UnconfiguredIntake{}
	_ ChannelSelectionDecisionQueryIntake = IsolatedOperationsReadIntake{}
)

// IntakeChannelSelectionDecisionQuery 不读请求（参数匿名）：渠道未配置就无从铸造作用域。
func (UnconfiguredIntake) IntakeChannelSelectionDecisionQuery(context.Context, *http.Request) (ChannelSelectionDecisionQuery, error) {
	return ChannelSelectionDecisionQuery{}, ErrAccessChannelNotConfigured
}

// IntakeChannelSelectionDecisionQuery 不读请求（参数匿名）：作用域整组来自注入，答复与请求内容及自报身份无关。
func (intake IsolatedOperationsReadIntake) IntakeChannelSelectionDecisionQuery(context.Context, *http.Request) (ChannelSelectionDecisionQuery, error) {
	return ChannelSelectionDecisionQuery{Tenant: intake.scope.TenantID(), Limit: intake.limit}, nil
}

// 业务结果的封闭集合。没有冲突仍是 LISTED：空列表是「该租户此刻没有等人工裁决的择优」这个如实答案，与
// 「接入渠道未配置」（403）分得开。单份查阅查不到不在这里——按 ADR-0022 它是 404，`统一不可见结果`不区分
// 「不存在」与「属别的租户」，也因此 4xx 不携带 outcome。
const (
	outcomeTiedChannelSelectionDecisionsListed = "TIED_CHANNEL_SELECTION_DECISIONS_LISTED"
	outcomeChannelSelectionDecision            = "CHANNEL_SELECTION_DECISION"
)

// codeChannelSelectionDecisionNotVisible 是`统一不可见结果`的传输层表达：单一 code、无自由文本，对象存在与否、
// 属谁，从这格答复里读不出来（ADR-0029 的探针纪律）。
const codeChannelSelectionDecisionNotVisible = "CHANNEL_SELECTION_DECISION_NOT_VISIBLE"

// 视图词是传输形状，封闭集今天只有一格：并列冲突。它仍然必备——「没传就当全部」是隐含默认，而「全部决定」
// 是另一种读法（择优历史按对象看，在写口伴生的 ListBySubject 上），不在本端点顺手长出来。
const viewTiedChannelSelections = "tied"

// NewQueryChannelSelectionDecisionsEndpoint 交回渠道择优决定查阅的 HTTP 入口（GET /channel-selection-decisions）。
//
// 列表与单份共用一个端点，按 `decisionId` 查询参数分派（与 /shipment-request-views 同一形状）：参数是否在场属
// 传输形状，先于 Intake；读它不构成读业务内容——未配置 Intake 对两条分支同答 403，分支选择不泄露任何东西。
//
// 列表分支：`view=tied` 必备；`scope` 与 `mapping` 要么都给（按对象收窄）、要么都不给，只给一半是说不清对象的
// 请求。不做任何按时间的隐式截断（票 23「先答再开工」第二条）：窄口由查询参数显式给，今天只有对象一维。
func NewQueryChannelSelectionDecisionsEndpoint(
	intake ChannelSelectionDecisionQueryIntake,
	reader ChannelSelectionDecisionsReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		if request.URL.Query().Has("decisionId") {
			serveChannelSelectionDecision(response, request, intake, reader)
			return
		}
		serveTiedChannelSelectionDecisions(response, request, intake, reader)
	})
}

func serveTiedChannelSelectionDecisions(
	response http.ResponseWriter,
	request *http.Request,
	intake ChannelSelectionDecisionQueryIntake,
	reader ChannelSelectionDecisionsReader,
) {
	filter, err := tiedChannelSelectionFilterOf(request)
	if err != nil {
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	}
	query, err := intake.IntakeChannelSelectionDecisionQuery(request.Context(), request)
	if err != nil {
		writeIntakeProblem(response, err)
		return
	}

	decisions, err := reader.ListTiedChannelSelectionDecisions(request.Context(), query.Tenant, filter, query.Limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	// 空列表交回空数组而不是 null：调用方判「没有冲突」不该先判「有没有字段」。
	bodies := make([]channelSelectionDecisionBody, 0, len(decisions))
	for _, decision := range decisions {
		bodies = append(bodies, channelSelectionDecisionBodyOf(decision))
	}
	writeJSON(response, http.StatusOK, channelSelectionDecisionsListResponse{
		Outcome:   outcomeTiedChannelSelectionDecisionsListed,
		Decisions: bodies,
	})
}

// tiedChannelSelectionFilterOf 从查询参数读列表分支的传输形状：视图词与可选的对象收窄。
func tiedChannelSelectionFilterOf(request *http.Request) (ports.TiedChannelSelectionFilter, error) {
	values := request.URL.Query()
	if values.Get("view") != viewTiedChannelSelections {
		return ports.TiedChannelSelectionFilter{}, fmt.Errorf("%w: view must be %q", ErrMalformedRequest, viewTiedChannelSelections)
	}
	hasScope, hasMapping := values.Has("scope"), values.Has("mapping")
	if !hasScope && !hasMapping {
		return ports.EveryTiedChannelSelection(), nil
	}
	if hasScope != hasMapping {
		return ports.TiedChannelSelectionFilter{}, fmt.Errorf("%w: scope and mapping must be given together", ErrMalformedRequest)
	}
	scope, err := domain.NewCommercialScopeReference(strings.TrimSpace(values.Get("scope")))
	if err != nil {
		return ports.TiedChannelSelectionFilter{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	mapping, err := domain.NewProductChannelMappingReference(strings.TrimSpace(values.Get("mapping")))
	if err != nil {
		return ports.TiedChannelSelectionFilter{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	subject, err := domain.NewChannelSelectionSubject(scope, mapping)
	if err != nil {
		return ports.TiedChannelSelectionFilter{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return ports.TiedChannelSelectionsOf(subject), nil
}

func serveChannelSelectionDecision(
	response http.ResponseWriter,
	request *http.Request,
	intake ChannelSelectionDecisionQueryIntake,
	reader ChannelSelectionDecisionsReader,
) {
	// 标识只定位候选对象，不单独证明查询权限——权限在 Intake 交出的租户上；读定位参数属传输形状，不是被禁的
	// 授权输入（ADR-0078 Decision 二）。
	id, err := domain.NewChannelSelectionDecisionID(strings.TrimSpace(request.URL.Query().Get("decisionId")))
	if err != nil {
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	}
	query, err := intake.IntakeChannelSelectionDecisionQuery(request.Context(), request)
	if err != nil {
		writeIntakeProblem(response, err)
		return
	}

	decision, found, err := reader.FindChannelSelectionDecision(request.Context(), query.Tenant, id)
	if err != nil {
		// 读不回是答案未形成，不是「不可见」——伪装成后者会让一次该重试的故障变成终局。
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	if !found {
		writeProblem(response, http.StatusNotFound, codeChannelSelectionDecisionNotVisible)
		return
	}
	body := channelSelectionDecisionBodyOf(decision)
	writeJSON(response, http.StatusOK, channelSelectionDecisionResponse{
		Outcome:  outcomeChannelSelectionDecision,
		Decision: &body,
	})
}

type channelSelectionDecisionsListResponse struct {
	Outcome   string                         `json:"outcome"`
	Decisions []channelSelectionDecisionBody `json:"decisions"`
}

type channelSelectionDecisionResponse struct {
	Outcome  string                        `json:"outcome"`
	Decision *channelSelectionDecisionBody `json:"decision"`
}

// channelSelectionDecisionBody 逐字段透出一条决定记录。selectedCandidate 只在有选中者时在场：并列冲突与无人参选
// 两格都没有选中者，缺席就是「这一次没选出来」，不拿空串顶上。四格与因由词原样透出，是领域封闭集的 String()。
type channelSelectionDecisionBody struct {
	DecisionID        string                          `json:"decisionId"`
	Scope             string                          `json:"scope"`
	Mapping           string                          `json:"mapping"`
	AssembledAsOf     string                          `json:"assembledAsOf"`
	Rule              string                          `json:"rule"`
	DecidedAt         string                          `json:"decidedAt"`
	Conclusion        string                          `json:"conclusion"`
	SelectedCandidate string                          `json:"selectedCandidate,omitempty"`
	Candidates        []channelSelectionCandidateBody `json:"candidates"`
}

// channelSelectionCandidateBody 是逐候选一行：评价引用有则带（没登记价卡的候选没经过评价，如实缺席），出局因由
// 只随 EXCLUDED 在场。
type channelSelectionCandidateBody struct {
	Candidate  string `json:"candidate"`
	Evaluation string `json:"evaluation,omitempty"`
	Outcome    string `json:"outcome"`
	Exclusion  string `json:"exclusion,omitempty"`
}

func channelSelectionDecisionBodyOf(decision domain.ChannelSelectionDecision) channelSelectionDecisionBody {
	results := decision.Results()
	body := channelSelectionDecisionBody{
		DecisionID:    decision.ID().String(),
		Scope:         decision.Subject().Scope().String(),
		Mapping:       decision.Subject().Mapping().String(),
		AssembledAsOf: decision.AssembledAsOf().UTC().Format(time.RFC3339Nano),
		Rule:          decision.Rule().String(),
		DecidedAt:     decision.DecidedAt().UTC().Format(time.RFC3339Nano),
		Conclusion:    decision.Conclusion().String(),
		Candidates:    make([]channelSelectionCandidateBody, 0, len(results)),
	}
	if selected, chosen := decision.Selected(); chosen {
		body.SelectedCandidate = selected.String()
	}
	for _, result := range results {
		candidate := channelSelectionCandidateBody{
			Candidate: result.Candidate().String(),
			Outcome:   result.Outcome().String(),
		}
		if evaluation, present := result.Evaluation(); present {
			candidate.Evaluation = evaluation.String()
		}
		if grade, excluded := result.Exclusion(); excluded {
			candidate.Exclusion = grade.String()
		}
		body.Candidates = append(body.Candidates, candidate)
	}
	return body
}
