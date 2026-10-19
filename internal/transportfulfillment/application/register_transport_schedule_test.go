package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 夹具全为合成值（S 级）。替身只记当前版——版本历史是适配器的事，真库用例另证。

type templateRegistryDouble struct {
	current map[string]ports.ScheduleTemplateRecord
	findErr error
	saveErr error
	// raceWith 非空时，下一次落版前先让「另一方」落下这一份同版本号的记录。
	raceWith *ports.ScheduleTemplateRecord
}

func (double *templateRegistryDouble) FindCurrentTemplate(_ context.Context, key ports.ScheduleTemplateKey) (ports.ScheduleTemplateRecord, bool, error) {
	if double.findErr != nil {
		return ports.ScheduleTemplateRecord{}, false, double.findErr
	}
	record, found := double.current[key.TenantID.String()+"|"+key.Code.String()]
	return record, found, nil
}

func (double *templateRegistryDouble) SaveTemplateVersion(_ context.Context, record ports.ScheduleTemplateRecord) (ports.ScheduleTemplateSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.ScheduleTemplateSaveOutcomeInvalid, double.saveErr
	}
	key := record.Key.TenantID.String() + "|" + record.Key.Code.String()
	if double.raceWith != nil {
		double.current[key] = *double.raceWith
		double.raceWith = nil
	}
	if existing, found := double.current[key]; found && existing.Template.Version() >= record.Template.Version() {
		return ports.ScheduleTemplateVersionTaken, nil
	}
	double.current[key] = record
	return ports.ScheduleTemplateSaved, nil
}

type transportScheduleRegistryDouble struct {
	current map[string]ports.ScheduleRecord
	findErr error
}

func (double *transportScheduleRegistryDouble) FindByKey(_ context.Context, key ports.ScheduleKey) (ports.ScheduleRecord, bool, error) {
	if double.findErr != nil {
		return ports.ScheduleRecord{}, false, double.findErr
	}
	record, found := double.current[key.TenantID.String()+"|"+key.Schedule.String()]
	return record, found, nil
}

func (double *transportScheduleRegistryDouble) Save(_ context.Context, record ports.ScheduleRecord) (ports.ScheduleSaveOutcome, error) {
	key := record.Key.TenantID.String() + "|" + record.Key.Schedule.String()
	if existing, found := double.current[key]; found && existing.Schedule.Version() >= record.Schedule.Version() {
		return ports.ScheduleAlreadyEstablished, nil
	}
	double.current[key] = record
	return ports.ScheduleSaved, nil
}

type scheduleRegistrationFixture struct {
	templates *templateRegistryDouble
	schedules *transportScheduleRegistryDouble
	handler   *application.RegisterTransportScheduleHandler
}

func newScheduleRegistrationFixture() *scheduleRegistrationFixture {
	fixture := &scheduleRegistrationFixture{
		templates: &templateRegistryDouble{current: map[string]ports.ScheduleTemplateRecord{}},
		schedules: &transportScheduleRegistryDouble{current: map[string]ports.ScheduleRecord{}},
	}
	fixture.handler = application.NewRegisterTransportScheduleHandler(application.RegisterTransportScheduleDeps{
		Templates: fixture.templates,
		Schedules: fixture.schedules,
		Clock:     opportunityClock{at: reserveAt},
	})
	return fixture
}

func templateCommand(t *testing.T) application.RegisterScheduleTemplateCommand {
	t.Helper()
	return application.RegisterScheduleTemplateCommand{
		TenantID:           opportunityTenant(t),
		Template:           "SYN-TPL-1",
		Connection:         "SYN-CONN-1",
		FromNode:           "SYN-NODE-A",
		ToNode:             "SYN-NODE-B",
		Weekdays:           []string{"MON", "WED", "FRI"},
		DepartureLocalTime: "22:00",
		DurationMinutes:    600,
		TimeZone:           "Asia/Shanghai",
		ServiceScope:       "SYN-SCOPE-01",
		EffectiveFrom:      "2026-01-01",
	}
}

