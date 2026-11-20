package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证迁移 0062：申报行与按不同实测判的行可以并排留着，旧行的实测引用是空串，违反来源与引用配对的写被 CHECK 拒。

func TestMeasuredDemandRowsShareALegWithTheDeclaration(t *testing.T) {
	stores := newPreparationStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	leg := preparationLeg(t, "SYN-RPV-1#1", "SYN-CONN-EMPTY", departure, departure.Add(time.Hour))
	object := deliveryValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-01")
	if err := insertLegacyDeclaration(t, stores, tenant.String(), leg.Reference().String(), object.String()); err != nil {
		t.Fatalf("按迁移前的列插入：%v", err)
	}

	var measurementRef string
	if err := stores.pool.QueryRow(t.Context(),
		`SELECT measurement_ref FROM transport_fulfillment.transport_preparation
		  WHERE tenant_id = $1 AND planned_leg_ref = $2 AND demand_source = 'declaration'`,
		tenant.String(), leg.Reference().String(),
	).Scan(&measurementRef); err != nil || measurementRef != "" {
		t.Fatalf("旧行实测引用 = %q err=%v，want 空串", measurementRef, err)
	}
	declaredKey := ports.TransportPreparationKey{
		TenantID: tenant, Leg: leg.Reference(), Object: object, Rule: domain.BuiltInPreparationRuleVersion(),
		Source: domain.DemandFromDeclaration,
	}
	if _, found, err := stores.preparations.FindByKey(t.Context(), declaredKey); err != nil || !found {
		t.Fatalf("按申报键读回旧行：found=%v err=%v", found, err)
	}

	for _, sourceID := range []string{"SYN-MEASURE-1", "SYN-MEASURE-2"} {
		if err := insertMeasuredPreparation(t, stores, tenant, leg, object, sourceID); err != nil {
			t.Fatalf("实测 %s：%v", sourceID, err)
		}
	}
	for _, sourceID := range []string{"SYN-MEASURE-1", "SYN-MEASURE-2"} {
		key := declaredKey
		key.Source = domain.DemandFromActualMeasurement
		key.MeasurementRef = sourceID
		record, found, err := stores.preparations.FindByKey(t.Context(), key)
		if err != nil || !found {
			t.Fatalf("读回 %s：found=%v err=%v", sourceID, found, err)
		}
		anchor, present := record.Preparation.Demand().Anchor()
		if record.Preparation.Demand().Source() != domain.DemandFromActualMeasurement || !present || anchor != sourceID {
			t.Fatalf("%s 读回来源 %s 锚 %q present=%v", sourceID, record.Preparation.Demand().Source(), anchor, present)
		}
	}
	if _, found, err := stores.preparations.FindByKey(t.Context(), declaredKey); err != nil || !found {
		t.Fatalf("申报行被实测行盖掉了：found=%v err=%v", found, err)
	}

	again := ports.TransportPreparationSaveOutcomeInvalid
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		record := measuredPreparationRecord(t, tenant, leg, object, "SYN-MEASURE-1")
		var saveErr error
		again, saveErr = stores.preparations.Save(txCtx, record)
		return saveErr
	})
	if err != nil || again != ports.TransportPreparationAlreadyRecorded {
		t.Fatalf("同一实测再落 = %s %v，want 已落过", again, err)
	}

	zero := declaredKey
	zero.Source = domain.DemandSourceInvalid
	if _, found, err := stores.preparations.FindByKey(t.Context(), zero); err == nil || found {
		t.Fatalf("零值来源被当成一份结果：found=%v err=%v", found, err)
	}
	var rowsBefore int
	if err := stores.pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.transport_preparation`).Scan(&rowsBefore); err != nil {
		t.Fatal(err)
	}
	err = stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		record := measuredPreparationRecord(t, tenant, leg, object, "SYN-MEASURE-1")
		record.Key.Source = domain.DemandSourceInvalid
		record.Key.MeasurementRef = ""
		_, saveErr := stores.preparations.Save(txCtx, record)
		return saveErr
	})
	if err == nil {
		t.Fatal("零值来源的键被收下了")
	}
	var rowsAfter int
	if err := stores.pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.transport_preparation`).Scan(&rowsAfter); err != nil || rowsAfter != rowsBefore {
		t.Fatalf("零值来源写进了库：之前 %d 之后 %d err=%v", rowsBefore, rowsAfter, err)
	}
}

