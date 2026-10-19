package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件对真实 PostgreSQL 证周期安排与具体班次两本册子（迁移 0024，ADR-0201）。夹具全为合成值（S 级）。

type scheduleRegistryStores struct {
	pool       *pgxpool.Pool
	db         *bentopg.DB
	templates  *adapter.ScheduleTemplates
	schedules  *adapter.TransportSchedules
	pools      *adapter.CapacityPools
	candidates *adapter.ScheduleCandidates
	catalogue  *adapter.ReviewCatalogue
	transactor bentoapp.Transactor
	handler    *application.RegisterTransportScheduleHandler
	source     *application.ScheduleCandidatesHandler
}

type fixedScheduleClock struct{ at time.Time }

func (clock fixedScheduleClock) Now() time.Time { return clock.at }

func newScheduleRegistryStores(t *testing.T) scheduleRegistryStores {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	stores := scheduleRegistryStores{pool: pool, db: db, transactor: db.Transactor()}
	if stores.templates, err = adapter.NewScheduleTemplates(db); err != nil {
		t.Fatalf("构造周期安排册：%v", err)
	}
	if stores.schedules, err = adapter.NewTransportSchedules(db); err != nil {
		t.Fatalf("构造班次册：%v", err)
	}
	if stores.pools, err = adapter.NewCapacityPools(db); err != nil {
		t.Fatalf("构造容量池册：%v", err)
	}
	if stores.candidates, err = adapter.NewScheduleCandidates(db); err != nil {
		t.Fatalf("构造候选来源：%v", err)
	}
	if stores.catalogue, err = adapter.NewReviewCatalogue(db); err != nil {
		t.Fatalf("构造查阅目录：%v", err)
	}
	stores.handler = application.NewRegisterTransportScheduleHandler(application.RegisterTransportScheduleDeps{
		Templates: stores.templates,
		Schedules: stores.schedules,
		Pools:     stores.pools,
		Clock:     fixedScheduleClock{at: time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC)},
	})
	stores.source = application.NewScheduleCandidatesHandler(application.ScheduleCandidatesDeps{Source: stores.candidates})
	return stores
}

func syntheticTemplateCommand(tenant domain.TenantID, code, connection, departure string) application.RegisterScheduleTemplateCommand {
	return application.RegisterScheduleTemplateCommand{
		TenantID:           tenant,
		Template:           code,
		Connection:         connection,
		FromNode:           "SYN-NODE-A",
		ToNode:             "SYN-NODE-B",
		Weekdays:           []string{"MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN"},
		DepartureLocalTime: departure,
		DurationMinutes:    600,
		TimeZone:           "Asia/Shanghai",
		ServiceScope:       "SYN-SCOPE-01",
		EffectiveFrom:      "2026-01-01",
	}
}

// registerTemplate 与 registerSchedule 在一笔事务里调编排：写口按框架合同无事务即拒。断言留在闭包外。
func (stores scheduleRegistryStores) registerTemplate(t *testing.T, command application.RegisterScheduleTemplateCommand) application.ScheduleRegistrationResult {
	t.Helper()
	var result application.ScheduleRegistrationResult
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		handled, err := stores.handler.RegisterTemplate(txCtx, command)
		result = handled
		return err
	})
	if err != nil {
		t.Fatalf("登周期安排：%v", err)
	}
	return result
}

func (stores scheduleRegistryStores) registerSchedule(t *testing.T, command application.RegisterTransportScheduleCommand) application.ScheduleRegistrationResult {
	t.Helper()
	var result application.ScheduleRegistrationResult
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		handled, err := stores.handler.RegisterSchedule(txCtx, command)
		result = handled
		return err
	})
	if err != nil {
		t.Fatalf("登班次：%v", err)
	}
	return result
}

func wantScheduleOutcome(t *testing.T, result application.ScheduleRegistrationResult, want application.ScheduleRegistrationOutcome) {
	t.Helper()
	if result.Outcome() != want {
		t.Fatalf("outcome = %s（%s），要 %s", result.Outcome(), result.UndecidedReason(), want)
	}
}

