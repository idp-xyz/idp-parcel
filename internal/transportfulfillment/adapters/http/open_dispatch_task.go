package tfhttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 票 tf-segment-lifecycle-closure/07 的第二个写面：授权角色建立派送任务（UC-TF-006 的第二个触发源）。
// 第一个触发源「尾程实际履约段到达派送范围」是内部触发，归票 09，不从这里进。

// DispatchTaskIntake 把已认证的运营写请求翻译成建立派送任务的命令。工作范围七件全从请求收——任务表达
// 需要完成什么，不表达已到场、取得控制或完成交付，所以这里没有任何到场或控制入参。
type DispatchTaskIntake interface {
	IntakeDispatchTask(ctx context.Context, request *http.Request) (application.OpenDispatchTaskCommand, error)
}

// DispatchTaskPayload 是建立派送任务的线格式，逐格镜像 application.OpenDispatchTaskCommand 去掉租户——工作范围加建立时刻。
// kind 取 domain.DispatchTaskKind 的封闭词，时刻一律取 RFC 3339。
type DispatchTaskPayload struct {
	Task       string   `json:"task"`
	Kind       string   `json:"kind"`
	Objects    []string `json:"objects"`
	Place      string   `json:"place"`
	WindowFrom string   `json:"windowFrom"`
	WindowTo   string   `json:"windowTo"`
	Conditions string   `json:"conditions"`
	OpenedAt   string   `json:"openedAt"`
}

// Command 把载荷连同信封给的租户翻成建立命令。种类词不在封闭集内、时刻解不出、任务引用落在派送发起执行器的保留起头下，
// 都是坏报文（400）；种类空着照零值交进去，工作范围齐不齐由编排答`输入未受理`。
//
// 保留起头在这里拒而不在编排里拒：执行器形成任务走的也是那条编排，拒在那里会把执行器自己挡住。存量里已经指名过保留引用
// 的手工任务不动，执行器认在册时核种类与对象集兜住它们（票 tf-face-algebra/09 裁定第 2、4 条）。
func (payload DispatchTaskPayload) Command(tenant domain.TenantID) (application.OpenDispatchTaskCommand, error) {
	none := application.OpenDispatchTaskCommand{}
	if tenant.String() == "" {
		return none, ErrOperatorIdentityMissing
	}
	if application.IsReservedDeliveryDispatchTaskReference(payload.Task) {
		return none, fmt.Errorf("%w: task=%q names a reference reserved for the delivery dispatch trigger", ErrMalformedRequest, payload.Task)
	}
	command := application.OpenDispatchTaskCommand{
		TenantID:   tenant,
		Task:       payload.Task,
		Objects:    payload.Objects,
		Place:      payload.Place,
		Conditions: payload.Conditions,
	}
	var err error
	if payload.Kind != "" {
		if command.Kind, err = dispatchTaskKindFromWord(payload.Kind); err != nil {
			return none, err
		}
	}
	if command.WindowFrom, err = parseOptionalInstant("windowFrom", payload.WindowFrom); err != nil {
		return none, err
	}
	if command.WindowTo, err = parseOptionalInstant("windowTo", payload.WindowTo); err != nil {
		return none, err
	}
	if command.OpenedAt, err = parseOptionalInstant("openedAt", payload.OpenedAt); err != nil {
		return none, err
	}
	return command, nil
}

// dispatchTaskKindFromWord 是 domain.DispatchTaskKind 封闭集的名称镜像，词取各常量自己的 String()。
func dispatchTaskKindFromWord(raw string) (domain.DispatchTaskKind, error) {
	for _, kind := range []domain.DispatchTaskKind{domain.PickupDispatch, domain.DeliveryDispatch} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return domain.DispatchTaskKindInvalid, fmt.Errorf("%w: kind=%q is not a dispatch task kind word", ErrMalformedRequest, raw)
}

// DispatchTaskOpener 是本适配器转交的应用编排。
type DispatchTaskOpener interface {
	Open(
		ctx context.Context,
		command application.OpenDispatchTaskCommand,
	) (application.OpenDispatchTaskResult, error)
}

// NewOpenDispatchTaskEndpoint 交回建立派送任务的 HTTP 入口。
func NewOpenDispatchTaskEndpoint(intake DispatchTaskIntake, opener DispatchTaskOpener) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.OpenDispatchTaskResult, error, bool) {
		command, err := intake.IntakeDispatchTask(request.Context(), request)
		if err != nil {
			return application.OpenDispatchTaskResult{}, err, false
		}
		result, err := opener.Open(request.Context(), command)
		return result, err, true
	}, writeDispatchTaskOutcome)
}

// dispatchTaskResponse 是派送任务口的封闭响应形状。`DISPATCH_TASK_ALREADY_OPEN` 交回的是**原任务**的工作
// 范围而不是这次送来的——一次重投不改写工作范围，响应照实透出原任务，调用方据以看出两者差在哪。
//
// 所以载荷收的内容格在这里逐格都要回显，conditions 也不例外：少一格，同一任务换那一格重投时调用方就比不出差异。
// 格序与 deliveryDispatchTriggerResponse 的任务那几格一致，同一项任务在两个口上的回显读起来是同一份。
type dispatchTaskResponse struct {
	Outcome               string   `json:"outcome"`
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

func writeDispatchTaskOutcome(response http.ResponseWriter, result application.OpenDispatchTaskResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := dispatchTaskResponse{Outcome: outcome, ContinuationReference: result.ContinuationReference()}
	if record, present := result.Record(); present {
		body.Task = record.Task.Task().String()
		body.Kind = record.Task.Kind().String()
		for _, object := range record.Task.Objects() {
			body.Objects = append(body.Objects, object.String())
		}
		body.Place = record.Task.Place().String()
		from, to := record.Task.Window()
		body.WindowFrom = rfc3339(from)
		body.WindowTo = rfc3339(to)
		body.Conditions = record.Task.Conditions().String()
		body.OpenedAt = rfc3339(record.Task.OpenedAt())
	}
	// ADR-0022：只有新建立了任务用 201；已在册、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.DispatchTaskOpened {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
