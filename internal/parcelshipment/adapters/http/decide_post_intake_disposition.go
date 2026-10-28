package shipmenthttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// codePostIntakeDispositionRequestNotVisible 是`统一不可见结果`的问题码：命令指名的请求查无、属于另一个租户、不属这件包裹、或不是一份待回应的停止请求，同一个码。
const codePostIntakeDispositionRequestNotVisible = "POST_INTAKE_DISPOSITION_REQUEST_NOT_VISIBLE"

// PostIntakeDispositionDecisionIntake 把一次已认证的接入请求翻译成收寄后服务处置决定命令。它是接口而非解析代码，理由同本包其余运营决定口：决定人与租户只能来自
// 认证结果，采信自报就是让任何调用方替任何人决定一件已在网络手里的包裹被扣、被退还是被终止服务。
type PostIntakeDispositionDecisionIntake interface {
	IntakePostIntakeDispositionDecision(
		ctx context.Context,
		request *http.Request,
	) (application.DecidePostIntakeDispositionCommand, error)
}

// PostIntakeDispositionDecisionHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。真实装配接 DecidePostIntakeDispositionHandler，外面包一笔
// 事务：决定留痕与交给各对象所有者的信封同生共死。
type PostIntakeDispositionDecisionHandler interface {
	Handle(
		ctx context.Context,
		command application.DecidePostIntakeDispositionCommand,
	) (application.DecidePostIntakeDispositionResult, error)
}

var _ PostIntakeDispositionDecisionIntake = UnconfiguredIntake{}

// IntakePostIntakeDispositionDecision 不读请求：渠道未配置就无从铸造决定人。
func (UnconfiguredIntake) IntakePostIntakeDispositionDecision(context.Context, *http.Request) (application.DecidePostIntakeDispositionCommand, error) {
	return application.DecidePostIntakeDispositionCommand{}, ErrAccessChannelNotConfigured
}

// NewDecidePostIntakeDispositionEndpoint 交回收寄后服务处置决定的 HTTP 入口（POST /post-intake-disposition-decisions；ADR-0332）。
func NewDecidePostIntakeDispositionEndpoint(
	intake PostIntakeDispositionDecisionIntake,
	handler PostIntakeDispositionDecisionHandler,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakePostIntakeDispositionDecision(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := handler.Handle(request.Context(), command)
		if errors.Is(err, application.ErrPostIntakeDispositionRequestNotVisible) {
			writeProblem(response, http.StatusNotFound, codePostIntakeDispositionRequestNotVisible)
			return
		}
		if err != nil {
			writeCommandProblem(response, err)
			return
		}
		writePostIntakeDispositionOutcome(response, result)
	})
}

// postIntakeDispositionResponse 是本端点的封闭响应形状。带决定的结果（已决定、已决定过、前一份已变、请求已被回应）逐字段给出那一份决定；其余结果只有结果名，
// `终局成立`另带指名终局的依据。回执不回显决定人：调用方就是决定人，谁决定的在留痕里。
type postIntakeDispositionResponse struct {
	Outcome    string                         `json:"outcome"`
	FinalBasis string                         `json:"finalBasis,omitempty"`
	Decision   *postIntakeDispositionDecision `json:"decision,omitempty"`
}

type postIntakeDispositionDecision struct {
	Version             int                          `json:"version"`
	Supersedes          int                          `json:"supersedes"`
	Direction           string                       `json:"direction"`
	Source              string                       `json:"source"`
	RequestKey          string                       `json:"requestKey,omitempty"`
	Destination         string                       `json:"destination,omitempty"`
	ResponsibilityStart string                       `json:"responsibilityStart"`
	DecidedAt           string                       `json:"decidedAt"`
	OwnerRequests       []postIntakeDispositionOwner `json:"ownerRequests"`
}

type postIntakeDispositionOwner struct {
	Owner  string `json:"owner"`
	Action string `json:"action"`
}

func writePostIntakeDispositionOutcome(response http.ResponseWriter, result application.DecidePostIntakeDispositionResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := postIntakeDispositionResponse{Outcome: outcome, FinalBasis: result.FinalBasis().String()}
	if disposition, present := result.Disposition(); present {
		requests := make([]postIntakeDispositionOwner, 0, len(disposition.Direction().OwnerRequests()))
		for _, request := range disposition.Direction().OwnerRequests() {
			requests = append(requests, postIntakeDispositionOwner{Owner: request.Owner.String(), Action: request.Action.String()})
		}
		body.Decision = &postIntakeDispositionDecision{
			Version:             disposition.Version(),
			Supersedes:          disposition.Supersedes(),
			Direction:           disposition.Direction().String(),
			Source:              disposition.Source().String(),
			RequestKey:          disposition.RequestKey().String(),
			Destination:         disposition.Destination().String(),
			ResponsibilityStart: disposition.ResponsibilityStart().String(),
			DecidedAt:           disposition.DecidedAt().UTC().Format(time.RFC3339Nano),
			OwnerRequests:       requests,
		}
	}
	status := http.StatusOK
	if result.Outcome() == application.PostIntakeDispositionDecided {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