// Covers: ADR-0201 决定四——周期安排同代码同内容答已存在、异内容成下一版，册上两版都在、当前版是后一版；同一版本
// 号再落答版本已占，不覆盖先到者。
func TestScheduleTemplateVersionsAccumulateAndTheLatestIsCurrent(t *testing.T) {
	stores := newScheduleRegistryStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	command := syntheticTemplateCommand(tenant, "SYN-TPL-1", "SYN-CONN-1", "22:00")

	wantScheduleOutcome(t, stores.registerTemplate(t, command), application.ScheduleTemplateRegistered)
	wantScheduleOutcome(t, stores.registerTemplate(t, command), application.ScheduleTemplateExisting)
	later := command
	later.DepartureLocalTime = "23:00"
	revised := stores.registerTemplate(t, later)
	wantScheduleOutcome(t, revised, application.ScheduleTemplateRevised)
	if record, _ := revised.Template(); record.Template.Version() != 2 {
		t.Fatalf("下一版版本 = %d", record.Template.Version())
	}

	key := ports.ScheduleTemplateKey{TenantID: tenant, Code: deliveryValue(t, domain.NewScheduleTemplateCode, "SYN-TPL-1")}
	current, found, err := stores.templates.FindCurrentTemplate(t.Context(), key)
	if err != nil || !found || current.Template.Version() != 2 || current.Template.Content().DepartureLocalMinute != 23*60 {
		t.Fatalf("当前版 = v%d %d，found=%v err=%v", current.Template.Version(), current.Template.Content().DepartureLocalMinute, found, err)
	}

	again := current
	again.ContentDigest = "TFC-1:other"
	var outcome ports.ScheduleTemplateSaveOutcome
	err = stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		saved, err := stores.templates.SaveTemplateVersion(txCtx, again)
		outcome = saved
		return err
	})
	if err != nil || outcome != ports.ScheduleTemplateVersionTaken {
		t.Fatalf("同一版本号再落 = %s，%v", outcome, err)
	}

	rows, err := stores.catalogue.ListScheduleTemplates(t.Context(), tenant, 10)
	if err != nil || len(rows) != 1 || rows[0].Version != 2 || rows[0].DepartureLocalMinute != 23*60 ||
		len(rows[0].Weekdays) != 7 || rows[0].Weekdays[0] != "MON" || rows[0].EffectiveUntil != "" {
		t.Fatalf("周期安排册只列当前版：rows=%+v err=%v", rows, err)
	}
}

