package visibilityhttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// ErrMalformedClaim 表示这次请求构造不出索赔受理命令，且重发同样的内容不会改变结果。
// 与查询端点分设哨兵：两个端点的翻译各自演化，共享一个哨兵会让一边的收紧误伤另一边。
var ErrMalformedClaim = errors.New("visibility exception http: malformed claim submission")

// ClaimIntake 把一次已认证的索赔提交翻译成受理命令。
//
// 接口不带实现的理由与 QueryIntake 相同：货主客户账户只能来自认证结果（采信自报账户
// 穿透隔离），提交时间取到达信封而非客户自报（首次索赔期限从可证明的提交事实起算，
// UC-VE-007），批次与项标识的客户侧唯一性口径属 `PAR-INT-01` 待提供。
type ClaimIntake interface {
	IntakeClaim(ctx context.Context, request *http.Request) (application.ReceiveClaimCommand, error)
}

// ClaimReceiver 是本端点转交的应用编排入口。只暴露受理：资格审核与责任结论是另外
// 两个判断（三判分步，CONTEXT「收到客户索赔、通过资格审核和确认赔偿责任是不同判断」），
// 由内部授权角色走各自入口——客户提交面连那两个入口的形状都看不见。
type ClaimReceiver interface {
	ReceiveClaim(
		ctx context.Context,
		command application.ReceiveClaimCommand,
	) (application.HandleClaimResult, error)
}

const codeUnnamedOutcome = "UNNAMED_OUTCOME"

// NewReceiveClaimEndpoint 交回 `UC-VE-007` 索赔提交的 HTTP 入口。
func NewReceiveClaimEndpoint(intake ClaimIntake, receiver ClaimReceiver) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeClaim(request.Context(), request)
		if err != nil {
			if errors.Is(err, ErrAccessChannelNotConfigured) {
				writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
				return
			}
			if errors.Is(err, ErrMalformedClaim) {
				writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
				return
			}
			writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
			return
		}

		result, err := receiver.ReceiveClaim(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeClaimOutcome(response, result)
	})
}

// claimResponse 是本端点的封闭响应形状。受理回执只带批次、项与提交时间——资格与
// 结论字段结构上不存在：已有项重放读回的索赔即便已过审，提交面也只答「已受理在案」，
// 审核进度是另一个授权面的事。
type claimResponse struct {
	Outcome         string        `json:"outcome"`
	UndecidedReason string        `json:"undecidedReason,omitempty"`
	Claim           *claimReceipt `json:"claim,omitempty"`
}

type claimReceipt struct {
	Batch       string `json:"batch"`
	Item        string `json:"item"`
	SubmittedAt string `json:"submittedAt"`
}

func writeClaimOutcome(response http.ResponseWriter, result application.HandleClaimResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		// 应用层交回了没有名字的结果：编程错误不是业务答案，不能带空 outcome 上线。
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := claimResponse{Outcome: outcome}
	if reason := result.UndecidedReason().String(); result.Outcome() == application.HandleClaimUndecided && reason != "" {
		body.UndecidedReason = reason
	}
	if claim, present := result.Claim(); present {
		body.Claim = &claimReceipt{
			Batch:       claim.Batch().String(),
			Item:        claim.ID().String(),
			SubmittedAt: claim.SubmittedAt().UTC().Format(time.RFC3339Nano),
		}
	}

	status := http.StatusOK
	if result.Outcome() == application.ClaimReceived {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
