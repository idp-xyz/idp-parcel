package main

import (
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证 buildConsolidationImporter 装配的整条链（隔离合成 S）：模板
// 译装 → 既有集运六口 → 真库。集运口没有收寄口那条身份核对缝，样例九行今天就能全部落地，
// 因此这里钉的是：来源标记两格真落库且录入者只在证据引用里、封装快照按模板所写的业务时间
// 冻结而记录时刻来自时钟、重放与来源冲突在真库上分得开（AT-NO-043 两向）、被拒的行一条
// 来源事实都不留。

// 样例模板几行的来源身份。写成字面量而不是调 consolidationSourceID 拼：要证的正是落库的键
// 长什么样，拿被测函数自己算一遍等于什么都没证。
const (
	sourceIDOpenFirst  = "FTI/CONSOLIDATION-1/SYN-BATCH-20260908-PM/SYN-WORK-0001"
	sourceIDSealFirst  = "FTI/CONSOLIDATION-1/SYN-BATCH-20260908-PM/SYN-WORK-0004"
	sourceIDSealSecond = "FTI/CONSOLIDATION-1/SYN-BATCH-20260908-PM/SYN-WORK-0007"
)

func consolidationVerticalImporter(t *testing.T) (consolidationImporter, *nopostgres.ConsolidationUnits, *nopostgres.ConsolidationFacts) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	// 记录时刻用固定钟，业务时间来自模板——用例据此证两者不是同一个来源。
	importer, err := buildConsolidationImporter(db, fixedClock{at: mustInstant(t, "2026-09-08T18:00:00+08:00")})
	if err != nil {
		t.Fatalf("装配集运导入口：%v", err)
	}
	units, err := nopostgres.NewConsolidationUnits(db)
	if err != nil {
		t.Fatalf("构造集运单元库读口：%v", err)
	}
	facts, err := nopostgres.NewConsolidationFacts(db)
	if err != nil {
		t.Fatalf("构造集运事实登记读口：%v", err)
	}
	return importer, units, facts
}

func importConsolidationTemplate(t *testing.T, importer consolidationImporter, raw []byte) []rowResult {
	t.Helper()
	batch, err := decodeConsolidationTemplate(synTenant(t), raw)
	if err != nil {
		t.Fatalf("集运模板译装失败：%v", err)
	}
	return importConsolidation(t.Context(), batch, importer)
}

func findUnit(t *testing.T, units *nopostgres.ConsolidationUnits, id string) *domain.ConsolidationUnit {
	t.Helper()
	unitID, err := domain.NewConsolidationUnitID(id)
	if err != nil {
		t.Fatalf("构造单元标识：%v", err)
	}
	unit, found, err := units.FindByID(t.Context(), synTenant(t), unitID)
	if err != nil {
		t.Fatalf("读回单元 %s：%v", id, err)
	}
	if !found {
		t.Fatalf("单元 %s 没落库", id)
	}
	return unit
}

func findFact(t *testing.T, facts *nopostgres.ConsolidationFacts, sourceID string) (ports.ConsolidationFactRecord, bool) {
	t.Helper()
	record, found, err := facts.FindByKey(t.Context(), ports.ConsolidationFactKey{TenantID: synTenant(t), SourceID: sourceID})
	if err != nil {
		t.Fatalf("读回来源事实 %s：%v", sourceID, err)
	}
	return record, found
}

func memberIDs(members []domain.HandlingUnitID) string {
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.String())
	}
	return strings.Join(ids, ",")
}