// Covers: `AT-TF-027` 与 ADR-0201 决定一、四——（周期安排、日期）在真库上物化确定且幂等：同一日期登两次得同一身份、
// 第二次答已存在不多出一版；周期安排换版后再物化同一日期，身份不变、成下一版；不开的日期与不在册的周期安排各答
// 一格，零写入。
func TestMaterializingATemplateDateOnTheRegistryIsDeterministicAndIdempotent(t *testing.T) {
	stores := newScheduleRegistryStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	template := syntheticTemplateCommand(tenant, "SYN-TPL-1", "SYN-CONN-1", "22:00")
	template.Weekdays = []string{"MON", "WED"}
	wantScheduleOutcome(t, stores.registerTemplate(t, template), application.ScheduleTemplateRegistered)

	materialize := application.RegisterTransportScheduleCommand{TenantID: tenant, Template: "SYN-TPL-1", RunningDate: "2026-10-21"}
	first := stores.registerSchedule(t, materialize)
	wantScheduleOutcome(t, first, application.TransportScheduleRegistered)
	firstRecord, _ := first.Schedule()
	if firstRecord.Schedule.Schedule().String() != "SYN-TPL-1/2026-10-21" ||
		!firstRecord.Schedule.DepartsAt().Equal(time.Date(2026, 10, 21, 14, 0, 0, 0, time.UTC)) {
		t.Fatalf("物化班次 = %s %s", firstRecord.Schedule.Schedule(), firstRecord.Schedule.DepartsAt())
	}
	replay := stores.registerSchedule(t, materialize)
	wantScheduleOutcome(t, replay, application.TransportScheduleExisting)
	replayRecord, _ := replay.Schedule()
	if replayRecord.Schedule.Schedule() != firstRecord.Schedule.Schedule() || replayRecord.Schedule.Version() != 1 ||
		replayRecord.ContentDigest != firstRecord.ContentDigest {
		t.Fatalf("重放读回 = %s v%d", replayRecord.Schedule.Schedule(), replayRecord.Schedule.Version())
	}

	wantScheduleOutcome(t, stores.registerSchedule(t, application.RegisterTransportScheduleCommand{
		TenantID: tenant, Template: "SYN-TPL-1", RunningDate: "2026-10-22",
	}), application.ScheduleTemplateNotRunning)
	wantScheduleOutcome(t, stores.registerSchedule(t, application.RegisterTransportScheduleCommand{
		TenantID: tenant, Template: "SYN-TPL-404", RunningDate: "2026-10-21",
	}), application.ScheduleTemplateNotFound)

	later := template
	later.DepartureLocalTime = "23:00"
	wantScheduleOutcome(t, stores.registerTemplate(t, later), application.ScheduleTemplateRevised)
	revised := stores.registerSchedule(t, materialize)
	wantScheduleOutcome(t, revised, application.TransportScheduleRevised)
	revisedRecord, _ := revised.Schedule()
	if revisedRecord.Schedule.Schedule() != firstRecord.Schedule.Schedule() || revisedRecord.Schedule.Version() != 2 ||
		!revisedRecord.Schedule.DepartsAt().Equal(firstRecord.Schedule.DepartsAt().Add(time.Hour)) {
		t.Fatalf("换版后再物化 = %s v%d %s", revisedRecord.Schedule.Schedule(), revisedRecord.Schedule.Version(), revisedRecord.Schedule.DepartsAt())
	}

	current, found, err := stores.schedules.FindByKey(t.Context(), ports.ScheduleKey{TenantID: tenant, Schedule: firstRecord.Schedule.Schedule()})
	if err != nil || !found || current.Schedule.Version() != 2 {
		t.Fatalf("当前版 = v%d，found=%v err=%v", current.Schedule.Version(), found, err)
	}
	rows, err := stores.catalogue.ListTransportSchedules(t.Context(), tenant, 10)
	if err != nil || len(rows) != 1 || rows[0].Version != 2 || rows[0].Origin != "TEMPLATE" ||
		rows[0].Template != "SYN-TPL-1" || rows[0].TemplateVersion != 2 || rows[0].RunningDate != "2026-10-21" {
		t.Fatalf("班次册只列当前版：rows=%+v err=%v", rows, err)
	}
}

// Covers: ADR-0201 决定五——临时班次同身份异内容成下一版，冒用物化身份与混着两种形状的输入都答未受理。
func TestAdHocSchedulesRegisterAndReviseOnTheRegistry(t *testing.T) {
	stores := newScheduleRegistryStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	departs := time.Date(2026, 10, 21, 2, 0, 0, 0, time.UTC)
	adHoc := application.RegisterTransportScheduleCommand{
		TenantID:     tenant,
		Schedule:     "SYN-ADHOC-1",
		Connection:   "SYN-CONN-1",
		FromNode:     "SYN-NODE-A",
		ToNode:       "SYN-NODE-B",
		DepartsAt:    departs,
		ArrivesAt:    departs.Add(3 * time.Hour),
		ServiceScope: "SYN-SCOPE-01",
	}
	wantScheduleOutcome(t, stores.registerSchedule(t, adHoc), application.TransportScheduleRegistered)
	wantScheduleOutcome(t, stores.registerSchedule(t, adHoc), application.TransportScheduleExisting)
	delayed := adHoc
	delayed.DepartsAt = departs.Add(time.Hour)
	delayed.ArrivesAt = departs.Add(4 * time.Hour)
	wantScheduleOutcome(t, stores.registerSchedule(t, delayed), application.TransportScheduleRevised)

	derivedShape := adHoc
	derivedShape.Schedule = "SYN-TPL-1/2026-10-21"
	wantScheduleOutcome(t, stores.registerSchedule(t, derivedShape), application.ScheduleRegistrationNotAccepted)
	mixed := adHoc
	mixed.Template = "SYN-TPL-1"
	mixed.RunningDate = "2026-10-21"
	wantScheduleOutcome(t, stores.registerSchedule(t, mixed), application.ScheduleRegistrationNotAccepted)
}

