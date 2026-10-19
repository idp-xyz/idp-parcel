package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	// ErrUnexpectedScheduleTemplateSave 说明周期安排登记册交回了封闭集合以外的写入结果。
	ErrUnexpectedScheduleTemplateSave = errors.New("transport fulfillment: unexpected schedule template save outcome")
	// ErrUnexpectedTransportScheduleSave 说明班次登记册交回了封闭集合以外的写入结果。
	ErrUnexpectedTransportScheduleSave = errors.New("transport fulfillment: unexpected transport schedule save outcome")
)

// ScheduleRegistrationOutcome 是周期安排与具体班次登记的应用结果代数（ADR-0201）。两本册子同一套格：
//
//   - 首登与下一版分两格答。同代码（同班次）异内容不是冲突而是下一版（ADR-0201 决定四）——周期安排与班次本来就会
//     改，「改了」要能登得进去；分格是因为读的人续办不同：下一版带着它顶掉的那一版之后的版本号。
//   - 同代码同内容答`已存在`，交回当前版，不多出一版。
//   - 物化时周期安排不在册、或那一天不开，各是一格：前者先登周期安排，后者换日期——都是形成了的业务答案。
type ScheduleRegistrationOutcome uint8

const (
	ScheduleRegistrationOutcomeInvalid ScheduleRegistrationOutcome = iota
	ScheduleTemplateRegistered
	ScheduleTemplateRevised
	ScheduleTemplateExisting
	TransportScheduleRegistered
	TransportScheduleRevised
	TransportScheduleExisting
	ScheduleTemplateNotFound
	ScheduleTemplateNotRunning
	ScheduleRegistrationNotAccepted
	ScheduleRegistrationUndecided
)

func (outcome ScheduleRegistrationOutcome) String() string {
	switch outcome {
	case ScheduleTemplateRegistered:
		return "TEMPLATE_REGISTERED"
	case ScheduleTemplateRevised:
		return "TEMPLATE_REVISED"
	case ScheduleTemplateExisting:
		return "EXISTING_TEMPLATE"
	case TransportScheduleRegistered:
		return "SCHEDULE_REGISTERED"
	case TransportScheduleRevised:
		return "SCHEDULE_REVISED"
	case TransportScheduleExisting:
		return "EXISTING_SCHEDULE"
	case ScheduleTemplateNotFound:
		return "SCHEDULE_TEMPLATE_NOT_FOUND"
	case ScheduleTemplateNotRunning:
		return "SCHEDULE_TEMPLATE_NOT_RUNNING"
	case ScheduleRegistrationNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case ScheduleRegistrationUndecided:
		return "REGISTRATION_UNDECIDED"
	default:
		return ""
	}
}

// Persisted 报这一答有没有新落一版——端点据此在 201 与 200 之间选（ADR-0022）。
func (outcome ScheduleRegistrationOutcome) Persisted() bool {
	switch outcome {
	case ScheduleTemplateRegistered, ScheduleTemplateRevised, TransportScheduleRegistered, TransportScheduleRevised:
		return true
	default:
		return false
	}
}

// ScheduleRegistrationUndecidedReason 指名登记停在哪本册子上。
type ScheduleRegistrationUndecidedReason uint8

const (
	ScheduleRegistrationUndecidedReasonNone ScheduleRegistrationUndecidedReason = iota
	ScheduleTemplateRegistryUnavailable
	TransportScheduleRegistryUnavailable
	CapacityPoolRegistryUnavailable
)

