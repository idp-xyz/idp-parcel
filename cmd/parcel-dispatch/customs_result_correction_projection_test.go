package main

import (
	"testing"
	"time"

	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 CC 层内更正关系在真库上一路译成 VE 的来源事实替代关系（ADR-0281）：CC 接收编排用生产的结果库判
// 更正生效，信封经生产 wireDispatcher 投到 VE，生产适配器沿 CC 结果库回读到链根，生效的更正落在链根引用下、
// 指名前身，成员投影里被更正的那一份不再派生里程碑。夹具与 successive_customs_results_projection_test.go 共用。

// correctedMemberEntries 列出成员当前投影里关务外部结果的各条目，按（引用，版本）取。
func (fixture *successiveResultsFixture) correctedMemberEntries(t *testing.T, member string) map[string]vedomain.MilestoneClassification {
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
	for _, entry := range projection.Entries() {
		entries[entry.Fact().Fact().String()+"@"+entry.Fact().Version().String()] = entry
	}
	return entries
}

// Covers: AT-VE-044、AT-CC-156 的 VE 半边——全部放行之后来源明确收窄成部分放行、再明确改回全部放行：三份都不记
// 冲突，两份更正落在原全部放行的引用下，版本是「提交版本/来源标识」，各自指名前一版；每一步成员投影里都只有链尾
// 当前有效，收窄之后不再归入清关，改回之后重新归入——VE 运营投影与 CC 放行读口按当下取的结论一致。
func TestAnInForceCustomsCorrectionSupersedesTheCorrectedEntryEndToEnd(t *testing.T) {
	fixture := newSuccessiveResultsFixture(t)
	full := fixture.releaseCommand(t, "SYN-RESULT-FULL-01", ccdomain.FullRelease, fixture.now.Add(-60*time.Minute))
	narrowed := fixture.releaseCommand(t, "SYN-RESULT-NARROW-01", ccdomain.PartialRelease, fixture.now.Add(-40*time.Minute))
	narrowed.Corrects = full.SourceID
	restored := fixture.releaseCommand(t, "SYN-RESULT-RESTORE-01", ccdomain.FullRelease, fixture.now.Add(-20*time.Minute))
	restored.Corrects = narrowed.SourceID

	root := func(member string) string { return "external-result/" + member + "/" + full.SourceID }
	steps := []struct {
		command customsapp.ReceiveExternalResultCommand
		version string
		cleared bool
	}{
		{full, externalResultVersion, true},
		{narrowed, externalResultVersion + "/" + narrowed.SourceID, false},
		{restored, externalResultVersion + "/" + restored.SourceID, true},
	}
	for _, step := range steps {
		requireOutcome(t, fixture.receiveAndDispatch(t, step.command), customsapp.ResultRecorded)
		if status := outboxStatus(t, fixture.db, resultEventID(fixture, step.command.SourceID)); status != "PUBLISHED" {
			t.Fatalf("%s 信封 = %q，失败码 = %q", step.command.SourceID, status,
				recordedFailureCode(t, fixture.db, resultEventID(fixture, step.command.SourceID)))
		}
		for _, member := range []string{externalResultMemberOne, externalResultMemberTwo} {
			entries := fixture.correctedMemberEntries(t, member)
			entry, present := entries[root(member)+"@"+step.version]
			if !present || len(entries) != 1 {
				t.Fatalf("%s 之后成员 %s 条目 = %v，want 只有链尾 %s", step.command.SourceID, member, entries, step.version)
			}
			milestone, classified := entry.Milestone()
			if cleared := classified && milestone.String() == "CUSTOMS_CLEARED"; cleared != step.cleared {
				t.Fatalf("%s 之后成员 %s 归类 = %q（%v），want 清关 %v", step.command.SourceID, member, milestone, classified, step.cleared)
			}
		}
	}

	tenant := fixture.identity.TenantID().String()
	for _, want := range []struct{ version, supersedes string }{
		{externalResultVersion + "/" + narrowed.SourceID, externalResultVersion},
		{externalResultVersion + "/" + restored.SourceID, externalResultVersion + "/" + narrowed.SourceID},
	} {
		if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.accepted_fact
			WHERE tenant_id = '`+tenant+`' AND source_context = 'CUSTOMS_COMPLIANCE'
			  AND fact_ref = '`+root(externalResultMemberOne)+`'
			  AND fact_version = '`+want.version+`' AND supersedes_version = '`+want.supersedes+`'`); n != 1 {
			t.Fatalf("事实库里版本 %s 指名前身 %s 的行 = %d, want 1", want.version, want.supersedes, n)
		}
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.accepted_fact
		WHERE tenant_id = '`+tenant+`' AND fact_ref LIKE 'external-result/%'`); n != 6 {
		t.Fatalf("外部结果事实 = %d 行, want 6——两成员各三版，都在链根引用下，被替代的留档", n)
	}
}
