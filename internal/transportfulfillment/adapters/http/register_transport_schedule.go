package tfhttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 周期安排与具体班次两本册子的在线登记口（ADR-0201 决定一）。登记册配置写：不搬包裹、不形成生产事实，挂操作者
// 渠道的登记写面（ADR-0100、ADR-0151 定族判据）；受控 CLI parcel-transport-register 收的是同一份载荷再加租户，
// 调的是同一个编排（ADR-0085）。载荷里只有内容，没有身份——租户取认证结果。

// ScheduleTemplateIntake 把已认证的接入请求翻译成周期安排登记命令。
type ScheduleTemplateIntake interface {
	IntakeScheduleTemplateRegistration(ctx context.Context, request *http.Request) (application.RegisterScheduleTemplateCommand, error)
}

// TransportScheduleIntake 把已认证的接入请求翻译成具体班次登记命令。
type TransportScheduleIntake interface {
	IntakeTransportScheduleRegistration(ctx context.Context, request *http.Request) (application.RegisterTransportScheduleCommand, error)
}

// TransportScheduleRegistrar 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。
type TransportScheduleRegistrar interface {
	RegisterTemplate(ctx context.Context, command application.RegisterScheduleTemplateCommand) (application.ScheduleRegistrationResult, error)
	RegisterSchedule(ctx context.Context, command application.RegisterTransportScheduleCommand) (application.ScheduleRegistrationResult, error)
}

// ScheduleTemplateRegistrationPayload 是周期安排登记载荷的线格式，字段与应用命令一一对应、只少租户一格。词取原词
// 交编排：星期不在 MON…SUN、时刻不是 HH:MM、日期不是 YYYY-MM-DD、容量维度不在封闭词里都由编排答`未受理`（200），
// 不在这里折成坏报文。
type ScheduleTemplateRegistrationPayload struct {
	Template           string                       `json:"template"`
	Connection         string                       `json:"connection"`
	FromNode           string                       `json:"fromNode"`
	ToNode             string                       `json:"toNode"`
	Weekdays           []string                     `json:"weekdays"`
	DepartureLocalTime string                       `json:"departureLocalTime"`
	DurationMinutes    int64                        `json:"durationMinutes"`
	TimeZone           string                       `json:"timeZone"`
	ServiceScope       string                       `json:"serviceScope"`
	EffectiveFrom      string                       `json:"effectiveFrom"`
	EffectiveUntil     string                       `json:"effectiveUntil,omitempty"`
	Capacities         []CapacityDeclarationPayload `json:"capacities,omitempty"`
}

// CapacityDeclarationPayload 是周期安排上一条逐维容量声明的线格式（ADR-0201 决定三）。
type CapacityDeclarationPayload struct {
	Dimension string `json:"dimension"`
	Unit      string `json:"unit"`
	Capacity  int64  `json:"capacity"`
}

func (payload ScheduleTemplateRegistrationPayload) Command(tenant domain.TenantID) (application.RegisterScheduleTemplateCommand, error) {
	if tenant.String() == "" {
		return application.RegisterScheduleTemplateCommand{}, ErrOperatorIdentityMissing
	}
	var capacities []application.CapacityDeclarationInput
	for _, declaration := range payload.Capacities {
		capacities = append(capacities, application.CapacityDeclarationInput{
			Dimension: declaration.Dimension,
			Unit:      declaration.Unit,
			Capacity:  declaration.Capacity,
		})
	}
	return application.RegisterScheduleTemplateCommand{
		TenantID:           tenant,
		Template:           payload.Template,
		Connection:         payload.Connection,
		FromNode:           payload.FromNode,
		ToNode:             payload.ToNode,
		Weekdays:           append([]string(nil), payload.Weekdays...),
		DepartureLocalTime: payload.DepartureLocalTime,
		DurationMinutes:    payload.DurationMinutes,
		TimeZone:           payload.TimeZone,
		ServiceScope:       payload.ServiceScope,
		EffectiveFrom:      payload.EffectiveFrom,
		EffectiveUntil:     payload.EffectiveUntil,
		Capacities:         capacities,
	}, nil
}

