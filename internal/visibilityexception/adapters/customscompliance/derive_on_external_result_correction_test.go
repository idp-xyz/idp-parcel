package customscompliance_test

import (
	"context"
	"testing"

	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// keyedResultFinder 按来源标识交回接收记录，一个夹具里可以先后处理几封信。
type keyedResultFinder map[string]ccports.ExternalResultRecord

func (finder keyedResultFinder) FindByKey(
	_ context.Context, key ccports.ExternalResultKey,
) (ccports.ExternalResultRecord, bool, error) {
	record, found := finder[key.SourceID]
	return record, found, nil
}

// asCorrection 把一份接收记录改成另一个来源标识上、指名 corrected 且更正已生效的更正。
func asCorrection(t *testing.T, record ccports.ExternalResultRecord, source, corrected string) ccports.ExternalResultRecord {
	t.Helper()
	result := record.Result
	rebuilt, err := ccdomain.InterpretExternalResult(ccdomain.ExternalResultSpec{
		Layer: result.Layer(), SourceID: source, Role: result.Role(), RawSemantics: result.RawSemantics(),
		Rule: result.Rule(), Version: result.Version(), Attempt: result.Attempt(), Scope: result.Scope(),
		OccurredAt: result.OccurredAt(), ReceivedAt: result.ReceivedAt(), Corrects: corrected,
	})
	if err != nil {
		t.Fatalf("解释更正：%v", err)
	}
	record.Key.SourceID = source
	record.Result = rebuilt
	record.CorrectionInForce = true
	return record
}

// Covers: 现状钉——CC 已形成层内更正关系，VE 投影今天不跟随：生效的更正按 ADR-0197 决定三另立事实引用
// `external-result/<成员>/<来源标识>`、版本取提交版本、不带前身，作为一条独立条目派生；被它更正的原全部放行
// 仍是当前有效条目，仍归入清关。这是票 docs-consistency/code/84 落地时要翻的红灯：那时更正应落在原结果的事实
// 引用下、以版本指名前身，原条目转为已被替代。
func TestAnInForceCorrectionIsDerivedAsAnIndependentEntryWithoutAPredecessor(t *testing.T) {
	fixture := newResultDeriveFixture(t, releaseOnlyMapping())
	units := &unitFinderDouble{unit: scopeUnit(t, "parcel-1"), found: true}
	original := layerRecord(t, ccdomain.ReleaseResultLayer, ccdomain.FullRelease)
	narrowed := asCorrection(t, layerRecord(t, ccdomain.ReleaseResultLayer, ccdomain.PartialRelease), "result-2", resultSource)
	finder := keyedResultFinder{resultSource: original, "result-2": narrowed}
	subject := newResultAdapter(t, finder, units, fixture.handler)

	for _, source := range []string{resultSource, "result-2"} {
		if err := subject.HandleReceivedExternalResult(t.Context(),
			veinbox.ReceivedExternalResult{TenantID: resultTenant, SourceID: source}); err != nil {
			t.Fatalf("处理 %s：%v", source, err)
		}
	}

	stored, found := fixture.facts.byKey[veports.FactKey{
		Tenant:  caseValue(t, vedomain.NewTenantID, resultTenant),
		Source:  vedomain.SourceCustomsCompliance,
		Fact:    caseValue(t, vedomain.NewSourceFactReference, "external-result/parcel-1/result-2"),
		Version: caseValue(t, vedomain.NewSourceFactVersion, resultVersion),
	}]
	if !found {
		t.Fatal("生效的更正没按自己的来源标识另立一条事实")
	}
	if _, superseding := stored.Fact.Supersedes(); superseding {
		t.Fatal("更正带上了前身——ADR-0197 决定三的引用取法下登不出前身；若这是新 ADR 落地，改这条用例")
	}

	projection, found := fixture.projections.byKey[resultTenant+"/parcel-1"]
	if !found {
		t.Fatal("成员没派生出投影")
	}
	entries := projection.Entries()
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2——原条目与更正各一条，都当前有效", len(entries))
	}
	cleared := 0
	for _, entry := range entries {
		if milestone, classified := entry.Milestone(); classified && milestone.String() == clearedMilestone {
			cleared++
		}
	}
	if cleared != 1 {
		t.Fatalf("归入清关的条目 = %d, want 1——被更正的原全部放行今天仍归入清关", cleared)
	}
}