func TestPreparationMeasurementReferenceIsPairedWithTheDemandSource(t *testing.T) {
	stores := newPreparationStores(t)
	base := preparationInsert{
		tenant: "SYN-TENANT-01", leg: "SYN-RPV-1#1", object: "SYN-PARCEL-01",
		rule: domain.BuiltInPreparationRuleVersion().String(), source: "declaration",
	}
	cases := []struct {
		name       string
		mutate     func(*preparationInsert)
		constraint string
	}{
		{"申报带非空引用", func(row *preparationInsert) { row.measurement = "SYN-MEASURE-1" }, "transport_preparation_measurement_ref_matches_source"},
		{"实测空引用", func(row *preparationInsert) {
			row.source = "actual-measurement"
			row.anchor = nil
		}, "transport_preparation_measurement_ref_matches_source"},
		{"实测锚为 NULL", func(row *preparationInsert) {
			row.source = "actual-measurement"
			row.measurement = "SYN-MEASURE-1"
			row.anchor = nil
		}, "transport_preparation_measurement_ref_matches_source"},
		{"实测锚与引用不等", func(row *preparationInsert) {
			row.source = "actual-measurement"
			row.measurement = "SYN-MEASURE-1"
			anchor := "SYN-MEASURE-2"
			row.anchor = &anchor
		}, "transport_preparation_measurement_ref_matches_source"},
		{"集外来源", func(row *preparationInsert) { row.source = "guessed" }, "transport_preparation_demand_source_closed"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			row := base
			row.leg = "SYN-RPV-" + test.name
			test.mutate(&row)
			err := insertPreparationRow(t, stores, row)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != test.constraint {
				t.Fatalf("err = %v，want 23514 / %s", err, test.constraint)
			}
		})
	}
}

func TestListDeclaredPreparationLegsKeepsToThatObjectAndRule(t *testing.T) {
	stores := newPreparationStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	object := deliveryValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-01")
	otherObject := deliveryValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-02")
	later := preparationLeg(t, "SYN-RPV-1#2", "SYN-CONN-EMPTY", departure, departure.Add(2*time.Hour))
	earlier := preparationLeg(t, "SYN-RPV-1#1", "SYN-CONN-EMPTY", departure, departure.Add(time.Hour))
	other := preparationLeg(t, "SYN-RPV-9#1", "SYN-CONN-EMPTY", departure, departure.Add(time.Hour))
	if err := insertMeasuredPreparation(t, stores, tenant, earlier, object, "SYN-MEASURE-1"); err != nil {
		t.Fatalf("实测行：%v", err)
	}
	for _, item := range []struct {
		leg    domain.PreparationLeg
		object domain.CarriedObjectReference
	}{
		{later, object},
		{earlier, object},
		{other, otherObject},
	} {
		if err := insertDeclaredPreparation(t, stores, tenant, item.leg, item.object); err != nil {
			t.Fatalf("申报行 %s：%v", item.leg.Reference(), err)
		}
	}
	if err := insertPreparationRow(t, stores, preparationInsert{
		tenant: tenant.String(), leg: earlier.Reference().String(), object: object.String(),
		rule: "TF-PREPARE/9", source: "declaration",
	}); err != nil {
		t.Fatalf("另一规则版本：%v", err)
	}

	legs, err := stores.preparations.ListDeclaredPreparationLegs(t.Context(), tenant, object, domain.BuiltInPreparationRuleVersion())
	if err != nil {
		t.Fatalf("列申报段：%v", err)
	}
	if len(legs) != 2 || legs[0].Reference().String() != earlier.Reference().String() || legs[1].Reference().String() != later.Reference().String() {
		got := make([]string, 0, len(legs))
		for _, leg := range legs {
			got = append(got, leg.Reference().String())
		}
		t.Fatalf("申报段 = %v，want 本对象本规则的两段且按段引用升序", got)
	}
	if legs[0].WindowEnd().Before(legs[0].WindowStart()) || legs[0].Direction().Connection().String() != "SYN-CONN-EMPTY" {
		t.Fatalf("读回的段不成一段：%s %s", legs[0].Direction().Connection(), legs[0].WindowEnd())
	}
}