func registerTemplate(t *testing.T, fixture *scheduleRegistrationFixture, command application.RegisterScheduleTemplateCommand, want application.ScheduleRegistrationOutcome) application.ScheduleRegistrationResult {
	t.Helper()
	result, err := fixture.handler.RegisterTemplate(context.Background(), command)
	if err != nil {
		t.Fatalf("登周期安排：%v", err)
	}
	if result.Outcome() != want {
		t.Fatalf("outcome = %s，要 %s", result.Outcome(), want)
	}
	return result
}

func registerSchedule(t *testing.T, fixture *scheduleRegistrationFixture, command application.RegisterTransportScheduleCommand, want application.ScheduleRegistrationOutcome) application.ScheduleRegistrationResult {
	t.Helper()
	result, err := fixture.handler.RegisterSchedule(context.Background(), command)
	if err != nil {
		t.Fatalf("登班次：%v", err)
	}
	if result.Outcome() != want {
		t.Fatalf("outcome = %s，要 %s", result.Outcome(), want)
	}
	return result
}

// Covers: ADR-0201 决定四——同代码同内容答已存在，异内容成下一版；两格都交回那一版；首登与下一版才算新落一版。
func TestTemplateRegistrationRevisesOnDifferentContent(t *testing.T) {
	fixture := newScheduleRegistrationFixture()
	command := templateCommand(t)

	first := registerTemplate(t, fixture, command, application.ScheduleTemplateRegistered)
	if record, present := first.Template(); !present || record.Template.Version() != 1 || !first.Outcome().Persisted() {
		t.Fatalf("首登 = %+v", record)
	}
	replayed := command
	replayed.Weekdays = []string{"FRI", "MON", "WED", "MON"}
	if result := registerTemplate(t, fixture, replayed, application.ScheduleTemplateExisting); result.Outcome().Persisted() {
		t.Fatal("同内容（星期换序带重复）不该新落一版")
	}
	changed := command
	changed.EffectiveUntil = "2027-01-01"
	revised := registerTemplate(t, fixture, changed, application.ScheduleTemplateRevised)
	if record, _ := revised.Template(); record.Template.Version() != 2 {
		t.Fatalf("下一版 = v%d", record.Template.Version())
	}
}

// Covers: ADR-0201 决定三的受理门——文法外的输入都答未受理（200），不落任何一版。
func TestTemplateRegistrationRefusesInputOutsideTheGrammar(t *testing.T) {
	broken := map[string]func(*application.RegisterScheduleTemplateCommand){
		"星期词":    func(command *application.RegisterScheduleTemplateCommand) { command.Weekdays = []string{"MONDAY"} },
		"出发时刻":   func(command *application.RegisterScheduleTemplateCommand) { command.DepartureLocalTime = "25:00" },
		"历时为零":   func(command *application.RegisterScheduleTemplateCommand) { command.DurationMinutes = 0 },
		"历时溢出":   func(command *application.RegisterScheduleTemplateCommand) { command.DurationMinutes = 1 << 62 },
		"时区":     func(command *application.RegisterScheduleTemplateCommand) { command.TimeZone = "Local" },
		"生效起点":   func(command *application.RegisterScheduleTemplateCommand) { command.EffectiveFrom = "2026/01/01" },
		"起讫同一节点": func(command *application.RegisterScheduleTemplateCommand) { command.ToNode = command.FromNode },
		"缺连接":    func(command *application.RegisterScheduleTemplateCommand) { command.Connection = " " },
		"缺服务范围":  func(command *application.RegisterScheduleTemplateCommand) { command.ServiceScope = "" },
		"缺租户":    func(command *application.RegisterScheduleTemplateCommand) { command.TenantID = domain.TenantID{} },
	}
	for name, breakCommand := range broken {
		t.Run(name, func(t *testing.T) {
			fixture := newScheduleRegistrationFixture()
			command := templateCommand(t)
			breakCommand(&command)
			registerTemplate(t, fixture, command, application.ScheduleRegistrationNotAccepted)
			if len(fixture.templates.current) != 0 {
				t.Fatal("未受理却落了一版")
			}
		})
	}
}

