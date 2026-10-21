package pricinghttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 价卡收尾的两个在线口（ADR-0234）：截止 `/pricing-price-card-successor-cuts` 与退役 `/pricing-price-card-retirements`。两口
// 都是登记册配置写（ADR-0151 的定族判据：提交方是管理价卡登记册的操作者，授予按登记册，与价卡登记同一份），挂操作者渠道
// 的登记册 Intake；载荷与受控登记口 parcel-pricing-register 的两种收尾文档同形，只少 tenant 一格。
//
// 两种收尾分两口而不合一口由服务端按起点挑：截止留着旧版的前一段，退役让旧版整版不再参选，登记方要说清自己落下的是哪
// 一种。也不并进价卡登记口：登记口收的是一版新卡，这两口不立任何版本。

// PriceCardCutIntake 把一次已认证的接入请求翻译成截止命令。租户格只来自认证结果。
type PriceCardCutIntake interface {
	IntakePriceCardCut(ctx context.Context, request *http.Request) (application.CutPriceCardVersionCommand, error)
}

// PriceCardRetirementIntake 同上，翻译一次退役。
type PriceCardRetirementIntake interface {
	IntakePriceCardRetirement(ctx context.Context, request *http.Request) (application.RetirePriceCardVersionCommand, error)
}

// PriceCardClosureRegistrar 是两口转交的编排。事务边界在编排侧给出：锁、读与写必须在同一笔里。
type PriceCardClosureRegistrar interface {
	Cut(ctx context.Context, command application.CutPriceCardVersionCommand) (application.PriceCardClosureResult, error)
	RetireAtCoveringSuccessor(ctx context.Context, command application.RetirePriceCardVersionCommand) (application.PriceCardClosureResult, error)
}

var (
	_ PriceCardClosureRegistrar = (*application.ClosePriceCardVersionHandler)(nil)
	_ PriceCardCutIntake        = (*OperatorRegistryIntake)(nil)
	_ PriceCardRetirementIntake = (*OperatorRegistryIntake)(nil)
)

// PriceCardClosurePayload 是两口共用的在线载荷：哪一定价方案的哪一版、让位给同一方案的哪一版、凭什么。没有截止点与退役
// 时刻两格——截止点取后继的起点，退役时刻取登记时刻，都由服务端定；租户从认证结果来。载荷里出现这几格即按未知键拒。
type PriceCardClosurePayload struct {
	PlanID           string `json:"planId"`
	Version          string `json:"version"`
	SuccessorVersion string `json:"successorVersion"`
	Reference        string `json:"reference"`
}

// Target 把载荷译成两种收尾共用的那一半；各格成不成立由编排与领域判，这里只搬。
func (payload PriceCardClosurePayload) Target(tenant domain.TenantID) application.PriceCardClosureTarget {
	return application.PriceCardClosureTarget{
		Tenant: tenant, PlanID: payload.PlanID, Version: payload.Version, Successor: payload.SuccessorVersion,
	}
}

// DecodePriceCardClosurePayload 封闭解码：未知键与尾随内容都答坏报文。
func DecodePriceCardClosurePayload(body io.Reader) (PriceCardClosurePayload, error) {
	var payload PriceCardClosurePayload
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return PriceCardClosurePayload{}, fmt.Errorf("%w: price card closure payload: %v", ErrMalformedRequest, err)
	}
	if decoder.More() {
		return PriceCardClosurePayload{}, fmt.Errorf("%w: price card closure payload: trailing content", ErrMalformedRequest)
	}
	return payload, nil
}

func (intake *OperatorRegistryIntake) IntakePriceCardCut(ctx context.Context, request *http.Request) (application.CutPriceCardVersionCommand, error) {
	tenant, payload, err := intake.closurePayload(ctx, request)
	if err != nil {
		return application.CutPriceCardVersionCommand{}, err
	}
	return application.CutPriceCardVersionCommand{PriceCardClosureTarget: payload.Target(tenant), Reference: payload.Reference}, nil
}

