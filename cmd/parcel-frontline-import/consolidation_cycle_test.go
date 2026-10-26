package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// nestingCommand 造一条「把 member 那只袋装进 unit」的移入命令，来源四格齐全、各带各的来源身份。
func nestingCommand(t *testing.T, unit, member, factRef string) application.AddMemberCommand {
	t.Helper()
	source, err := domain.NewWorkFactSource(
		"FTI/CONSOLIDATION-1/SYN-BATCH-RACE/"+factRef,
		mustValue(t, domain.NewPerformingPartyReference, "SYN-PACKER-1"),
		mustValue(t, domain.NewExecutionEvidenceReference, "FTI/CONSOLIDATION-1/SYN-CLERK-A/"+factRef),
		mustInstant(t, "2026-09-08T16:00:00+08:00"),
	)
	if err != nil {
		t.Fatalf("来源表达：%v", err)
	}
	return application.AddMemberCommand{
		TenantID: synTenant(t),
		Unit:     mustValue(t, domain.NewConsolidationUnitID, unit),
		Member:   mustHandlingUnit(t, member),
		Source:   source,
	}
}

func mustValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

// Covers: `AT-NO-039` 的并发半边（UC-NO-003「一致性、幂等与并发」）——两袋互装的两笔事务经生产装配
// （buildConsolidationImporter）同时到达真库，只有一方成立：第二笔在第一笔提交之前就已开始、等在租户成员关系锁上，
// 第一笔提交后它读到的链路里已经有对方，答`包含循环`；提交后容纳索引里只有一条边、来源事实只有一条。没有这把锁，
// 两笔各读到对方还是根、双双答 MEMBER_ADDED，提交后 CU-A 在 CU-B 里、CU-B 也在 CU-A 里。
func TestConcurrentMutualNestingAdmitsOnlyOneSideOnTheDatabase(t *testing.T) {
	importer, units, facts := consolidationVerticalImporter(t)
	ctx := t.Context()
	opened := importConsolidationTemplate(t, importer, []byte(consolidationHeader+
		consolidationLineFor("SYN-WORK-OPEN-A", "OPEN_UNIT", "SYN-CU-A", "SYN-BAG-A", "", "", "")+
		consolidationLineFor("SYN-WORK-OPEN-B", "OPEN_UNIT", "SYN-CU-B", "SYN-BAG-B", "", "", "")))
	assertDisposition(t, opened, 0, rowLanded, "OPENED")
	assertDisposition(t, opened, 1, rowLanded, "OPENED")

	bIntoA := nestingCommand(t, "SYN-CU-A", "SYN-CU-B", "SYN-WORK-B-INTO-A")
	aIntoB := nestingCommand(t, "SYN-CU-B", "SYN-CU-A", "SYN-WORK-A-INTO-B")
	basisBIntoA, err := importer.judge.Judge(ctx, bIntoA)
	if err != nil {
		t.Fatalf("判方向依据：%v", err)
	}
	basisAIntoB, err := importer.judge.Judge(ctx, aIntoB)
	if err != nil {
		t.Fatalf("判方向依据：%v", err)
	}

	firstJudged := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	// 用例半途失败时也要放第一笔走、等两笔退出：悬着的事务会拖住本用例库的收尾（DROP DATABASE 等连接释放）。
	var workers sync.WaitGroup
	t.Cleanup(func() { unblock(); workers.Wait() })

	type outcome struct {
		result application.ConsolidationResult
		err    error
	}
	firstDone := make(chan outcome, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		var result application.ConsolidationResult
		err := importer.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			judged, err := importer.handler.AddMember(txCtx, bIntoA, basisBIntoA)
			if err != nil {
				return err
			}
			result = judged
			close(firstJudged)
			<-release
			return nil
		})
		firstDone <- outcome{result, err}
	}()
	select {
	case <-firstJudged:
	case first := <-firstDone:
		t.Fatalf("第一笔没拿住就退出了：outcome=%q err=%v", first.result.Outcome(), first.err)
	}

	secondDone := make(chan outcome, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		var result application.ConsolidationResult
		err := importer.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			judged, err := importer.handler.AddMember(txCtx, aIntoB, basisAIntoB)
			if err != nil {
				return err
			}
			result = judged
			return nil
		})
		secondDone <- outcome{result, err}
	}()

	select {
	case second := <-secondDone:
		t.Fatalf("第二笔没有等第一笔提交就答了 %q（err=%v）", second.result.Outcome(), second.err)
	case <-time.After(300 * time.Millisecond):
	}
	unblock()
	first := <-firstDone
	if first.err != nil || first.result.Outcome() != application.MemberAdded {
		t.Fatalf("第一笔：outcome=%q err=%v", first.result.Outcome(), first.err)
	}
	var second outcome
	select {
	case second = <-secondDone:
	case <-time.After(5 * time.Second):
		t.Fatal("第一笔提交后第二笔仍没放行")
	}
	if second.err != nil || second.result.Outcome() != application.ContainmentCycle {
		t.Fatalf("第二笔：outcome=%q err=%v；要答包含循环", second.result.Outcome(), second.err)
	}
	if lineage := second.result.Lineage(); len(lineage) != 2 || lineage[0].String() != "SYN-CU-B" || lineage[1].String() != "SYN-CU-A" {
		t.Fatalf("第二笔带回的链路 = %v，要从目标 CU-B 到成员 CU-A", lineage)
	}

	if members := memberIDs(findUnit(t, units, "SYN-CU-A").Members()); members != "SYN-CU-B" {
		t.Fatalf("CU-A 的成员 = %q，要只有 CU-B", members)
	}
	if members := memberIDs(findUnit(t, units, "SYN-CU-B").Members()); members != "" {
		t.Fatalf("CU-B 的成员 = %q，要为空；互装的另一半落地了", members)
	}
	if parent, contained, err := units.CurrentParent(ctx, synTenant(t), mustHandlingUnit(t, "SYN-CU-A")); err != nil || contained {
		t.Fatalf("CU-A 有了父级 %v（contained=%v err=%v）：册上成环", parent, contained, err)
	}
	if _, found := findFact(t, facts, "FTI/CONSOLIDATION-1/SYN-BATCH-RACE/SYN-WORK-B-INTO-A"); !found {
		t.Fatal("成立的那一半没留来源事实")
	}
	if _, found := findFact(t, facts, "FTI/CONSOLIDATION-1/SYN-BATCH-RACE/SYN-WORK-A-INTO-B"); found {
		t.Fatal("被拒的那一半留了来源事实")
	}
}