// TestFrontlineConsolidationImportVerticalOnRealPostgres 用样例九行走完开启→移入→封装→开封→
// 移出→重封与开启→关闭：全部落地、退出码 0；单元一两版快照都在（AT-NO-036），封装时刻是
// 模板所写的业务时间；来源事实登记带执行方与两处 FTI 标记，记录时刻来自固定钟；同一份文件
// 重跑九行全答重放；封装行换业务时间答来源冲突且原登记与原快照都不动。
func TestFrontlineConsolidationImportVerticalOnRealPostgres(t *testing.T) {
	importer, units, facts := consolidationVerticalImporter(t)

	results := importConsolidationTemplate(t, importer, sampleConsolidationTemplate(t))
	wantOutcomes := []string{"OPENED", "MEMBER_ADDED", "MEMBER_ADDED", "SEALED", "UNSEALED", "MEMBER_REMOVED", "SEALED", "OPENED", "CLOSED"}
	if len(results) != len(wantOutcomes) {
		t.Fatalf("结果行数 = %d，要 %d", len(results), len(wantOutcomes))
	}
	for index, want := range wantOutcomes {
		assertDisposition(t, results, index, rowLanded, want)
	}
	if code := exitCodeFor(results); code != exitLanded {
		t.Fatalf("九行全落地退出码 = %d，要 %d", code, exitLanded)
	}

	// 单元一：已封装，成员只剩 HU-0001，两版快照都保留——重新封装创建新快照，一份都不覆盖。
	first := findUnit(t, units, "SYN-CU-0001")
	if !first.Sealed() || memberIDs(first.Members()) != "SYN-HU-0001" {
		t.Fatalf("单元一 封装=%v 成员=%q，要已封装且只剩 SYN-HU-0001", first.Sealed(), memberIDs(first.Members()))
	}
	if first.OpenedBy().SourceID() != sourceIDOpenFirst {
		t.Fatalf("开启来源 = %q，要带 FTI 标记的行事实号", first.OpenedBy().SourceID())
	}
	snapshots := first.Snapshots()
	if len(snapshots) != 2 {
		t.Fatalf("快照数 = %d，要 2（封装、开封后重封）", len(snapshots))
	}
	if memberIDs(snapshots[0].Members()) != "SYN-HU-0001,SYN-HU-0002" || snapshots[0].Seal().String() != "SYN-SEAL-0001" || snapshots[0].Basis().String() != "SYN-BASIS-SEAL-01" {
		t.Fatalf("第一版快照失真：成员 %q 封签 %s 依据 %s", memberIDs(snapshots[0].Members()), snapshots[0].Seal(), snapshots[0].Basis())
	}
	if memberIDs(snapshots[1].Members()) != "SYN-HU-0001" || snapshots[1].Seal().String() != "SYN-SEAL-0002" {
		t.Fatalf("第二版快照失真：成员 %q 封签 %s", memberIDs(snapshots[1].Members()), snapshots[1].Seal())
	}
	// 封装时刻是模板所写的现场时间，不是导入这一刻；来源身份不带录入者，证据引用带。
	if !snapshots[0].SealedAt().Equal(mustInstant(t, "2026-09-08T14:10:00+08:00")) {
		t.Fatalf("第一版封装时刻 = %s，要模板所写的 14:10+08:00", snapshots[0].SealedAt())
	}
	if got := snapshots[0].Source(); got.SourceID() != sourceIDSealFirst ||
		got.Evidence().String() != "FTI/CONSOLIDATION-1/SYN-CLERK-A/SYN-WORKSHEET-0001" ||
		got.PerformedBy().String() != "SYN-PACKER-1" {
		t.Fatalf("第一版快照来源失真：身份 %q 证据 %q 执行方 %q", got.SourceID(), got.Evidence(), got.PerformedBy())
	}

	// 来源事实登记：封装那一行带动作、封签、执行方与两处标记；业务时间来自模板，记录时刻来自钟。
	sealFact, found := findFact(t, facts, sourceIDSealFirst)
	if !found {
		t.Fatalf("封装行没留来源事实：%s", sourceIDSealFirst)
	}
	if sealFact.Action != domain.SealUnitAction || sealFact.Unit.String() != "SYN-CU-0001" || sealFact.Seal.String() != "SYN-SEAL-0001" {
		t.Fatalf("封装登记失真：%s %s %s", sealFact.Action, sealFact.Unit, sealFact.Seal)
	}
	if sealFact.PerformedBy.String() != "SYN-PACKER-1" || sealFact.Evidence.String() != "FTI/CONSOLIDATION-1/SYN-CLERK-A/SYN-WORKSHEET-0001" {
		t.Fatalf("封装登记的执行方/证据失真：%q %q", sealFact.PerformedBy, sealFact.Evidence)
	}
	if !sealFact.OccurredAt.Equal(mustInstant(t, "2026-09-08T14:10:00+08:00")) {
		t.Fatalf("登记业务时间 = %s，要模板所写的 14:10+08:00", sealFact.OccurredAt)
	}
	if !sealFact.RecordedAt.Equal(mustInstant(t, "2026-09-08T18:00:00+08:00")) || !sealFact.RecordedAt.After(sealFact.OccurredAt) {
		t.Fatalf("登记记录时刻 = %s，要固定钟给的 18:00+08:00 且晚于业务时间", sealFact.RecordedAt)
	}

	// 单元二：开启后空着关闭，关闭时刻同样取模板所写。
	second := findUnit(t, units, "SYN-CU-0002")
	if !second.Closed() || len(second.Members()) != 0 || !second.ClosedAt().Equal(mustInstant(t, "2026-09-08T15:05:00+08:00")) {
		t.Fatalf("单元二 关闭=%v 成员=%d 关闭时刻=%s", second.Closed(), len(second.Members()), second.ClosedAt())
	}

	// 重放：同一份文件重跑，九行全答已有结果，单元状态不动。
	replayed := importConsolidationTemplate(t, importer, sampleConsolidationTemplate(t))
	for index := range wantOutcomes {
		assertDisposition(t, replayed, index, rowReplayed, "EXISTING_RESULT")
	}
	if code := exitCodeFor(replayed); code != exitLanded {
		t.Fatalf("全重放退出码 = %d，要 %d", code, exitLanded)
	}
	if again := findUnit(t, units, "SYN-CU-0001"); len(again.Snapshots()) != 2 {
		t.Fatalf("重放后快照数 = %d，要仍为 2", len(again.Snapshots()))
	}

	// 来源冲突：第一封装行换了业务时间即另一份内容（摘要含业务时间）。绝不覆盖；其余行照常
	// 重放，因此退出码是 2 而不是 3。
	conflicting := strings.Replace(string(sampleConsolidationTemplate(t)),
		"SYN-BASIS-SEAL-01,SYN-WORKSHEET-0001,2026-09-08T14:10:00+08:00",
		"SYN-BASIS-SEAL-01,SYN-WORKSHEET-0001,2026-09-08T14:11:00+08:00", 1)
	conflicted := importConsolidationTemplate(t, importer, []byte(conflicting))
	assertDisposition(t, conflicted, 3, rowRejected, "SOURCE_CONFLICT")
	assertDisposition(t, conflicted, 6, rowReplayed, "EXISTING_RESULT")
	if code := exitCodeFor(conflicted); code != exitRejected {
		t.Fatalf("只有被拒没有未决：退出码 = %d，要 %d", code, exitRejected)
	}
	after, found := findFact(t, facts, sourceIDSealFirst)
	if !found || !after.RecordedAt.Equal(sealFact.RecordedAt) || after.ContentDigest != sealFact.ContentDigest {
		t.Fatalf("冲突把原登记改了：记录时刻 %s→%s，摘要 %s→%s", sealFact.RecordedAt, after.RecordedAt, sealFact.ContentDigest, after.ContentDigest)
	}
	if unitAfter := findUnit(t, units, "SYN-CU-0001"); len(unitAfter.Snapshots()) != 2 || !unitAfter.Snapshots()[0].SealedAt().Equal(snapshots[0].SealedAt()) {
		t.Fatalf("冲突动了单元快照：数 %d，第一版封装时刻 %s", len(unitAfter.Snapshots()), unitAfter.Snapshots()[0].SealedAt())
	}
}

