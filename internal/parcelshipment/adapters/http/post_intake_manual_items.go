package shipmenthttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 待人工处理项的两个运营口（ADR-0340）：GET /post-intake-disposition-manual-items 查阅，POST /post-intake-disposition-manual-item-results 逐项登记处理结果。

// OperatorDecisionPostIntakeManualItemHandling 与收寄后服务处置分开授予（ADR-0340 决定三）：形成决定的人与在现场办完这些事的人通常不是同一批。
const OperatorDecisionPostIntakeManualItemHandling OperatorDecision = "POST_INTAKE_MANUAL_ITEM_HANDLING"

var (
	_ PostIntakeManualItemResultIntake = (*OperatorDecisionIntake)(nil)
	_ PostIntakeManualItemResultIntake = UnconfiguredIntake{}
	_ PostIntakeManualItemQueryIntake  = (*OperatorOperationsReadIntake)(nil)
	_ PostIntakeManualItemQueryIntake  = UnconfiguredIntake{}
	_ PostIntakeManualItemReader       = ports.PostIntakeManualItems(nil)
)

const (
	codePostIntakeManualItemNotVisible = "POST_INTAKE_MANUAL_ITEM_NOT_VISIBLE"
	// 空清单仍是 LISTED：「此刻没有待人工处理项」是如实答案，与「接入渠道未配置」（403）分得开。
	outcomePostIntakeManualItemsListed = "POST_INTAKE_MANUAL_ITEMS_LISTED"
)

// ---- 登记处理结果 ----

// PostIntakeManualItemResultIntake 把一次已认证的接入请求翻译成登记命令：租户与操作者只能来自认证结果。
type PostIntakeManualItemResultIntake interface {
	IntakePostIntakeManualItemResult(ctx context.Context, request *http.Request) (application.RegisterPostIntakeManualItemResultCommand, error)
}

// PostIntakeManualItemResultHandler 是本适配器转交的应用编排；真实装配外面包一笔事务。
type PostIntakeManualItemResultHandler interface {
	Handle(ctx context.Context, command application.RegisterPostIntakeManualItemResultCommand) (application.RegisterPostIntakeManualItemResultResult, error)
}

// IntakePostIntakeManualItemResult 不读请求：渠道未配置就无从铸造操作者。
func (UnconfiguredIntake) IntakePostIntakeManualItemResult(context.Context, *http.Request) (application.RegisterPostIntakeManualItemResultCommand, error) {
	return application.RegisterPostIntakeManualItemResultCommand{}, ErrAccessChannelNotConfigured
}

type postIntakeManualItemResultPayload struct {
	Parcel          string `json:"parcel"`
	DecisionVersion int    `json:"decisionVersion"`
	Owner           string `json:"owner"`
	Action          string `json:"action"`
	Outcome         string `json:"outcome"`
	Note            string `json:"note"`
}

// IntakePostIntakeManualItemResult 译登记命令（`/post-intake-disposition-manual-item-results`）。租户取信封，操作者取认证出的操作者；先认证后封闭解码。
// 坏载荷（所有者或动作不在封闭集、系统执行的动作、结果不在封闭集、说明缺）在这里就答 400；项在不在册是编排的事，统一不可见。
func (intake *OperatorDecisionIntake) IntakePostIntakeManualItemResult(ctx context.Context, request *http.Request) (application.RegisterPostIntakeManualItemResultCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionPostIntakeManualItemHandling)
	if err != nil {
		return application.RegisterPostIntakeManualItemResultCommand{}, err
	}
	var payload postIntakeManualItemResultPayload
	if err := decodeDecisionPayload(request.Body, &payload); err != nil {
		return application.RegisterPostIntakeManualItemResultCommand{}, err
	}
	parcel, err := domain.NewDeclaredParcelID(payload.Parcel)
	if err != nil {
		return application.RegisterPostIntakeManualItemResultCommand{}, fmt.Errorf("%w: parcel: %v", ErrMalformedRequest, err)
	}
	owner, err := domain.ParseDispositionOwner(payload.Owner)
	if err != nil {
		return application.RegisterPostIntakeManualItemResultCommand{}, fmt.Errorf("%w: owner: %v", ErrMalformedRequest, err)
	}
	action, err := domain.ParseDispositionOwnerAction(payload.Action)
	if err != nil {
		return application.RegisterPostIntakeManualItemResultCommand{}, fmt.Errorf("%w: action: %v", ErrMalformedRequest, err)
	}
	key := domain.ManualItemKey{Tenant: operator.Tenant, Parcel: parcel, DecisionVersion: payload.DecisionVersion, Owner: owner, Action: action}
	if err := domain.ManualItemKeyError(key); err != nil {
		return application.RegisterPostIntakeManualItemResultCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	outcome, err := domain.ParseManualItemOutcome(payload.Outcome)
	if err != nil {
		return application.RegisterPostIntakeManualItemResultCommand{}, fmt.Errorf("%w: outcome: %v", ErrMalformedRequest, err)
	}
	note, err := domain.NewManualItemNote(payload.Note)
	if err != nil {
		return application.RegisterPostIntakeManualItemResultCommand{}, fmt.Errorf("%w: note: %v", ErrMalformedRequest, err)
	}
	handler, err := domain.NewManualItemHandlerReference(operator.Operator)
	if err != nil {
		return application.RegisterPostIntakeManualItemResultCommand{}, fmt.Errorf("parcel shipment http: handler: %w", err)
	}
	return application.RegisterPostIntakeManualItemResultCommand{Key: key, Outcome: outcome, Note: note, Handler: handler}, nil
}

