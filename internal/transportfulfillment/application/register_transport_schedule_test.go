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
	pools     *poolStoreDouble
	handler   *application.RegisterTransportScheduleHandler
}

func newScheduleRegistrationFixture() *scheduleRegistrationFixture {
	fixture := &scheduleRegistrationFixture{
		templates: &templateRegistryDouble{current: map[string]ports.ScheduleTemplateRecord{}},
		schedules: &transportScheduleRegistryDouble{current: map[string]ports.ScheduleRecord{}},
		pools:     newPoolStore(),
	}
	fixture.handler = application.NewRegisterTransportScheduleHandler(application.RegisterTransportScheduleDeps{
		Templates: fixture.templates,
		Schedules: fixture.schedules,
		Pools:     fixture.pools,
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
			application.CapacityPoolRegistryUnavailable,
		} {
			if reason.String() == "" {
				t.Fatalf("原因 %d 没有标签", reason)
			}
			labels[reason.String()] = true
		}
		if len(labels) != 3 || application.ScheduleRegistrationUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("未决原因集合被悄悄放开或折并")
		}
	})
}

func capacitiesOf(declarations ...application.CapacityDeclarationInput) []application.CapacityDeclarationInput {
	return declarations
}

func poolOutcomes(result application.ScheduleRegistrationResult) map[string]application.OpportunityOutcome {
	outcomes := map[string]application.OpportunityOutcome{}
	for _, pool := range result.Pools() {
		outcomes[pool.Pool.Key.Pool.String()] = pool.Outcome
	}
	return outcomes
}

// Covers: ADR-0201 决定三「逐维容量声明随物化建池且幂等」与 `AT-TF-106`——按周期安排物化登记一班时，那一版声明的
// 每一维在那一班上各建一个池（多维分存，AT-TF-030）；同一日期再登，班次与池都答`已存在`、不多落；周期安排换版改了
// 某维容量又加了一维，再登同一日期时班次照样答`已存在`，改了的那一维答`池冲突`且库里那一池不改写，新加的那一维
// 建池——新落了池就算新落（201 的依据）。
func TestMaterializingAScheduleEstablishesTheDeclaredPools(t *testing.T) {
	fixture := newScheduleRegistrationFixture()
	declared := templateCommand(t)
	declared.Capacities = capacitiesOf(
		application.CapacityDeclarationInput{Dimension: "PIECES", Unit: "PCS", Capacity: 800},
		application.CapacityDeclarationInput{Dimension: "WEIGHT", Unit: "KG", Capacity: 5000},
	)
	registerTemplate(t, fixture, declared, application.ScheduleTemplateRegistered)
	materialize := application.RegisterTransportScheduleCommand{
		TenantID: opportunityTenant(t), Template: "SYN-TPL-1", RunningDate: "2026-10-21",
	}

	first := registerSchedule(t, fixture, materialize, application.TransportScheduleRegistered)
	want := map[string]application.OpportunityOutcome{
		"SYN-TPL-1/2026-10-21/WEIGHT": application.PoolEstablished,
		"SYN-TPL-1/2026-10-21/PIECES": application.PoolEstablished,
	}
	if got := poolOutcomes(first); len(got) != len(want) || got["SYN-TPL-1/2026-10-21/WEIGHT"] != want["SYN-TPL-1/2026-10-21/WEIGHT"] ||
		got["SYN-TPL-1/2026-10-21/PIECES"] != want["SYN-TPL-1/2026-10-21/PIECES"] {
		t.Fatalf("首登池 = %v", got)
	}
	if pools := first.Pools(); pools[0].Pool.Pool.Dimension() != domain.CapacityWeight {
		t.Fatalf("池未按维度次序列：%s 在前", pools[0].Pool.Pool.Dimension())
	}
	if !first.Persisted() || len(fixture.pools.records) != 2 {
		t.Fatalf("首登 persisted=%v 池 %d 个", first.Persisted(), len(fixture.pools.records))
	}

	again := registerSchedule(t, fixture, materialize, application.TransportScheduleExisting)
	for pool, outcome := range poolOutcomes(again) {
		if outcome != application.PoolExistingResult {
			t.Fatalf("再登 %s = %s，要已存在", pool, outcome)
		}
	}
	if again.Persisted() || fixture.pools.saves != 2 {
		t.Fatalf("再登不该新落：persisted=%v saves=%d", again.Persisted(), fixture.pools.saves)
	}

	revised := templateCommand(t)
	revised.Capacities = capacitiesOf(
		application.CapacityDeclarationInput{Dimension: "WEIGHT", Unit: "KG", Capacity: 6000},
		application.CapacityDeclarationInput{Dimension: "PIECES", Unit: "PCS", Capacity: 800},
		application.CapacityDeclarationInput{Dimension: "BAG_POSITIONS", Unit: "BAG", Capacity: 40},
	)
	registerTemplate(t, fixture, revised, application.ScheduleTemplateRevised)
	followed := registerSchedule(t, fixture, materialize, application.TransportScheduleExisting)
	got := poolOutcomes(followed)
	if got["SYN-TPL-1/2026-10-21/WEIGHT"] != application.PoolConflict ||
		got["SYN-TPL-1/2026-10-21/PIECES"] != application.PoolExistingResult ||
		got["SYN-TPL-1/2026-10-21/BAG_POSITIONS"] != application.PoolEstablished {
		t.Fatalf("换版后再登池 = %v", got)
	}
	if !followed.Persisted() {
		t.Fatal("新建了袋位池却答没有新落")
	}
	for _, pool := range followed.Pools() {
		if pool.Outcome == application.PoolConflict && pool.Pool.Pool.Capacity() != 5000 {
			t.Fatalf("冲突交回的不是库里那一池：容量 %d", pool.Pool.Pool.Capacity())
		}
	}
	stored := fixture.pools.records[opportunityTenant(t).String()+"|SYN-TPL-1/2026-10-21/WEIGHT"]
	if stored.Pool.Capacity() != 5000 {
		t.Fatalf("周期安排换版改写了已建的池：容量 %d", stored.Pool.Capacity())
	}
}

