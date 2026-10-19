package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证运输准备（迁移 0028，ADR-0201 决定六至八）：五格结论各落一行、重放答已有结果、依据变了答结果
// 冲突不覆盖、读不动零写入，以及查阅册转写。夹具全为合成值（S 级）：五条连接各登一份 Asia/Shanghai 当地 22:00 出发、
// 历时 600 分钟的周期安排（2026-10-21 那一班 14:00Z 出发、次日 00:00Z 到达），段窗口按各格需要取。

// stubDeclaredDemand 是申报读口的替身：PS 侧由消费侧适配器的用例证，这里只证 TF 的库面与编排。
type stubDeclaredDemand struct {
	spec domain.DeclaredDemandSpec
	err  error
}

func (stub *stubDeclaredDemand) LoadDeclaredDemand(context.Context, domain.TenantID, domain.CarriedObjectReference) (domain.DemandSnapshot, error) {
	if stub.err != nil {
		return domain.DemandSnapshot{}, stub.err
	}
	return domain.DeclaredDemand(stub.spec)
}

type preparationStores struct {
	scheduleRegistryStores
	preparations *adapter.TransportPreparations
	demand       *stubDeclaredDemand
	prepare      *application.PrepareOnPlannedLegsHandler
	opportunity  *application.PrepareTransportOpportunityHandler
}

// newPreparationStores 与 newScheduleRegistryStores 同形，只是把库留在手上：准备结果库要与两本班次册落在同一个库里，
// 而 pgtest.Pool 每调一次就是一个新库。
func newPreparationStores(t *testing.T) preparationStores {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry := scheduleRegistryStores{transactor: db.Transactor()}
	if registry.templates, err = adapter.NewScheduleTemplates(db); err != nil {
		t.Fatalf("构造周期安排册：%v", err)
	}
	if registry.schedules, err = adapter.NewTransportSchedules(db); err != nil {
		t.Fatalf("构造班次册：%v", err)
	}
	if registry.pools, err = adapter.NewCapacityPools(db); err != nil {
		t.Fatalf("构造容量池册：%v", err)
	}
	if registry.candidates, err = adapter.NewScheduleCandidates(db); err != nil {
		t.Fatalf("构造候选来源：%v", err)
	}
	if registry.catalogue, err = adapter.NewReviewCatalogue(db); err != nil {
		t.Fatalf("构造查阅目录：%v", err)
	}
	registry.handler = application.NewRegisterTransportScheduleHandler(application.RegisterTransportScheduleDeps{
		Templates: registry.templates,
		Schedules: registry.schedules,
		Pools:     registry.pools,
		Clock:     fixedScheduleClock{at: time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC)},
	})
	stores := preparationStores{scheduleRegistryStores: registry}
	if stores.preparations, err = adapter.NewTransportPreparations(db); err != nil {
		t.Fatalf("构造准备结果库：%v", err)
	}
	stores.demand = &stubDeclaredDemand{spec: domain.DeclaredDemandSpec{
		Anchor: "baseline", WeightValue: "1.50", WeightUnit: "KG",
		Length: "20", Width: "15", Height: "10", DimensionUnit: "CM",
	}}
	clock := fixedScheduleClock{at: time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC)}
	stores.prepare = application.NewPrepareOnPlannedLegsHandler(application.PrepareOnPlannedLegsDeps{
		Demand:       stores.demand,
		Schedules:    stores.candidates,
		Pools:        stores.pools,
		Preparations: stores.preparations,
		Clock:        clock,
	})
	stores.opportunity = application.NewPrepareTransportOpportunityHandler(application.PrepareTransportOpportunityDeps{
		Pools:     stores.pools,
		Schedules: stores.schedules,
		Clock:     clock,
	})
	return stores
}

func (stores preparationStores) run(t *testing.T, command application.PrepareOnPlannedLegsCommand) application.PrepareOnPlannedLegsResult {
	t.Helper()
	var result application.PrepareOnPlannedLegsResult
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		prepared, err := stores.prepare.Prepare(txCtx, command)
		result = prepared
		return err
	})
	if err != nil {
		t.Fatalf("准备：%v", err)
	}
	return result
}