// consolidationLineFor 拼一行合成值，单元可指定——被拒那组用例要跨两个单元。
func consolidationLineFor(factRef, action, unit, asset, member, seal, basis string) string {
	return strings.Join([]string{
		"CONSOLIDATION-1", "SYN-BATCH-2", factRef, "SYN-CLERK-A", "SYN-PACKER-1", action, unit,
		asset, member, seal, basis, "SYN-WORKSHEET-2", "2026-09-08T16:00:00+08:00",
	}, ",") + "\n"
}

// TestFrontlineConsolidationImportRejectsWithoutRecording 证四种被拒在真库上各自成立且**一条来源
// 事实都不留**：单元不在册、成员在别的单元（报出对方标识）、空单元封装与成员未清空却无处置
// 依据的关闭（领域拦，答未受理）；另证「单元已由别的来源开启」答重放而非冲突。被拒的行不留
// 登记，改文件重跑同一行事实号才不会被当成冲突。
func TestFrontlineConsolidationImportRejectsWithoutRecording(t *testing.T) {
	importer, units, facts := consolidationVerticalImporter(t)

	raw := consolidationHeader +
		consolidationLineFor("SYN-WORK-A", "ADD_MEMBER", "SYN-CU-9", "", "SYN-HU-9", "", "") + // 单元从未开启
		consolidationLineFor("SYN-WORK-B", "OPEN_UNIT", "SYN-CU-7", "SYN-CAGE-7", "", "", "") +
		consolidationLineFor("SYN-WORK-C", "ADD_MEMBER", "SYN-CU-7", "", "SYN-HU-7", "", "") +
		consolidationLineFor("SYN-WORK-D", "OPEN_UNIT", "SYN-CU-8", "SYN-BAG-8", "", "", "") +
		consolidationLineFor("SYN-WORK-E", "ADD_MEMBER", "SYN-CU-8", "", "SYN-HU-7", "", "") + // 成员已在 CU-7
		consolidationLineFor("SYN-WORK-F", "SEAL_UNIT", "SYN-CU-8", "", "", "SYN-SEAL-8", "SYN-BASIS-8") + // 空单元
		consolidationLineFor("SYN-WORK-G", "CLOSE_UNIT", "SYN-CU-7", "", "", "", "") + // 有成员无处置依据
		consolidationLineFor("SYN-WORK-H", "OPEN_UNIT", "SYN-CU-7", "SYN-CAGE-7B", "", "", "") // 别的来源再开同一单元
	results := importConsolidationTemplate(t, importer, []byte(raw))
	if len(results) != 8 {
		t.Fatalf("结果行数 = %d，要 8", len(results))
	}
	assertDisposition(t, results, 0, rowRejected, "UNIT_NOT_FOUND")
	assertDisposition(t, results, 1, rowLanded, "OPENED")
	assertDisposition(t, results, 2, rowLanded, "MEMBER_ADDED")
	assertDisposition(t, results, 3, rowLanded, "OPENED")
	assertDisposition(t, results, 4, rowRejected, "MEMBER_ELSEWHERE_CONTAINED")
	assertDisposition(t, results, 5, rowRejected, "NOT_ACCEPTED")
	assertDisposition(t, results, 6, rowRejected, "NOT_ACCEPTED")
	assertDisposition(t, results, 7, rowReplayed, "EXISTING")
	if !strings.Contains(results[4].Detail, "SYN-CU-7") {
		t.Fatalf("成员在别处的报文没点名对方单元：%q", results[4].Detail)
	}
	if code := exitCodeFor(results); code != exitRejected {
		t.Fatalf("退出码 = %d，要 %d", code, exitRejected)
	}

	// 被拒与重放的行都不留来源事实；落地的行留。
	for _, factRef := range []string{"SYN-WORK-A", "SYN-WORK-E", "SYN-WORK-F", "SYN-WORK-G", "SYN-WORK-H"} {
		if _, found := findFact(t, facts, "FTI/CONSOLIDATION-1/SYN-BATCH-2/"+factRef); found {
			t.Fatalf("%s 没有作业发生却留了来源事实", factRef)
		}
	}
	for _, factRef := range []string{"SYN-WORK-B", "SYN-WORK-C", "SYN-WORK-D"} {
		if _, found := findFact(t, facts, "FTI/CONSOLIDATION-1/SYN-BATCH-2/"+factRef); !found {
			t.Fatalf("%s 落地了却没留来源事实", factRef)
		}
	}
	// 被拒的行没留半截状态：CU-8 仍开放且空，CU-7 仍开放且载具是第一次开启时的。
	eight := findUnit(t, units, "SYN-CU-8")
	if eight.Sealed() || eight.Closed() || len(eight.Members()) != 0 {
		t.Fatalf("空单元封装被拒后 CU-8 状态变了：封装=%v 关闭=%v 成员=%d", eight.Sealed(), eight.Closed(), len(eight.Members()))
	}
	seven := findUnit(t, units, "SYN-CU-7")
	if seven.Closed() || seven.Asset().String() != "SYN-CAGE-7" || memberIDs(seven.Members()) != "SYN-HU-7" {
		t.Fatalf("CU-7 状态变了：关闭=%v 载具=%s 成员=%q", seven.Closed(), seven.Asset(), memberIDs(seven.Members()))
	}
}