// Covers: 键上的来源必须与快照的来源一致，实测键上的引用必须与快照的锚一致。对不上时适配器直接拒，一行都不落。
func TestSaveRejectsAKeyThatDisagreesWithTheDemandSnapshot(t *testing.T) {
	stores := newPreparationStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	object := deliveryValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-01")
	leg := func(reference string) domain.PreparationLeg {
		t.Helper()
		return preparationLeg(t, reference, "SYN-CONN-EMPTY", departure, departure.Add(time.Hour))
	}

	keyDeclared := measuredPreparationRecord(t, tenant, leg("SYN-RPV-1#1"), object, "SYN-MEASURE-1")
	keyDeclared.Key.Source = domain.DemandFromDeclaration
	keyDeclared.Key.MeasurementRef = ""

	keyMeasured := declaredPreparationRecord(t, tenant, leg("SYN-RPV-1#2"), object)
	// 引用写成与锚同一串，这一格就只剩来源对不上。锚不等由下一格钉；库上的 CHECK 也看锚，不能拿来顶来源这一道。
	anchor, present := keyMeasured.Preparation.Demand().Anchor()
	if !present {
		t.Fatal("申报快照没有锚，引用没法与锚写成同一串")
	}
	keyMeasured.Key.Source = domain.DemandFromActualMeasurement
	keyMeasured.Key.MeasurementRef = anchor

	anchorDisagrees := measuredPreparationRecord(t, tenant, leg("SYN-RPV-1#3"), object, "SYN-MEASURE-1")
	anchorDisagrees.Key.MeasurementRef = "SYN-MEASURE-2"

	for _, test := range []struct {
		name   string
		record ports.TransportPreparationRecord
	}{
		{"键是申报快照是实测", keyDeclared},
		{"键是实测快照是申报", keyMeasured},
		{"实测引用与锚不等", anchorDisagrees},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertAdapterRefusesPreparationSave(t, stores, test.record)
		})
	}
}

// Covers: 申报键带了实测引用时，Save 与 FindByKey 都在适配器这一道拒。FindByKey 碰不到库约束，Save 的错误也不是约束违反。
func TestADeclarationKeyCarryingAMeasurementReferenceIsRefusedByTheAdapter(t *testing.T) {
	stores := newPreparationStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	object := deliveryValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-01")
	record := declaredPreparationRecord(t, tenant,
		preparationLeg(t, "SYN-RPV-1#1", "SYN-CONN-EMPTY", departure, departure.Add(time.Hour)), object)
	record.Key.MeasurementRef = "SYN-MEASURE-1"

	t.Run("FindByKey", func(t *testing.T) {
		_, found, err := stores.preparations.FindByKey(t.Context(), record.Key)
		if err == nil {
			t.Fatalf("found=%v，适配器应直接拒；没找到是库上没这一行，不是这一道", found)
		}
	})
	t.Run("Save", func(t *testing.T) {
		assertAdapterRefusesPreparationSave(t, stores, record)
	})
}

