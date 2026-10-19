package nodeopshttp

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件是送站预报与送站交接单的运营查阅读（票 ps-external-mark-relations/13；UC-NO-006「权限、隔离与审计」）：管理台按节点与业务时刻列
// 两本册、按标识读一份。它与节点作业查阅页那一口同挂「主数据与运营查阅读」（ADR-0100 决定四），同一个 Intake，作用域只有租户；不收任何
// 登记或确认动作——交接单详情上的「确认」走运营决定口 /node-operations/delivery-handover-confirmations，不在这里。
//
// 门的次序是方法 → Intake → 请求内容（筛选与路径参数）：未配置要拒在一切请求内容之前（ADR-0055），路径与查询串也是请求内容。
//
// 按标识读查无答 404 + 各自那一个「看不见」码，与指名别的租户的对象逐字节同答——读口只答 found=false，端点只有一种答法（UC-NO-006
// 「指名别的账户的对象，与查无此对象同答」）。码取两本册命令口已在用的那两个，同一件事不立第二个词。

// DeliveryScopeCatalogueReader 是这四口消费的读口。
type DeliveryScopeCatalogueReader interface {
	ListDeliveryPreAdvices(
		ctx context.Context,
		tenant domain.TenantID,
		filter ports.DeliveryScopeCatalogueFilter,
		limit int,
	) ([]domain.DeliveryPreAdviceState, error)
	FindDeliveryPreAdvice(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.DeliveryPreAdviceID,
	) (domain.DeliveryPreAdviceState, bool, error)
	ListDeliveryHandoverNotes(
		ctx context.Context,
		tenant domain.TenantID,
		filter ports.DeliveryScopeCatalogueFilter,
		limit int,
	) ([]ports.DeliveryHandoverNoteCatalogueRow, error)
	FindDeliveryHandoverNoteDetail(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.DeliveryHandoverNoteID,
	) (ports.DeliveryHandoverNoteDetail, bool, error)
}

// 编译期锁缝：读口形状与端口保持一致。
var _ DeliveryScopeCatalogueReader = ports.DeliveryScopeCatalogueRead(nil)

const (
	outcomeDeliveryPreAdvicesListed    = "DELIVERY_PRE_ADVICES_LISTED"
	outcomeDeliveryPreAdviceFound      = "DELIVERY_PRE_ADVICE_FOUND"
	outcomeDeliveryHandoverNotesListed = "DELIVERY_HANDOVER_NOTES_LISTED"
	outcomeDeliveryHandoverNoteFound   = "DELIVERY_HANDOVER_NOTE_FOUND"
)

// 路由模式里的路径参数名；装配点的 Pattern 与这里必须同字，否则 PathValue 恒空、端点恒答 400——cmd/parcel-api 的操作者查阅读用例经真
// 路由钉住这一格。
const (
	preAdviceIDPathValue    = "preAdviceId"
	handoverNoteIDPathValue = "noteId"
)

// 两本册列表各自认的查询键：节点与业务时刻区间的两端。键名带上是哪一种业务时刻，免得一个 from 在两本册上说两件事。
var (
	preAdviceFilterKeys    = catalogueFilterKeys{from: "expectedArrivalFrom", until: "expectedArrivalUntil"}
	handoverNoteFilterKeys = catalogueFilterKeys{from: "openedFrom", until: "openedUntil"}
)

type catalogueFilterKeys struct {
	from  string
	until string
}