// NewRegisterPostIntakeManualItemResultEndpoint 交回登记处理结果的 HTTP 入口（POST /post-intake-disposition-manual-item-results）。
// 首次登记答 201，重放答 200，冲突答 409，三者都带这一项的当前结果；项查无、属别的租户或别的包裹同一个 404。
func NewRegisterPostIntakeManualItemResultEndpoint(
	intake PostIntakeManualItemResultIntake,
	handler PostIntakeManualItemResultHandler,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakePostIntakeManualItemResult(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := handler.Handle(request.Context(), command)
		if errors.Is(err, application.ErrPostIntakeManualItemNotVisible) {
			writeProblem(response, http.StatusNotFound, codePostIntakeManualItemNotVisible)
			return
		}
		if errors.Is(err, domain.ErrInvalidManualItemResult) {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		if err != nil {
			writeCommandProblem(response, err)
			return
		}
		outcome := result.Outcome().String()
		if outcome == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		status := http.StatusOK
		switch result.Outcome() {
		case application.PostIntakeManualOutcomeRegistered:
			status = http.StatusCreated
		case application.PostIntakeManualOutcomeConflict:
			status = http.StatusConflict
		}
		item := postIntakeManualItemBodyOf(result.Item())
		writeJSON(response, status, postIntakeManualItemResultResponse{Outcome: outcome, Item: &item})
	})
}

type postIntakeManualItemResultResponse struct {
	Outcome string                    `json:"outcome"`
	Item    *postIntakeManualItemBody `json:"item,omitempty"`
}

// ---- 查阅 ----

// PostIntakeManualItemQuery 是一次已授权的查阅：作用域只有租户，页大小由接入面按渠道契约裁决，两样都不采信调用方自报。
type PostIntakeManualItemQuery struct {
	Tenant domain.TenantID
	Limit  int
}

type PostIntakeManualItemQueryIntake interface {
	IntakePostIntakeManualItemQuery(ctx context.Context, request *http.Request) (PostIntakeManualItemQuery, error)
}

// PostIntakeManualItemReader 是查阅端点消费的读口：查阅不触发任何登记，所以接存储读面。
type PostIntakeManualItemReader interface {
	ListPendingPostIntakeManualItems(ctx context.Context, tenant domain.TenantID, limit int) (ports.PendingPostIntakeManualItems, error)
	ListPostIntakeManualItemsOfParcel(ctx context.Context, tenant domain.TenantID, parcel domain.DeclaredParcelID) ([]ports.PostIntakeManualItem, error)
}

func (UnconfiguredIntake) IntakePostIntakeManualItemQuery(context.Context, *http.Request) (PostIntakeManualItemQuery, error) {
	return PostIntakeManualItemQuery{}, ErrAccessChannelNotConfigured
}

// IntakePostIntakeManualItemQuery 同冲突册等口：租户只取自信封，页大小是这一渠道的产品策略。
func (intake *OperatorOperationsReadIntake) IntakePostIntakeManualItemQuery(ctx context.Context, request *http.Request) (PostIntakeManualItemQuery, error) {
	tenant, err := intake.authenticator.AuthenticateOperationsRead(ctx, httpapi.BearerToken(request))
	if err != nil {
		return PostIntakeManualItemQuery{}, err
	}
	return PostIntakeManualItemQuery{Tenant: tenant, Limit: operatorOperationsReadLimit}, nil
}