// raceAssembly 装与 consolidationVerticalImporter 同一条生产集运链，另交回框架 DB 与一条池外的连接：下面两条并发用例
// 要另装一只会停住的编排，还要从库里看「有一笔正等在锁上」，拿它代替睡一段时间。看锁的连接不能从用例池里取——池只有
// 两条，正好被两笔在途事务占着。
func raceAssembly(t *testing.T) (consolidationImporter, *bentopg.DB, *pgx.Conn, *nopostgres.ConsolidationUnits) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	importer, err := buildConsolidationImporter(db, fixedClock{at: mustInstant(t, "2026-09-08T18:00:00+08:00")})
	if err != nil {
		t.Fatalf("装配集运导入口：%v", err)
	}
	units, err := nopostgres.NewConsolidationUnits(db)
	if err != nil {
		t.Fatalf("构造集运单元库读口：%v", err)
	}
	watcher, err := pgx.ConnectConfig(t.Context(), pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("连看锁的连接：%v", err)
	}
	// 先于用例库的收尾关掉：DROP DATABASE 等全部连接释放。
	t.Cleanup(func() { _ = watcher.Close(context.Background()) })
	return importer, db, watcher, units
}

// awaitLockWaiter 轮询本用例库里有没有一条会话正等在锁上（pg_locks 里未授予的那一行），看见即交回 true；finished 先关
// 交回 false——被测的那一笔压根没等就走完了。五秒两样都没有即失败。
func awaitLockWaiter(t *testing.T, watcher *pgx.Conn, finished <-chan struct{}) bool {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		var waiting bool
		if err := watcher.QueryRow(t.Context(),
			`SELECT EXISTS (
			        SELECT 1
			          FROM pg_locks AS lock
			          JOIN pg_stat_activity AS activity ON activity.pid = lock.pid
			         WHERE NOT lock.granted
			           AND activity.datname = current_database())`).Scan(&waiting); err != nil {
			t.Fatalf("查锁等待：%v", err)
		}
		if waiting {
			return true
		}
		select {
		case <-finished:
			return false
		case <-deadline:
			t.Fatal("五秒内既没有会话等锁，被测的那一笔也没走完")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// stallingUnitStore 在第一次读到 stallOn 那只单元之后停住，只读与读来改两问都停：用例借它摆出「一口读完单元、还没
// 写回」的那一刻，让别的事务插进来。读来改那一问停住时，行锁已经在手。
type stallingUnitStore struct {
	ports.ConsolidationStore
	stallOn string
	read    chan struct{}
	release chan struct{}
	once    sync.Once
}

func (store *stallingUnitStore) stall(id domain.ConsolidationUnitID) {
	if id.String() != store.stallOn {
		return
	}
	store.once.Do(func() {
		close(store.read)
		<-store.release
	})
}

func (store *stallingUnitStore) FindByID(ctx context.Context, tenant domain.TenantID, id domain.ConsolidationUnitID) (*domain.ConsolidationUnit, bool, error) {
	unit, found, err := store.ConsolidationStore.FindByID(ctx, tenant, id)
	store.stall(id)
	return unit, found, err
}

func (store *stallingUnitStore) FindByIDForUpdate(ctx context.Context, tenant domain.TenantID, id domain.ConsolidationUnitID) (*domain.ConsolidationUnit, bool, error) {
	unit, found, err := store.ConsolidationStore.FindByIDForUpdate(ctx, tenant, id)
	store.stall(id)
	return unit, found, err
}

// Covers: `AT-NO-039`「拒绝成员关系，原关系不被覆盖」的另一条来路——不经移入、经别的口按旧读数把删掉的容纳边写回。
// 封装、开封、移出、关闭都经 Update 把单元的成员整份写回容纳索引，写回不判环、也不在租户锁下。B 在 A 里时：封装 A
// 的那一笔读完 A 停住；此间导入把 B 从 A 移出，再把 A 装进 B——B 的链路此刻只有它自己，移入照判答`已移入`；封装
// 续行，按旧读数把 B→A 写回，册上 A 在 B 里、B 也在 A 里，此后这条链上任何移入都只能答`NO_ANSWER_FORMED`。
//
// 改单元的各口读单元持单元行锁（ConsolidationStore.FindByIDForUpdate），这一交错走不出来：移出等在 A 的行锁上，
// 封装提交后它读到已封装的 A、答未受理（封装后改成员先开封）；随后的反向移入沿 B 的链路看到 A，答`包含循环`。
// 封装那一笔若改回不持锁的读，或者移出那一笔改回不持锁的读，本用例都红。
func TestAStaleUnitReadCannotWriteBackARemovedEdgeIntoACycle(t *testing.T) {
	importer, db, watcher, units := raceAssembly(t)
	clock := fixedClock{at: mustInstant(t, "2026-09-08T18:00:00+08:00")}
	facts, err := nopostgres.NewConsolidationFacts(db)
	if err != nil {
		t.Fatalf("构造集运事实登记：%v", err)
	}
	outboxStore, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 outbox 存储：%v", err)
	}
	downstream, err := nopostgres.NewOutboxSealedSnapshotHandoff(db, outboxStore, clock)
	if err != nil {
		t.Fatalf("构造封装快照意图交付：%v", err)
	}
	stalled := &stallingUnitStore{ConsolidationStore: units, stallOn: "SYN-CU-A", read: make(chan struct{}), release: make(chan struct{})}
	sealer, err := application.NewConsolidateParcelsHandler(application.ConsolidateParcelsDeps{
		Store: stalled, Facts: facts, Containment: units, Downstream: downstream, Clock: clock,
	})
	if err != nil {
		t.Fatalf("装配会停住的集运编排：%v", err)
	}

	setup := importConsolidationTemplate(t, importer, []byte(consolidationHeader+
		consolidationLineFor("SYN-WORK-OPEN-A", "OPEN_UNIT", "SYN-CU-A", "SYN-BAG-A", "", "", "")+
		consolidationLineFor("SYN-WORK-OPEN-B", "OPEN_UNIT", "SYN-CU-B", "SYN-BAG-B", "", "", "")+
		consolidationLineFor("SYN-WORK-B-INTO-A", "ADD_MEMBER", "SYN-CU-A", "", "SYN-CU-B", "", "")))
	assertDisposition(t, setup, 2, rowLanded, "MEMBER_ADDED")

	sealSource, err := domain.NewWorkFactSource("FTI/CONSOLIDATION-1/SYN-BATCH-RACE/SYN-WORK-SEAL-A",
		mustValue(t, domain.NewPerformingPartyReference, "SYN-PACKER-1"),
		mustValue(t, domain.NewExecutionEvidenceReference, "FTI/CONSOLIDATION-1/SYN-CLERK-A/SYN-WORK-SEAL-A"),
		mustInstant(t, "2026-09-08T16:05:00+08:00"))
	if err != nil {
		t.Fatalf("来源表达：%v", err)
	}
	sealCommand := application.SealUnitCommand{
		TenantID: synTenant(t),
		Unit:     mustValue(t, domain.NewConsolidationUnitID, "SYN-CU-A"),
		Seal:     mustValue(t, domain.NewSealReference, "SYN-SEAL-A"),
		Basis:    mustValue(t, domain.NewWorkBasisReference, "SYN-BASIS-A"),
		Source:   sealSource,
	}
	var releaseOnce sync.Once
	unstall := func() { releaseOnce.Do(func() { close(stalled.release) }) }
	// 用例半途失败时也要放封装那一笔走、等各笔退出：悬着的事务会拖住本用例库的收尾（DROP DATABASE 等连接释放）。
	var workers sync.WaitGroup
	t.Cleanup(func() { unstall(); workers.Wait() })

	type outcome struct {
		result application.ConsolidationResult
		err    error
	}
	sealDone := make(chan outcome, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		var sealed application.ConsolidationResult
		err := importer.transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			result, err := sealer.Seal(txCtx, sealCommand)
			sealed = result
			return err
		})
		sealDone <- outcome{sealed, err}
	}()
	select {
	case <-stalled.read:
	case sealed := <-sealDone:
		t.Fatalf("封装那一笔没读到单元就退出了：outcome=%q err=%v", sealed.result.Outcome(), sealed.err)
	}

	// 封装那一笔读完 A 之后：另一份纸单先把 B 从 A 移出，再把 A 装进 B。
	middleBatch, err := decodeConsolidationTemplate(synTenant(t), []byte(consolidationHeader+
		consolidationLineFor("SYN-WORK-B-OUT-OF-A", "REMOVE_MEMBER", "SYN-CU-A", "", "SYN-CU-B", "", "")+
		consolidationLineFor("SYN-WORK-A-INTO-B", "ADD_MEMBER", "SYN-CU-B", "", "SYN-CU-A", "", "")))
	if err != nil {
		t.Fatalf("集运模板译装失败：%v", err)
	}
	middleResults := make(chan []rowResult, 1)
	middleFinished := make(chan struct{})
	workers.Add(1)
	go func() {
		defer workers.Done()
		defer close(middleFinished)
		results, _ := importConsolidation(context.Background(), middleBatch, importer)
		middleResults <- results
	}()
	waited := awaitLockWaiter(t, watcher, middleFinished)
	unstall()

	var sealed outcome
	select {
	case sealed = <-sealDone:
	case <-time.After(5 * time.Second):
		t.Fatal("放行后封装那一笔没走完")
	}
	var middle []rowResult
	select {
	case middle = <-middleResults:
	case <-time.After(5 * time.Second):
		t.Fatal("封装提交后移出与反向移入没走完")
	}
	if len(middle) != 2 {
		t.Fatalf("移出与反向移入的结果行数 = %d，要 2", len(middle))
	}

	lineageErr := importer.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := units.LineageForUpdate(txCtx, synTenant(t), mustValue(t, domain.NewConsolidationUnitID, "SYN-CU-A"))
		return err
	})
	if errors.Is(lineageErr, ports.ErrContainmentLineageCyclic) {
		t.Fatalf("册上成环：封装按旧读数把 B→A 写回，与随后的 A→B 合成一个环（封装 %q，移出 %q，反向移入 %q，移出等过锁=%v）",
			sealed.result.Outcome(), middle[0].Outcome, middle[1].Outcome, waited)
	}
	if lineageErr != nil {
		t.Fatalf("读 A 的链路：%v", lineageErr)
	}
	if !waited {
		t.Fatal("移出那一笔没有等封装那一笔的单元行锁就走完了")
	}
	if sealed.err != nil || sealed.result.Outcome() != application.UnitSealedRecorded {
		t.Fatalf("封装那一笔：outcome=%q err=%v", sealed.result.Outcome(), sealed.err)
	}
	assertDisposition(t, middle, 0, rowRejected, "NOT_ACCEPTED")
	assertDisposition(t, middle, 1, rowRejected, "CONTAINMENT_CYCLE")

	a := findUnit(t, units, "SYN-CU-A")
	if !a.Sealed() || memberIDs(a.Members()) != "SYN-CU-B" {
		t.Fatalf("A：封装=%v 成员=%q，要已封装且只装着 B", a.Sealed(), memberIDs(a.Members()))
	}
	if members := memberIDs(findUnit(t, units, "SYN-CU-B").Members()); members != "" {
		t.Fatalf("B 的成员 = %q，要为空", members)
	}
	if parent, contained, err := units.CurrentParent(t.Context(), synTenant(t), mustHandlingUnit(t, "SYN-CU-B")); err != nil || !contained || parent.String() != "SYN-CU-A" {
		t.Fatalf("B 的父级 = %v（contained=%v err=%v），要 A", parent, contained, err)
	}
	if parent, contained, err := units.CurrentParent(t.Context(), synTenant(t), mustHandlingUnit(t, "SYN-CU-A")); err != nil || contained {
		t.Fatalf("A 有了父级 %v（contained=%v err=%v）", parent, contained, err)
	}
}

