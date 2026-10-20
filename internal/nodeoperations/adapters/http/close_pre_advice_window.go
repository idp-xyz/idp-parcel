package nodeopshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件是预报有效时窗的在线收口写面（票 ps-external-mark-relations/14 裁定；ADR-0085 决定一；参数登记册 PAR-NET-18）。它与受控批量
// 口 parcel-node-operations-register 的 pre-advice-window-closure 族调同一个编排；挂的是登记写面那一只 Intake、同一个登记册配置写
// 能力面（ADR-0100 决定四），写开关换不了它（ADR-0150 决定三）。
//
// 另立一个端点，不与登记口共用一个以载荷区分：两种写的载荷各自封闭解码，共用一口就得先放进一个两边字段的并集再判是哪一种，打错
// 一个键会被当成另一种写的合法字段，「未知字段一律拒」随之失效；两种写的答案格也不同，一口两套 outcome 让调用方先猜是哪一种写。
// 端点表上本来就是一种登记写一行（注册与撤销、登记与修订各占一行），收口照办。

// PreAdviceWindowClosureIntake 把一次已认证的时窗收口请求译成命令。租户只取认证结果。
type PreAdviceWindowClosureIntake interface {
	IntakePreAdviceWindowClosure(ctx context.Context, request *http.Request) (application.ClosePreAdviceWindowCommand, error)
}

// PreAdviceWindowCloser 是本口转交的收口编排，装配负责把它包进一笔事务。
type PreAdviceWindowCloser interface {
	Handle(ctx context.Context, command application.ClosePreAdviceWindowCommand) (application.PreAdviceWindowClosureResult, error)
}

// PreAdviceWindowClosurePayload 是时窗收口的线格式。windowEffectiveFrom 指名要收口的那一行（同租户起点唯一）；收口时刻与依据必给，
// 都没有默认——不给收口时刻就收口，等于替操作员挑了一个时刻。批量口把它原样嵌进自己的输入、另加租户。
type PreAdviceWindowClosurePayload struct {
	WindowEffectiveFrom time.Time `json:"windowEffectiveFrom"`
	ClosedAt            time.Time `json:"closedAt"`
	Basis               string    `json:"basis"`
}

// Command 把载荷连同租户译成命令。缺格、依据带 NUL 答坏载荷；收口时刻不晚于起点、依据全空白一类交给编排里的形成门判，两个入口
// 同一道。
func (payload PreAdviceWindowClosurePayload) Command(tenant domain.TenantID) (application.ClosePreAdviceWindowCommand, error) {
	none := application.ClosePreAdviceWindowCommand{}
	if payload.WindowEffectiveFrom.IsZero() {
		return none, fmt.Errorf("%w: windowEffectiveFrom is required; it names the window to close", ErrMalformedRequest)
	}
	if payload.ClosedAt.IsZero() {
		return none, fmt.Errorf("%w: closedAt is required; it has no default", ErrMalformedRequest)
	}
	if strings.ContainsRune(payload.Basis, 0) {
		return none, fmt.Errorf("%w: basis carries NUL", ErrMalformedRequest)
	}
	return application.ClosePreAdviceWindowCommand{
		TenantID:   tenant,
		WindowFrom: payload.WindowEffectiveFrom,
		ClosedAt:   payload.ClosedAt,
		Basis:      payload.Basis,
	}, nil
}

// NewClosePreAdviceWindowEndpoint 交回时窗收口的 HTTP 入口。按 ADR-0022 映射：收口落下答 201；原样再收是形成了的答案，答 200 带
// outcome 并回显那一行。收不了口（裁定归坏输入的各格）答坏载荷；册读写不动答没形成答案，册上什么也没落。
func NewClosePreAdviceWindowEndpoint(intake PreAdviceWindowClosureIntake, closer PreAdviceWindowCloser) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakePreAdviceWindowClosure(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := closer.Handle(request.Context(), command)
		switch {
		case errors.Is(err, application.ErrPreAdviceWindowClosureRejected):
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		case err != nil:
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		outcome := result.Outcome().String()
		if outcome == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		status := http.StatusOK
		if result.Outcome() == application.PreAdviceWindowClosed {
			status = http.StatusCreated
		}
		writeJSON(response, status, preAdviceWindowResponse{Outcome: outcome, Window: windowBody(result.Window())})
	})
}