// Covers: ADR-0201 决定三的受理门与决定五——容量声明文法外（维度不在封闭词里、同一维度两次、硬上限不为正）答未受理
// 零写入；不声明容量的周期安排物化出的班次与临时班次都不带池；池册读不动答未决并指名池册。
func TestMaterializedPoolsStayWithinTheGrammar(t *testing.T) {
	for name, capacities := range map[string][]application.CapacityDeclarationInput{
		"维度不在封闭词里": {{Dimension: "TEU", Unit: "TEU", Capacity: 2}},
		"同一维度两次": {
			{Dimension: "WEIGHT", Unit: "KG", Capacity: 5000},
			{Dimension: "WEIGHT", Unit: "LB", Capacity: 11000},
		},
		"硬上限不为正": {{Dimension: "WEIGHT", Unit: "KG", Capacity: -1}},
		"缺单位":    {{Dimension: "WEIGHT", Capacity: 5000}},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newScheduleRegistrationFixture()
			command := templateCommand(t)
			command.Capacities = capacities
			registerTemplate(t, fixture, command, application.ScheduleRegistrationNotAccepted)
			if len(fixture.templates.current) != 0 {
				t.Fatal("未受理却落了一版")
			}
		})
	}

	t.Run("不声明容量与临时班次都不带池", func(t *testing.T) {
		fixture := newScheduleRegistrationFixture()
		registerTemplate(t, fixture, templateCommand(t), application.ScheduleTemplateRegistered)
		materialized := registerSchedule(t, fixture, application.RegisterTransportScheduleCommand{
			TenantID: opportunityTenant(t), Template: "SYN-TPL-1", RunningDate: "2026-10-21",
		}, application.TransportScheduleRegistered)
		adHoc := registerSchedule(t, fixture, application.RegisterTransportScheduleCommand{
			TenantID: opportunityTenant(t), Schedule: "SYN-ADHOC-1", Connection: "SYN-CONN-1",
			FromNode: "SYN-NODE-A", ToNode: "SYN-NODE-B", DepartsAt: reserveAt, ArrivesAt: reserveAt.Add(time.Hour),
			ServiceScope: "SYN-SCOPE-01",
		}, application.TransportScheduleRegistered)
		if len(materialized.Pools()) != 0 || len(adHoc.Pools()) != 0 || len(fixture.pools.records) != 0 {
			t.Fatalf("不该带池：%d / %d / %d", len(materialized.Pools()), len(adHoc.Pools()), len(fixture.pools.records))
		}
	})

	t.Run("池册读不动", func(t *testing.T) {
		fixture := newScheduleRegistrationFixture()
		command := templateCommand(t)
		command.Capacities = capacitiesOf(application.CapacityDeclarationInput{Dimension: "WEIGHT", Unit: "KG", Capacity: 5000})
		registerTemplate(t, fixture, command, application.ScheduleTemplateRegistered)
		fixture.pools.findErr = errors.New("pool registry down")
		result := registerSchedule(t, fixture, application.RegisterTransportScheduleCommand{
			TenantID: opportunityTenant(t), Template: "SYN-TPL-1", RunningDate: "2026-10-21",
		}, application.ScheduleRegistrationUndecided)
		if result.UndecidedReason() != application.CapacityPoolRegistryUnavailable || result.ContinuationReference() == "" {
			t.Fatalf("未决 = %s %q", result.UndecidedReason(), result.ContinuationReference())
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
