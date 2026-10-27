package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件对真实 PostgreSQL 16 证 ETA 预测线（租户+包裹+里程碑+来源）：两路来源各是一条线、版本只增不改写、
// 过期写者被乐观条件拒绝且不留版本、同一条线上并发的写者经线锁串行。断言一律在事务闭包外。

func etaOf(t *testing.T, source domain.ETASourceKind, version, inputs string) domain.ETAPrediction {
	t.Helper()
	spec := domain.ETAPredictionSpec{
		Version:     etaGapValue(t, domain.NewETAVersionID, version),
		Parcel:      etaGapValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Milestone:   etaGapValue(t, domain.NewMilestoneReference, "DELIVERED"),
		Source:      source,
		Inputs:      etaGapValue(t, domain.NewPredictionInputsReference, inputs),
		Model:       etaGapValue(t, domain.NewPredictionModelReference, "eta-model/v1"),
		RangeFrom:   etaGapBaseAt.Add(24 * time.Hour),
		RangeTo:     etaGapBaseAt.Add(48 * time.Hour),
		Confidence:  etaGapValue(t, domain.NewConfidenceReference, "MEDIUM/history"),
		PredictedAt: etaGapBaseAt,
	}
	eta, err := domain.FormETAPrediction(spec)
	if err != nil {
		t.Fatalf("构造 ETA：%v", err)
	}
	return eta
}