// Covers: ADR-0201 决定一、四、五——物化形按周期安排当前版算出班次，再登同一日期答已存在；周期安排换版后再物化成
// 班次下一版；周期安排不在册、那天不开、两种形状混着给各自成格。
func TestScheduleRegistrationMaterializesFromTheCurrentTemplate(t *testing.T) {
	fixture := newScheduleRegistrationFixture()
	tenant := opportunityTenant(t)
	materialize := application.RegisterTransportScheduleCommand{TenantID: tenant, Template: "SYN-TPL-1", RunningDate: "2026-10-21"}

	registerSchedule(t, fixture, materialize, application.ScheduleTemplateNotFound)
	registerTemplate(t, fixture, templateCommand(t), application.ScheduleTemplateRegistered)
	first := registerSchedule(t, fixture, materialize, application.TransportScheduleRegistered)
	record, _ := first.Schedule()
	if record.Schedule.Schedule().String() != "SYN-TPL-1/2026-10-21" || record.Schedule.Version() != 1 {
		t.Fatalf("物化 = %s v%d", record.Schedule.Schedule(), record.Schedule.Version())
	}
	registerSchedule(t, fixture, materialize, application.TransportScheduleExisting)

	revisedTemplate := templateCommand(t)
	revisedTemplate.DurationMinutes = 660
	registerTemplate(t, fixture, revisedTemplate, application.ScheduleTemplateRevised)
	revised := registerSchedule(t, fixture, materialize, application.TransportScheduleRevised)
	if record, _ := revised.Schedule(); record.Schedule.Version() != 2 || !record.Schedule.ArrivesAt().Equal(record.Schedule.DepartsAt().Add(11*time.Hour)) {
		t.Fatalf("下一版 = v%d %s", record.Schedule.Version(), record.Schedule.ArrivesAt())
	}

	// 周期安排换版只改了生效终点，这一天的班次本身不变：按更新的一版再物化答已存在，不多出一版。
	scopeOnly := revisedTemplate
	scopeOnly.EffectiveUntil = "2027-01-01"
	registerTemplate(t, fixture, scopeOnly, application.ScheduleTemplateRevised)
	registerSchedule(t, fixture, materialize, application.TransportScheduleExisting)

	thursday := materialize
	thursday.RunningDate = "2026-10-22"
	registerSchedule(t, fixture, thursday, application.ScheduleTemplateNotRunning)
	badDate := materialize
	badDate.RunningDate = "21/10/2026"
	registerSchedule(t, fixture, badDate, application.ScheduleRegistrationNotAccepted)
	mixed := materialize
	mixed.Connection = "SYN-CONN-1"
	registerSchedule(t, fixture, mixed, application.ScheduleRegistrationNotAccepted)
}

// Covers: ADR-0029 与 ADR-0031——册子读不动答未决并指名哪本；同一版本号被另一方先落：内容相同是并发重放，答已存
// 在；内容不同答未决，重跑时由当前版重新判。
func TestScheduleRegistrationRecoveryDiscipline(t *testing.T) {
	t.Run("周期安排册读不动", func(t *testing.T) {
		fixture := newScheduleRegistrationFixture()
		fixture.templates.findErr = errors.New("registry down")
		result := registerTemplate(t, fixture, templateCommand(t), application.ScheduleRegistrationUndecided)
		if result.UndecidedReason() != application.ScheduleTemplateRegistryUnavailable || result.ContinuationReference() == "" {
			t.Fatalf("未决 = %s %q", result.UndecidedReason(), result.ContinuationReference())
		}
	})
	t.Run("班次册读不动", func(t *testing.T) {
		fixture := newScheduleRegistrationFixture()
		registerTemplate(t, fixture, templateCommand(t), application.ScheduleTemplateRegistered)
		fixture.schedules.findErr = errors.New("registry down")
		result := registerSchedule(t, fixture, application.RegisterTransportScheduleCommand{
			TenantID: opportunityTenant(t), Template: "SYN-TPL-1", RunningDate: "2026-10-21",
		}, application.ScheduleRegistrationUndecided)
		if result.UndecidedReason() != application.TransportScheduleRegistryUnavailable {
			t.Fatalf("未决原因 = %s", result.UndecidedReason())
		}
	})
	t.Run("并发同内容是重放", func(t *testing.T) {
		fixture := newScheduleRegistrationFixture()
		winner := registerTemplate(t, newScheduleRegistrationFixture(), templateCommand(t), application.ScheduleTemplateRegistered)
		record, _ := winner.Template()
		fixture.templates.raceWith = &record
		registerTemplate(t, fixture, templateCommand(t), application.ScheduleTemplateExisting)
	})
	t.Run("并发异内容留给重跑", func(t *testing.T) {
		fixture := newScheduleRegistrationFixture()
		other := templateCommand(t)
		other.DepartureLocalTime = "06:00"
		winner := registerTemplate(t, newScheduleRegistrationFixture(), other, application.ScheduleTemplateRegistered)
		record, _ := winner.Template()
		fixture.templates.raceWith = &record
		registerTemplate(t, fixture, templateCommand(t), application.ScheduleRegistrationUndecided)
	})
	t.Run("未决原因集合封闭", func(t *testing.T) {
		labels := map[string]bool{}
		for _, reason := range []application.ScheduleRegistrationUndecidedReason{
			application.ScheduleTemplateRegistryUnavailable, application.TransportScheduleRegistryUnavailable,
		} {
			if reason.String() == "" {
				t.Fatalf("原因 %d 没有标签", reason)
			}
			labels[reason.String()] = true
		}
		if len(labels) != 2 || application.ScheduleRegistrationUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("未决原因集合被悄悄放开或折并")
		}
	})
}