func (reason ScheduleRegistrationUndecidedReason) String() string {
	switch reason {
	case ScheduleTemplateRegistryUnavailable:
		return "SCHEDULE_TEMPLATE_REGISTRY_UNAVAILABLE"
	case TransportScheduleRegistryUnavailable:
		return "TRANSPORT_SCHEDULE_REGISTRY_UNAVAILABLE"
	case CapacityPoolRegistryUnavailable:
		return "CAPACITY_POOL_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// RegisterScheduleTemplateCommand 是一版周期安排的全部输入（ADR-0201 决定三的文法）。星期取 MON…SUN，出发时刻
// 取当地 HH:MM，生效区间取 YYYY-MM-DD，终点不含、空着即不设终点；容量声明可缺，给了就逐维一条。
type RegisterScheduleTemplateCommand struct {
	TenantID           domain.TenantID
	Template           string
	Connection         string
	FromNode           string
	ToNode             string
	Weekdays           []string
	DepartureLocalTime string
	DurationMinutes    int64
	TimeZone           string
	ServiceScope       string
	EffectiveFrom      string
	EffectiveUntil     string
	Capacities         []CapacityDeclarationInput
}

// CapacityDeclarationInput 是周期安排上一条逐维容量声明的输入：维度取 domain.CapacityDimension 的封闭词，单位
// 原样引用，硬上限为正。
type CapacityDeclarationInput struct {
	Dimension string
	Unit      string
	Capacity  int64
}

// RegisterTransportScheduleCommand 登一个具体班次，两种形状二选一（ADR-0201 决定五）：
//
//   - 由周期安排物化：只给 Template 与 RunningDate，班次身份与正文全由那份周期安排的当前版算出；
//   - 临时班次：给 Schedule 与正文各格，不给 Template 与 RunningDate。
//
// 两种形状的字段混着给即`未受理`：登记方到底要哪一种说不清，按任何一种去登都是替他选。
type RegisterTransportScheduleCommand struct {
	TenantID     domain.TenantID
	Template     string
	RunningDate  string
	Schedule     string
	Connection   string
	FromNode     string
	ToNode       string
	DepartsAt    time.Time
	ArrivesAt    time.Time
	ServiceScope string
}

func (command RegisterTransportScheduleCommand) materializes() bool {
	return command.Template != "" || command.RunningDate != ""
}

func (command RegisterTransportScheduleCommand) declaresAdHocContent() bool {
	return command.Schedule != "" || command.Connection != "" || command.FromNode != "" || command.ToNode != "" ||
		!command.DepartsAt.IsZero() || !command.ArrivesAt.IsZero() || command.ServiceScope != ""
}

type ScheduleRegistrationResult struct {
	outcome      ScheduleRegistrationOutcome
	reason       ScheduleRegistrationUndecidedReason
	template     ports.ScheduleTemplateRecord
	hasTemplate  bool
	schedule     ports.ScheduleRecord
	hasSchedule  bool
	pools        []MaterializedPool
	continuation string
}

// MaterializedPool 是按周期安排容量声明物化登记一班时，一个声明维度上的池答案：`已建立`、`已存在`或`池冲突`，
// 各带那个池——冲突带的是库里已建的那一个。
type MaterializedPool struct {
	Outcome OpportunityOutcome
	Pool    ports.CapacityPoolRecord
}

func (result ScheduleRegistrationResult) Outcome() ScheduleRegistrationOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result ScheduleRegistrationResult) UndecidedReason() ScheduleRegistrationUndecidedReason {
	return result.reason
}

// Template 在周期安排登记的`已登记`、`下一版`、`已存在`上交回那一版。
func (result ScheduleRegistrationResult) Template() (ports.ScheduleTemplateRecord, bool) {
	return result.template, result.hasTemplate
}

// Schedule 在班次登记的`已登记`、`下一版`、`已存在`上交回那一版。
func (result ScheduleRegistrationResult) Schedule() (ports.ScheduleRecord, bool) {
	return result.schedule, result.hasSchedule
}

// Pools 在物化登记的`已登记`、`下一版`、`已存在`上交回周期安排当前版逐维声明的池，按维度次序；周期安排不声明
// 容量、或登的是临时班次时为空。
func (result ScheduleRegistrationResult) Pools() []MaterializedPool {
	return append([]MaterializedPool(nil), result.pools...)
}

// Persisted 报这一次有没有新落任何东西：班次落了一版，或任何一个池新建立。端点据此在 201 与 200 之间选
// （ADR-0022）——班次`已存在`而周期安排新声明了一个维度时，那个池是这一次新落的。
func (result ScheduleRegistrationResult) Persisted() bool {
	if result.outcome.Persisted() {
		return true
	}
	for _, pool := range result.pools {
		if pool.Outcome == PoolEstablished {
			return true
		}
	}
	return false
}

func (result ScheduleRegistrationResult) ContinuationReference() string {
	return result.continuation
}

type RegisterTransportScheduleDeps struct {
	Templates ports.ScheduleTemplateRegistry
	Schedules ports.TransportScheduleStore
	Pools     ports.CapacityPoolStore
	Clock     ports.Clock
}

// RegisterTransportScheduleHandler 是周期安排与具体班次两本册子的登记编排（ADR-0201 决定一）。登记册配置写：建
// 班次不搬包裹，也不形成生产事实，CLI 与端点调的是同一个它（ADR-0085）。没有意图交付——班次是本上下文自己的
// 执行准备，用到它的（准备、订舱、装载分配）按需来读。按周期安排物化登记一班时，那一版的逐维容量声明随班次建池
// （ADR-0201 决定三），池落在容量池册上，与班次同一笔。
type RegisterTransportScheduleHandler struct {
	deps RegisterTransportScheduleDeps
}

func NewRegisterTransportScheduleHandler(deps RegisterTransportScheduleDeps) *RegisterTransportScheduleHandler {
	return &RegisterTransportScheduleHandler{deps: deps}
}

// RegisterTemplate 登一版周期安排：受理 → 读当前版 → 同内容答`已存在`，异内容形成下一版，没有当前版立首版 →
// 原子提交。
func (handler *RegisterTransportScheduleHandler) RegisterTemplate(
	ctx context.Context,
	command RegisterScheduleTemplateCommand,
) (ScheduleRegistrationResult, error) {
	spec, err := scheduleTemplateSpecFrom(command)
	if err != nil {
		return ScheduleRegistrationResult{outcome: ScheduleRegistrationNotAccepted}, nil
	}
	if _, err := domain.RegisterScheduleTemplate(spec); err != nil {
		return ScheduleRegistrationResult{outcome: ScheduleRegistrationNotAccepted}, nil
	}
	_, digest, err := domain.CanonicalizeScheduleTemplatePayload(spec.Content)
	if err != nil {
		return ScheduleRegistrationResult{}, err
	}
	key := ports.ScheduleTemplateKey{TenantID: spec.TenantID, Code: spec.Code}

	current, found, err := handler.deps.Templates.FindCurrentTemplate(ctx, key)
	if err != nil {
		return scheduleRegistrationUndecided(ScheduleTemplateRegistryUnavailable, key.TenantID, key.Code.String()), nil
	}
	var template domain.ScheduleTemplate
	outcome := ScheduleTemplateRegistered
	if found {
		switch compareScheduleRegistryDigest(current.ContentDigest, digest) {
		case domain.SamePayload:
			return ScheduleRegistrationResult{outcome: ScheduleTemplateExisting, template: current, hasTemplate: true}, nil
		case domain.DifferentPayload:
			template, err = current.Template.Revise(spec.Content)
			outcome = ScheduleTemplateRevised
		default:
			return scheduleRegistrationUndecided(ScheduleTemplateRegistryUnavailable, key.TenantID, key.Code.String()), nil
		}
	} else {
		template, err = domain.RegisterScheduleTemplate(spec)
	}
	if err != nil {
		return ScheduleRegistrationResult{outcome: ScheduleRegistrationNotAccepted}, nil
	}

	record := ports.ScheduleTemplateRecord{
		Key:           key,
		ContentDigest: digest,
		Template:      template,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Templates.SaveTemplateVersion(ctx, record)
	if err != nil {
		return scheduleRegistrationUndecided(ScheduleTemplateRegistryUnavailable, key.TenantID, key.Code.String()), nil
	}
	switch saved {
	case ports.ScheduleTemplateSaved:
		return ScheduleRegistrationResult{outcome: outcome, template: record, hasTemplate: true}, nil
	case ports.ScheduleTemplateVersionTaken:
		// 另一方先落了同一个版本号。它的内容与本次相同就是并发重放；不同就答未决——重跑时读到它已是当前版，
		// 再答一个正当的下一版或`已存在`，不在这里替两方排先后。
		winner, found, err := handler.deps.Templates.FindCurrentTemplate(ctx, key)
		if err != nil || !found || compareScheduleRegistryDigest(winner.ContentDigest, digest) != domain.SamePayload {
			return scheduleRegistrationUndecided(ScheduleTemplateRegistryUnavailable, key.TenantID, key.Code.String()), nil
		}
		return ScheduleRegistrationResult{outcome: ScheduleTemplateExisting, template: winner, hasTemplate: true}, nil
	default:
		return ScheduleRegistrationResult{}, fmt.Errorf("%w: %d", ErrUnexpectedScheduleTemplateSave, saved)
	}
}

// RegisterSchedule 登一个具体班次：物化形取周期安排当前版按运行日期算出班次；临时形按给的正文立班次。之后同一
// 条路：读当前版 → 同内容答`已存在`，异内容形成下一版 → 原子提交。
func (handler *RegisterTransportScheduleHandler) RegisterSchedule(
	ctx context.Context,
	command RegisterTransportScheduleCommand,
) (ScheduleRegistrationResult, error) {
	if command.materializes() {
		if command.declaresAdHocContent() {
			return ScheduleRegistrationResult{outcome: ScheduleRegistrationNotAccepted}, nil
		}
		return handler.materialize(ctx, command)
	}
	schedule, err := adHocScheduleFrom(command)
	if err != nil {
		return ScheduleRegistrationResult{outcome: ScheduleRegistrationNotAccepted}, nil
	}
	return handler.registerSchedule(ctx, schedule)
}

func (handler *RegisterTransportScheduleHandler) materialize(
	ctx context.Context,
	command RegisterTransportScheduleCommand,
) (ScheduleRegistrationResult, error) {
	code, err := domain.NewScheduleTemplateCode(command.Template)
	if err != nil {
		return ScheduleRegistrationResult{outcome: ScheduleRegistrationNotAccepted}, nil
	}
	date, err := domain.ParseRunningDate(command.RunningDate)
	if err != nil {
		return ScheduleRegistrationResult{outcome: ScheduleRegistrationNotAccepted}, nil
	}
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return ScheduleRegistrationResult{outcome: ScheduleRegistrationNotAccepted}, nil
	}
	current, found, err := handler.deps.Templates.FindCurrentTemplate(ctx, ports.ScheduleTemplateKey{
		TenantID: command.TenantID,
		Code:     code,
	})
	if err != nil {
		return scheduleRegistrationUndecided(ScheduleTemplateRegistryUnavailable, command.TenantID, code.String()), nil
	}
	if !found {
		return ScheduleRegistrationResult{outcome: ScheduleTemplateNotFound}, nil
	}
	schedule, err := domain.MaterializeSchedule(current.Template, date)
	if errors.Is(err, domain.ErrScheduleTemplateNotRunning) {
		return ScheduleRegistrationResult{outcome: ScheduleTemplateNotRunning, template: current, hasTemplate: true}, nil
	}
	if err != nil {
		return ScheduleRegistrationResult{outcome: ScheduleRegistrationNotAccepted}, nil
	}
	pools, err := current.Template.MaterializeCapacityPools(schedule)
	if err != nil {
		return ScheduleRegistrationResult{outcome: ScheduleRegistrationNotAccepted}, nil
	}
	result, err := handler.registerSchedule(ctx, schedule)
	if err != nil {
		return ScheduleRegistrationResult{}, err
	}
	switch result.outcome {
	case TransportScheduleRegistered, TransportScheduleRevised, TransportScheduleExisting:
	default:
		return result, nil
	}
	return handler.establishPools(ctx, result, pools)
}

// establishPools 在登记下来的那一班上逐维立池，答案逐池列在班次答案旁。
//
// 已建的池同内容答`已存在`，异内容答`池冲突`并保留库里那一个——周期安排换版改了某维容量，再按同一日期登记时班次
// 跟着成下一版或答`已存在`，池却不改写（ADR-0201 补记「池不改版」）。冲突不挡班次：一维容量对不上不是这一班不能
// 登的理由，挡住它只会让班次也跟不上新的一版；冲突逐池摆在答案里，登记方看得见哪一维没跟上。
//
// 池册读不动答未决：重跑同一登记即续办——班次那一半已落就答`已存在`，池在那一次补齐。
func (handler *RegisterTransportScheduleHandler) establishPools(
	ctx context.Context,
	result ScheduleRegistrationResult,
	pools []domain.CapacityPool,
) (ScheduleRegistrationResult, error) {
	for _, pool := range pools {
		established, err := establishMaterializedPool(ctx, handler.deps.Pools, handler.deps.Clock, pool)
		if err != nil {
			return ScheduleRegistrationResult{}, err
		}
		record, _ := established.Pool()
		switch established.Outcome() {
		case PoolEstablished, PoolExistingResult, PoolConflict:
			result.pools = append(result.pools, MaterializedPool{Outcome: established.Outcome(), Pool: record})
		default:
			key := result.schedule.Key
			return scheduleRegistrationUndecided(CapacityPoolRegistryUnavailable, key.TenantID, pool.Pool().String()), nil
		}
	}
	return result, nil
}

func (handler *RegisterTransportScheduleHandler) registerSchedule(
	ctx context.Context,
	schedule domain.TransportSchedule,
) (ScheduleRegistrationResult, error) {
	_, digest, err := domain.CanonicalizeTransportSchedulePayload(schedule)
	if err != nil {
		return ScheduleRegistrationResult{}, err
	}
	key := ports.ScheduleKey{TenantID: schedule.TenantID(), Schedule: schedule.Schedule()}
	undecided := scheduleRegistrationUndecided(TransportScheduleRegistryUnavailable, key.TenantID, key.Schedule.String())

	current, found, err := handler.deps.Schedules.FindByKey(ctx, key)
	if err != nil {
		return undecided, nil
	}
	outcome := TransportScheduleRegistered
	if found {
		switch compareScheduleRegistryDigest(current.ContentDigest, digest) {
		case domain.SamePayload:
			return ScheduleRegistrationResult{outcome: TransportScheduleExisting, schedule: current, hasSchedule: true}, nil
		case domain.DifferentPayload:
			if schedule, err = current.Schedule.Revise(schedule); err != nil {
				return ScheduleRegistrationResult{outcome: ScheduleRegistrationNotAccepted}, nil
			}
			outcome = TransportScheduleRevised
		default:
			return undecided, nil
		}
	}

	record := ports.ScheduleRecord{
		Key:           key,
		ContentDigest: digest,
		Schedule:      schedule,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Schedules.Save(ctx, record)
	if err != nil {
		return undecided, nil
	}
	switch saved {
	case ports.ScheduleSaved:
		return ScheduleRegistrationResult{outcome: outcome, schedule: record, hasSchedule: true}, nil
	case ports.ScheduleAlreadyEstablished:
		// 理由同周期安排那一格：同内容是并发重放，异内容留给重跑。
		winner, found, err := handler.deps.Schedules.FindByKey(ctx, key)
		if err != nil || !found || compareScheduleRegistryDigest(winner.ContentDigest, digest) != domain.SamePayload {
			return undecided, nil
		}
		return ScheduleRegistrationResult{outcome: TransportScheduleExisting, schedule: winner, hasSchedule: true}, nil
	default:
		return ScheduleRegistrationResult{}, fmt.Errorf("%w: %d", ErrUnexpectedTransportScheduleSave, saved)
	}
}

// compareScheduleRegistryDigest 比两本册子上的已存摘要。两本册子立于 TFC-1 之后（迁移 0024 立表时班次表为空），
// 没有无版本那一形可比，所以 CompareStoredDigest 的无版本一格交空串：不带前缀的已存摘要只会是坏行，比不上就不是
// 重放。
func compareScheduleRegistryDigest(stored, current string) domain.StoredDigestComparison {
	return domain.CompareStoredDigest(stored, current, "")
}

// maxDurationMinutes 是历时折成 time.Duration 不溢出的上限。它不是业务上限——多长合理归租户——只挡算出来的
// 到达时刻会绕回去的那种输入。
const maxDurationMinutes = math.MaxInt64 / int64(time.Minute)

func scheduleTemplateSpecFrom(command RegisterScheduleTemplateCommand) (domain.ScheduleTemplateSpec, error) {
	none := domain.ScheduleTemplateSpec{}
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return none, errors.New("blank tenant")
	}
	code, err := domain.NewScheduleTemplateCode(command.Template)
	if err != nil {
		return none, err
	}
	direction, err := domain.NewScheduleDirection(command.Connection, command.FromNode, command.ToNode)
	if err != nil {
		return none, err
	}
	scope, err := domain.NewScheduleServiceScopeReference(command.ServiceScope)
	if err != nil {
		return none, err
	}
	weekdays := make([]time.Weekday, 0, len(command.Weekdays))
	for _, word := range command.Weekdays {
		weekday, err := domain.ParseScheduleWeekday(word)
		if err != nil {
			return none, err
		}
		weekdays = append(weekdays, weekday)
	}
	departure, err := domain.ParseLocalTimeOfDay(command.DepartureLocalTime)
	if err != nil {
		return none, err
	}
	if command.DurationMinutes <= 0 || command.DurationMinutes > maxDurationMinutes {
		return none, errors.New("duration out of range")
	}
	effectiveFrom, err := domain.ParseRunningDate(command.EffectiveFrom)
	if err != nil {
		return none, err
	}
	var effectiveUntil domain.RunningDate
	if command.EffectiveUntil != "" {
		if effectiveUntil, err = domain.ParseRunningDate(command.EffectiveUntil); err != nil {
			return none, err
		}
	}
	capacities := make([]domain.CapacityDeclaration, 0, len(command.Capacities))
	for _, input := range command.Capacities {
		dimension, err := domain.ParseCapacityDimension(input.Dimension)
		if err != nil {
			return none, err
		}
		unit, err := domain.NewQuantityUnitReference(input.Unit)
		if err != nil {
			return none, err
		}
		capacities = append(capacities, domain.CapacityDeclaration{Dimension: dimension, Unit: unit, Capacity: input.Capacity})
	}
	return domain.ScheduleTemplateSpec{
		TenantID: command.TenantID,
		Code:     code,
		Content: domain.ScheduleTemplateContent{
			Direction:            direction,
			Weekdays:             weekdays,
			DepartureLocalMinute: departure,
			Duration:             time.Duration(command.DurationMinutes) * time.Minute,
			TimeZone:             command.TimeZone,
			ServiceScope:         scope,
			EffectiveFrom:        effectiveFrom,
			EffectiveUntil:       effectiveUntil,
			Capacities:           capacities,
		},
	}, nil
}

func adHocScheduleFrom(command RegisterTransportScheduleCommand) (domain.TransportSchedule, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return domain.TransportSchedule{}, errors.New("blank tenant")
	}
	reference, err := domain.NewScheduleReference(command.Schedule)
	if err != nil {
		return domain.TransportSchedule{}, err
	}
	direction, err := domain.NewScheduleDirection(command.Connection, command.FromNode, command.ToNode)
	if err != nil {
		return domain.TransportSchedule{}, err
	}
	scope, err := domain.NewScheduleServiceScopeReference(command.ServiceScope)
	if err != nil {
		return domain.TransportSchedule{}, err
	}
	return domain.FormTransportSchedule(domain.TransportScheduleSpec{
		TenantID:     command.TenantID,
		Schedule:     reference,
		Direction:    direction,
		DepartsAt:    command.DepartsAt,
		ArrivesAt:    command.ArrivesAt,
		ServiceScope: scope,
	})
}

func scheduleRegistrationUndecided(
	reason ScheduleRegistrationUndecidedReason,
	tenant domain.TenantID,
	subject string,
) ScheduleRegistrationResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{reason.String(), tenant.String(), subject}, "\x00")))
	return ScheduleRegistrationResult{
		outcome:      ScheduleRegistrationUndecided,
		reason:       reason,
		continuation: "CONT-" + hex.EncodeToString(digest[:8]),
	}
}
