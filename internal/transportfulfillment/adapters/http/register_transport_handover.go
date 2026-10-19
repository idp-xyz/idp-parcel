package tfhttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// HandoverIntake 把已认证的接入请求翻译成交接判断的首登 / 更正命令。
//
// 端点收接口的理由同 DeliveryIntake。实现是操作者渠道「作业事实登记」能力面的 WorkFactCommandIntake：首登（ADR-0149
// 决定二）与更正（随原事实归族、与首登同一份认证与授予，ADR-0151 决定四）两口都在。
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
// （UnconfiguredIntake 两口同堵）。分开是为了端点在类型上只依赖自己那一口：装配点逐端点换 Intake
// （assembleBusinessEndpoints 头注「逐端点替换」），往既有接口加方法又会打断它的每个实现者（理由同
// PickupCorrectionIntake）。
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

// HandoverRegistrationPayload 是交接判断首登的线格式，逐格镜像 application.RegisterTransportHandoverCommand 去掉租户，与既有
// 传输层替身的请求体同形。verdict 取 domain.HandoverVerdict 的封闭词，judgedAt 取 RFC 3339；segment / plannedSegment /
// segmentServiceAction 可缺，缺席即不立段。releasingNode / receivingNode / onwardNode 是交出、接收与去向节点的声明
// （NR 节点代码，ADR-0213），各自可缺，缺席即没有声明——不带这三键的报文与它们进文法之前的报文同形同答。
type HandoverRegistrationPayload struct {
	Object               string `json:"object"`
	Scope                string `json:"scope"`
	ReleasedBy           string `json:"releasedBy"`
	ReceivedBy           string `json:"receivedBy"`
	Verdict              string `json:"verdict"`
	ReleasingEvidence    string `json:"releasingEvidence"`
	ReceivingEvidence    string `json:"receivingEvidence"`
	Rule                 string `json:"rule"`
	Basis                string `json:"basis"`
	Version              string `json:"version"`
	JudgedAt             string `json:"judgedAt"`
	Segment              string `json:"segment,omitempty"`
	PlannedSegment       string `json:"plannedSegment,omitempty"`
	SegmentServiceAction string `json:"segmentServiceAction,omitempty"`
	ReleasingNode        string `json:"releasingNode,omitempty"`
	ReceivingNode        string `json:"receivingNode,omitempty"`
	OnwardNode           string `json:"onwardNode,omitempty"`
}

// Command 把载荷连同信封给的租户翻成首登命令。结论词不在封闭集内、时刻解不出是坏报文（400）；结论空着照零值交进去，
// 其余成不成形都留给编排答`未受理`。
func (payload HandoverRegistrationPayload) Command(tenant domain.TenantID) (application.RegisterTransportHandoverCommand, error) {
	none := application.RegisterTransportHandoverCommand{}
	if tenant.String() == "" {
		return none, ErrOperatorIdentityMissing
	}
	command := application.RegisterTransportHandoverCommand{
		TenantID:             tenant,
		Object:               payload.Object,
		Scope:                payload.Scope,
		ReleasedBy:           payload.ReleasedBy,
		ReceivedBy:           payload.ReceivedBy,
		ReleasingEvidence:    payload.ReleasingEvidence,
		ReceivingEvidence:    payload.ReceivingEvidence,
		Rule:                 payload.Rule,
		Basis:                payload.Basis,
		Version:              payload.Version,
		Segment:              payload.Segment,
		PlannedSegment:       payload.PlannedSegment,
		SegmentServiceAction: payload.SegmentServiceAction,
		ReleasingNode:        payload.ReleasingNode,
		ReceivingNode:        payload.ReceivingNode,
		OnwardNode:           payload.OnwardNode,
	}
	var err error
	if payload.Verdict != "" {
		if command.Verdict, err = handoverVerdictFromWord(payload.Verdict); err != nil {
			return none, err
		}
	}
	if command.JudgedAt, err = parseOptionalInstant("judgedAt", payload.JudgedAt); err != nil {
		return none, err
	}
	return command, nil
}