// TransportScheduleRegistrationPayload 是具体班次登记载荷的线格式。两种形状二选一：物化只给 template 与
// runningDate；临时班次给 schedule 与正文各格。混着给由编排答`未受理`。时刻按 RFC 3339 解，给了却解不出是坏
// 报文（400）——与「没给」的续办不同。
type TransportScheduleRegistrationPayload struct {
	Template     string `json:"template,omitempty"`
	RunningDate  string `json:"runningDate,omitempty"`
	Schedule     string `json:"schedule,omitempty"`
	Connection   string `json:"connection,omitempty"`
	FromNode     string `json:"fromNode,omitempty"`
	ToNode       string `json:"toNode,omitempty"`
	DepartsAt    string `json:"departsAt,omitempty"`
	ArrivesAt    string `json:"arrivesAt,omitempty"`
	ServiceScope string `json:"serviceScope,omitempty"`
}

func (payload TransportScheduleRegistrationPayload) Command(tenant domain.TenantID) (application.RegisterTransportScheduleCommand, error) {
	if tenant.String() == "" {
		return application.RegisterTransportScheduleCommand{}, ErrOperatorIdentityMissing
	}
	departsAt, err := parseOptionalInstant("departsAt", payload.DepartsAt)
	if err != nil {
		return application.RegisterTransportScheduleCommand{}, err
	}
	arrivesAt, err := parseOptionalInstant("arrivesAt", payload.ArrivesAt)
	if err != nil {
		return application.RegisterTransportScheduleCommand{}, err
	}
	return application.RegisterTransportScheduleCommand{
		TenantID:     tenant,
		Template:     payload.Template,
		RunningDate:  payload.RunningDate,
		Schedule:     payload.Schedule,
		Connection:   payload.Connection,
		FromNode:     payload.FromNode,
		ToNode:       payload.ToNode,
		DepartsAt:    departsAt,
		ArrivesAt:    arrivesAt,
		ServiceScope: payload.ServiceScope,
	}, nil
}

var (
	_ ScheduleTemplateIntake  = (*OperatorRegistryIntake)(nil)
	_ TransportScheduleIntake = (*OperatorRegistryIntake)(nil)
)

func (intake *OperatorRegistryIntake) IntakeScheduleTemplateRegistration(ctx context.Context, request *http.Request) (application.RegisterScheduleTemplateCommand, error) {
	return registryCommand(ctx, intake, request, ScheduleTemplateRegistrationPayload.Command)
}

func (intake *OperatorRegistryIntake) IntakeTransportScheduleRegistration(ctx context.Context, request *http.Request) (application.RegisterTransportScheduleCommand, error) {
	return registryCommand(ctx, intake, request, TransportScheduleRegistrationPayload.Command)
}

// NewRegisterScheduleTemplateEndpoint 交回周期安排登记的 HTTP 入口。首登与下一版是同一个入口：同代码有没有当前版
// 由编排看册子，登记方只登「这一份周期安排现在是什么样」。
func NewRegisterScheduleTemplateEndpoint(intake ScheduleTemplateIntake, registrar TransportScheduleRegistrar) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.ScheduleRegistrationResult, error, bool) {
		command, err := intake.IntakeScheduleTemplateRegistration(request.Context(), request)
		if err != nil {
			return application.ScheduleRegistrationResult{}, err, false
		}
		result, err := registrar.RegisterTemplate(request.Context(), command)
		return result, err, true
	}, writeScheduleRegistrationOutcome)
}

// NewRegisterTransportScheduleEndpoint 交回具体班次登记的 HTTP 入口：按日期物化一班，或登一个临时班次。
func NewRegisterTransportScheduleEndpoint(intake TransportScheduleIntake, registrar TransportScheduleRegistrar) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.ScheduleRegistrationResult, error, bool) {
		command, err := intake.IntakeTransportScheduleRegistration(request.Context(), request)
		if err != nil {
			return application.ScheduleRegistrationResult{}, err, false
		}
		result, err := registrar.RegisterSchedule(request.Context(), command)
		return result, err, true
	}, writeScheduleRegistrationOutcome)
}