func preparationLeg(t *testing.T, reference, connection string, start, end time.Time) domain.PreparationLeg {
	t.Helper()
	direction, err := domain.NewScheduleDirection(connection, "SYN-NODE-A", "SYN-NODE-B")
	if err != nil {
		t.Fatalf("方向：%v", err)
	}
	leg, err := domain.NewPreparationLeg(deliveryValue(t, domain.NewPlannedSegmentReference, reference), direction, start, end)
	if err != nil {
		t.Fatalf("段：%v", err)
	}
	return leg
}

// departure 是各周期安排 2026-10-21 那一班的出发时刻（Asia/Shanghai 22:00）。
var departure = time.Date(2026, 10, 21, 14, 0, 0, 0, time.UTC)

// Covers: AT-TF-103、AT-TF-028/128、AT-TF-129、AT-TF-130、AT-TF-131、AT-TF-133——五格各落一行：空册`未配置`；到达晚于窗口
// `无当前运输机会`；没有池`容量待确认`；登记的那一班重量池被预占到只剩 1 KG，`当前机会不可用`；登记的那一班按登记册上的
// 池判够，`运输机会已准备`。再投同一份答已有结果、库里行数不变；客户更正了申报再来答结果冲突，原结果不被覆盖。
func TestPreparationAnswersEveryOutcomeOnTheRegisters(t *testing.T) {
	stores := newPreparationStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	for code, connection := range map[string]string{
		"SYN-TPL-NC": "SYN-CONN-NC", "SYN-TPL-UC": "SYN-CONN-UC", "SYN-TPL-UN": "SYN-CONN-UN", "SYN-TPL-OK": "SYN-CONN-OK",
	} {
		command := syntheticTemplateCommand(tenant, code, connection, "22:00")
		switch code {
		case "SYN-TPL-UN":
			command.Capacities = []application.CapacityDeclarationInput{{Dimension: "WEIGHT", Unit: "KG", Capacity: 2}}
		case "SYN-TPL-OK":
			command.Capacities = []application.CapacityDeclarationInput{
				{Dimension: "WEIGHT", Unit: "T", Capacity: 5},
				{Dimension: "PIECES", Unit: "PCS", Capacity: 100},
			}
		}
		wantScheduleOutcome(t, stores.registerTemplate(t, command), application.ScheduleTemplateRegistered)
	}
	for _, code := range []string{"SYN-TPL-UN", "SYN-TPL-OK"} {
		wantScheduleOutcome(t, stores.registerSchedule(t, application.RegisterTransportScheduleCommand{
			TenantID: tenant, Template: code, RunningDate: "2026-10-21",
		}), application.TransportScheduleRegistered)
	}
	var reserved application.OpportunityResult
	if err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var err error
		reserved, err = stores.opportunity.Reserve(txCtx, application.ReserveCapacityCommand{
			TenantID: tenant, Pool: "SYN-TPL-UN/2026-10-21/WEIGHT", Reservation: "SYN-RES-1", Quantity: 1,
			ValidUntil: departure.Add(48 * time.Hour), At: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC),
		})
		return err
	}); err != nil || reserved.Outcome() != application.CapacityReserved {
		t.Fatalf("预占：%s %v", reserved.Outcome(), err)
	}

	wide := departure.Add(12 * time.Hour)
	command := application.PrepareOnPlannedLegsCommand{
		TenantID: tenant,
		Object:   "SYN-PARCEL-01",
		Route:    application.PlannedRouteFormed,
		Legs: []domain.PreparationLeg{
			preparationLeg(t, "SYN-RPV-1#1", "SYN-CONN-EMPTY", departure.Add(-time.Hour), wide),
			preparationLeg(t, "SYN-RPV-1#2", "SYN-CONN-NC", departure.Add(-time.Hour), departure.Add(6*time.Hour)),
			preparationLeg(t, "SYN-RPV-1#3", "SYN-CONN-UC", departure.Add(-time.Hour), wide),
			preparationLeg(t, "SYN-RPV-1#4", "SYN-CONN-UN", departure.Add(-time.Hour), wide),
			preparationLeg(t, "SYN-RPV-1#5", "SYN-CONN-OK", departure.Add(-time.Hour), wide),
		},
	}
	first := stores.run(t, command)
	if first.Outcome() != application.PreparationRunAnswered {
		t.Fatalf("首判 = %s（%s）", first.Outcome(), first.UndecidedReason())
	}
	want := []domain.PreparationOutcome{
		domain.PreparationNotConfigured,
		domain.PreparationNoCurrentOpportunity,
		domain.PreparationCapacityUnconfirmed,
		domain.PreparationOpportunityUnavailable,
		domain.PreparationOpportunityPrepared,
	}
	for index, leg := range first.Legs() {
		if leg.Answer != application.LegPreparationFormed || leg.Record.Preparation.Outcome() != want[index] {
			t.Fatalf("段 %d = %s / %s, want 新形成 / %s", index+1, leg.Answer, leg.Record.Preparation.Outcome(), want[index])
		}
	}

	key := func(leg string) ports.TransportPreparationKey {
		return ports.TransportPreparationKey{
			TenantID: tenant,
			Leg:      deliveryValue(t, domain.NewPlannedSegmentReference, leg),
			Object:   deliveryValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-01"),
			Rule:     domain.BuiltInPreparationRuleVersion(),
		}
	}
	prepared, found, err := stores.preparations.FindByKey(t.Context(), key("SYN-RPV-1#5"))
	if err != nil || !found {
		t.Fatalf("读回已准备那一份：found=%v err=%v", found, err)
	}
	if selected, _ := prepared.Preparation.Selected(); selected.String() != "SYN-TPL-OK/2026-10-21" {
		t.Fatalf("选中 %s", selected)
	}
	candidate := prepared.Preparation.Candidates()[0]
	if !candidate.Registered() || len(candidate.Pools) != 2 || candidate.Pools[0].Requested != "0.0015" {
		t.Fatalf("已准备的候选依据 = %+v, want 已登记、两池、重量按吨写 0.0015", candidate)
	}
	if anchor, _ := prepared.Preparation.Demand().Anchor(); anchor != "baseline" {
		t.Fatalf("需求量锚 = %q", anchor)
	}
	unavailable, _, err := stores.preparations.FindByKey(t.Context(), key("SYN-RPV-1#4"))
	if err != nil {
		t.Fatalf("读回不可用那一份：%v", err)
	}
	if pool := unavailable.Preparation.Candidates()[0].Pools[0]; pool.Available != 1 || pool.Verdict != domain.PoolCapacityInsufficient {
		t.Fatalf("预占后的池依据 = %+v, want 可用 1、不够", pool)
	}
	noOpportunity, _, err := stores.preparations.FindByKey(t.Context(), key("SYN-RPV-1#2"))
	if err != nil {
		t.Fatalf("读回无机会那一份：%v", err)
	}
	if candidates := noOpportunity.Preparation.Candidates(); len(candidates) != 1 ||
		candidates[0].Verdict != domain.CandidateArrivesAfterWindow || candidates[0].Registered() {
		t.Fatalf("无机会的候选依据 = %+v", candidates)
	}

	replay := stores.run(t, command)
	for index, leg := range replay.Legs() {
		if leg.Answer != application.LegPreparationExisting || leg.Record.Preparation.Outcome() != want[index] {
			t.Fatalf("重放段 %d = %s / %s", index+1, leg.Answer, leg.Record.Preparation.Outcome())
		}
	}

	stores.demand.spec = domain.DeclaredDemandSpec{Anchor: "SYN-SDV-2", WeightValue: "1.60", WeightUnit: "KG"}
	conflict := stores.run(t, command)
	for index, leg := range conflict.Legs() {
		if leg.Answer != application.LegPreparationConflict || leg.Record.Preparation.Outcome() != want[index] {
			t.Fatalf("冲突段 %d = %s / %s", index+1, leg.Answer, leg.Record.Preparation.Outcome())
		}
	}
	kept, _, err := stores.preparations.FindByKey(t.Context(), key("SYN-RPV-1#5"))
	if err != nil {
		t.Fatalf("冲突后读回：%v", err)
	}
	if anchor, _ := kept.Preparation.Demand().Anchor(); anchor != "baseline" || kept.InputDigest != prepared.InputDigest {
		t.Fatalf("冲突覆盖了原结果：锚 %q", anchor)
	}

	rows, err := stores.catalogue.ListTransportPreparations(t.Context(), tenant, 10)
	if err != nil {
		t.Fatalf("上列准备结果册：%v", err)
	}
	if len(rows) != 5 {
		t.Fatalf("册面行数 = %d, want 5", len(rows))
	}
	for _, row := range rows {
		if row.PlannedLeg == "SYN-RPV-1#5" &&
			(row.Outcome != "OPPORTUNITY_PREPARED" || row.Selected != "SYN-TPL-OK/2026-10-21" || row.DemandAnchor != "baseline" ||
				len(row.Demand) != 3 || len(row.Candidates) != 1 || row.Candidates[0].Version != 1) {
			t.Fatalf("已准备一行转写走样：%+v", row)
		}
		if row.PlannedLeg == "SYN-RPV-1#1" && (row.Outcome != "NOT_CONFIGURED" || row.Selected != "" || len(row.Candidates) != 0) {
			t.Fatalf("未配置一行转写走样：%+v", row)
		}
	}
	if other, err := stores.catalogue.ListTransportPreparations(t.Context(),
		deliveryValue(t, domain.NewTenantID, "SYN-TENANT-02"), 10); err != nil || len(other) != 0 {
		t.Fatalf("他租户看见了：%d %v", len(other), err)
	}
}