func (intake *OperatorRegistryIntake) IntakePriceCardRetirement(ctx context.Context, request *http.Request) (application.RetirePriceCardVersionCommand, error) {
	tenant, payload, err := intake.closurePayload(ctx, request)
	if err != nil {
		return application.RetirePriceCardVersionCommand{}, err
	}
	return application.RetirePriceCardVersionCommand{PriceCardClosureTarget: payload.Target(tenant), Reference: payload.Reference}, nil
}

func (intake *OperatorRegistryIntake) closurePayload(ctx context.Context, request *http.Request) (domain.TenantID, PriceCardClosurePayload, error) {
	operator, body, err := intake.authenticate(ctx, request)
	if err != nil {
		return domain.TenantID{}, PriceCardClosurePayload{}, err
	}
	if operator.Tenant.String() == "" {
		return domain.TenantID{}, PriceCardClosurePayload{}, ErrOperatorIdentityMissing
	}
	payload, err := DecodePriceCardClosurePayload(body)
	if err != nil {
		return domain.TenantID{}, PriceCardClosurePayload{}, err
	}
	return operator.Tenant, payload, nil
}

// NewCutPriceCardVersionEndpoint 交回截止的 HTTP 入口。
func NewCutPriceCardVersionEndpoint(intake PriceCardCutIntake, registrar PriceCardClosureRegistrar) http.Handler {
	return newPriceCardClosureEndpoint(intake.IntakePriceCardCut, registrar.Cut)
}

// NewRetirePriceCardVersionEndpoint 交回退役的 HTTP 入口。
func NewRetirePriceCardVersionEndpoint(intake PriceCardRetirementIntake, registrar PriceCardClosureRegistrar) http.Handler {
	return newPriceCardClosureEndpoint(intake.IntakePriceCardRetirement, registrar.RetireAtCoveringSuccessor)
}

func newPriceCardClosureEndpoint[Command any](
	intake func(context.Context, *http.Request) (Command, error),
	handle func(context.Context, Command) (application.PriceCardClosureResult, error),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		result, err := handle(request.Context(), command)
		if err != nil {
			// 收尾与否未知（依赖故障）不是业务答案（ADR-0022）：整笔已回滚，客户端据以重试。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writePriceCardClosureAnswer(response, result)
	})
}

// priceCardClosureAnswer 在登记族的 outcome 之外回显落下（或重放认出）的那一次收尾：截止回显截止点与后继，退役回显后继与
// 退役时刻，登记方据它核对、不必另去查册；`未受理`时这几格缺席、原因随 cause 过线。
type priceCardClosureAnswer struct {
	Outcome          string `json:"outcome"`
	Cause            string `json:"cause,omitempty"`
	SelectionEndsAt  string `json:"selectionEndsAt,omitempty"`
	SuccessorVersion string `json:"successorVersion,omitempty"`
	RetiredAt        string `json:"retiredAt,omitempty"`
}

// writePriceCardClosureAnswer 判据同本包其余登记口：`已登记`取 201，重放与`未受理`是治理答案取 200。
func writePriceCardClosureAnswer(response http.ResponseWriter, result application.PriceCardClosureResult) {
	outcome := result.Outcome()
	name := outcome.String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := priceCardClosureAnswer{Outcome: name}
	if cause := result.Cause(); cause != nil {
		answer.Cause = cause.Error()
	}
	if cut, held := result.Cut(); held {
		answer.SelectionEndsAt = rfc3339(cut.SelectionEndsAt())
		answer.SuccessorVersion = cut.Successor().Version()
	}
	if retirement, held := result.Retirement(); held {
		answer.RetiredAt = rfc3339(retirement.RetiredAt())
		answer.SuccessorVersion = retirement.Successor().Version()
	}
	status := http.StatusOK
	if outcome == application.PriceCardClosureRecorded {
		status = http.StatusCreated
	}
	writeJSON(response, status, answer)
}
