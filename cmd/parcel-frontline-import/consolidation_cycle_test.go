package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
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