// Covers: X6——申报读口读不动答未决、零写入，带续办引用；无路线判断零写入。
func TestPreparationWritesNothingWhenUndecidedOrWithoutARoute(t *testing.T) {
	stores := newPreparationStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	stores.demand.err = errors.New("parcel shipment is down")
	undecided := stores.run(t, application.PrepareOnPlannedLegsCommand{
		TenantID: tenant, Object: "SYN-PARCEL-01", Route: application.PlannedRouteFormed,
		Legs: []domain.PreparationLeg{preparationLeg(t, "SYN-RPV-1#1", "SYN-CONN-EMPTY", departure, departure.Add(time.Hour))},
	})
	if undecided.Outcome() != application.PreparationRunUndecided ||
		undecided.UndecidedReason() != application.DeclaredDemandUnavailable || undecided.ContinuationReference() == "" {
		t.Fatalf("读不动 = %s %s %q", undecided.Outcome(), undecided.UndecidedReason(), undecided.ContinuationReference())
	}
	noRoute := stores.run(t, application.PrepareOnPlannedLegsCommand{
		TenantID: tenant, Object: "SYN-PARCEL-01", Route: application.PlannedRouteNotFormed,
	})
	if noRoute.Outcome() != application.PreparationRunNoRoute {
		t.Fatalf("无路线 = %s", noRoute.Outcome())
	}
	if rows, err := stores.catalogue.ListTransportPreparations(t.Context(), tenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("未决或无路线写了结果：%d %v", len(rows), err)
	}
}