// catalogueFilterOf 读列表的筛选。键封闭：认不出的键、重复的键、空白节点、读不成 RFC 3339 的时刻、起点不早于终点都答坏请求——静默丢掉
// 一个拼错的键，人会把全租户的列表当成按条件筛过的。
func catalogueFilterOf(query url.Values, keys catalogueFilterKeys) (ports.DeliveryScopeCatalogueFilter, bool) {
	var filter ports.DeliveryScopeCatalogueFilter
	for key, values := range query {
		if len(values) != 1 {
			return ports.DeliveryScopeCatalogueFilter{}, false
		}
		switch key {
		case "node":
			node, err := domain.NewNodeReference(values[0])
			if err != nil {
				return ports.DeliveryScopeCatalogueFilter{}, false
			}
			filter.Node = node
		case keys.from, keys.until:
			at, err := time.Parse(time.RFC3339Nano, values[0])
			if err != nil {
				return ports.DeliveryScopeCatalogueFilter{}, false
			}
			if key == keys.from {
				filter.From = at
			} else {
				filter.Until = at
			}
		default:
			return ports.DeliveryScopeCatalogueFilter{}, false
		}
	}
	if !filter.From.IsZero() && !filter.Until.IsZero() && !filter.From.Before(filter.Until) {
		return ports.DeliveryScopeCatalogueFilter{}, false
	}
	return filter, true
}

// scopeReadEndpoint 是四口共用的外壳：方法门 → Intake → 交给各口自己读请求内容与读口。
func scopeReadEndpoint(intake CatalogueQueryIntake, serve func(http.ResponseWriter, *http.Request, CatalogueQuery)) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !guardGet(response, request) {
			return
		}
		query, ok := intakeCatalogueQuery(response, request, intake)
		if !ok {
			return
		}
		serve(response, request, query)
	})
}

// NewQueryDeliveryPreAdvicesEndpoint 交回预报列表（GET /node-operations-delivery-pre-advices）：按节点（node）与预计送达区间
// （expectedArrivalFrom 闭、expectedArrivalUntil 开）筛，新近在前。空册答 200 + []。
func NewQueryDeliveryPreAdvicesEndpoint(intake CatalogueQueryIntake, reader DeliveryScopeCatalogueReader) http.Handler {
	return scopeReadEndpoint(intake, func(response http.ResponseWriter, request *http.Request, query CatalogueQuery) {
		filter, ok := catalogueFilterOf(request.URL.Query(), preAdviceFilterKeys)
		if !ok {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		states, err := reader.ListDeliveryPreAdvices(request.Context(), query.Scope.Tenant(), filter, query.Limit)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]preAdviceCatalogueBody, 0, len(states))
		for _, state := range states {
			bodies = append(bodies, preAdviceCatalogueBodyOf(state))
		}
		writeJSON(response, http.StatusOK, preAdviceListResponse{Outcome: outcomeDeliveryPreAdvicesListed, PreAdvices: bodies})
	})
}

// NewQueryDeliveryPreAdviceEndpoint 交回一份预报（GET /node-operations-delivery-pre-advices/{preAdviceId}）。
func NewQueryDeliveryPreAdviceEndpoint(intake CatalogueQueryIntake, reader DeliveryScopeCatalogueReader) http.Handler {
	return scopeReadEndpoint(intake, func(response http.ResponseWriter, request *http.Request, query CatalogueQuery) {
		id, err := domain.NewDeliveryPreAdviceID(request.PathValue(preAdviceIDPathValue))
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		state, found, err := reader.FindDeliveryPreAdvice(request.Context(), query.Scope.Tenant(), id)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if !found {
			writeProblem(response, http.StatusNotFound, codeDeliveryPreAdviceNotVisible)
			return
		}
		writeJSON(response, http.StatusOK, preAdviceFoundResponse{Outcome: outcomeDeliveryPreAdviceFound, PreAdvice: preAdviceCatalogueBodyOf(state)})
	})
}

// NewQueryDeliveryHandoverNotesEndpoint 交回交接单列表（GET /node-operations-delivery-handover-notes）：按节点（node）与开立区间
// （openedFrom 闭、openedUntil 开）筛，新近在前。空册答 200 + []。
func NewQueryDeliveryHandoverNotesEndpoint(intake CatalogueQueryIntake, reader DeliveryScopeCatalogueReader) http.Handler {
	return scopeReadEndpoint(intake, func(response http.ResponseWriter, request *http.Request, query CatalogueQuery) {
		filter, ok := catalogueFilterOf(request.URL.Query(), handoverNoteFilterKeys)
		if !ok {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		rows, err := reader.ListDeliveryHandoverNotes(request.Context(), query.Scope.Tenant(), filter, query.Limit)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]handoverNoteCatalogueBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, handoverNoteCatalogueBodyOf(row.State, row.MemberCount))
		}
		writeJSON(response, http.StatusOK, handoverNoteListResponse{Outcome: outcomeDeliveryHandoverNotesListed, HandoverNotes: bodies})
	})
}

