package tfhttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 票 tf-segment-lifecycle-closure/12「生产入口」节：末端派送任务**内部触发执行器**的生产入口（ADR-0114 决定二末句
// 「执行器的生产入口随第一条派送要求缝的实施票立」）。形照 ADR-0106 决定四 / ADR-0124 与本包既有的
// `/transport-fulfillment-dispatch-task-registrations`：触发面是端点不是进程内循环。**谁按拍调、拍频多大属调用方**
// （实例半边，随租户运营节拍配置，仓内不写默认）；机制半边是这一口 + 装配点接真执行器；Intake 以 UnconfiguredIntake{}
// 起步，如实答未配置——不猜一个拍频、不在进程里空转。
//
// 与 `-dispatch-task-registrations` 那口是两件事：那口是授权角色手工建立任务（工作范围七件全从请求收）；本口只报
// 「哪个对象凭`已交接`进了哪个段」这一触发事实，七件由执行器按派送要求向各所有者取，请求里没有任何地点、时间窗或
// 条件——调用方给不了也不该给（ADR-0114 决定三）。

// DeliveryDispatchTriggerIntake 把已认证的一拍翻译成触发命令：租户从信封给，段、对象与这一拍的业务时间从请求收。
type DeliveryDispatchTriggerIntake interface {
	IntakeDeliveryDispatchTrigger(ctx context.Context, request *http.Request) (application.TriggerDeliveryDispatchCommand, error)
}

// DeliveryDispatchTriggerer 是本适配器转交的执行器。
type DeliveryDispatchTriggerer interface {
	Trigger(
		ctx context.Context,
		command application.TriggerDeliveryDispatchCommand,
	) (application.TriggerDeliveryDispatchResult, error)
}

// UnconfiguredIntake 对本口的答法与其余各口同：不读请求、不构造命令，一律未配置。声明在本文件而不是
// unconfigured_intake.go，是为了让这一口的三件（Intake 形、端点、未配置答法）同文件可读；语义一字不差。
var _ DeliveryDispatchTriggerIntake = UnconfiguredIntake{}

func (UnconfiguredIntake) IntakeDeliveryDispatchTrigger(context.Context, *http.Request) (application.TriggerDeliveryDispatchCommand, error) {
	return application.TriggerDeliveryDispatchCommand{}, ErrAccessChannelNotConfigured
}

// NewTriggerDeliveryDispatchEndpoint 交回按拍触发一次末端派送任务形成的 HTTP 入口。
func NewTriggerDeliveryDispatchEndpoint(intake DeliveryDispatchTriggerIntake, triggerer DeliveryDispatchTriggerer) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.TriggerDeliveryDispatchResult, error, bool) {
		command, err := intake.IntakeDeliveryDispatchTrigger(request.Context(), request)
		if err != nil {
			return application.TriggerDeliveryDispatchResult{}, err, false
		}
		result, err := triggerer.Trigger(request.Context(), command)
		return result, err, true
	}, writeDeliveryDispatchTriggerOutcome)
}

// deliveryDispatchTriggerResponse 是触发口的封闭响应形状：结果代数六格逐字透出，`不是触发事实`带 refusal、`要求缺失`
// 带 missing 名单、`未决`带 undecidedReason 与续办引用，`已形成`/`已在册`带任务。任务的 Place 就是所有者交回的引用串
// ——这里照抄不解析（ADR-0130 决定一「TF 只当不透明串」）。
type deliveryDispatchTriggerResponse struct {
	Outcome               string   `json:"outcome"`
	Refusal               string   `json:"refusal,omitempty"`
	Missing               []string `json:"missing,omitempty"`
	UndecidedReason       string   `json:"undecidedReason,omitempty"`
	Task                  string   `json:"task,omitempty"`
	Kind                  string   `json:"kind,omitempty"`
	Objects               []string `json:"objects,omitempty"`
	Place                 string   `json:"place,omitempty"`
	WindowFrom            string   `json:"windowFrom,omitempty"`
	WindowTo              string   `json:"windowTo,omitempty"`
	Conditions            string   `json:"conditions,omitempty"`
	OpenedAt              string   `json:"openedAt,omitempty"`
	ContinuationReference string   `json:"continuationReference,omitempty"`
}

func writeDeliveryDispatchTriggerOutcome(response http.ResponseWriter, result application.TriggerDeliveryDispatchResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := deliveryDispatchTriggerResponse{
		Outcome:               outcome,
		Refusal:               result.Refusal().String(),
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
	}
	for _, requirement := range result.Missing() {
		body.Missing = append(body.Missing, requirement.String())
	}
	if record, present := result.Record(); present {
		body.Task = record.Task.Task().String()
		body.Kind = record.Task.Kind().String()
		for _, object := range record.Task.Objects() {
			body.Objects = append(body.Objects, object.String())
		}
		body.Place = record.Task.Place().String()
		from, to := record.Task.Window()
		body.WindowFrom = from.UTC().Format(time.RFC3339)
		body.WindowTo = to.UTC().Format(time.RFC3339)
		body.Conditions = record.Task.Conditions().String()
		body.OpenedAt = record.Task.OpenedAt().UTC().Format(time.RFC3339)
	}
	// ADR-0022：只有这一拍新形成了任务用 201；已在册、不是触发事实、要求缺失、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.DeliveryDispatchTaskFormed {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