// Covers: PBC-08 写口纪律——准备结果库在无事务上下文里拒写，不落半份结果。
func TestTransportPreparationsRefuseToRunOutsideATransaction(t *testing.T) {
	stores := newPreparationStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	demand, err := domain.DeclaredDemand(domain.DeclaredDemandSpec{})
	if err != nil {
		t.Fatalf("需求量：%v", err)
	}
	leg := preparationLeg(t, "SYN-RPV-1#1", "SYN-CONN-EMPTY", departure, departure.Add(time.Hour))
	object := deliveryValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-01")
	preparation, err := domain.PrepareTransportOpportunity(domain.PrepareTransportOpportunitySpec{
		TenantID: tenant, Leg: leg, Object: object, Rule: domain.BuiltInPreparationRuleVersion(),
		Demand: demand, JudgedAt: departure,
	})
	if err != nil {
		t.Fatalf("准备：%v", err)
	}
	record := ports.TransportPreparationRecord{
		Key:         ports.TransportPreparationKey{TenantID: tenant, Leg: leg.Reference(), Object: object, Rule: domain.BuiltInPreparationRuleVersion()},
		InputDigest: "TFC-1:d",
		Preparation: preparation,
		RecordedAt:  departure,
	}
	if _, err := stores.preparations.Save(t.Context(), record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务落准备结果应返回 ErrTransactionRequired，实得：%v", err)
	}
}
