package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUnexpectedDeliveryAttemptSave 说明派送尝试库交回了封闭集合以外的写入结果。
var ErrUnexpectedDeliveryAttemptSave = errors.New("transport fulfillment: unexpected delivery attempt save outcome")

// DeliveryAttemptOutcome 是一次派送尝试登记的应用处理结果。逐对象成败在记录里并存（UC-TF-006：
// 任务汇总只能由对象结果派生），这里只回答登记本身的走向。
type DeliveryAttemptOutcome uint8

const (
	DeliveryAttemptOutcomeInvalid DeliveryAttemptOutcome = iota
	DeliveryAttemptRecorded
	DeliveryAttemptExistingResult
	DeliveryAttemptSourceConflict
	DeliveryAttemptTaskNotOpen
	DeliveryAttemptObjectOutsideTask
	DeliveryAttemptNotAccepted
	DeliveryAttemptUndecided
)

func (outcome DeliveryAttemptOutcome) String() string {
	switch outcome {
	case DeliveryAttemptRecorded:
		return "ATTEMPT_RECORDED"
	case DeliveryAttemptExistingResult:
		return "EXISTING_RESULT"
	case DeliveryAttemptSourceConflict:
		return "SOURCE_CONFLICT"
	case DeliveryAttemptTaskNotOpen:
		return "DELIVERY_TASK_NOT_OPEN"
	case DeliveryAttemptObjectOutsideTask:
		return "OBJECT_OUTSIDE_TASK"
	case DeliveryAttemptNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case DeliveryAttemptUndecided:
		return "DELIVERY_ATTEMPT_UNDECIDED"
	default:
		return ""
	}
}

// DeliveryAttemptUndecidedReason 指名登记停在哪一步等谁。封闭集合：新依赖故障必须补格，不许借用裸字符串
// 标签（同揽收侧 PickupUndecidedReason 的纪律）。
type DeliveryAttemptUndecidedReason uint8

const (
	DeliveryAttemptUndecidedReasonNone DeliveryAttemptUndecidedReason = iota
	DeliveryAttemptStoreUnavailable
	DispatchTaskRegistryUnavailable
)