// Covers: ADR-0201 决定六与方案 D01-5——候选按连接答：生产空册答`未配置`；登过但这段时间没有班次答`已列出`空集；
// 周期安排在评估时物化、不落册；已登记的那一班顶替同一身份的物化结果，改出了时间段就不再是候选；他租户不可见；
// 「配过」只看当前版，周期安排改登到别的连接，原连接答`未配置`。
func TestScheduleCandidatesAnswerNotConfiguredOnAnEmptyRegistry(t *testing.T) {
	stores := newScheduleRegistryStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	other := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-02")
	from := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	candidates := func(tenant domain.TenantID, connection string, from, to time.Time) application.ScheduleCandidatesResult {
		t.Helper()
		result, err := stores.source.Candidates(t.Context(), application.ScheduleCandidatesQuery{
			TenantID: tenant, Connection: connection, DepartFrom: from, DepartTo: to,
		})
		if err != nil {
			t.Fatalf("问候选：%v", err)
		}
		return result
	}

	if result := candidates(tenant, "SYN-CONN-1", from, to); result.Outcome() != application.ScheduleCandidatesNotConfigured {
		t.Fatalf("空册 = %s，要 NOT_CONFIGURED", result.Outcome())
	}

	wantScheduleOutcome(t, stores.registerTemplate(t, syntheticTemplateCommand(tenant, "SYN-TPL-1", "SYN-CONN-1", "22:00")),
		application.ScheduleTemplateRegistered)
	listed := candidates(tenant, "SYN-CONN-1", from, to)
	var identities []string
	for _, schedule := range listed.Candidates() {
		identities = append(identities, schedule.Schedule().String())
	}
	if listed.Outcome() != application.ScheduleCandidatesListed ||
		strings.Join(identities, ",") != "SYN-TPL-1/2026-10-20,SYN-TPL-1/2026-10-21,SYN-TPL-1/2026-10-22" {
		t.Fatalf("候选 = %s %v", listed.Outcome(), identities)
	}
	if rows, err := stores.catalogue.ListTransportSchedules(t.Context(), tenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("评估候选不应落册：rows=%+v err=%v", rows, err)
	}

	beforeRegistry := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if result := candidates(tenant, "SYN-CONN-1", beforeRegistry, beforeRegistry.Add(48*time.Hour)); result.Outcome() != application.ScheduleCandidatesListed || len(result.Candidates()) != 0 {
		t.Fatalf("登过但区间外 = %s %d，要已列出空集", result.Outcome(), len(result.Candidates()))
	}
	if result := candidates(tenant, "SYN-CONN-2", from, to); result.Outcome() != application.ScheduleCandidatesNotConfigured {
		t.Fatalf("另一条连接 = %s，要 NOT_CONFIGURED", result.Outcome())
	}
	if result := candidates(other, "SYN-CONN-1", from, to); result.Outcome() != application.ScheduleCandidatesNotConfigured {
		t.Fatalf("他租户 = %s，要 NOT_CONFIGURED", result.Outcome())
	}

	// 另一份周期安排 SYN-TPL-2 先按当地 06:00 登、把 10 月 21 日那一班登进册子（出发在 20 日 22:00Z），再换版到
	// 22:00：按新版物化，这一班会落进下面的区间；但册上那一班才是它现在的样子，它在区间外，就不是候选。
	early := syntheticTemplateCommand(tenant, "SYN-TPL-2", "SYN-CONN-1", "06:00")
	wantScheduleOutcome(t, stores.registerTemplate(t, early), application.ScheduleTemplateRegistered)
	wantScheduleOutcome(t, stores.registerSchedule(t, application.RegisterTransportScheduleCommand{
		TenantID: tenant, Template: "SYN-TPL-2", RunningDate: "2026-10-21",
	}), application.TransportScheduleRegistered)
	late := early
	late.DepartureLocalTime = "22:00"
	wantScheduleOutcome(t, stores.registerTemplate(t, late), application.ScheduleTemplateRevised)

	narrowFrom := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	narrowTo := time.Date(2026, 10, 21, 23, 0, 0, 0, time.UTC)
	narrow := candidates(tenant, "SYN-CONN-1", narrowFrom, narrowTo)
	identities = identities[:0]
	for _, schedule := range narrow.Candidates() {
		identities = append(identities, schedule.Schedule().String())
	}
	if strings.Join(identities, ",") != "SYN-TPL-1/2026-10-21" {
		t.Fatalf("窄区间候选 = %v，要只有 SYN-TPL-1/2026-10-21——册上那一班改在区间外，不由物化结果顶回来", identities)
	}

	// `AT-TF-103`「没有任何当前版」：连接上唯一的一份周期安排换版改登到别的连接之后，原连接只剩被顶掉的旧版，
	// 答`未配置`，新连接才算配过。
	moved := syntheticTemplateCommand(tenant, "SYN-TPL-3", "SYN-CONN-3", "22:00")
	wantScheduleOutcome(t, stores.registerTemplate(t, moved), application.ScheduleTemplateRegistered)
	moved.Connection = "SYN-CONN-4"
	wantScheduleOutcome(t, stores.registerTemplate(t, moved), application.ScheduleTemplateRevised)
	if result := candidates(tenant, "SYN-CONN-3", from, to); result.Outcome() != application.ScheduleCandidatesNotConfigured {
		t.Fatalf("周期安排改登走之后的原连接 = %s，要 NOT_CONFIGURED", result.Outcome())
	}
	if result := candidates(tenant, "SYN-CONN-4", from, to); result.Outcome() != application.ScheduleCandidatesListed || len(result.Candidates()) == 0 {
		t.Fatalf("改登到的连接 = %s %d，要已列出且有候选", result.Outcome(), len(result.Candidates()))
	}
}