// NewQueryDeliveryHandoverNoteEndpoint 交回一张交接单与名下成员（GET /node-operations-delivery-handover-notes/{noteId}）。
func NewQueryDeliveryHandoverNoteEndpoint(intake CatalogueQueryIntake, reader DeliveryScopeCatalogueReader) http.Handler {
	return scopeReadEndpoint(intake, func(response http.ResponseWriter, request *http.Request, query CatalogueQuery) {
		id, err := domain.NewDeliveryHandoverNoteID(request.PathValue(handoverNoteIDPathValue))
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		detail, found, err := reader.FindDeliveryHandoverNoteDetail(request.Context(), query.Scope.Tenant(), id)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if !found {
			writeProblem(response, http.StatusNotFound, codeDeliveryHandoverNoteNotVisible)
			return
		}
		members := make([]deliveryHandoverMemberBody, 0, len(detail.Members))
		for _, member := range detail.Members {
			members = append(members, deliveryHandoverMemberBody{
				FactID: member.FactID(), Unit: member.Unit().String(), Membership: member.Kind().String(), FormedAt: rfc3339(member.FormedAt()),
			})
		}
		writeJSON(response, http.StatusOK, handoverNoteFoundResponse{
			Outcome:      outcomeDeliveryHandoverNoteFound,
			HandoverNote: handoverNoteCatalogueBodyOf(detail.State, int64(len(detail.Members))),
			Members:      members,
		})
	})
}

type preAdviceListResponse struct {
	Outcome    string                   `json:"outcome"`
	PreAdvices []preAdviceCatalogueBody `json:"preAdvices"`
}

type preAdviceFoundResponse struct {
	Outcome   string                 `json:"outcome"`
	PreAdvice preAdviceCatalogueBody `json:"preAdvice"`
}

// preAdviceCatalogueBody 是查阅读里的一份预报：提交口回显的那几格（同一个形，取 preAdviceBodyOf），加撤回与否。withdrawn 恒在场，
// withdrawnAt 只随已撤回出现。来源请求键与内容摘要照提交口不回显。
type preAdviceCatalogueBody struct {
	deliveryPreAdviceBody
	Withdrawn   bool   `json:"withdrawn"`
	WithdrawnAt string `json:"withdrawnAt,omitempty"`
}

func preAdviceCatalogueBodyOf(state domain.DeliveryPreAdviceState) preAdviceCatalogueBody {
	body := preAdviceCatalogueBody{deliveryPreAdviceBody: *preAdviceBodyOf(state.PreAdvice), Withdrawn: state.Withdrawn()}
	if state.Withdrawn() {
		body.WithdrawnAt = rfc3339(state.Withdrawal.WithdrawnAt())
	}
	return body
}

type handoverNoteListResponse struct {
	Outcome       string                      `json:"outcome"`
	HandoverNotes []handoverNoteCatalogueBody `json:"handoverNotes"`
}

// handoverNoteFoundResponse 是详情：交接单与列表同一个形，另带名下成员（形成方式、事实身份、业务时间），按落册先后。
type handoverNoteFoundResponse struct {
	Outcome      string                       `json:"outcome"`
	HandoverNote handoverNoteCatalogueBody    `json:"handoverNote"`
	Members      []deliveryHandoverMemberBody `json:"members"`
}