// Covers: 同一只单元的两笔并发移入都落地——UC-NO-003「一致性、幂等与并发」里每次成员变化各是一条事实，后一笔不按旧读数
// 把前一笔的成员与容纳边盖掉。第二笔在第一笔提交之前就已开始、等在锁上；第一笔提交后它读到已装着 X 的单元，再装 Y。
// 第二笔若不取租户锁、也不持单元行锁读单元，它读到的是空单元，Update 写回只有 Y 的一份，X 的成员与容纳边都没了。
func TestTwoConcurrentAddsIntoOneUnitBothLand(t *testing.T) {
	importer, _, watcher, units := raceAssembly(t)
	ctx := t.Context()
	opened := importConsolidationTemplate(t, importer, []byte(consolidationHeader+
		consolidationLineFor("SYN-WORK-OPEN-A", "OPEN_UNIT", "SYN-CU-A", "SYN-BAG-A", "", "", "")))
	assertDisposition(t, opened, 0, rowLanded, "OPENED")
	addX := nestingCommand(t, "SYN-CU-A", "SYN-HU-X", "SYN-WORK-X-INTO-A")
	addY := nestingCommand(t, "SYN-CU-A", "SYN-HU-Y", "SYN-WORK-Y-INTO-A")
	basisX, err := importer.judge.Judge(ctx, addX)
	if err != nil {
		t.Fatalf("判方向依据：%v", err)
	}
	basisY, err := importer.judge.Judge(ctx, addY)
	if err != nil {
		t.Fatalf("判方向依据：%v", err)
	}

	firstHolds := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	// 用例半途失败时也要放第一笔走、等两笔退出：悬着的事务会拖住本用例库的收尾（DROP DATABASE 等连接释放）。
	var workers sync.WaitGroup
	t.Cleanup(func() { unblock(); workers.Wait() })

	type outcome struct {
		result application.ConsolidationResult
		err    error
	}
	firstDone := make(chan outcome, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		var result application.ConsolidationResult
		err := importer.transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			judged, err := importer.handler.AddMember(txCtx, addX, basisX)
			if err != nil {
				return err
			}
			result = judged
			close(firstHolds)
			<-release
			return nil
		})
		firstDone <- outcome{result, err}
	}()
	select {
	case <-firstHolds:
	case first := <-firstDone:
		t.Fatalf("第一笔没拿住就退出了：outcome=%q err=%v", first.result.Outcome(), first.err)
	}

	secondDone := make(chan outcome, 1)
	secondFinished := make(chan struct{})
	workers.Add(1)
	go func() {
		defer workers.Done()
		defer close(secondFinished)
		var result application.ConsolidationResult
		err := importer.transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			judged, err := importer.handler.AddMember(txCtx, addY, basisY)
			result = judged
			return err
		})
		secondDone <- outcome{result, err}
	}()
	if !awaitLockWaiter(t, watcher, secondFinished) {
		second := <-secondDone
		t.Fatalf("第二笔没有等第一笔提交就答了 %q（err=%v）", second.result.Outcome(), second.err)
	}
	unblock()

	if first := <-firstDone; first.err != nil || first.result.Outcome() != application.MemberAdded {
		t.Fatalf("第一笔：outcome=%q err=%v", first.result.Outcome(), first.err)
	}
	select {
	case second := <-secondDone:
		if second.err != nil || second.result.Outcome() != application.MemberAdded {
			t.Fatalf("第二笔：outcome=%q err=%v", second.result.Outcome(), second.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("第一笔提交后第二笔仍没放行")
	}
	if members := memberIDs(findUnit(t, units, "SYN-CU-A").Members()); members != "SYN-HU-X,SYN-HU-Y" {
		t.Fatalf("A 的成员 = %q，要 X 与 Y 都在；后一笔按旧读数盖掉了前一笔", members)
	}
	for _, member := range []string{"SYN-HU-X", "SYN-HU-Y"} {
		if parent, contained, err := units.CurrentParent(ctx, synTenant(t), mustHandlingUnit(t, member)); err != nil || !contained || parent.String() != "SYN-CU-A" {
			t.Fatalf("%s 的父级 = %v（contained=%v err=%v），要 A", member, parent, contained, err)
		}
	}
}
