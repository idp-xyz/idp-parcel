package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证同层先后到达的关务结果在真库上走完整条链（ADR-0226）：CC 接收编排用生产的结果库、提交
// 索引、版本化解释规则读口、辖区回指链与 Outbox 交接判同层后续还是矛盾，信封经生产 wireDispatcher
// 投到 VE，成员投影按演示映射归类。范围指名的申报单元双成员，夹具与 external_result_projection_test.go
// 共用。

const successiveResultJurisdiction = "SYN-JURISDICTION-ER"

type successiveResultsFixture struct {
	*synVerticalFixture
	receive *customsapp.ReceiveExternalResultHandler
	now     time.Time
}

// newSuccessiveResultsFixture 在外部结果投影夹具之上补齐接收编排要读的两样：结果范围所属单元的一版
// 提交（提交索引按版本认归属），以及放行层与过程决定层在该辖区的解释规则。
func newSuccessiveResultsFixture(t *testing.T) *successiveResultsFixture {
	t.Helper()

	fixture := newSYNVerticalFixture(t)
	registerDemoMilestoneMapping(t, fixture)
	saveExternalResultScope(t, fixture)
	now := time.Now().UTC()
	tenant := fixture.identity.TenantID().String()

	// 提交行从库这一层直插：本文件证的是接收编排对提交索引的消费，不是提交编排。is_current 显式给
	// true——迁移 0012 撤掉了列默认。
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO customs_compliance.declaration_submission
			(tenant_id, unit_id, procedure_ref, version_id, content_digest,
			 members, dossier_ref, roles_ref, readiness_basis, authority_ref,
			 is_current, fixed_at, recorded_at)
		 VALUES ($1, $2, 'SYN-PROCEDURE-ER', $3, 'SYN-DIGEST-ER-01',
			 $4::jsonb, 'SYN-DOSSIER/v1', 'SYN-ROLES/v1', 'SYN-READINESS/v1', 'SYN-AUTHORITY/v1',
			 true, $5, $5)`,
		tenant, externalResultUnit, externalResultVersion,
		`["`+externalResultMemberOne+`","`+externalResultMemberTwo+`"]`, now.Add(-time.Hour),
	); err != nil {
		t.Fatalf("植入申报提交：%v", err)
	}

	rules, err := ccpostgres.NewInterpretationRuleRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造解释规则登记口：%v", err)
	}
	ccTenant := mustCC(t, ccdomain.NewTenantID, tenant)
	jurisdiction := mustCC(t, ccdomain.NewRegulatoryJurisdictionReference, successiveResultJurisdiction)
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		for _, layer := range []ccdomain.ResultLayer{ccdomain.ReleaseResultLayer, ccdomain.ProcessDecisionLayer} {
			if _, err := rules.RegisterInterpretationRule(txCtx, ccTenant, layer, jurisdiction,
				mustCC(t, ccdomain.NewInterpretationRuleReference, "SYN-INTERPRET/"+layer.String()+"/v1"),
				now.Add(-48*time.Hour)); err != nil {
				return err
			}
		}
		return nil
	})

	results, err := ccpostgres.NewExternalResults(fixture.db)
	if err != nil {
		t.Fatalf("构造外部结果库：%v", err)
	}
	submissions, err := ccpostgres.NewSubmissionIndex(fixture.db)
	if err != nil {
		t.Fatalf("构造提交索引：%v", err)
	}
	ruleView, err := ccpostgres.NewInterpretationRuleView(fixture.db)
	if err != nil {
		t.Fatalf("构造解释规则读口：%v", err)
	}
	units, err := ccpostgres.NewDeclarationUnits(fixture.db)
	if err != nil {
		t.Fatalf("构造申报单元库：%v", err)
	}
	cases, err := ccpostgres.NewCustomsCases(fixture.db)
	if err != nil {
		t.Fatalf("构造案件库：%v", err)
	}
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := ccpostgres.NewOutboxExternalResultHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造外部结果交接：%v", err)
	}
	return &successiveResultsFixture{
		synVerticalFixture: fixture,
		receive: customsapp.NewReceiveExternalResultHandler(customsapp.ReceiveExternalResultDeps{
			Results:     results,
			Submissions: submissions,
			Rules:       ruleView,
			Units:       units,
			Cases:       cases,
			Downstream:  handoff,
			Clock:       systemClock{},
		}),
		now: now,
	}
}

func (fixture *successiveResultsFixture) command(
	t *testing.T,
	sourceID string,
	layer ccdomain.ResultLayer,
	semantics string,
	kind ccdomain.ReleaseKind,
	occurredAt time.Time,
) customsapp.ReceiveExternalResultCommand {
	t.Helper()
	command := customsapp.ReceiveExternalResultCommand{
		TenantID:       mustCC(t, ccdomain.NewTenantID, fixture.identity.TenantID().String()),
		SourceID:       sourceID,
		Layer:          layer,
		Role:           "SYN-REGULATOR",
		RawSemantics:   semantics,
		ClaimedVersion: externalResultVersion,
		Attempt:        1,
		Scope:          externalResultUnit,
		OccurredAt:     occurredAt,
		ReceivedAt:     fixture.now,
	}
	if layer == ccdomain.ReleaseResultLayer {
		command.Release = &customsapp.ReleaseContent{
			Kind:      kind,
			Authority: mustCC(t, ccdomain.NewRegulatoryAuthorityReference, "SYN-AUTHORITY-ER"),
		}
	}
	return command
}

func (fixture *successiveResultsFixture) releaseCommand(t *testing.T, sourceID string, kind ccdomain.ReleaseKind, occurredAt time.Time) customsapp.ReceiveExternalResultCommand {
	t.Helper()
	return fixture.command(t, sourceID, ccdomain.ReleaseResultLayer, "SYN-RELEASE/"+kind.String(), kind, occurredAt)
}

// receiveAndDispatch 照生产装配把一次接收包进一笔事务（结果行与信封同生同灭），随后打一拍派发。
func (fixture *successiveResultsFixture) receiveAndDispatch(
	t *testing.T,
	command customsapp.ReceiveExternalResultCommand,
) customsapp.ReceiveExternalResultResult {
	t.Helper()
	var result customsapp.ReceiveExternalResultResult
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = fixture.receive.Handle(txCtx, command)
		return err
	})
	if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("派发 %s：%v", command.SourceID, err)
	}
	return result
}

func resultEventID(fixture *successiveResultsFixture, sourceID string) string {
	return string(outboxintent.FingerprintEventID("external-result", fixture.identity.TenantID().String(), sourceID))
}

// memberEntries 按来源标识列出成员当前投影里关务外部结果的各条目。
func (fixture *successiveResultsFixture) memberEntries(t *testing.T, member string) map[string]vedomain.MilestoneClassification {
	t.Helper()
	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	projection, found, err := projections.FindCurrent(t.Context(),
		mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String()),
		mustVE(t, vedomain.NewTrackedParcelReference, member))
	if err != nil || !found {
		t.Fatalf("成员 %s 当前投影：found=%v err=%v", member, found, err)
	}
	entries := map[string]vedomain.MilestoneClassification{}
	prefix := "external-result/" + member + "/"
	for _, entry := range projection.Entries() {
		reference := entry.Fact().Fact().String()
		if len(reference) <= len(prefix) || reference[:len(prefix)] != prefix {
			t.Fatalf("成员 %s 投影里混进了别的引用 %q", member, reference)
		}
		entries[reference[len(prefix):]] = entry
	}
	return entries
}

// listedReleases 是 CC 放行门禁核对与动作准入读的那张放行读口在这个单元上交回的来源标识。
func (fixture *successiveResultsFixture) listedReleases(t *testing.T) map[string]ccdomain.ReleaseKind {
	t.Helper()
	view, err := ccpostgres.NewReleaseOutcomeView(fixture.db)
	if err != nil {
		t.Fatalf("构造放行读口：%v", err)
	}
	readings, err := view.ListReleaseOutcomes(t.Context(),
		mustCC(t, ccdomain.NewTenantID, fixture.identity.TenantID().String()),
		mustCC(t, ccdomain.NewDecisionScopeReference, externalResultUnit))
	if err != nil {
		t.Fatalf("读放行：%v", err)
	}
	listed := map[string]ccdomain.ReleaseKind{}
	for _, reading := range readings {
		listed[reading.Source] = reading.Release.Kind()
	}
	return listed
}

func requireOutcome(t *testing.T, result customsapp.ReceiveExternalResultResult, want customsapp.ExternalResultOutcome) {
	t.Helper()
	if result.Outcome() != want {
		t.Fatalf("outcome = %q, want %q", result.Outcome(), want)
	}
}

// Covers: AT-CC-431、AT-CC-432、AT-VE-186——先部分放行、后全部放行，两份都不记冲突，两个成员都归入
// CUSTOMS_CLEARED、部分放行那条如实未归类，CC 的放行读口两份都交出（门禁读得到全部放行）；全部放行先到、
// 业务时间更早的部分放行后到，终态一模一样，后到的部分放行不把里程碑拉回。
func TestAPartialThenFullReleaseReachesCustomsClearedInEitherArrivalOrder(t *testing.T) {
	for _, arrivedFirst := range []string{"partial", "full"} {
		t.Run(arrivedFirst+" 先到", func(t *testing.T) {
			fixture := newSuccessiveResultsFixture(t)
			partial := fixture.releaseCommand(t, "SYN-RESULT-PARTIAL-01", ccdomain.PartialRelease, fixture.now.Add(-40*time.Minute))
			full := fixture.releaseCommand(t, "SYN-RESULT-FULL-01", ccdomain.FullRelease, fixture.now.Add(-20*time.Minute))
			order := []customsapp.ReceiveExternalResultCommand{partial, full}
			if arrivedFirst == "full" {
				order = []customsapp.ReceiveExternalResultCommand{full, partial}
			}
			for _, command := range order {
				requireOutcome(t, fixture.receiveAndDispatch(t, command), customsapp.ResultRecorded)
				if status := outboxStatus(t, fixture.db, resultEventID(fixture, command.SourceID)); status != "PUBLISHED" {
					t.Fatalf("%s 信封 = %q，失败码 = %q", command.SourceID, status,
						recordedFailureCode(t, fixture.db, resultEventID(fixture, command.SourceID)))
				}
			}

			for _, member := range []string{externalResultMemberOne, externalResultMemberTwo} {
				entries := fixture.memberEntries(t, member)
				if len(entries) != 2 {
					t.Fatalf("成员 %s 条目 = %d, want 2——后到的那份被吞了", member, len(entries))
				}
				milestone, classified := entries[full.SourceID].Milestone()
				if !classified || milestone.String() != "CUSTOMS_CLEARED" {
					t.Fatalf("成员 %s 全部放行归类 = %q（%v），want CUSTOMS_CLEARED", member, milestone, classified)
				}
				if kind := entries[partial.SourceID].Fact().Kind().String(); kind != "external-result-release-partial" {
					t.Fatalf("成员 %s 部分放行 kind = %q", member, kind)
				}
				if milestone, classified := entries[partial.SourceID].Milestone(); classified {
					t.Fatalf("成员 %s 部分放行归进了 %q", member, milestone)
				}
			}
			listed := fixture.listedReleases(t)
			if listed[partial.SourceID] != ccdomain.PartialRelease || listed[full.SourceID] != ccdomain.FullRelease || len(listed) != 2 {
				t.Fatalf("放行读口交回 %v——全部放行没进当前判断，门禁会一直答只有部分放行", listed)
			}
		})
	}
}

// Covers: AT-CC-433——同一层先查验、后补资料，两项过程决定都形成、都投给成员；乱序到达同样两项都在。
func TestSuccessiveProcessDecisionsBothReachTheProjection(t *testing.T) {
	fixture := newSuccessiveResultsFixture(t)
	documents := fixture.command(t, "SYN-RESULT-DOCS-01", ccdomain.ProcessDecisionLayer, "SYN-DOCUMENTS-REQUESTED",
		ccdomain.ReleaseKindInvalid, fixture.now.Add(-20*time.Minute))
	inspection := fixture.command(t, "SYN-RESULT-INSPECT-01", ccdomain.ProcessDecisionLayer, "SYN-INSPECTION-ORDERED",
		ccdomain.ReleaseKindInvalid, fixture.now.Add(-40*time.Minute))
	requireOutcome(t, fixture.receiveAndDispatch(t, documents), customsapp.ResultRecorded)
	requireOutcome(t, fixture.receiveAndDispatch(t, inspection), customsapp.ResultRecorded)

	for _, member := range []string{externalResultMemberOne, externalResultMemberTwo} {
		entries := fixture.memberEntries(t, member)
		for _, source := range []string{documents.SourceID, inspection.SourceID} {
			entry, present := entries[source]
			if !present || entry.Fact().Kind().String() != "external-result-process-decision" {
				t.Fatalf("成员 %s 缺过程决定 %s：%v", member, source, entries)
			}
		}
	}
}

// Covers: AT-CC-434——同一业务发生时刻、同一范围两种放行种类：后到的部分放行记同层冲突，信封照样定稿但
// 不派生；成员只留先到的全部放行，放行读口也只交出它。
func TestTwoKindsAtTheSameInstantStayAConflict(t *testing.T) {
	fixture := newSuccessiveResultsFixture(t)
	at := fixture.now.Add(-20 * time.Minute)
	full := fixture.releaseCommand(t, "SYN-RESULT-FULL-01", ccdomain.FullRelease, at)
	partial := fixture.releaseCommand(t, "SYN-RESULT-PARTIAL-01", ccdomain.PartialRelease, at)
	requireOutcome(t, fixture.receiveAndDispatch(t, full), customsapp.ResultRecorded)
	conflicted := fixture.receiveAndDispatch(t, partial)
	requireOutcome(t, conflicted, customsapp.ResultLayerConflict)
	if status := outboxStatus(t, fixture.db, resultEventID(fixture, partial.SourceID)); status != "PUBLISHED" {
		t.Fatalf("冲突那份的信封 = %q，该入账定稿", status)
	}

	for _, member := range []string{externalResultMemberOne, externalResultMemberTwo} {
		entries := fixture.memberEntries(t, member)
		if _, present := entries[partial.SourceID]; present || len(entries) != 1 {
			t.Fatalf("成员 %s 条目 = %v——同一时刻的另一种结论不该进投影", member, entries)
		}
	}
	if listed := fixture.listedReleases(t); len(listed) != 1 || listed[full.SourceID] != ccdomain.FullRelease {
		t.Fatalf("放行读口交回 %v，want 只有先到的全部放行", listed)
	}
}

// Covers: AT-CC-436——先部分、后全部之后，全部放行按同一来源标识同内容重投：答已有记录，Outbox 仍只一行，
// 再打一拍什么也不发，成员上仍是两条、VE 收件账一行。
func TestReplayingTheFullReleaseFormsNothingNew(t *testing.T) {
	fixture := newSuccessiveResultsFixture(t)
	partial := fixture.releaseCommand(t, "SYN-RESULT-PARTIAL-01", ccdomain.PartialRelease, fixture.now.Add(-40*time.Minute))
	full := fixture.releaseCommand(t, "SYN-RESULT-FULL-01", ccdomain.FullRelease, fixture.now.Add(-20*time.Minute))
	fixture.receiveAndDispatch(t, partial)
	fixture.receiveAndDispatch(t, full)

	requireOutcome(t, fixture.receiveAndDispatch(t, full), customsapp.ResultExistingResult)
	eventID := resultEventID(fixture, full.SourceID)
	if n := fixture.countSQL(t, `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_id = '`+eventID+`'`); n != 1 {
		t.Fatalf("全部放行的信封 = %d 行, want 1", n)
	}
	if n := fixture.countInbox(t, deriveExternalResultConsumerName, eventID); n != 1 {
		t.Fatalf("VE 收件账 = %d, want 1", n)
	}
	for _, member := range []string{externalResultMemberOne, externalResultMemberTwo} {
		if entries := fixture.memberEntries(t, member); len(entries) != 2 {
			t.Fatalf("成员 %s 条目 = %d, want 2", member, len(entries))
		}
	}
}

// Covers: AT-CC-434 的并发半边——同一提交版本上两份相互矛盾的同层结果（同一时刻的全部与部分放行）并发到达，恰有
// 一份记冲突。先到那笔持着提交版本锁落下全部放行、尚未提交时，后到那笔的比对读挂在锁上，不读到缺了对方的在册事实；
// 先到那笔提交之后它读得见全部放行，判冲突。不串行的话，两笔在读已提交下互相读不到，双双记成无冲突。
func TestTwoContradictingResultsArrivingConcurrentlyRecordExactlyOneConflict(t *testing.T) {
	fixture := newSuccessiveResultsFixture(t)
	ctx := t.Context()
	at := fixture.now.Add(-20 * time.Minute)
	full := fixture.releaseCommand(t, "SYN-RESULT-FULL-01", ccdomain.FullRelease, at)
	partial := fixture.releaseCommand(t, "SYN-RESULT-PARTIAL-01", ccdomain.PartialRelease, at)

	observer, err := pgx.Connect(ctx, pgtest.AdminDSN(t))
	if err != nil {
		t.Fatalf("开观察连接：%v", err)
	}
	t.Cleanup(func() { _ = observer.Close(context.Background()) })
	database := fixture.pool.Config().ConnConfig.Database

	type answered struct {
		result customsapp.ReceiveExternalResultResult
		err    error
	}
	held, release := make(chan struct{}), make(chan struct{})
	first := make(chan answered, 1)
	// 放行登记进清理：放行之前任何一处 Fatal，先到那笔会一直占着连接，连接池清理挂到 go test 超时。
	var releaseOnce sync.Once
	releaseFirst := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseFirst)
	go func() {
		var got customsapp.ReceiveExternalResultResult
		err := fixture.transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			var handleErr error
			got, handleErr = fixture.receive.Handle(txCtx, full)
			close(held)
			<-release
			return handleErr
		})
		first <- answered{got, err}
	}()
	<-held

	second := make(chan answered, 1)
	go func() {
		var got customsapp.ReceiveExternalResultResult
		err := fixture.transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			var handleErr error
			got, handleErr = fixture.receive.Handle(txCtx, partial)
			return handleErr
		})
		second <- answered{got, err}
	}()

	for deadline := time.Now().Add(10 * time.Second); ; {
		select {
		case early := <-second:
			t.Fatalf("先到那笔提交之前，后到那笔就答了 %q（%v）——比对读没有串行", early.result.Outcome(), early.err)
		default:
		}
		var waiting int
		if err := observer.QueryRow(ctx,
			`SELECT count(*) FROM pg_stat_activity
			  WHERE datname = $1 AND wait_event_type = 'Lock' AND wait_event = 'advisory'`, database).Scan(&waiting); err != nil {
			t.Fatalf("看锁等待：%v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("后到那笔既没答也没挂在锁上")
		}
		time.Sleep(10 * time.Millisecond)
	}
	releaseFirst()

	answers := []answered{<-first, <-second}
	for _, answer := range answers {
		if answer.err != nil {
			t.Fatalf("并发接收：%v", answer.err)
		}
	}
	requireOutcome(t, answers[0].result, customsapp.ResultRecorded)
	requireOutcome(t, answers[1].result, customsapp.ResultLayerConflict)
	if n := fixture.countSQL(t, `SELECT count(*) FROM customs_compliance.external_result WHERE layer_conflict`); n != 1 {
		t.Fatalf("记冲突的接收 = %d 份, want 恰 1 份", n)
	}
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("派发：%v", err)
	}
	for _, member := range []string{externalResultMemberOne, externalResultMemberTwo} {
		if entries := fixture.memberEntries(t, member); len(entries) != 1 {
			t.Fatalf("成员 %s 条目 = %v，want 只有先到的全部放行", member, entries)
		}
	}
}