// Covers: 方案 D01-4「自由串方向退役」——迁移 0024 之后班次表没有自由串方向一列；表里有行时 0024 拒绝施加，不替
// 任何人把自由串猜成连接代码。
func TestMigration0024RetiresTheFreeStringDirection(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()
	var directionColumns int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.columns
		  WHERE table_schema = 'transport_fulfillment' AND table_name = 'transport_schedule' AND column_name = 'direction'`,
	).Scan(&directionColumns); err != nil || directionColumns != 0 {
		t.Fatalf("班次表仍有 direction 列：%d，%v", directionColumns, err)
	}

	assets, err := migrations.TransportFulfillment()
	if err != nil {
		t.Fatalf("读迁移资产：%v", err)
	}
	var retirement string
	for _, asset := range assets {
		if strings.HasPrefix(asset.Name, "0024_") {
			retirement = asset.SQL
		}
	}
	if retirement == "" {
		t.Fatal("找不到迁移 0024")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("开事务：%v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_schedule
			(tenant_id, schedule_id, schedule_version, connection_code, from_node, to_node,
			 departs_at, arrives_at, service_scope_ref, origin, content_digest, recorded_at)
		 VALUES ('SYN-TENANT-01', 'SYN-ADHOC-1', 1, 'SYN-CONN-1', 'A', 'B', now(), now() + interval '1 hour',
		         'SYN-SCOPE-01', 'AD_HOC', 'd', now())`); err != nil {
		t.Fatalf("插探针行：%v", err)
	}
	if _, err := tx.Exec(ctx, retirement); err == nil || !strings.Contains(err.Error(), "已有行") {
		t.Fatalf("表里有行时 0024 应拒绝施加：%v", err)
	}
}

