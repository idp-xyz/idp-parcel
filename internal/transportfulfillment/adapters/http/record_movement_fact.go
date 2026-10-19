package tfhttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// MovementFactIntake 把已认证的接入请求翻译成一条实际移动事实的登记命令（UC-TF-005「执行方接入：
// 接收出发、移动、到达、中断」）。接口而非解析代码的理由同 DeliveryIntake。
//
// **这个口只收自营执行方的事实；外部承运轨迹不从这里进**（票 tf-segment-lifecycle-closure/03 简报
// 第 4 行）。外部轨迹走 `TrackingSource` 入站口的采纳执行器，那边要先判外部事实的有效时间与归属
// （ADR-0102），本端点若顺手替它做判断，两条来源就并成一个口、两套口径就并成一套。「自营还是
// 外部」由 Intake 的认证结果说（自营走作业事实登记能力面、外部走集成客户端族，ADR-0149），本包不写任何判定。
//
// `segment` / `resource` 是登记方声明的段与资源，可缺（ADR-0205 决定四）。`gateClearance` 是设备出示的放行凭据，原样收作
// 证据；`gateRequired` 是退役键，照收照弃（理由见 MovementFactPayload.RetiredGateRequired）。
type MovementFactIntake interface {
	IntakeMovementFact(ctx context.Context, request *http.Request) (application.RecordMovementFactCommand, error)
}

// MovementFactPayload 是移动事实的线格式，逐格镜像 application.RecordMovementFactCommand 去掉租户与来源——两格都归认证结果
// （来源说的是「自营还是外部」，见 MovementFactIntake）；命令里没有的只有退役键 `gateRequired`。kind 取
// domain.MovementFactKind 的封闭词，occurredAt 取 RFC 3339。
type MovementFactPayload struct {
	Fact          string `json:"fact"`
	Schedule      string `json:"schedule"`
	Kind          string `json:"kind"`
	Location      string `json:"location"`
	Version       string `json:"version"`
	OccurredAt    string `json:"occurredAt"`
	Segment       string `json:"segment,omitempty"`
	Resource      string `json:"resource,omitempty"`
	GateClearance string `json:"gateClearance,omitempty"`

	// RetiredGateRequired 是退役键 `gateRequired`：照收照弃，不进命令，也就进不了内容摘要与读数——带真、带假、不带，登记的是
	// 同一份。设门与否由关务按对象与起讫判，设备自报失了权威（ADR-0205 决定三与文末复核记录）。
	//
	// 键不删：载荷按封闭形状解，删了它，仍按旧形状报出发的离线设备会收到 400，而离线队列对 4xx 出队不重发（ADR-0022），一次
	// 已经发生的出发就此从记录里消失——正是 ADR-0205 决定一不许的。类型照旧是 bool：退役只是不读，不借机把这一格放宽成任意值。
	RetiredGateRequired bool `json:"gateRequired,omitempty"`
}

// Command 把载荷连同信封给的租户与来源翻成登记命令。种类词不在封闭集内、时刻解不出是坏报文（400）；种类空着照零值交进去，
// 其余格成不成形都留给编排答。
func (payload MovementFactPayload) Command(tenant domain.TenantID, source string) (application.RecordMovementFactCommand, error) {
	none := application.RecordMovementFactCommand{}
	if tenant.String() == "" {
		return none, ErrOperatorIdentityMissing
	}
	command := application.RecordMovementFactCommand{
		TenantID:      tenant,
		Fact:          payload.Fact,
		Schedule:      payload.Schedule,
		Location:      payload.Location,
		Source:        source,
		Version:       payload.Version,
		Segment:       payload.Segment,
		Resource:      payload.Resource,
		GateClearance: payload.GateClearance,
	}
	var err error
	if payload.Kind != "" {
		if command.Kind, err = movementFactKindFromWord(payload.Kind); err != nil {
			return none, err
		}
	}
	if command.OccurredAt, err = parseOptionalInstant("occurredAt", payload.OccurredAt); err != nil {
		return none, err
	}
	return command, nil
}

// movementFactKindFromWord 是 domain.MovementFactKind 封闭集的名称镜像，词取各常量自己的 String()。
func movementFactKindFromWord(raw string) (domain.MovementFactKind, error) {
	for _, kind := range []domain.MovementFactKind{domain.DepartureFact, domain.InTransitFact, domain.ArrivalFact} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return domain.MovementFactKindInvalid, fmt.Errorf("%w: kind=%q is not a movement fact kind word", ErrMalformedRequest, raw)
}

// MovementFactHandler 是本适配器转交的应用编排。
type MovementFactHandler interface {
	Record(
		ctx context.Context,
		command application.RecordMovementFactCommand,
	) (application.RecordMovementFactResult, error)
}

