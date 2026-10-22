package customscompliance_test

import (
	"context"
	"errors"
	"testing"

	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/customscompliance"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证 CC 层内更正关系怎样译成 VE 的来源事实替代关系（ADR-0281）：生效的更正落在它所更正那条链的
// 链根引用下、以「提交版本/来源标识」为版本、指名前身那一版；不生效的更正照独立条目派生；沿链回读的坏法
// 按仓储不变量已破与续办分格。

// keyedResultFinder 按来源标识交回接收记录，一个夹具里可以先后处理几封信；failing 里的来源标识读不动。
type keyedResultFinder struct {
	records map[string]ccports.ExternalResultRecord
	failing map[string]bool
}

func (finder keyedResultFinder) FindByKey(
	_ context.Context, key ccports.ExternalResultKey,
) (ccports.ExternalResultRecord, bool, error) {
	if finder.failing[key.SourceID] {
		return ccports.ExternalResultRecord{}, false, errors.New("result store unavailable")
	}
	record, found := finder.records[key.SourceID]
	return record, found, nil
}

func recordsBySource(records ...ccports.ExternalResultRecord) keyedResultFinder {
	finder := keyedResultFinder{records: map[string]ccports.ExternalResultRecord{}, failing: map[string]bool{}}
	for _, record := range records {
		finder.records[record.Key.SourceID] = record
	}
	return finder
}

// asCorrection 把一份接收记录改成另一个来源标识上、指名 corrected 且更正已生效的更正。
func asCorrection(t *testing.T, record ccports.ExternalResultRecord, source, corrected string) ccports.ExternalResultRecord {
	t.Helper()
	record = reinterpreted(t, record, source, corrected, record.Result.Version(), record.Result.Scope())
	record.CorrectionInForce = true
	return record
}

// reinterpreted 经领域真路径把记录换到另一个来源标识、提交版本或范围上，corrected 非空时带上更正指向。
func reinterpreted(
	t *testing.T,
	record ccports.ExternalResultRecord,
	source, corrected string,
	version ccdomain.SubmissionVersionID,
	scope ccdomain.DecisionScopeReference,
) ccports.ExternalResultRecord {
	t.Helper()
	result := record.Result
	rebuilt, err := ccdomain.InterpretExternalResult(ccdomain.ExternalResultSpec{
		Layer: result.Layer(), SourceID: source, Role: result.Role(), RawSemantics: result.RawSemantics(),
		Rule: result.Rule(), Version: version, Attempt: result.Attempt(), Scope: scope,
		OccurredAt: result.OccurredAt(), ReceivedAt: result.ReceivedAt(), Corrects: corrected,
	})
	if err != nil {
		t.Fatalf("解释外部结果：%v", err)
	}
	record.Key.SourceID = source
	record.Result = rebuilt
	return record
}

func deliverResults(t *testing.T, subject *adapter.DeriveOnExternalResultAdapter, sources ...string) {
	t.Helper()
	for _, source := range sources {
		if err := subject.HandleReceivedExternalResult(t.Context(),
			veinbox.ReceivedExternalResult{TenantID: resultTenant, SourceID: source}); err != nil {
			t.Fatalf("处理 %s：%v", source, err)
		}
	}
}

func storedResultFact(t *testing.T, fixture resultDeriveFixture, reference, version string) (vedomain.AcceptedSourceFact, bool) {
	t.Helper()
	stored, found := fixture.facts.byKey[veports.FactKey{
		Tenant:  caseValue(t, vedomain.NewTenantID, resultTenant),
		Source:  vedomain.SourceCustomsCompliance,
		Fact:    caseValue(t, vedomain.NewSourceFactReference, reference),
		Version: caseValue(t, vedomain.NewSourceFactVersion, version),
	}]
	return stored.Fact, found
}

func requireSupersedes(t *testing.T, fact vedomain.AcceptedSourceFact, want string) {
	t.Helper()
	predecessor, superseding := fact.Supersedes()
	if !superseding || predecessor.String() != want {
		t.Fatalf("前身 = %q（%v），want %q", predecessor, superseding, want)
	}
}

func releaseRecord(t *testing.T, kind ccdomain.ReleaseKind) ccports.ExternalResultRecord {
	t.Helper()
	return layerRecord(t, ccdomain.ReleaseResultLayer, kind)
}

// Covers: AT-VE-044、AT-CC-156 的 VE 半边——放行收窄的更正生效之后，更正落在原全部放行的事实引用
// `external-result/<成员>/<原来源标识>` 下，版本取「提交版本/更正的来源标识」，指名原结果那一版为前身；
// 当前有效集里只剩部分放行，原全部放行留档、不再归入清关——与 CC 放行读口在更正之后只交出部分放行对齐。
func TestAnInForceCorrectionLandsOnItsChainRootAndSupersedesTheCorrectedResult(t *testing.T) {
	fixture := newResultDeriveFixture(t, releaseOnlyMapping())
	units := &unitFinderDouble{unit: scopeUnit(t, "parcel-1"), found: true}
	original := releaseRecord(t, ccdomain.FullRelease)
	narrowed := asCorrection(t, releaseRecord(t, ccdomain.PartialRelease), "result-2", resultSource)
	subject := newResultAdapter(t, recordsBySource(original, narrowed), units, fixture.handler)

	deliverResults(t, subject, resultSource, "result-2")

	rootReference := "external-result/parcel-1/" + resultSource
	correction, found := storedResultFact(t, fixture, rootReference, resultVersion+"/result-2")
	if !found {
		t.Fatal("生效的更正没落在链根引用下、以「提交版本/来源标识」为版本")
	}
	requireSupersedes(t, correction, resultVersion)
	if _, found := storedResultFact(t, fixture, rootReference, resultVersion); !found {
		t.Fatal("原全部放行不在了——被替代的事实留档不删")
	}
	if _, independent := storedResultFact(t, fixture, "external-result/parcel-1/result-2", resultVersion); independent {
		t.Fatal("生效的更正另立了自己的引用——又是两条并存")
	}

	entry := onlyEntry(t, fixture.projections, "parcel-1")
	if kind := entry.Fact().Kind().String(); kind != "external-result-release-partial" {
		t.Fatalf("当前有效条目 kind = %q, want 部分放行", kind)
	}
	if milestone, classified := entry.Milestone(); classified {
		t.Fatalf("更正之后仍归入 %q——被更正的全部放行不该再派生里程碑", milestone)
	}
}

// Covers: AT-VE-044——链上两次生效更正：第二次更正指名的是第一次更正，三份都落在链根引用下，第二次的前身是
// 「提交版本/第一次更正的来源标识」；当前有效的只有链尾。
func TestASecondCorrectionNamesTheFirstCorrectionVersionAsItsPredecessor(t *testing.T) {
	fixture := newResultDeriveFixture(t, releaseOnlyMapping())
	units := &unitFinderDouble{unit: scopeUnit(t, "parcel-1"), found: true}
	original := releaseRecord(t, ccdomain.FullRelease)
	narrowed := asCorrection(t, releaseRecord(t, ccdomain.PartialRelease), "result-2", resultSource)
	restored := asCorrection(t, releaseRecord(t, ccdomain.FullRelease), "result-3", "result-2")
	subject := newResultAdapter(t, recordsBySource(original, narrowed, restored), units, fixture.handler)

	deliverResults(t, subject, resultSource, "result-2", "result-3")

	tail, found := storedResultFact(t, fixture, "external-result/parcel-1/"+resultSource, resultVersion+"/result-3")
	if !found {
		t.Fatal("第二次更正没落在链根引用下")
	}
	requireSupersedes(t, tail, resultVersion+"/result-2")
	entry := onlyEntry(t, fixture.projections, "parcel-1")
	if entry.Fact().Version().String() != resultVersion+"/result-3" {
		t.Fatalf("当前有效条目版本 = %q, want 链尾", entry.Fact().Version())
	}
	if milestone, classified := entry.Milestone(); !classified || milestone.String() != clearedMilestone {
		t.Fatalf("链尾全部放行归类 = %q（%v）", milestone, classified)
	}
}

// Covers: AT-VE-044——更正先于原结果到 VE：「已被替代」是派生问答，原结果后到也只是留档，终态与按序到达相同。
func TestACorrectionDeliveredBeforeItsOriginalStillSupersedesIt(t *testing.T) {
	fixture := newResultDeriveFixture(t, releaseOnlyMapping())
	units := &unitFinderDouble{unit: scopeUnit(t, "parcel-1"), found: true}
	original := releaseRecord(t, ccdomain.FullRelease)
	narrowed := asCorrection(t, releaseRecord(t, ccdomain.PartialRelease), "result-2", resultSource)
	subject := newResultAdapter(t, recordsBySource(original, narrowed), units, fixture.handler)

	deliverResults(t, subject, "result-2", resultSource)

	entry := onlyEntry(t, fixture.projections, "parcel-1")
	if entry.Fact().Version().String() != resultVersion+"/result-2" {
		t.Fatalf("当前有效条目版本 = %q, want 更正那一版", entry.Fact().Version())
	}
}

// Covers: AT-CC-156 的 VE 半边——范围不等的更正在 CC 不生效、在自己的范围上独立判断，VE 照独立条目派生：自己的
// 引用、提交版本、不带前身，原结果仍当前有效。
func TestACorrectionNotInForceIsDerivedAsAnIndependentEntry(t *testing.T) {
	fixture := newResultDeriveFixture(t, releaseOnlyMapping())
	units := &unitFinderDouble{unit: scopeUnit(t, "parcel-1"), found: true}
	original := releaseRecord(t, ccdomain.FullRelease)
	elsewhere := asCorrection(t, releaseRecord(t, ccdomain.PartialRelease), "result-2", resultSource)
	elsewhere.CorrectionInForce = false
	subject := newResultAdapter(t, recordsBySource(original, elsewhere), units, fixture.handler)

	deliverResults(t, subject, resultSource, "result-2")

	independent, found := storedResultFact(t, fixture, "external-result/parcel-1/result-2", resultVersion)
	if !found {
		t.Fatal("不生效的更正没按自己的来源标识派生")
	}
	if _, superseding := independent.Supersedes(); superseding {
		t.Fatal("不生效的更正带上了前身——CC 没让它取代任何结果")
	}
	if entries := fixture.projections.byKey[resultTenant+"/parcel-1"].Entries(); len(entries) != 2 {
		t.Fatalf("entries = %d, want 2——原结果与不生效的更正都当前有效", len(entries))
	}
}

// Covers: 链根本身记了同层冲突（CC 准许更正一份冲突的结果生效）：链根从没进投影，更正照样落在它的引用下、指名
// 它为前身；前身不在场无害，当前有效的就是更正。
func TestACorrectionOfAConflictedResultStillLandsOnThatResultsReference(t *testing.T) {
	fixture := newResultDeriveFixture(t, releaseOnlyMapping())
	units := &unitFinderDouble{unit: scopeUnit(t, "parcel-1"), found: true}
	original := releaseRecord(t, ccdomain.FullRelease)
	original.LayerConflict = true
	narrowed := asCorrection(t, releaseRecord(t, ccdomain.PartialRelease), "result-2", resultSource)
	subject := newResultAdapter(t, recordsBySource(original, narrowed), units, fixture.handler)

	deliverResults(t, subject, resultSource, "result-2")

	rootReference := "external-result/parcel-1/" + resultSource
	correction, found := storedResultFact(t, fixture, rootReference, resultVersion+"/result-2")
	if !found {
		t.Fatal("更正没落在冲突那份的引用下")
	}
	requireSupersedes(t, correction, resultVersion)
	if _, found := storedResultFact(t, fixture, rootReference, resultVersion); found {
		t.Fatal("记了冲突的链根进了投影")
	}
	if entry := onlyEntry(t, fixture.projections, "parcel-1"); entry.Fact().Version().String() != resultVersion+"/result-2" {
		t.Fatalf("当前有效条目版本 = %q", entry.Fact().Version())
	}
}

// Covers: 沿更正链回读读不动是续办——与按信封键读不动接收记录同一格，进未决哨兵名单（ADR-0029）。
func TestAPredecessorThatCannotBeReadIsContinuable(t *testing.T) {
	fixture := newResultDeriveFixture(t, releaseOnlyMapping())
	units := &unitFinderDouble{unit: scopeUnit(t, "parcel-1"), found: true}
	original := releaseRecord(t, ccdomain.FullRelease)
	narrowed := asCorrection(t, releaseRecord(t, ccdomain.PartialRelease), "result-2", resultSource)
	finder := recordsBySource(original, narrowed)
	finder.failing[resultSource] = true
	subject := newResultAdapter(t, finder, units, fixture.handler)

	err := subject.HandleReceivedExternalResult(t.Context(),
		veinbox.ReceivedExternalResult{TenantID: resultTenant, SourceID: "result-2"})
	if !errors.Is(err, adapter.ErrExternalResultNotVisible) {
		t.Fatalf("err = %v, want ErrExternalResultNotVisible", err)
	}
	if len(fixture.facts.byKey) != 0 {
		t.Fatal("前身读不动时已经派生——链根还没定就落了引用")
	}
}

// Covers: 沿更正链回读的坏链是仓储不变量已破，不进未决（ADR-0029）：CC 的外键钉住所指在册、记录只建立不改写、
// 生效的更正与所更正那份同层同提交版本同范围，所以前身读不回、成环、跨层、跨提交版本、跨范围、是归属不上的
// 留存、指着另一个键、生效却没有所指或链中段记了冲突，重投都不会自愈。
func TestABrokenCorrectionChainIsInconsistentNotUndecided(t *testing.T) {
	scope := caseValue(t, ccdomain.NewDecisionScopeReference, resultUnit)
	version := caseValue(t, ccdomain.NewSubmissionVersionID, resultVersion)
	tests := []struct {
		name  string
		tail  string
		chain func(t *testing.T) keyedResultFinder
	}{
		{"前身读不回", "result-2", func(t *testing.T) keyedResultFinder {
			return recordsBySource(asCorrection(t, releaseRecord(t, ccdomain.PartialRelease), "result-2", resultSource))
		}},
		{"成环", "result-2", func(t *testing.T) keyedResultFinder {
			return recordsBySource(
				asCorrection(t, releaseRecord(t, ccdomain.FullRelease), resultSource, "result-2"),
				asCorrection(t, releaseRecord(t, ccdomain.PartialRelease), "result-2", resultSource))
		}},
		{"前身在另一层", "result-2", func(t *testing.T) keyedResultFinder {
			return recordsBySource(
				layerRecord(t, ccdomain.ProcessDecisionLayer, ccdomain.ReleaseKindInvalid),
				asCorrection(t, releaseRecord(t, ccdomain.PartialRelease), "result-2", resultSource))
		}},
		{"前身在另一提交版本", "result-2", func(t *testing.T) keyedResultFinder {
			root := reinterpreted(t, releaseRecord(t, ccdomain.FullRelease), resultSource, "",
				caseValue(t, ccdomain.NewSubmissionVersionID, "version-0"), scope)
			return recordsBySource(root, asCorrection(t, releaseRecord(t, ccdomain.PartialRelease), "result-2", resultSource))
		}},
		{"前身在另一范围", "result-2", func(t *testing.T) keyedResultFinder {
			decision := layerRecord(t, ccdomain.ProcessDecisionLayer, ccdomain.ReleaseKindInvalid)
			root := reinterpreted(t, decision, resultSource, "", version,
				caseValue(t, ccdomain.NewDecisionScopeReference, "unit-0"))
			return recordsBySource(root, asCorrection(t, decision, "result-2", resultSource))
		}},
		{"前身是归属不上的留存", "result-2", func(t *testing.T) keyedResultFinder {
			root := releaseRecord(t, ccdomain.FullRelease)
			root.Unattributable = true
			return recordsBySource(root, asCorrection(t, releaseRecord(t, ccdomain.PartialRelease), "result-2", resultSource))
		}},
		{"前身指着另一个键", "result-2", func(t *testing.T) keyedResultFinder {
			finder := recordsBySource(releaseRecord(t, ccdomain.FullRelease),
				asCorrection(t, releaseRecord(t, ccdomain.PartialRelease), "result-2", resultSource))
			misfiled := finder.records[resultSource]
			misfiled.Key.SourceID = "result-9"
			finder.records[resultSource] = misfiled
			return finder
		}},
		{"生效却没有所指", "result-2", func(t *testing.T) keyedResultFinder {
			orphan := reinterpreted(t, releaseRecord(t, ccdomain.PartialRelease), "result-2", "", version, scope)
			orphan.CorrectionInForce = true
			return recordsBySource(releaseRecord(t, ccdomain.FullRelease), orphan)
		}},
		{"链中段记了冲突", "result-3", func(t *testing.T) keyedResultFinder {
			middle := asCorrection(t, releaseRecord(t, ccdomain.PartialRelease), "result-2", resultSource)
			middle.LayerConflict = true
			return recordsBySource(releaseRecord(t, ccdomain.FullRelease), middle,
				asCorrection(t, releaseRecord(t, ccdomain.FullRelease), "result-3", "result-2"))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResultDeriveFixture(t, releaseOnlyMapping())
			units := &unitFinderDouble{unit: scopeUnit(t, "parcel-1"), found: true}
			subject := newResultAdapter(t, test.chain(t), units, fixture.handler)
			err := subject.HandleReceivedExternalResult(t.Context(),
				veinbox.ReceivedExternalResult{TenantID: resultTenant, SourceID: test.tail})
			if !errors.Is(err, adapter.ErrExternalResultRecordInconsistent) {
				t.Fatalf("err = %v, want ErrExternalResultRecordInconsistent", err)
			}
			if len(fixture.facts.byKey) != 0 {
				t.Fatal("坏链上已经派生")
			}
		})
	}
}