// Covers: 迁移 0024 的库面防线——星期集合封闭、物化来历三格齐备、临时来历三格全空，坏行插不进。
func TestScheduleRegistryChecksRefuseMalformedRows(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()
	probes := map[string]string{
		"星期含认不出的词": `INSERT INTO transport_fulfillment.schedule_template
			(tenant_id, template_code, template_version, connection_code, from_node, to_node, weekdays,
			 departure_local_minute, duration_minutes, time_zone, service_scope_ref, effective_from, content_digest, recorded_at)
		 VALUES ('T', 'TPL', 1, 'C', 'A', 'B', ARRAY['MON','FUNDAY'], 60, 60, 'Asia/Shanghai', 'S', '2026-01-01', 'd', now())`,
		"星期为空": `INSERT INTO transport_fulfillment.schedule_template
			(tenant_id, template_code, template_version, connection_code, from_node, to_node, weekdays,
			 departure_local_minute, duration_minutes, time_zone, service_scope_ref, effective_from, content_digest, recorded_at)
		 VALUES ('T', 'TPL', 1, 'C', 'A', 'B', ARRAY[]::text[], 60, 60, 'Asia/Shanghai', 'S', '2026-01-01', 'd', now())`,
		"物化来历缺运行日期": `INSERT INTO transport_fulfillment.transport_schedule
			(tenant_id, schedule_id, schedule_version, connection_code, from_node, to_node,
			 departs_at, arrives_at, service_scope_ref, origin, template_code, template_version, content_digest, recorded_at)
		 VALUES ('T', 'TPL/2026-01-01', 1, 'C', 'A', 'B', now(), now() + interval '1 hour', 'S', 'TEMPLATE', 'TPL', 1, 'd', now())`,
		"临时来历带周期安排": `INSERT INTO transport_fulfillment.transport_schedule
			(tenant_id, schedule_id, schedule_version, connection_code, from_node, to_node,
			 departs_at, arrives_at, service_scope_ref, origin, template_code, content_digest, recorded_at)
		 VALUES ('T', 'ADHOC', 1, 'C', 'A', 'B', now(), now() + interval '1 hour', 'S', 'AD_HOC', 'TPL', 'd', now())`,
		"到达不晚于出发": `INSERT INTO transport_fulfillment.transport_schedule
			(tenant_id, schedule_id, schedule_version, connection_code, from_node, to_node,
			 departs_at, arrives_at, service_scope_ref, origin, content_digest, recorded_at)
		 VALUES ('T', 'ADHOC', 1, 'C', 'A', 'B', now(), now(), 'S', 'AD_HOC', 'd', now())`,
	}
	for name, probe := range probes {
		if _, err := pool.Exec(ctx, probe); err == nil {
			t.Errorf("%s：坏行溜进了册子", name)
		}
	}
}

// Covers: 写口按框架合同无环境事务即拒（RequireExecutor）——周期安排与班次两本册子都不在事务外落版。
func TestScheduleRegistryWritesRefuseToRunOutsideATransaction(t *testing.T) {
	stores := newScheduleRegistryStores(t)
	ctx := t.Context()
	template, err := domain.RegisterScheduleTemplate(domain.ScheduleTemplateSpec{
		TenantID: deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Code:     deliveryValue(t, domain.NewScheduleTemplateCode, "SYN-TPL-1"),
		Content: domain.ScheduleTemplateContent{
			Direction:            mustScheduleDirection(t),
			Weekdays:             []time.Weekday{time.Monday},
			DepartureLocalMinute: 60,
			Duration:             time.Hour,
			TimeZone:             "Asia/Shanghai",
			ServiceScope:         deliveryValue(t, domain.NewScheduleServiceScopeReference, "SYN-SCOPE-01"),
			EffectiveFrom:        deliveryValue(t, domain.ParseRunningDate, "2026-01-01"),
		},
	})
	if err != nil {
		t.Fatalf("构造周期安排：%v", err)
	}
	if _, err := stores.templates.SaveTemplateVersion(ctx, ports.ScheduleTemplateRecord{
		Key:           ports.ScheduleTemplateKey{TenantID: template.TenantID(), Code: template.Code()},
		ContentDigest: "TFC-1:d",
		Template:      template,
		RecordedAt:    time.Now(),
	}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务落周期安排应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := stores.schedules.Save(ctx, formedSchedule(t, "SYN-TENANT-01", "SYN-ADHOC-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务落班次应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func mustScheduleDirection(t *testing.T) domain.ScheduleDirection {
	t.Helper()
	direction, err := domain.NewScheduleDirection("SYN-CONN-1", "SYN-NODE-A", "SYN-NODE-B")
	if err != nil {
		t.Fatalf("构造方向：%v", err)
	}
	return direction
}