// NewRecordMovementFactEndpoint 交回实际移动事实登记的 HTTP 入口。
func NewRecordMovementFactEndpoint(intake MovementFactIntake, handler MovementFactHandler) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.RecordMovementFactResult, error, bool) {
		command, err := intake.IntakeMovementFact(request.Context(), request)
		if err != nil {
			return application.RecordMovementFactResult{}, err, false
		}
		result, err := handler.Record(request.Context(), command)
		return result, err, true
	}, writeMovementFactOutcome)
}

// movementFactResponse 是移动事实口的封闭响应形状。`outcome` 取应用枚举原名。`gateClearance` 是设备出示的凭据，照实透出；
// `gateReading` 只在出发上有，是登记那一刻向关务问来的读数——重放交回的是首次登记时那一份，不是此刻重问的。`SOURCE_CONFLICT`
// 只有 `outcome`：编排不交回在册那一份，它可能是另一作业范围登的。
type movementFactResponse struct {
	Outcome               string                    `json:"outcome"`
	Fact                  string                    `json:"fact,omitempty"`
	Schedule              string                    `json:"schedule,omitempty"`
	Kind                  string                    `json:"kind,omitempty"`
	Location              string                    `json:"location,omitempty"`
	Version               string                    `json:"version,omitempty"`
	OccurredAt            string                    `json:"occurredAt,omitempty"`
	Segment               string                    `json:"segment,omitempty"`
	Resource              string                    `json:"resource,omitempty"`
	GateClearance         string                    `json:"gateClearance,omitempty"`
	GateReading           *departureGateReadingView `json:"gateReading,omitempty"`
	Corrects              string                    `json:"corrects,omitempty"`
	ContinuationReference string                    `json:"continuationReference,omitempty"`
}

// departureGateReadingView 是出发门禁读数回给报事实一方的线格式：整格、问关务时的起讫、本上下文这一侧的停点与逐对象的
// 那一格。逐对象列出而不只给整格：执行方要知道车上被挡的是哪一件，才能去处置那一件而不是整车。
//
// 关务判断标识与依据原文（限制标识、放行来源、申报单元或程序引用）不回给设备，只随事实落册供运营与 VE 查：同段可能
// 是跨客户共同运输，执行方只拿当前履约所需的最小范围（UC-TF-005 规则，ADR-0205 决定一）。
type departureGateReadingView struct {
	Answer  string                    `json:"answer"`
	From    string                    `json:"from,omitempty"`
	To      string                    `json:"to,omitempty"`
	Stops   []string                  `json:"stops,omitempty"`
	Objects []departureGateObjectView `json:"objects,omitempty"`
}

type departureGateObjectView struct {
	Object string `json:"object"`
	Answer string `json:"answer"`
}

func departureGateReadingViewOf(reading domain.DepartureGateReading) *departureGateReadingView {
	view := &departureGateReadingView{Answer: reading.Answer().String()}
	if from, to, directed := reading.Direction(); directed {
		view.From, view.To = from.String(), to.String()
	}
	for _, stop := range reading.Stops() {
		view.Stops = append(view.Stops, stop.String())
	}
	for _, finding := range reading.Findings() {
		view.Objects = append(view.Objects, departureGateObjectView{Object: finding.Object().String(), Answer: finding.Answer().String()})
	}
	return view
}

func writeMovementFactOutcome(response http.ResponseWriter, result application.RecordMovementFactResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := movementFactResponse{
		Outcome:               outcome,
		ContinuationReference: result.ContinuationReference(),
	}
	if record, present := result.Record(); present {
		body.Fact = record.Fact.Fact().String()
		body.Schedule = record.Fact.Schedule().String()
		body.Kind = record.Fact.Kind().String()
		body.Location = record.Fact.Location().String()
		body.Version = record.Fact.Version().String()
		body.OccurredAt = rfc3339(record.Fact.OccurredAt())
		if segment, declared := record.Fact.Segment(); declared {
			body.Segment = segment.String()
		}
		if resource, declared := record.Fact.Resource(); declared {
			body.Resource = resource.String()
		}
		if clearance, present := record.Fact.GateClearance(); present {
			body.GateClearance = clearance.String()
		}
		if reading, read := record.Fact.GateReading(); read {
			body.GateReading = departureGateReadingViewOf(reading)
		}
		if predecessor, corrected := record.Fact.Corrects(); corrected {
			body.Corrects = predecessor.String()
		}
	}

	// ADR-0022：只有新落一版用 201；重放、内容冲突、未受理、未决都是形成了的答案（200）。门禁读数是阻断也照样 201：
	// 出发落册了，读数是记录里的一格，不是这一次登记的答复（ADR-0205 决定一）。
	status := http.StatusOK
	if result.Outcome() == application.MovementFactRecorded {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