func countPreparationRows(t *testing.T, stores preparationStores) int {
	t.Helper()
	var count int
	if err := stores.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.transport_preparation`).Scan(&count); err != nil {
		t.Fatalf("数行：%v", err)
	}
	return count
}

func assertAdapterRefusesPreparationSave(t *testing.T, stores preparationStores, record ports.TransportPreparationRecord) {
	t.Helper()
	before := countPreparationRows(t, stores)
	var saveErr error
	_ = stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, saveErr = stores.preparations.Save(txCtx, record)
		return saveErr
	})
	var pgErr *pgconn.PgError
	if saveErr == nil || (errors.As(saveErr, &pgErr) && pgErr.Code == "23514") {
		constraint := ""
		if pgErr != nil {
			constraint = pgErr.ConstraintName
		}
		t.Fatalf("Save = %v 约束 %s，适配器应拒且不是约束违反", saveErr, constraint)
	}
	if after := countPreparationRows(t, stores); after != before {
		t.Fatalf("落了 %d 行", after-before)
	}
}

type preparationInsert struct {
	tenant, leg, object, rule, source, measurement string
	anchor                                         *string
}

// insertLegacyDeclaration 不写 measurement_ref，让迁移加上的默认空串自己填上。
func insertLegacyDeclaration(t *testing.T, stores preparationStores, tenant, leg, object string) error {
	t.Helper()
	_, err := stores.pool.Exec(t.Context(),
		`INSERT INTO transport_fulfillment.transport_preparation
			(tenant_id, planned_leg_ref, object_ref, rule_version,
			 connection_code, from_node, to_node, window_start, window_end,
			 demand_source, demand, outcome, candidates, input_digest, judged_at, recorded_at)
		 VALUES ($1, $2, $3, $4, 'SYN-CONN-EMPTY', 'SYN-NODE-A', 'SYN-NODE-B', $5, $6,
		         'declaration', '[{"dimension":"PIECES","amount":"1","unit":"PCS"}]'::jsonb,
		         'NOT_CONFIGURED', '[]'::jsonb, 'TFC-1:pinned', $5, $5)`,
		tenant, leg, object, domain.BuiltInPreparationRuleVersion().String(), departure, departure.Add(time.Hour),
	)
	return err
}

func insertPreparationRow(t *testing.T, stores preparationStores, row preparationInsert) error {
	t.Helper()
	_, err := stores.pool.Exec(t.Context(),
		`INSERT INTO transport_fulfillment.transport_preparation
			(tenant_id, planned_leg_ref, object_ref, rule_version,
			 connection_code, from_node, to_node, window_start, window_end,
			 demand_source, demand_anchor, measurement_ref, demand, outcome, candidates,
			 input_digest, judged_at, recorded_at)
		 VALUES ($1, $2, $3, $4, 'SYN-CONN-EMPTY', 'SYN-NODE-A', 'SYN-NODE-B', $5, $6,
		         $7, $8, $9, '[{"dimension":"PIECES","amount":"1","unit":"PCS"}]'::jsonb,
		         'NOT_CONFIGURED', '[]'::jsonb, 'TFC-1:pinned', $5, $5)`,
		row.tenant, row.leg, row.object, row.rule, departure, departure.Add(time.Hour),
		row.source, row.anchor, row.measurement,
	)
	return err
}

func insertDeclaredPreparation(t *testing.T, stores preparationStores, tenant domain.TenantID, leg domain.PreparationLeg, object domain.CarriedObjectReference) error {
	t.Helper()
	record := declaredPreparationRecord(t, tenant, leg, object)
	return stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, err := stores.preparations.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.TransportPreparationSaved {
			return errors.New(outcome.String())
		}
		return nil
	})
}

func insertMeasuredPreparation(t *testing.T, stores preparationStores, tenant domain.TenantID, leg domain.PreparationLeg, object domain.CarriedObjectReference, sourceID string) error {
	t.Helper()
	record := measuredPreparationRecord(t, tenant, leg, object, sourceID)
	return stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, err := stores.preparations.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.TransportPreparationSaved {
			return errors.New(outcome.String())
		}
		return nil
	})
}

func declaredPreparationRecord(t *testing.T, tenant domain.TenantID, leg domain.PreparationLeg, object domain.CarriedObjectReference) ports.TransportPreparationRecord {
	t.Helper()
	demand, err := domain.DeclaredDemand(domain.DeclaredDemandSpec{Anchor: "baseline", WeightValue: "1", WeightUnit: "KG"})
	if err != nil {
		t.Fatal(err)
	}
	return preparationRecord(t, tenant, leg, object, demand, "")
}

func measuredPreparationRecord(t *testing.T, tenant domain.TenantID, leg domain.PreparationLeg, object domain.CarriedObjectReference, sourceID string) ports.TransportPreparationRecord {
	t.Helper()
	demand, err := domain.MeasuredDemand(domain.MeasuredDemandSpec{
		Anchor: sourceID, WeightValue: "2", WeightUnit: "KG",
		Length: "20", Width: "15", Height: "10", DimensionUnit: "CM",
	})
	if err != nil {
		t.Fatal(err)
	}
	return preparationRecord(t, tenant, leg, object, demand, sourceID)
}

// stagedMeasuredDemand 第一次被问时答没有实测，并停在这一问里：调用方事务已经写下申报行、却还没提交。
// 主协程收到 readNone 后把后续答复换成 M1，再放行。停在答完之后、返回之前，申报行对别的事务仍不可见。
type stagedMeasuredDemand struct {
	mu       sync.Mutex
	m1       ports.MeasuredDemandReading
	released bool
	readNone chan struct{}
	release  chan struct{}
}

func (source *stagedMeasuredDemand) LoadMeasuredDemand(context.Context, domain.TenantID, domain.CarriedObjectReference) (ports.MeasuredDemandReading, error) {
	source.mu.Lock()
	released := source.released
	reading := source.m1
	source.mu.Unlock()
	if released {
		return reading, nil
	}
	select {
	case <-source.readNone:
	default:
		close(source.readNone)
	}
	<-source.release
	return ports.MeasuredDemandReading{Answer: ports.MeasuredDemandNone}, nil
}

func (source *stagedMeasuredDemand) publish(reading ports.MeasuredDemandReading) {
	source.mu.Lock()
	source.m1 = reading
	source.released = true
	source.mu.Unlock()
}

type preparationRaceEnd struct {
	outcome string
	err     error
}

// Covers: 初始准备与实测再准备并行时，后提交的那一笔必须看见先提交的申报段与实测。屏障停在初始准备已经写下申报行、
// 实测读口却答了没有的那一问；称重侧把实测换成 M1 之后，再准备若不等锁，会在申报行提交前读到零段并入账
// NO_DECLARATION_PREPARATION，M1 的实测准备行就漏了。
func TestAMeasurementChangeRunningBeforeTheInitialPreparationCommitsStillRecordsTheMeasuredRow(t *testing.T) {
	stores := newPreparationStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	object := "SYN-PARCEL-RACE-01"
	leg := preparationLeg(t, "SYN-RPV-RACE#1", "SYN-CONN-EMPTY", departure, departure.Add(time.Hour))
	measured := measuredReadingForRace(t, "SYN-MEASURE-RACE-1", departure)
	source := &stagedMeasuredDemand{
		readNone: make(chan struct{}),
		release:  make(chan struct{}),
	}
	releaseOnce := sync.Once{}
	release := func() { releaseOnce.Do(func() { close(source.release) }) }
	t.Cleanup(release)

	watcher, err := pgx.ConnectConfig(t.Context(), stores.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("开看锁连接：%v", err)
	}
	t.Cleanup(func() { _ = watcher.Close(context.Background()) })

	handler := application.NewPrepareOnPlannedLegsHandler(application.PrepareOnPlannedLegsDeps{
		Demand: stores.demand, Schedules: stores.candidates, Pools: stores.pools,
		Preparations: stores.preparations, Clock: fixedScheduleClock{at: departure}, Measured: source,
	})
	command := application.PrepareOnPlannedLegsCommand{
		TenantID: tenant, Object: object, Route: application.PlannedRouteFormed,
		Legs: []domain.PreparationLeg{leg},
	}
	reprepareCommand := application.RePrepareOnMeasurementChangeCommand{TenantID: tenant, Object: object}

	initialDone := make(chan preparationRaceEnd, 1)
	go func() {
		var outcome string
		err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
			result, err := handler.Prepare(txCtx, command)
			if err != nil {
				return err
			}
			outcome = result.Outcome().String()
			if result.Outcome() != application.PreparationRunAnswered {
				return fmt.Errorf("初始准备 %s", outcome)
			}
			return nil
		})
		initialDone <- preparationRaceEnd{outcome: outcome, err: err}
	}()

	select {
	case <-source.readNone:
	case <-time.After(8 * time.Second):
		t.Fatal("初始准备没有停在第一次实测读取上")
	}
	source.publish(measured)

	changeDone := make(chan preparationRaceEnd, 1)
	go func() {
		var outcome string
		err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
			result, err := handler.RePrepareOnMeasurementChange(txCtx, reprepareCommand)
			if err != nil {
				return err
			}
			outcome = result.Outcome().String()
			return nil
		})
		changeDone <- preparationRaceEnd{outcome: outcome, err: err}
	}()

	deadline := time.Now().Add(8 * time.Second)
	var change preparationRaceEnd
	changeFinished := false
	sawWaiter := false
	for !changeFinished && !sawWaiter && time.Now().Before(deadline) {
		select {
		case change = <-changeDone:
			changeFinished = true
		default:
		}
		if changeFinished {
			break
		}
		var waiting int
		if err := watcher.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_locks
			  WHERE locktype = 'advisory' AND NOT granted
			    AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`,
		).Scan(&waiting); err != nil {
			t.Fatalf("看咨询锁：%v", err)
		}
		if waiting > 0 {
			sawWaiter = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !changeFinished && !sawWaiter {
		t.Fatal("再准备既没提交，也没等在咨询锁上")
	}
	release()

	initial := waitPreparationRace(t, initialDone)
	if !changeFinished {
		change = waitPreparationRace(t, changeDone)
	}
	if initial.err != nil || change.err != nil {
		t.Fatalf("初始准备 %s err=%v；再准备 %s err=%v；等锁=%v", initial.outcome, initial.err, change.outcome, change.err, sawWaiter)
	}

	rows, err := preparationSourceRows(t, stores)
	if err != nil {
		t.Fatalf("读准备行：%v", err)
	}
	want := map[string]string{
		domain.DemandFromDeclaration.String():       "",
		domain.DemandFromActualMeasurement.String(): "SYN-MEASURE-RACE-1",
	}
	if len(rows) != len(want) {
		t.Fatalf("准备行 = %v，初始准备 %s，再准备 %s，等锁=%v；want 申报行与实测行并排", rows, initial.outcome, change.outcome, sawWaiter)
	}
	for sourceName, measurement := range want {
		if got, ok := rows[sourceName]; !ok || got != measurement {
			t.Fatalf("来源 %s 引用 %q，准备行 = %v，再准备 %s", sourceName, got, rows, change.outcome)
		}
	}
}