// handoverVerdictFromWord 是 domain.HandoverVerdict 封闭集的名称镜像，词取各常量自己的 String()。
func handoverVerdictFromWord(raw string) (domain.HandoverVerdict, error) {
	for _, verdict := range []domain.HandoverVerdict{
		domain.ObjectHandedOver, domain.HandoverRefused, domain.HandoverPendingConfirmation,
	} {
		if verdict.String() == raw {
			return verdict, nil
		}
	}
	return domain.HandoverVerdictInvalid, fmt.Errorf("%w: verdict=%q is not a handover verdict word", ErrMalformedRequest, raw)
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
	// 三格节点声明回显在册那一版（ADR-0213）：重放答的是首登时的声明，更正答的是从前版承继来的声明。没声明的格不出现，
	// 不声明节点的登记答复与节点进文法之前逐键相同。
	ReleasingNode string `json:"releasingNode,omitempty"`
	ReceivingNode string `json:"receivingNode,omitempty"`
	OnwardNode    string `json:"onwardNode,omitempty"`
	// GateReading 是在册那一版随记的交出门禁读数（ADR-0213 决定四），只在`已交接`上有：重放交回的是那一版落册时问来的
	// 那一份，不是此刻重问的；阻断也照样 201。拒收与待确认上缺席。
	GateReading *handoverGateReadingView `json:"gateReading,omitempty"`
}

// handoverGateReadingView 是交出门禁读数回给登记方的线格式，与出发读数（departureGateReadingView）同形：整格、问关务时的
// 那一份意图（at 交出节点、onward 去向节点，代出发的 from / to）、本上下文这一侧的停点与那件对象的那一格。问了关务而
// 这一版没声明去向时写明 onwardNotDeclared：跨关务区域那一道因此不在答复里，读到「不受门禁」的人得知道那不是关务核过跨区
// （ADR-0213 越权风险点 7）。
//
// 关务判断标识与依据原文不回给登记方，只随交接落册供运营与 VE 查，口径同出发读数（ADR-0205 决定一）。
type handoverGateReadingView struct {
	Answer            string                    `json:"answer"`
	At                string                    `json:"at,omitempty"`
	Onward            string                    `json:"onward,omitempty"`
	OnwardNotDeclared bool                      `json:"onwardNotDeclared,omitempty"`
	Stops             []string                  `json:"stops,omitempty"`
	Objects           []departureGateObjectView `json:"objects,omitempty"`
}

func handoverGateReadingViewOf(reading domain.HandoverGateReading) *handoverGateReadingView {
	view := &handoverGateReadingView{Answer: reading.Answer().String(), OnwardNotDeclared: reading.OnwardNotDeclared()}
	if node, asked := reading.Node(); asked {
		view.At = node.String()
	}
	if onward, declared := reading.Onward(); declared {
		view.Onward = onward.String()
	}
	if stop, stopped := reading.Stop(); stopped {
		view.Stops = []string{stop.String()}
	}
	if finding, asked := reading.Finding(); asked {
		view.Objects = []departureGateObjectView{{Object: finding.Object().String(), Answer: finding.Answer().String()}}
	}
	return view
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
		nodes := record.Handover.Nodes()
		body.ReleasingNode = nodes.Releasing.String()
		body.ReceivingNode = nodes.Receiving.String()
		body.OnwardNode = nodes.Onward.String()
		if reading, read := record.Handover.GateReading(); read {
			body.GateReading = handoverGateReadingViewOf(reading)
		}
	}

	// ADR-0022：状态码只报有没有新落一版。首登与更正都持久化了新版本用 201；重放、冲突、
	// 未受理、未决都是形成了的答案（200）。交出门禁读数是阻断也照样 201：交接落册了，读数是记录里的一格，不是这一次
	// 登记的答复（ADR-0213 决定四）。
	status := http.StatusOK
	if result.Outcome() == application.HandoverRegistered ||
		result.Outcome() == application.HandoverCorrected {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
