package tfhttp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// MovementFactIntake 把已认证的接入请求翻译成一条实际移动事实的登记命令（UC-TF-005「执行方接入：
// 接收出发、移动、到达、中断」）。接口而非解析代码的理由同 DeliveryIntake。
//
// **这个口只收自营执行方的事实；外部承运轨迹不从这里进**（票 tf-segment-lifecycle-closure/03 简报
// 第 4 行）。外部轨迹走 `TrackingSource` 入站口的采纳执行器，那边要先判外部事实的有效时间与归属
// （ADR-0102），本端点若顺手替它做判断，两条来源就并成一个口、两套口径就并成一套。「自营还是
// 外部」由 Intake 的认证结果说，属 PAR-INT-01 实例半边，本包不写任何判定。
//
// `GateRequired` / `GateClearance` 必须收：门禁约束的是装载出发动作，是不是要门禁、放行依据是什么
// 由请求给，领域按种类判它该不该在场；Intake 漏收这两格，一条该被门禁挡住的出发就登上了。
type MovementFactIntake interface {
	IntakeMovementFact(ctx context.Context, request *http.Request) (application.RecordMovementFactCommand, error)
}

// MovementFactPayload 是移动事实的线格式，逐格镜像 application.RecordMovementFactCommand 去掉租户与来源——两格都归认证结果
// （来源说的是「自营还是外部」，见 MovementFactIntake）。kind 取 domain.MovementFactKind 的封闭词，occurredAt 取 RFC 3339。
type MovementFactPayload struct {
	Fact          string `json:"fact"`
	Schedule      string `json:"schedule"`
	Kind          string `json:"kind"`
	Location      string `json:"location"`
	Version       string `json:"version"`
	OccurredAt    string `json:"occurredAt"`
	GateRequired  bool   `json:"gateRequired"`
	GateClearance string `json:"gateClearance,omitempty"`
}

// Command 把载荷连同信封给的租户与来源翻成登记命令。种类词不在封闭集内、时刻解不出是坏报文（400）；种类空着照零值交进去，
// 门禁该不该在场、其余格成不成形都留给编排答（`门禁未放行`与`未受理`各自成格）。
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
		GateRequired:  payload.GateRequired,
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

// movementFactResponse 是移动事实口的封闭响应形状。`outcome` 取应用枚举原名，`DEPARTURE_GATE_BLOCKED`
// 与 `INPUT_NOT_ACCEPTED` 各自成格不合并——前者去取放行结果，后者去改输入（理由在
// application.MovementFactOutcome）。`gateClearance` 只在出发那一格有意义，照实透出不做判断。
type movementFactResponse struct {
	Outcome               string `json:"outcome"`
	Fact                  string `json:"fact,omitempty"`
	Schedule              string `json:"schedule,omitempty"`
	Kind                  string `json:"kind,omitempty"`
	Location              string `json:"location,omitempty"`
	Version               string `json:"version,omitempty"`
	OccurredAt            string `json:"occurredAt,omitempty"`
	GateClearance         string `json:"gateClearance,omitempty"`
	Corrects              string `json:"corrects,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
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
		body.OccurredAt = record.Fact.OccurredAt().UTC().Format(time.RFC3339)
		if clearance, present := record.Fact.GateClearance(); present {
			body.GateClearance = clearance.String()
		}
		if predecessor, corrected := record.Fact.Corrects(); corrected {
			body.Corrects = predecessor.String()
		}
	}

	// ADR-0022：只有新落一版用 201；重放、门禁未放行、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.MovementFactRecorded {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