// scheduleRegistrationResponse 是两口共用的封闭响应形状：带周期安排的答案透出 template 一组，带班次的答案透出
// schedule 一组，按周期安排物化登记的班次再透出 pools——逐维一个池，各带自己的答案；`未决`只带原因与续办引用。
type scheduleRegistrationResponse struct {
	Outcome               string                 `json:"outcome"`
	UndecidedReason       string                 `json:"undecidedReason,omitempty"`
	ContinuationReference string                 `json:"continuationReference,omitempty"`
	Template              *scheduleTemplateBody  `json:"template,omitempty"`
	Schedule              *transportScheduleBody `json:"schedule,omitempty"`
	Pools                 []materializedPoolBody `json:"pools,omitempty"`
}

// materializedPoolBody 是物化登记答案里的一个池：池本身照池登记口的形状，另带这一池自己的答案。
type materializedPoolBody struct {
	Outcome string `json:"outcome"`
	registeredPoolBody
}

func writeScheduleRegistrationOutcome(response http.ResponseWriter, result application.ScheduleRegistrationResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := scheduleRegistrationResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
	}
	if record, present := result.Template(); present {
		template := record.Template
		content := template.Content()
		weekdays := make([]string, 0, len(content.Weekdays))
		for _, weekday := range content.Weekdays {
			weekdays = append(weekdays, domain.ScheduleWeekdayWord(weekday))
		}
		body.Template = &scheduleTemplateBody{
			Template:           template.Code().String(),
			Version:            int(template.Version()),
			Connection:         content.Direction.Connection().String(),
			FromNode:           content.Direction.From().String(),
			ToNode:             content.Direction.To().String(),
			Weekdays:           weekdays,
			DepartureLocalTime: domain.LocalTimeOfDay(content.DepartureLocalMinute),
			DurationMinutes:    int64(content.Duration / time.Minute),
			TimeZone:           content.TimeZone,
			ServiceScope:       content.ServiceScope.String(),
			EffectiveFrom:      content.EffectiveFrom.String(),
			EffectiveUntil:     content.EffectiveUntil.String(),
			Capacities:         capacityDeclarationBodies(content.Capacities),
			RecordedAt:         rfc3339(record.RecordedAt),
		}
	}
	if record, present := result.Schedule(); present {
		schedule := record.Schedule
		scheduleBody := transportScheduleBody{
			ScheduleID:   schedule.Schedule().String(),
			Version:      int(schedule.Version()),
			Connection:   schedule.Direction().Connection().String(),
			FromNode:     schedule.Direction().From().String(),
			ToNode:       schedule.Direction().To().String(),
			DepartsAt:    rfc3339(schedule.DepartsAt()),
			ArrivesAt:    rfc3339(schedule.ArrivesAt()),
			ServiceScope: schedule.ServiceScope().String(),
			Origin:       schedule.Origin().Kind().String(),
			RecordedAt:   rfc3339(record.RecordedAt),
		}
		if code, version, date, materialized := schedule.Origin().Template(); materialized {
			scheduleBody.Template = code.String()
			scheduleBody.TemplateVersion = int(version)
			scheduleBody.RunningDate = date.String()
		}
		body.Schedule = &scheduleBody
	}
	for _, pool := range result.Pools() {
		body.Pools = append(body.Pools, materializedPoolBody{
			Outcome:            pool.Outcome.String(),
			registeredPoolBody: registeredPool(pool.Pool),
		})
	}

	// ADR-0022：状态码只报有没有新落东西。首登与下一版都持久化了新版本用 201，班次`已存在`而新立了一个池也是 201；
	// `已存在`、周期安排不在册或那天不开、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Persisted() {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

func capacityDeclarationBodies(declarations []domain.CapacityDeclaration) []capacityDeclarationBody {
	bodies := make([]capacityDeclarationBody, 0, len(declarations))
	for _, declaration := range declarations {
		bodies = append(bodies, capacityDeclarationBody{
			Dimension: declaration.Dimension.String(),
			Unit:      declaration.Unit.String(),
			Capacity:  quantity(declaration.Capacity),
		})
	}
	return bodies
}