func refreshedFrom(t *testing.T, prior domain.ETAPrediction, version, inputs string) domain.ETAPrediction {
	t.Helper()
	next, err := prior.Refresh(domain.ETAPredictionSpec{
		Version:     etaGapValue(t, domain.NewETAVersionID, version),
		Parcel:      prior.Parcel(),
		Milestone:   prior.Milestone(),
		Source:      prior.Source(),
		Inputs:      etaGapValue(t, domain.NewPredictionInputsReference, inputs),
		Model:       prior.Model(),
		RangeFrom:   etaGapBaseAt.Add(30 * time.Hour),
		RangeTo:     etaGapBaseAt.Add(50 * time.Hour),
		Confidence:  prior.Confidence(),
		PredictedAt: etaGapBaseAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("刷新：%v", err)
	}
	return next
}

func (fixture *etaGapNotificationFixture) countETARows(t *testing.T, ctx context.Context, table, tenant string) int {
	t.Helper()
	var rows int
	if err := fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM visibility_exception.`+table+` WHERE tenant_id = $1`, tenant).Scan(&rows); err != nil {
		t.Fatalf("数 %s：%v", table, err)
	}
	return rows
}

// Covers: AT-VE-051 与 CONTEXT「承运商提供的预测与运营企业派生的预测分别保留来源和口径」——同一（包裹，里程碑）下两路来源各是
// 一条预测线：各有各的当前版，一次读出两路并存；运营企业一路刷新不触碰承运商那一路（版本行与当前标记原样）；
// 版本只在自己那条线名下读得到。
func TestTheTwoSourcesAreTwoLinesAndOneNeverTouchesTheOther(t *testing.T) {
	fixture := newETAGapNotificationFixture(t)
	ctx := t.Context()
	tenant := etaGapValue(t, domain.NewTenantID, "tenant-a")

	operatorV1 := etaOf(t, domain.OperatorDerivedETA, "eta-op/v1", "inputs/op-1")
	carrierV1 := etaOf(t, domain.CarrierProvidedETA, "eta-carrier/v1", "inputs/carrier-1")
	fixture.saveETA(t, ctx, "tenant-a", operatorV1)
	fixture.saveETA(t, ctx, "tenant-a", carrierV1)
	fixture.saveETA(t, ctx, "tenant-a", refreshedFrom(t, operatorV1, "eta-op/v2", "inputs/op-2"))

	currents, err := fixture.etas.ListCurrent(ctx, tenant, operatorV1.Parcel(), operatorV1.Milestone())
	if err != nil {
		t.Fatalf("读各线当前版：%v", err)
	}
	if len(currents) != 2 ||
		currents[0].Source() != domain.CarrierProvidedETA || currents[0].Version().String() != "eta-carrier/v1" ||
		currents[1].Source() != domain.OperatorDerivedETA || currents[1].Version().String() != "eta-op/v2" {
		t.Fatalf("各线当前版 = %v，want 承运商 eta-carrier/v1 与运营企业 eta-op/v2", currents)
	}
	if _, linked := currents[0].PriorVersion(); linked {
		t.Fatal("承运商那一路被运营企业的刷新指回了")
	}
	if prior, linked := currents[1].PriorVersion(); !linked || prior.String() != "eta-op/v1" {
		t.Fatalf("运营企业那一路指回 = %q，want eta-op/v1", prior)
	}
	if carrier, found := fixture.loadETAFrom(t, ctx, "tenant-a", domain.CarrierProvidedETA); !found ||
		carrier.Inputs().String() != "inputs/carrier-1" || carrier.Model().String() != carrierV1.Model().String() {
		t.Fatalf("承运商线的当前版被改动：%+v", carrier)
	}

	if _, found, _ := fixture.etas.FindVersion(ctx, tenant, operatorV1.Parcel(), operatorV1.Milestone(),
		domain.CarrierProvidedETA, operatorV1.Version()); found {
		t.Fatal("运营企业的版本在承运商线名下也读得到")
	}
	if got := fixture.countETARows(t, ctx, "eta_prediction_version", "tenant-a"); got != 3 {
		t.Fatalf("版本行 = %d，want 3（两路首版与一次刷新）", got)
	}
}

// Covers: ADR-0003 租户隔离——另一个租户同名的包裹与里程碑读不到这个租户的预测线，版本也读不到。
func TestAnotherTenantSeesNeitherTheLinesNorTheVersions(t *testing.T) {
	fixture := newETAGapNotificationFixture(t)
	ctx := t.Context()
	operatorV1 := etaOf(t, domain.OperatorDerivedETA, "eta-op/v1", "inputs/op-1")
	fixture.saveETA(t, ctx, "tenant-a", operatorV1)
	other := etaGapValue(t, domain.NewTenantID, "tenant-b")

	if _, found := fixture.loadETA(t, ctx, "tenant-b"); found {
		t.Fatal("另一个租户读到了当前版")
	}
	if currents, err := fixture.etas.ListCurrent(ctx, other, operatorV1.Parcel(), operatorV1.Milestone()); err != nil || len(currents) != 0 {
		t.Fatalf("另一个租户列出了 %d 条线 err=%v", len(currents), err)
	}
	if _, found, _ := fixture.etas.FindVersion(ctx, other, operatorV1.Parcel(), operatorV1.Milestone(),
		operatorV1.Source(), operatorV1.Version()); found {
		t.Fatal("另一个租户按版本读到了预测")
	}
}

func (fixture *etaGapNotificationFixture) saveETAWithin(ctx context.Context, tenant string, eta domain.ETAPrediction) error {
	return fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.etas.Save(txCtx, mustTenant(tenant), eta)
	})
}

func mustTenant(raw string) domain.TenantID {
	tenant, err := domain.NewTenantID(raw)
	if err != nil {
		panic(err)
	}
	return tenant
}

// Covers: 过期写者被乐观条件拒绝且不留版本——首版撞上线上已有的当前版、刷新所指的前版已不是当前版，都交回
// ports.ErrETACurrentChanged；输家的版本行随事务回滚，不留一版从未成为当前的预测，也不动赢家。
func TestAStaleWriterIsRefusedAndLeavesNoVersionBehind(t *testing.T) {
	fixture := newETAGapNotificationFixture(t)
	ctx := t.Context()
	first := etaOf(t, domain.OperatorDerivedETA, "eta-op/v1", "inputs/op-1")
	fixture.saveETA(t, ctx, "tenant-a", first)

	duplicateFirst := etaOf(t, domain.OperatorDerivedETA, "eta-op/rival-first", "inputs/rival")
	if err := fixture.saveETAWithin(ctx, "tenant-a", duplicateFirst); !errors.Is(err, ports.ErrETACurrentChanged) {
		t.Fatalf("线上已有当前版时再写首版应交回 ErrETACurrentChanged，实得：%v", err)
	}

	winner := refreshedFrom(t, first, "eta-op/v2", "inputs/op-2")
	fixture.saveETA(t, ctx, "tenant-a", winner)
	staleRefresh := refreshedFrom(t, first, "eta-op/stale-v2", "inputs/stale")
	if err := fixture.saveETAWithin(ctx, "tenant-a", staleRefresh); !errors.Is(err, ports.ErrETACurrentChanged) {
		t.Fatalf("前版已被接替时再刷新应交回 ErrETACurrentChanged，实得：%v", err)
	}

	if got := fixture.countETARows(t, ctx, "eta_prediction_version", "tenant-a"); got != 2 {
		t.Fatalf("版本行 = %d，want 2：输家写下的版本行应随事务回滚", got)
	}
	current, found := fixture.loadETA(t, ctx, "tenant-a")
	if !found || current.Version() != winner.Version() {
		t.Fatalf("当前版 = %q，want 赢家 %q", current.Version(), winner.Version())
	}
}

// Covers: 当前标记只能指它所在那条线名下存下来的版本——不存在的版本、别的来源线的版本、别的包裹的版本都被外键拒绝，
// 当前版因此不会指着读不回的行，也不会指着别条线的行。
func TestTheCurrentMarkerCanOnlyNameAStoredVersionOfItsOwnLine(t *testing.T) {
	fixture := newETAGapNotificationFixture(t)
	ctx := t.Context()
	fixture.saveETA(t, ctx, "tenant-a", etaOf(t, domain.OperatorDerivedETA, "eta-op/v1", "inputs/op-1"))

	for name, statement := range map[string]string{
		"不存在的版本": `INSERT INTO visibility_exception.eta_prediction_current
			(tenant_id, parcel_ref, milestone_ref, source, version_id)
			VALUES ('tenant-a', 'parcel-1', 'PICKED_UP', 'OPERATOR_DERIVED', 'eta-missing')`,
		"别的来源线的版本": `INSERT INTO visibility_exception.eta_prediction_current
			(tenant_id, parcel_ref, milestone_ref, source, version_id)
			VALUES ('tenant-a', 'parcel-1', 'DELIVERED', 'CARRIER_PROVIDED', 'eta-op/v1')`,
		"别的包裹的版本": `INSERT INTO visibility_exception.eta_prediction_current
			(tenant_id, parcel_ref, milestone_ref, source, version_id)
			VALUES ('tenant-a', 'parcel-2', 'DELIVERED', 'OPERATOR_DERIVED', 'eta-op/v1')`,
	} {
		_, err := fixture.pool.Exec(ctx, statement)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23503" ||
			pgErr.ConstraintName != "eta_prediction_current_names_stored_version" {
			t.Errorf("%s：应被版本外键拒绝，实得：%v", name, err)
		}
	}
}

// Covers: 版本表的形状 CHECK——来源取值封闭，空白键与倒置区间被库拦下；同租户内版本号唯一（预测版本同时是 Outbox 意图的认领标识）。
func TestTheVersionTableRefusesAnOpenSourceABlankKeyAndADuplicateVersion(t *testing.T) {
	fixture := newETAGapNotificationFixture(t)
	ctx := t.Context()
	fixture.saveETA(t, ctx, "tenant-a", etaOf(t, domain.OperatorDerivedETA, "eta-op/v1", "inputs/op-1"))

	const columns = `(tenant_id, parcel_ref, milestone_ref, source, version_id, inputs_ref, model_ref,
		range_from, range_to, confidence_ref, predicted_at)`
	for name, tc := range map[string]struct{ values, constraint string }{
		"来源不在封闭集合": {`('tenant-a', 'parcel-1', 'DELIVERED', 'GUESSED', 'eta-x', 'i', 'm',
			'2026-08-15T00:00:00Z', '2026-08-16T00:00:00Z', 'c', '2026-08-14T00:00:00Z')`, "eta_prediction_version_source_closed"},
		"空白输入引用": {`('tenant-a', 'parcel-1', 'DELIVERED', 'OPERATOR_DERIVED', 'eta-x', ' ', 'm',
			'2026-08-15T00:00:00Z', '2026-08-16T00:00:00Z', 'c', '2026-08-14T00:00:00Z')`, "eta_prediction_version_not_blank"},
		"倒置区间": {`('tenant-a', 'parcel-1', 'DELIVERED', 'OPERATOR_DERIVED', 'eta-x', 'i', 'm',
			'2026-08-16T00:00:00Z', '2026-08-15T00:00:00Z', 'c', '2026-08-14T00:00:00Z')`, "eta_prediction_version_range_order"},
		"租户内版本号重复": {`('tenant-a', 'parcel-9', 'DELIVERED', 'CARRIER_PROVIDED', 'eta-op/v1', 'i', 'm',
			'2026-08-15T00:00:00Z', '2026-08-16T00:00:00Z', 'c', '2026-08-14T00:00:00Z')`, "eta_prediction_version_pkey"},
	} {
		_, err := fixture.pool.Exec(ctx, `INSERT INTO visibility_exception.eta_prediction_version `+columns+` VALUES `+tc.values)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != tc.constraint {
			t.Errorf("%s：应被 %s 拒绝，实得：%v", name, tc.constraint, err)
		}
	}
}

type concurrentETAIdentities struct{ next atomic.Int64 }

func (identities *concurrentETAIdentities) NextETAVersionID(context.Context) (domain.ETAVersionID, error) {
	return domain.NewETAVersionID(fmt.Sprintf("eta-concurrent-%d", identities.next.Add(1)))
}

type discardETAHandoff struct{}

func (discardETAHandoff) HandOffETA(context.Context, ports.ETAHandoffIntent) error { return nil }

type discardGapHandoff struct{}

func (discardGapHandoff) HandOffVisibilityGap(context.Context, ports.VisibilityGapHandoffIntent) error {
	return nil
}

type fixedETAClock struct{ at time.Time }

func (clock fixedETAClock) Now() time.Time { return clock.at }

// formConcurrently 让 workers 个写者各开一个事务、同时放行去 FormETA，返回各自的结果。
func (fixture *etaGapNotificationFixture) formConcurrently(
	t *testing.T,
	commands []application.FormETACommand,
) []application.FormETAResult {
	t.Helper()
	handler := application.NewFormETAHandler(application.FormETADeps{
		Predictions: fixture.etas,
		Gaps:        fixture.gaps,
		Identities:  &concurrentETAIdentities{},
		ViewChain:   discardETAHandoff{},
		SignalChain: discardGapHandoff{},
		Clock:       fixedETAClock{at: etaGapBaseAt},
	})
	results := make([]application.FormETAResult, len(commands))
	errs := make([]error, len(commands))
	start := make(chan struct{})
	var group sync.WaitGroup
	for index := range commands {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			errs[index] = fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
				result, err := handler.FormETA(txCtx, commands[index])
				results[index] = result
				return err
			})
		}()
	}
	close(start)
	group.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("第 %d 个写者的事务失败：%v", index, err)
		}
	}
	return results
}

func concurrentCommand(t *testing.T, source domain.ETASourceKind, inputs string) application.FormETACommand {
	t.Helper()
	return application.FormETACommand{
		TenantID:   etaGapValue(t, domain.NewTenantID, "tenant-a"),
		Parcel:     etaGapValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Milestone:  etaGapValue(t, domain.NewMilestoneReference, "DELIVERED"),
		Source:     source,
		Inputs:     etaGapValue(t, domain.NewPredictionInputsReference, inputs),
		Model:      etaGapValue(t, domain.NewPredictionModelReference, "eta-model/v1"),
		RangeFrom:  etaGapBaseAt.Add(24 * time.Hour),
		RangeTo:    etaGapBaseAt.Add(48 * time.Hour),
		Confidence: etaGapValue(t, domain.NewConfidenceReference, "MEDIUM/history"),
	}
}

// chainOf 从当前版沿指回走到首版，交回途经的版本号（当前版在前）。
func (fixture *etaGapNotificationFixture) chainOf(t *testing.T, ctx context.Context, source domain.ETASourceKind) []string {
	t.Helper()
	current, found := fixture.loadETAFrom(t, ctx, "tenant-a", source)
	if !found {
		t.Fatalf("%v 线上没有当前版", source)
	}
	tenant := etaGapValue(t, domain.NewTenantID, "tenant-a")
	chain := []string{current.Version().String()}
	for {
		prior, linked := current.PriorVersion()
		if !linked {
			return chain
		}
		previous, found, err := fixture.etas.FindVersion(ctx, tenant, current.Parcel(), current.Milestone(), source, prior)
		if err != nil || !found {
			t.Fatalf("指回的前版 %s 读不回：found=%v err=%v", prior, found, err)
		}
		chain = append(chain, previous.Version().String())
		current = previous
		if len(chain) > 64 {
			t.Fatal("指回成环")
		}
	}
}

// Covers: 同一条预测线上并发的相同命令（同来源、同输入版本）收敛成一个版本——线锁让输家醒来读到赢家已提交的当前版，落重放
// （ETAExistingResult）；没有线锁时输家只会撞上乐观条件，落未决。ADR-0043 的「重放重发同一份」在并发下同样成立。
func TestConcurrentFormsWithTheSameInputsOnOneLineConvergeOnOneVersion(t *testing.T) {
	fixture := newETAGapNotificationFixture(t)
	ctx := t.Context()

	const writers = 8
	commands := make([]application.FormETACommand, writers)
	for index := range commands {
		commands[index] = concurrentCommand(t, domain.OperatorDerivedETA, "inputs/same")
	}
	results := fixture.formConcurrently(t, commands)

	formed := map[string]int{}
	for index, result := range results {
		formed[result.Outcome().String()]++
		if result.Outcome() != application.ETAFormed && result.Outcome() != application.ETAExistingResult {
			t.Fatalf("写者 %d 的 outcome = %q（%q），want FORMED 或 EXISTING_RESULT", index, result.Outcome(), result.UndecidedReason())
		}
	}
	if formed["ETA_FORMED"] != 1 || formed["ETA_EXISTING_RESULT"] != writers-1 {
		t.Fatalf("结果分布 = %v，want 恰一个 FORMED、其余 EXISTING_RESULT", formed)
	}
	if got := fixture.countETARows(t, ctx, "eta_prediction_version", "tenant-a"); got != 1 {
		t.Fatalf("版本行 = %d，want 1", got)
	}
}

// Covers: 同一条预测线上并发的不同输入版本排成一条线性的指回链——没有分叉、没有丢失，每个版本都读得回；
// 两路来源并发时各自成链、互不穿插。
func TestConcurrentFormsWithDifferentInputsMakeOneLinearChainPerLine(t *testing.T) {
	fixture := newETAGapNotificationFixture(t)
	ctx := t.Context()

	const perSource = 6
	var commands []application.FormETACommand
	for index := range perSource {
		commands = append(commands,
			concurrentCommand(t, domain.OperatorDerivedETA, fmt.Sprintf("inputs/op-%d", index)),
			concurrentCommand(t, domain.CarrierProvidedETA, fmt.Sprintf("inputs/carrier-%d", index)))
	}
	for index, result := range fixture.formConcurrently(t, commands) {
		if result.Outcome() != application.ETAFormed && result.Outcome() != application.ETARefreshed {
			t.Fatalf("写者 %d 的 outcome = %q（%q），want FORMED 或 REFRESHED", index, result.Outcome(), result.UndecidedReason())
		}
	}

	for _, source := range []domain.ETASourceKind{domain.OperatorDerivedETA, domain.CarrierProvidedETA} {
		chain := fixture.chainOf(t, ctx, source)
		if len(chain) != perSource {
			t.Fatalf("%v 线的指回链长 = %d（%v），want %d：分叉或丢失", source, len(chain), chain, perSource)
		}
		var rows int
		if err := fixture.pool.QueryRow(ctx,
			`SELECT count(*) FROM visibility_exception.eta_prediction_version WHERE tenant_id = 'tenant-a' AND source = $1`,
			source.String()).Scan(&rows); err != nil || rows != perSource {
			t.Fatalf("%v 线版本行 = %d err=%v，want %d", source, rows, err, perSource)
		}
	}
}