// candidateSourceDouble 只答「配没配过」与读失败；列表各问由真库用例证。
type candidateSourceDouble struct {
	configured bool
	err        error
}

func (double candidateSourceDouble) ConnectionHasSchedules(context.Context, domain.TenantID, domain.NetworkConnectionReference) (bool, error) {
	return double.configured, double.err
}

func (double candidateSourceDouble) ListCurrentTemplatesOnConnection(context.Context, domain.TenantID, domain.NetworkConnectionReference) ([]ports.ScheduleTemplateRecord, error) {
	return nil, double.err
}

func (double candidateSourceDouble) ListCurrentSchedulesDepartingWithin(context.Context, domain.TenantID, domain.NetworkConnectionReference, time.Time, time.Time) ([]ports.ScheduleRecord, error) {
	return nil, double.err
}

func (double candidateSourceDouble) FindCurrentSchedules(context.Context, domain.TenantID, []domain.ScheduleReference) ([]ports.ScheduleRecord, error) {
	return nil, double.err
}

// Covers: 方案 D01-5、X6——空册答`未配置`（入账不重投），读不动答未决带续办引用，倒置区间答未受理；三格各不相同。
func TestScheduleCandidatesSeparateNotConfiguredFromUndecided(t *testing.T) {
	query := application.ScheduleCandidatesQuery{
		TenantID:   opportunityTenant(t),
		Connection: "SYN-CONN-1",
		DepartFrom: reserveAt,
		DepartTo:   reserveAt.Add(24 * time.Hour),
	}
	ask := func(source candidateSourceDouble, query application.ScheduleCandidatesQuery) application.ScheduleCandidatesResult {
		t.Helper()
		result, err := application.NewScheduleCandidatesHandler(application.ScheduleCandidatesDeps{Source: source}).Candidates(context.Background(), query)
		if err != nil {
			t.Fatalf("问候选：%v", err)
		}
		return result
	}
	if result := ask(candidateSourceDouble{}, query); result.Outcome() != application.ScheduleCandidatesNotConfigured {
		t.Fatalf("空册 = %s", result.Outcome())
	}
	if result := ask(candidateSourceDouble{configured: true}, query); result.Outcome() != application.ScheduleCandidatesListed || len(result.Candidates()) != 0 {
		t.Fatalf("配过但没有班次 = %s %d", result.Outcome(), len(result.Candidates()))
	}
	if result := ask(candidateSourceDouble{err: errors.New("down")}, query); result.Outcome() != application.ScheduleCandidatesUndecided || result.ContinuationReference() == "" {
		t.Fatalf("读不动 = %s %q", result.Outcome(), result.ContinuationReference())
	}
	inverted := query
	inverted.DepartFrom, inverted.DepartTo = query.DepartTo, query.DepartFrom
	if result := ask(candidateSourceDouble{}, inverted); result.Outcome() != application.ScheduleCandidatesNotAccepted {
		t.Fatalf("倒置区间 = %s", result.Outcome())
	}
}