func (reason DeliveryAttemptUndecidedReason) String() string {
	switch reason {
	case DeliveryAttemptStoreUnavailable:
		return "DELIVERY_ATTEMPT_STORE_UNAVAILABLE"
	case DispatchTaskRegistryUnavailable:
		return "DISPATCH_TASK_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// ObjectDeliverySubmission 是执行方对单个载运对象报回的派送结果。失败、拒收必须带原因依据；
// 妥投不带——妥投的证据走 POD 进有效交付（FormDeliveryAttemptResult 的纪律）。
type ObjectDeliverySubmission struct {
	Object     domain.CarriedObjectReference
	Outcome    domain.DeliveryObjectOutcome
	Basis      domain.AttemptResultBasisReference
	OccurredAt time.Time
}

// RecordDeliveryAttemptCommand 携带一次实际到场的全部来源：尝试身份与内容、逐对象结果。业务时间与
// 执行方随命令进来，服务端不代铸（ADR-0023）。改约或重派由 RescheduledFrom 指向旧尝试。
type RecordDeliveryAttemptCommand struct {
	TenantID        domain.TenantID
	Task            string
	Attempt         string
	ExecutedBy      string
	Place           string
	PlannedFrom     time.Time
	PlannedTo       time.Time
	ArrivedAt       time.Time
	Evidence        string
	RescheduledFrom string
	Objects         []ObjectDeliverySubmission
}

type RecordDeliveryAttemptResult struct {
	outcome      DeliveryAttemptOutcome
	reason       DeliveryAttemptUndecidedReason
	record       ports.DeliveryAttemptRecord
	hasRecord    bool
	continuation string
}

func (result RecordDeliveryAttemptResult) Outcome() DeliveryAttemptOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零：它指名要等哪个依赖恢复。其余各格没有它——那些格的恢复动作是改请求
// 或换任务，不是等谁。
func (result RecordDeliveryAttemptResult) UndecidedReason() DeliveryAttemptUndecidedReason {
	return result.reason
}

func (result RecordDeliveryAttemptResult) Record() (ports.DeliveryAttemptRecord, bool) {
	return result.record, result.hasRecord
}

// ContinuationReference 非空说明登记未决，等依赖恢复后重试同一份提交。
func (result RecordDeliveryAttemptResult) ContinuationReference() string {
	return result.continuation
}

type RecordDeliveryAttemptDeps struct {
	Attempts ports.DeliveryAttemptStore
	Tasks    ports.DispatchTaskReader
	Clock    ports.Clock
}

type RecordDeliveryAttemptHandler struct {
	deps RecordDeliveryAttemptDeps
}

func NewRecordDeliveryAttemptHandler(deps RecordDeliveryAttemptDeps) *RecordDeliveryAttemptHandler {
	return &RecordDeliveryAttemptHandler{deps: deps}
}

// Handle 登记一次派送尝试及其逐对象结果。
func (handler *RecordDeliveryAttemptHandler) Handle(
	ctx context.Context,
	command RecordDeliveryAttemptCommand,
) (RecordDeliveryAttemptResult, error) {
	// 立不起来的尝试与结果是提交自身的矛盾：改请求，重来多少次都一样（ADR-0029 按恢复动作分格），
	// 所以答`来源未受理`而不是报错——报错会被传输层读成「没形成答案」。
	attempt, err := formDeliveryAttempt(command)
	if err != nil {
		return RecordDeliveryAttemptResult{outcome: DeliveryAttemptNotAccepted}, nil
	}
	record := ports.DeliveryAttemptRecord{
		Key:        ports.DeliveryAttemptKey{TenantID: command.TenantID, Attempt: attempt.Attempt()},
		Attempt:    attempt,
		RecordedAt: handler.deps.Clock.Now(),
	}
	for _, submission := range command.Objects {
		result, err := domain.FormDeliveryAttemptResult(
			attempt, submission.Object, submission.Outcome, submission.Basis, submission.OccurredAt)
		if err != nil {
			return RecordDeliveryAttemptResult{outcome: DeliveryAttemptNotAccepted}, nil
		}
		record.Results = append(record.Results, result)
	}

	existing, found, err := handler.deps.Attempts.FindByKey(ctx, record.Key)
	if err != nil {
		return deliveryAttemptUndecided(DeliveryAttemptStoreUnavailable, record.Key), nil
	}
	if found {
		return answerFromExisting(existing, record), nil
	}

	task, found, err := handler.deps.Tasks.FindByKey(ctx,
		ports.DispatchTaskKey{TenantID: command.TenantID, Task: attempt.Task()})
	if err != nil {
		return deliveryAttemptUndecided(DispatchTaskRegistryUnavailable, record.Key), nil
	}
	// 三种情形并一格：任务不存在、是揽收任务、已终止或已完成——恢复动作同一个，都是指向一项开着的派送
	// 任务。已关闭的任务不再接新尝试：后续要派送是新任务（DispatchTask 的关闭纪律）。
	if !found || task.Task.Kind() != domain.DeliveryDispatch || task.Task.State() != domain.TaskOpen {
		return RecordDeliveryAttemptResult{outcome: DeliveryAttemptTaskNotOpen}, nil
	}
	if !taskCoversAll(task.Task, attempt.Objects()) {
		return RecordDeliveryAttemptResult{outcome: DeliveryAttemptObjectOutsideTask}, nil
	}

	saved, err := handler.deps.Attempts.Save(ctx, record)
	if err != nil {
		return deliveryAttemptUndecided(DeliveryAttemptStoreUnavailable, record.Key), nil
	}
	switch saved {
	case ports.DeliveryAttemptSaved:
		return RecordDeliveryAttemptResult{outcome: DeliveryAttemptRecorded, record: record, hasRecord: true}, nil
	case ports.DeliveryAttemptAlreadyRecorded:
		// 查与存之间被另一份同身份的提交抢先：读回赢家按内容作答，不覆盖它。
		winner, found, err := handler.deps.Attempts.FindByKey(ctx, record.Key)
		if err != nil || !found {
			return deliveryAttemptUndecided(DeliveryAttemptStoreUnavailable, record.Key), nil
		}
		return answerFromExisting(winner, record), nil
	default:
		return RecordDeliveryAttemptResult{}, fmt.Errorf("%w: %d", ErrUnexpectedDeliveryAttemptSave, saved)
	}
}

func deliveryAttemptUndecided(reason DeliveryAttemptUndecidedReason, key ports.DeliveryAttemptKey) RecordDeliveryAttemptResult {
	return RecordDeliveryAttemptResult{
		outcome:      DeliveryAttemptUndecided,
		reason:       reason,
		continuation: deliveryAttemptContinuation(reason.String(), key.TenantID.String(), key.Attempt.String()),
	}
}

func deliveryAttemptContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

// taskCoversAll 判一次到场报来的每个对象都在任务的工作范围里。任务表达需要派送哪些对象，范围之外的对象
// 在这项任务下没有可记的执行结果。
func taskCoversAll(task domain.DispatchTask, objects []domain.CarriedObjectReference) bool {
	scope := make(map[domain.CarriedObjectReference]struct{}, len(task.Objects()))
	for _, object := range task.Objects() {
		scope[object] = struct{}{}
	}
	for _, object := range objects {
		if _, covered := scope[object]; !covered {
			return false
		}
	}
	return true
}

// answerFromExisting 按同一尝试身份已有的记录作答：内容相同是重复回传，返回原结果；内容不同是冲突，
// 原记录不动——不按最后一条消息覆盖（UC-TF-006「同一任务、尝试、对象和来源版本只能形成一个结果」）。
func answerFromExisting(existing, submitted ports.DeliveryAttemptRecord) RecordDeliveryAttemptResult {
	if deliveryAttemptContent(existing) != deliveryAttemptContent(submitted) {
		return RecordDeliveryAttemptResult{outcome: DeliveryAttemptSourceConflict}
	}
	return RecordDeliveryAttemptResult{outcome: DeliveryAttemptExistingResult, record: existing, hasRecord: true}
}

// deliveryAttemptContent 是同一尝试身份的内容比对锚，两侧都从领域记录算：库里没有存摘要的列，读回的记录
// 经构造门重建后与新提交同形，逐格比即可。登记时刻不在其中——那是本方何时收到，不是执行方报了什么。
// 对象与结果先排序：提交顺序不构成不同的内容。
func deliveryAttemptContent(record ports.DeliveryAttemptRecord) string {
	attempt := record.Attempt
	rescheduled := ""
	if from, present := attempt.RescheduledFrom(); present {
		rescheduled = from.String()
	}
	objects := make([]string, 0, len(attempt.Objects()))
	for _, object := range attempt.Objects() {
		objects = append(objects, object.String())
	}
	sort.Strings(objects)
	results := make([]string, 0, len(record.Results))
	for _, result := range record.Results {
		basis := ""
		if value, present := result.Basis(); present {
			basis = value.String()
		}
		results = append(results, strings.Join([]string{
			result.Object().String(),
			result.Outcome().String(),
			basis,
			result.OccurredAt().UTC().Format(time.RFC3339Nano),
		}, "\x1f"))
	}
	sort.Strings(results)
	fields := []string{
		attempt.Attempt().String(),
		attempt.Task().String(),
		attempt.ExecutedBy().String(),
		attempt.Place().String(),
		attempt.PlannedFrom().UTC().Format(time.RFC3339Nano),
		attempt.PlannedTo().UTC().Format(time.RFC3339Nano),
		attempt.ArrivedAt().UTC().Format(time.RFC3339Nano),
		attempt.Evidence().String(),
		rescheduled,
		strings.Join(objects, "\x1f"),
	}
	digest := sha256.Sum256([]byte(strings.Join(append(fields, results...), "\x00")))
	return hex.EncodeToString(digest[:])
}

// formDeliveryAttempt 把来源字符串折成领域尝试。立不起来的输入由领域构造器拒绝，编排不代答。
func formDeliveryAttempt(command RecordDeliveryAttemptCommand) (domain.FulfillmentAttempt, error) {
	spec := domain.FulfillmentAttemptSpec{
		TenantID:    command.TenantID,
		PlannedFrom: command.PlannedFrom,
		PlannedTo:   command.PlannedTo,
		ArrivedAt:   command.ArrivedAt,
	}
	var err error
	if spec.Attempt, err = domain.NewAttemptReference(command.Attempt); err != nil {
		return domain.FulfillmentAttempt{}, err
	}
	if spec.Task, err = domain.NewDispatchTaskReference(command.Task); err != nil {
		return domain.FulfillmentAttempt{}, err
	}
	if spec.ExecutedBy, err = domain.NewExecutingPartyReference(command.ExecutedBy); err != nil {
		return domain.FulfillmentAttempt{}, err
	}
	if spec.Place, err = domain.NewAttemptPlaceReference(command.Place); err != nil {
		return domain.FulfillmentAttempt{}, err
	}
	if spec.Evidence, err = domain.NewAttemptEvidenceReference(command.Evidence); err != nil {
		return domain.FulfillmentAttempt{}, err
	}
	if strings.TrimSpace(command.RescheduledFrom) != "" {
		if spec.RescheduledFrom, err = domain.NewAttemptReference(command.RescheduledFrom); err != nil {
			return domain.FulfillmentAttempt{}, err
		}
	}
	for _, submission := range command.Objects {
		spec.Objects = append(spec.Objects, submission.Object)
	}
	return domain.FormFulfillmentAttempt(spec)
}