// NewQueryPostIntakeManualItemsEndpoint 交回待人工处理项查阅的 HTTP 入口（GET /post-intake-disposition-manual-items）。不带参数上列本租户尚未登记结果的项；
// 带 `parcel` 读回这件包裹名下的全部项连同各自的结果（PS 读回承接结果走这条）。查阅面里没有动作。
func NewQueryPostIntakeManualItemsEndpoint(intake PostIntakeManualItemQueryIntake, reader PostIntakeManualItemReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		query, err := intake.IntakePostIntakeManualItemQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		var listed []ports.PostIntakeManualItem
		pendingCount, truncated := 0, false
		if raw := request.URL.Query().Get("parcel"); raw != "" {
			parcel, err := domain.NewDeclaredParcelID(raw)
			if err != nil {
				writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
				return
			}
			if listed, err = reader.ListPostIntakeManualItemsOfParcel(request.Context(), query.Tenant, parcel); err != nil {
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			for _, item := range listed {
				if !item.HasResult {
					pendingCount++
				}
			}
		} else {
			pending, err := reader.ListPendingPostIntakeManualItems(request.Context(), query.Tenant, query.Limit)
			if err != nil {
				// 读不回是答案未形成，不伪装成空清单：把一次该重试的故障读成「没有待人工处理项」，卡住的包裹就更没人看见了。
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			listed, pendingCount, truncated = pending.Items, pending.Count, pending.Count > len(pending.Items)
		}
		bodies := make([]postIntakeManualItemBody, 0, len(listed))
		for _, item := range listed {
			bodies = append(bodies, postIntakeManualItemBodyOf(item))
		}
		writeJSON(response, http.StatusOK, postIntakeManualItemsResponse{
			Outcome: outcomePostIntakeManualItemsListed, PendingCount: pendingCount, Truncated: truncated, Items: bodies,
		})
	})
}

type postIntakeManualItemsResponse struct {
	Outcome      string                     `json:"outcome"`
	PendingCount int                        `json:"pendingCount"`
	Truncated    bool                       `json:"truncated"`
	Items        []postIntakeManualItemBody `json:"items"`
}

// postIntakeManualItemBody 逐字段透出一项：登记命令要的几段（包裹、决定版本、所有者、动作）原样给出，另带所属决定的去向、目的地与包裹决定链此刻的版本；
// 结果在场时带结果、说明、操作者与处理时刻。
type postIntakeManualItemBody struct {
	ParcelID        string                       `json:"parcelId"`
	DecisionVersion int                          `json:"decisionVersion"`
	Owner           string                       `json:"owner"`
	Action          string                       `json:"action"`
	Direction       string                       `json:"direction"`
	Destination     string                       `json:"destination,omitempty"`
	DecidedAt       string                       `json:"decidedAt"`
	CurrentVersion  int                          `json:"currentVersion"`
	Result          *postIntakeManualItemOutcome `json:"result,omitempty"`
}

type postIntakeManualItemOutcome struct {
	Outcome   string `json:"outcome"`
	Note      string `json:"note"`
	HandledBy string `json:"handledBy"`
	HandledAt string `json:"handledAt"`
}

func postIntakeManualItemBodyOf(item ports.PostIntakeManualItem) postIntakeManualItemBody {
	body := postIntakeManualItemBody{
		ParcelID:        item.Key.Parcel.String(),
		DecisionVersion: item.Key.DecisionVersion,
		Owner:           item.Key.Owner.String(),
		Action:          item.Key.Action.String(),
		Direction:       item.Direction.String(),
		Destination:     item.Destination.String(),
		DecidedAt:       item.DecidedAt.UTC().Format(time.RFC3339Nano),
		CurrentVersion:  item.CurrentVersion,
	}
	if item.HasResult {
		body.Result = &postIntakeManualItemOutcome{
			Outcome:   item.Result.Outcome.String(),
			Note:      item.Result.Note.String(),
			HandledBy: item.Result.Handler.String(),
			HandledAt: item.Result.HandledAt.UTC().Format(time.RFC3339Nano),
		}
	}
	return body
}
