package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// HandoverIntake 把已认证的接入请求翻译成交接判断的首登 / 更正命令。
//
// 接口而非本包内解析代码的理由同 DeliveryIntake：租户身份只能来自认证结果（ADR-0003），
// 真实接入渠道属 PAR-INT-01 待提供，未决期间本包不带任何实现。
//
// **`Segment` 与 `PlannedSegment` 是命令的一部分，Intake 必须收。** 交接是 CONTEXT 成立边界的
// 来源事实，对象进哪个实际履约段由这两格指名（缺席时编排不立段）。Intake 若不收它们，编排
// 那道进段的门在端点层就没有路走到——那是「编排接上了、端点没让它接上」，票 04 专钉。
//
// 方法名带 `Handover` 而不照交付那份叫 `IntakeRegistration`：UnconfiguredIntake 一个类型要同时
// 堵住本包全部命令面（未配置是渠道这一层的状态，不是某个端点的状态），而 Go 不允许同名方法
// 返回不同命令类型。交付那份先落、占了通名；此后的 Intake 都按事实具名，同 IntakeCatalogueQuery。
//
// 两口各有自己的单方法接口，端点各收其一；HandoverIntake 是两者的合集，留给要整组实现的一方
// （UnconfiguredIntake 两口同堵）。分开的理由是逐口放行（ADR-0091 Consequences）：一个只放行了
// 首登的实现在类型上就装不进更正口，不必为凑齐合集去写一个假装未配置的更正方法。
type HandoverIntake interface {
	HandoverRegistrationIntake
	HandoverCorrectionIntake
}

// HandoverRegistrationIntake 是交接首登口的 Intake。
type HandoverRegistrationIntake interface {
	IntakeHandoverRegistration(ctx context.Context, request *http.Request) (application.RegisterTransportHandoverCommand, error)
}

// HandoverCorrectionIntake 是交接更正口的 Intake。
type HandoverCorrectionIntake interface {
	IntakeHandoverCorrection(ctx context.Context, request *http.Request) (application.CorrectTransportHandoverCommand, error)
}

// HandoverHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。
type HandoverHandler interface {
	Register(
		ctx context.Context,
		command application.RegisterTransportHandoverCommand,
	) (application.RegisterTransportHandoverResult, error)
	Correct(
		ctx context.Context,
		command application.CorrectTransportHandoverCommand,
	) (application.RegisterTransportHandoverResult, error)
}

// NewRegisterTransportHandoverEndpoint 交回交接判断首登的 HTTP 入口。
func NewRegisterTransportHandoverEndpoint(intake HandoverRegistrationIntake, handler HandoverHandler) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.RegisterTransportHandoverResult, error, bool) {
		command, err := intake.IntakeHandoverRegistration(request.Context(), request)
		if err != nil {
			return application.RegisterTransportHandoverResult{}, err, false
		}
		result, err := handler.Register(request.Context(), command)
		return result, err, true
	}, writeHandoverOutcome)
}

// NewCorrectTransportHandoverEndpoint 交回交接判断更正的 HTTP 入口。首登与更正分两个端点，
// 理由同交付：命令形状与恢复动作不同，合成一个入口就得靠请求体里的模式字段分路。
func NewCorrectTransportHandoverEndpoint(intake HandoverCorrectionIntake, handler HandoverHandler) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.RegisterTransportHandoverResult, error, bool) {
		command, err := intake.IntakeHandoverCorrection(request.Context(), request)
		if err != nil {
			return application.RegisterTransportHandoverResult{}, err, false
		}
		result, err := handler.Correct(request.Context(), command)
		return result, err, true
	}, writeHandoverOutcome)
}

// handoverResponse 是两个交接端点共用的封闭响应形状。`outcome` 取应用结果枚举原名。
//
// 三个引用格各自透出，一个都不能省：`continuationReference` 是`未决`时续办同一次登记的把手；
// `handoffReference` 说记录已提交但意图还没交出去；`segmentContinuationReference` 说交接已登记、
// 进段那一半还欠着。编排把它们与 outcome 分开交回，正因为来源保全成立而派生一侧欠着时，
// 调用方要做的事不同——传输层吞掉任何一格，调用方就没法续办那一半。
//
// `segmentEntryRefusal` 是第四格（票 03 裁决 3）：段那一半被领域正当拒绝——首登进段答 `SEGMENT_CLOSED`
// （「去另立新段」），更正口答重派生那一半的各格（ADR-0112：无可替代、撤回控制、起点晚于继承的终点），
// 与欠账那格恰相反（一个说别重试、一个说等恢复重试）。取值以 application.SegmentEntryRefusal 为准。
type handoverResponse struct {
	Outcome                      string `json:"outcome"`
	UndecidedReason              string `json:"undecidedReason,omitempty"`
	HandoverVersion              string `json:"handoverVersion,omitempty"`
	Object                       string `json:"object,omitempty"`
	Scope                        string `json:"scope,omitempty"`
	Verdict                      string `json:"verdict,omitempty"`
	Corrects                     string `json:"corrects,omitempty"`
	ContinuationReference        string `json:"continuationReference,omitempty"`
	HandoffReference             string `json:"handoffReference,omitempty"`
	SegmentContinuationReference string `json:"segmentContinuationReference,omitempty"`
	SegmentEntryRefusal          string `json:"segmentEntryRefusal,omitempty"`
	// ParticipationEnd 是`已交接`落库后结束前段参与那一半的答案（票 06）：PARTICIPATION_ENDED、
	// NO_ACTIVE_PARTICIPATION 等，拒收/待确认与重放时为空。
	ParticipationEnd string `json:"participationEnd,omitempty"`
}

func writeHandoverOutcome(response http.ResponseWriter, result application.RegisterTransportHandoverResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := handoverResponse{
		Outcome:                      outcome,
		UndecidedReason:              result.UndecidedReason().String(),
		ContinuationReference:        result.ContinuationReference(),
		HandoffReference:             result.HandoverHandoffReference(),
		SegmentContinuationReference: result.SegmentContinuationReference(),
		SegmentEntryRefusal:          result.SegmentEntryRefusal().String(),
		ParticipationEnd:             result.ParticipationEnd().String(),
	}
	if record, present := result.Record(); present {
		body.HandoverVersion = record.Handover.Version().String()
		body.Object = record.Handover.Object().String()
		body.Scope = record.Handover.Scope().String()
		body.Verdict = record.Handover.Verdict().String()
		if predecessor, corrected := record.Handover.Corrects(); corrected {
			body.Corrects = predecessor.String()
		}
	}

	// ADR-0022：状态码只报有没有新落一版。首登与更正都持久化了新版本用 201；重放、冲突、
	// 未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.HandoverRegistered ||
		result.Outcome() == application.HandoverCorrected {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