func waitPreparationRace(t *testing.T, done <-chan preparationRaceEnd) preparationRaceEnd {
	t.Helper()
	select {
	case end := <-done:
		return end
	case <-time.After(8 * time.Second):
		t.Fatal("事务没有在期限内结束")
		return preparationRaceEnd{}
	}
}

func preparationSourceRows(t *testing.T, stores preparationStores) (map[string]string, error) {
	t.Helper()
	queried, err := stores.pool.Query(t.Context(),
		`SELECT demand_source, measurement_ref FROM transport_fulfillment.transport_preparation ORDER BY demand_source`)
	if err != nil {
		return nil, err
	}
	defer queried.Close()
	rows := map[string]string{}
	for queried.Next() {
		var sourceName, measurement string
		if err := queried.Scan(&sourceName, &measurement); err != nil {
			return nil, err
		}
		rows[sourceName] = measurement
	}
	return rows, queried.Err()
}

func measuredReadingForRace(t *testing.T, anchor string, occurred time.Time) ports.MeasuredDemandReading {
	t.Helper()
	snapshot, err := domain.MeasuredDemand(domain.MeasuredDemandSpec{
		Anchor: anchor, WeightValue: "2.00", WeightUnit: "KG",
		Length: "20", Width: "15", Height: "10", DimensionUnit: "CM",
	})
	if err != nil {
		t.Fatalf("实测快照：%v", err)
	}
	return ports.MeasuredDemandReading{Answer: ports.MeasuredDemandOne, Snapshot: snapshot, OccurredAt: occurred}
}

func preparationRecord(t *testing.T, tenant domain.TenantID, leg domain.PreparationLeg, object domain.CarriedObjectReference, demand domain.DemandSnapshot, measurement string) ports.TransportPreparationRecord {
	t.Helper()
	preparation, err := domain.PrepareTransportOpportunity(domain.PrepareTransportOpportunitySpec{
		TenantID: tenant, Leg: leg, Object: object, Rule: domain.BuiltInPreparationRuleVersion(),
		Demand: demand, JudgedAt: departure,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := domain.CanonicalizePreparationInput(leg, demand)
	if err != nil {
		t.Fatal(err)
	}
	return ports.TransportPreparationRecord{
		Key: ports.TransportPreparationKey{
			TenantID: tenant, Leg: leg.Reference(), Object: object, Rule: domain.BuiltInPreparationRuleVersion(),
			Source: demand.Source(), MeasurementRef: measurement,
		},
		InputDigest: digest,
		Preparation: preparation,
		RecordedAt:  departure,
	}
}