// handoverNoteCatalogueBody 是查阅读里的一张交接单。交付方线索照开立时的原样；closed 恒在场，关闭的事实身份与时刻只随已关闭出现；
// claims 是声称的每个客户账户各一格，按标识定序；scopeEstablished 恒在场，取领域按结论派生的那一格（声称的全部已确认），成立才带
// scopeEstablishedAt。
type handoverNoteCatalogueBody struct {
	NoteID             string                    `json:"noteId"`
	Node               string                    `json:"node"`
	DeliveringParty    string                    `json:"deliveringParty"`
	OpenedAt           string                    `json:"openedAt"`
	Closed             bool                      `json:"closed"`
	ClosureFactID      string                    `json:"closureFactId,omitempty"`
	ClosedAt           string                    `json:"closedAt,omitempty"`
	MemberCount        int64                     `json:"memberCount"`
	Claims             []handoverClaimReviewBody `json:"claims"`
	ScopeEstablished   bool                      `json:"scopeEstablished"`
	ScopeEstablishedAt string                    `json:"scopeEstablishedAt,omitempty"`
}

// handoverClaimReviewBody 是一个声称的客户账户与它的绑定结论。没有结论（待确认）的只带账户，其余各格整键缺席：缺席就是「还没有人作
// 结论」，不另立一个领域里没有的「待确认」词。有结论时带结论、途径、作结论方与时刻（UC-NO-006 结果语义契约「绑定已确认」要保存的那几格），
// 运营决定另带交付方参与方与依据引用。客户渠道的来源请求键不回显：它是客户手里的接入请求标识推出来的，运营看它没有用处。
type handoverClaimReviewBody struct {
	CustomerAccount       string `json:"customerAccount"`
	Conclusion            string `json:"conclusion,omitempty"`
	Route                 string `json:"route,omitempty"`
	Concluder             string `json:"concluder,omitempty"`
	ConcludedAt           string `json:"concludedAt,omitempty"`
	DeliveringParticipant string `json:"deliveringParticipant,omitempty"`
	Basis                 string `json:"basis,omitempty"`
}

func handoverNoteCatalogueBodyOf(state domain.DeliveryHandoverNoteState, memberCount int64) handoverNoteCatalogueBody {
	note := state.Note
	body := handoverNoteCatalogueBody{
		NoteID:           note.ID().String(),
		Node:             note.Node().String(),
		DeliveringParty:  note.DeliveringParty().String(),
		OpenedAt:         rfc3339(note.OpenedAt()),
		Closed:           state.Closed(),
		MemberCount:      memberCount,
		Claims:           []handoverClaimReviewBody{},
		ScopeEstablished: state.ScopeEstablished(),
	}
	if state.Closed() {
		body.ClosureFactID, body.ClosedAt = state.Closure.FactID(), rfc3339(state.Closure.ClosedAt())
	}
	for _, account := range note.ClaimedAccounts() {
		claim := handoverClaimReviewBody{CustomerAccount: account.String()}
		if binding, present := state.BindingFor(account); present {
			claim.Conclusion = binding.Conclusion().String()
			claim.Route = binding.Route().String()
			claim.Concluder = binding.Concluder()
			claim.ConcludedAt = rfc3339(binding.ConcludedAt())
			claim.DeliveringParticipant, claim.Basis = binding.OperationDecisionEvidence()
		}
		body.Claims = append(body.Claims, claim)
	}
	if at, established := state.ScopeEstablishedAt(); established {
		body.ScopeEstablishedAt = rfc3339(at)
	}
	return body
}

// deliveryScopeBasisBodyOf 把待识别行的两样依据译成两格：来源里的依据引用（没带即 nil），与最后归入的交接单（没归入过即空串）。
func deliveryScopeBasisBodyOf(basis domain.UnitDeliveryScopeBasis) (*deliveryScopeBasisDocument, string) {
	var source *deliveryScopeBasisDocument
	if basis.Source.Present() {
		source = &deliveryScopeBasisDocument{Kind: basis.Source.Kind().String(), ID: basis.Source.Identifier()}
	}
	return source, basis.LastAssignedNote.String()
}
