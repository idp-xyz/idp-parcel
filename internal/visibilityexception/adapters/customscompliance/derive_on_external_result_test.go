package customscompliance_test

import (
	"context"
	"errors"
	"testing"
	"time"

	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/customscompliance"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证外部结果 → VE 投影的消费侧翻译：一封外部结果只派生它自己那一层，放行层再按
// 全部 / 部分 / 附条件分开；沿结果范围→申报单元逐成员展开（ADR-0066）；同层冲突的那份
// 入账不派生；重放不多形成。派生编排用真实的 DeriveProjectionHandler，里程碑映射用按
// （源上下文，事实类型）键的替身——与生产目录同一个键，才证得出「按层区分」在键上做得到。

var resultOccurredAt = time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC)

const (
	resultTenant     = "tenant-1"
	resultSource     = "result-1"
	resultUnit       = "unit-1"
	resultVersion    = "version-1"
	clearedMilestone = "CUSTOMS_CLEARED"
	resultMapping    = "MAP-V1"
)

type resultFinderDouble struct {
	record  ccports.ExternalResultRecord
	found   bool
	err     error
	lastKey ccports.ExternalResultKey
}

func (double *resultFinderDouble) FindByKey(
	_ context.Context, key ccports.ExternalResultKey,
) (ccports.ExternalResultRecord, bool, error) {
	double.lastKey = key
	if double.err != nil {
		return ccports.ExternalResultRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

type unitFinderDouble struct {
	unit       ccdomain.DeclarationUnit
	found      bool
	err        error
	lastTenant ccdomain.TenantID
	lastUnit   ccdomain.DeclarationUnitID
}

func (double *unitFinderDouble) FindByID(
	_ context.Context, tenant ccdomain.TenantID, unit ccdomain.DeclarationUnitID,
) (ccdomain.DeclarationUnit, bool, error) {
	double.lastTenant = tenant
	double.lastUnit = unit
	if double.err != nil {
		return ccdomain.DeclarationUnit{}, false, double.err
	}
	return double.unit, double.found, nil
}

// kindKeyedMappingDouble 按（源上下文，事实类型）答归类，与生产目录的键同形：只登记了
// 的那一行归类，其余在同一映射版本下如实未归类。
type kindKeyedMappingDouble struct {
	rows map[string]string
}

func (double *kindKeyedMappingDouble) ClassifyFact(
	_ context.Context, fact vedomain.AcceptedSourceFact,
) (veports.MilestoneAnswer, bool, error) {
	mapping, err := vedomain.NewMappingVersionReference(resultMapping)
	if err != nil {
		return veports.MilestoneAnswer{}, false, err
	}
	milestone, found := double.rows[fact.Source().String()+"/"+fact.Kind().String()]
	if !found {
		return veports.MilestoneAnswer{Mapping: mapping}, true, nil
	}
	reference, err := vedomain.NewMilestoneReference(milestone)
	if err != nil {
		return veports.MilestoneAnswer{}, false, err
	}
	return veports.MilestoneAnswer{Milestone: reference, Classified: true, Mapping: mapping}, true, nil
}

// releaseOnlyMapping 摹写演示参考映射在关务这一格的形状：只有全部放行映成清关里程碑。
func releaseOnlyMapping() *kindKeyedMappingDouble {
	return &kindKeyedMappingDouble{rows: map[string]string{
		"CUSTOMS_COMPLIANCE/external-result-release-full": clearedMilestone,
	}}
}

type resultDeriveFixture struct {
	handler     *veapplication.DeriveProjectionHandler
	facts       *caseFactStoreDouble
	projections *caseProjectionStoreDouble
	identities  *caseProjectionIdentityDouble
}

func newResultDeriveFixture(t *testing.T, mapping veports.MilestoneMappingView) resultDeriveFixture {
	t.Helper()
	facts := &caseFactStoreDouble{byKey: map[veports.FactKey]veports.FactRecord{}}
	projections := &caseProjectionStoreDouble{byKey: map[string]vedomain.TrackingProjection{}}
	identities := &caseProjectionIdentityDouble{}
	handler := veapplication.NewDeriveProjectionHandler(veapplication.DeriveProjectionDeps{
		Facts:       facts,
		Mapping:     mapping,
		Projections: projections,
		Identities:  identities,
		Downstream:  &caseProjectionDownstreamDouble{},
		Clock:       caseClock{at: resultOccurredAt.Add(2 * time.Hour)},
	})
	return resultDeriveFixture{handler: handler, facts: facts, projections: projections, identities: identities}
}

func scopeUnit(t *testing.T, members ...string) ccdomain.DeclarationUnit {
	t.Helper()
	declared := make([]ccdomain.DeclaredParcelReference, 0, len(members))
	for _, member := range members {
		declared = append(declared, caseValue(t, ccdomain.NewDeclaredParcelReference, member))
	}
	unit, err := ccdomain.FormDeclarationUnit(
		caseValue(t, ccdomain.NewDeclarationUnitID, resultUnit),
		caseValue(t, ccdomain.NewCustomsCaseID, "case-1"),
		caseValue(t, ccdomain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		declared,
	)
	if err != nil {
		t.Fatalf("构造申报单元：%v", err)
	}
	return unit
}

// layerRecord 经领域真路径解释一层外部结果并装成接收记录；放行层另经放行事实构造器
// 拆出放行三件——夹具不手搓库里不可能出现的形状。
func layerRecord(t *testing.T, layer ccdomain.ResultLayer, release ccdomain.ReleaseKind) ccports.ExternalResultRecord {
	t.Helper()
	scope := caseValue(t, ccdomain.NewDecisionScopeReference, resultUnit)
	result, err := ccdomain.InterpretExternalResult(ccdomain.ExternalResultSpec{
		Layer:        layer,
		SourceID:     resultSource,
		Role:         caseValue(t, ccdomain.NewSourceAuthorityRole, "REGULATOR"),
		RawSemantics: "raw/" + layer.String(),
		Rule:         caseValue(t, ccdomain.NewInterpretationRuleReference, "rule-1"),
		Version:      caseValue(t, ccdomain.NewSubmissionVersionID, resultVersion),
		Attempt:      1,
		Scope:        scope,
		OccurredAt:   resultOccurredAt,
		ReceivedAt:   resultOccurredAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("解释外部结果：%v", err)
	}
	record := ccports.ExternalResultRecord{
		Key: ccports.ExternalResultKey{
			TenantID: caseValue(t, ccdomain.NewTenantID, resultTenant),
			SourceID: resultSource,
		},
		ContentDigest: "digest-result-1",
		Result:        result,
		RecordedAt:    resultOccurredAt.Add(2 * time.Minute),
	}
	if layer == ccdomain.ReleaseResultLayer {
		condition := ""
		if release == ccdomain.ConditionalRelease {
			condition = "guarantee posted"
		}
		outcome, err := ccdomain.ReceiveReleaseOutcome(
			release, caseValue(t, ccdomain.NewRegulatoryAuthorityReference, "US-CBP"),
			scope, condition, resultOccurredAt.Add(time.Minute))
		if err != nil {
			t.Fatalf("构造放行事实：%v", err)
		}
		record.Release = &outcome
	}
	return record
}

func receivedResultRef() veinbox.ReceivedExternalResult {
	return veinbox.ReceivedExternalResult{TenantID: resultTenant, SourceID: resultSource}
}

func resultFactKey(t *testing.T, member string) veports.FactKey {
	t.Helper()
	return veports.FactKey{
		Tenant:  caseValue(t, vedomain.NewTenantID, resultTenant),
		Source:  vedomain.SourceCustomsCompliance,
		Fact:    caseValue(t, vedomain.NewSourceFactReference, "external-result/"+member+"/"+resultSource),
		Version: caseValue(t, vedomain.NewSourceFactVersion, resultVersion),
	}
}

func onlyEntry(t *testing.T, projections *caseProjectionStoreDouble, member string) vedomain.MilestoneClassification {
	t.Helper()
	projection, found := projections.byKey[resultTenant+"/"+member]
	if !found {
		t.Fatalf("成员 %s 没派生出自己的投影", member)
	}
	entries := projection.Entries()
	if len(entries) != 1 {
		t.Fatalf("成员 %s entries = %d, want 1", member, len(entries))
	}
	return entries[0]
}

func newResultAdapter(
	t *testing.T, results adapter.ExternalResultFinder, units adapter.DeclarationUnitFinder, derive adapter.ResultProjectionHandler,
) *adapter.DeriveOnExternalResultAdapter {
	t.Helper()
	subject, err := adapter.NewDeriveOnExternalResultAdapter(results, units, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	return subject
}

// Covers: 放行层形成投影——全部放行在范围单元的每个成员上各落一份本层事实，命中「全部
// 放行」那一行映射归入清关里程碑；成员维与来源标识进引用、提交版本进版本维（ADR-0066）；
// 发生与有效取 CC 的业务发生或适用时间，接收取 CC 落记录的时刻。
func TestAFullReleaseDerivesAReleaseFactOnEveryMemberOfTheScopeUnit(t *testing.T) {
	fixture := newResultDeriveFixture(t, releaseOnlyMapping())
	units := &unitFinderDouble{unit: scopeUnit(t, "parcel-1", "parcel-2"), found: true}
	results := &resultFinderDouble{record: layerRecord(t, ccdomain.ReleaseResultLayer, ccdomain.FullRelease), found: true}
	subject := newResultAdapter(t, results, units, fixture.handler)

	if err := subject.HandleReceivedExternalResult(t.Context(), receivedResultRef()); err != nil {
		t.Fatalf("处理外部结果：%v", err)
	}

	if results.lastKey.TenantID.String() != resultTenant || results.lastKey.SourceID != resultSource {
		t.Fatalf("FindByKey 键 = %s/%s；租户与来源标识缺一不可", results.lastKey.TenantID, results.lastKey.SourceID)
	}
	if units.lastTenant.String() != resultTenant || units.lastUnit.String() != resultUnit {
		t.Fatalf("FindByID 键 = %s/%s；范围单元须按结果范围取、带租户", units.lastTenant, units.lastUnit)
	}
	for _, member := range []string{"parcel-1", "parcel-2"} {
		stored, found := fixture.facts.byKey[resultFactKey(t, member)]
		if !found {
			t.Fatalf("成员 %s 的事实没落库；成员维必须进事实引用", member)
		}
		fact := stored.Fact
		if fact.Kind().String() != "external-result-release-full" {
			t.Fatalf("成员 %s 的事实类型 = %s, want external-result-release-full", member, fact.Kind())
		}
		if !fact.OccurredAt().Equal(resultOccurredAt) || !fact.EffectiveAt().Equal(resultOccurredAt) ||
			!fact.ReceivedAt().Equal(resultOccurredAt.Add(2*time.Minute)) {
			t.Fatalf("成员 %s 的时间走样：occurred=%v effective=%v received=%v",
				member, fact.OccurredAt(), fact.EffectiveAt(), fact.ReceivedAt())
		}
		if _, superseding := fact.Supersedes(); superseding {
			t.Fatalf("成员 %s 的外部结果长出了前身——接收记录不在同一引用下换版", member)
		}
		milestone, classified := onlyEntry(t, fixture.projections, member).Milestone()
		if !classified || milestone.String() != clearedMilestone {
			t.Fatalf("成员 %s 归类 = %q（%v），want %s", member, milestone, classified, clearedMilestone)
		}
	}
}

// Covers: 非放行层不形成清关——每层只形成它自己的类型，映射只登记了全部放行时一律未归类；
// 一封外部结果不被当成「清关成功」（CC CONTEXT「不能使用一个“清关成功”状态覆盖各层事实」）。
// 部分与附条件放行同样各有自己的类型，不借全部放行那一行归类。
func TestEveryOtherLayerFormsOnlyItsOwnKindAndNeverTheClearedMilestone(t *testing.T) {
	for name, test := range map[string]struct {
		layer   ccdomain.ResultLayer
		release ccdomain.ReleaseKind
		kind    string
	}{
		"监管接收":   {ccdomain.RegulatoryReceiptLayer, ccdomain.ReleaseKindInvalid, "external-result-regulatory-receipt"},
		"业务受理":   {ccdomain.BusinessAcceptanceLayer, ccdomain.ReleaseKindInvalid, "external-result-business-acceptance"},
		"监管过程决定": {ccdomain.ProcessDecisionLayer, ccdomain.ReleaseKindInvalid, "external-result-process-decision"},
		"监管核定税费": {ccdomain.AssessedDutyLayer, ccdomain.ReleaseKindInvalid, "external-result-assessed-duty"},
		"监管处置决定": {ccdomain.DispositionDecisionLayer, ccdomain.ReleaseKindInvalid, "external-result-disposition-decision"},
		"部分放行":   {ccdomain.ReleaseResultLayer, ccdomain.PartialRelease, "external-result-release-partial"},
		"附条件放行":  {ccdomain.ReleaseResultLayer, ccdomain.ConditionalRelease, "external-result-release-conditional"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newResultDeriveFixture(t, releaseOnlyMapping())
			subject := newResultAdapter(t,
				&resultFinderDouble{record: layerRecord(t, test.layer, test.release), found: true},
				&unitFinderDouble{unit: scopeUnit(t, "parcel-1"), found: true},
				fixture.handler)

			if err := subject.HandleReceivedExternalResult(t.Context(), receivedResultRef()); err != nil {
				t.Fatalf("处理外部结果：%v", err)
			}
			entry := onlyEntry(t, fixture.projections, "parcel-1")
			if entry.Fact().Kind().String() != test.kind {
				t.Fatalf("事实类型 = %s, want %s", entry.Fact().Kind(), test.kind)
			}
			if milestone, classified := entry.Milestone(); classified {
				t.Fatalf("%s 归进了 %q——只登记了全部放行的映射不得把别的层映成清关", name, milestone)
			}
			if entry.MappingVersion().String() != resultMapping {
				t.Fatalf("mapping = %q, want %s（同一映射版本下如实未归类）", entry.MappingVersion(), resultMapping)
			}
		})
	}
}

// Covers: 重放不多形成——同一封信再处理一次，各成员答已有结果，不再签发投影版本。
func TestARedeliveredExternalResultDerivesNothingMore(t *testing.T) {
	fixture := newResultDeriveFixture(t, releaseOnlyMapping())
	subject := newResultAdapter(t,
		&resultFinderDouble{record: layerRecord(t, ccdomain.ReleaseResultLayer, ccdomain.FullRelease), found: true},
		&unitFinderDouble{unit: scopeUnit(t, "parcel-1", "parcel-2"), found: true},
		fixture.handler)

	for round := 1; round <= 2; round++ {
		if err := subject.HandleReceivedExternalResult(t.Context(), receivedResultRef()); err != nil {
			t.Fatalf("第 %d 次处理：%v", round, err)
		}
	}
	if fixture.identities.n != 2 {
		t.Fatalf("签发投影版本 %d 次, want 2（每成员一次，重放不再签发）", fixture.identities.n)
	}
	if len(fixture.facts.byKey) != 2 {
		t.Fatalf("事实行 = %d, want 2", len(fixture.facts.byKey))
	}
	_ = onlyEntry(t, fixture.projections, "parcel-1")
	_ = onlyEntry(t, fixture.projections, "parcel-2")
}

// Covers: 同层冲突的那份入账不派生——CC 的当前判断不因它改变，投影也不让它改掉包裹当前
// 的关务进展；连范围单元都不必去取。
func TestALayerConflictRecordSettlesWithoutAProjection(t *testing.T) {
	record := layerRecord(t, ccdomain.ReleaseResultLayer, ccdomain.FullRelease)
	record.LayerConflict = true
	units := &unitFinderDouble{unit: scopeUnit(t, "parcel-1"), found: true}
	derive := &caseCountingHandler{}
	subject := newResultAdapter(t, &resultFinderDouble{record: record, found: true}, units, derive)

	if err := subject.HandleReceivedExternalResult(t.Context(), receivedResultRef()); err != nil {
		t.Fatalf("同层冲突应入账收工：%v", err)
	}
	if derive.calls != 0 {
		t.Fatal("同层冲突的那份不该走到派生")
	}
	if units.lastUnit.String() != "" {
		t.Fatal("同层冲突的那份不该去取范围单元")
	}
}

// Covers: ADR-0066 决定三——某成员未决即整封报错回滚重投，不留部分状态。
func TestAMemberUndecidedRollsBackTheWholeExternalResultEnvelope(t *testing.T) {
	fixture := newResultDeriveFixture(t, &caseMappingViewDouble{err: errors.New("mapping view unavailable")})
	subject := newResultAdapter(t,
		&resultFinderDouble{record: layerRecord(t, ccdomain.ReleaseResultLayer, ccdomain.FullRelease), found: true},
		&unitFinderDouble{unit: scopeUnit(t, "parcel-1", "parcel-2"), found: true},
		fixture.handler)

	if err := subject.HandleReceivedExternalResult(t.Context(), receivedResultRef()); !errors.Is(err, adapter.ErrProjectionUndecided) {
		t.Fatalf("err = %v, want ErrProjectionUndecided", err)
	}
	if len(fixture.projections.byKey) != 0 {
		t.Fatal("未决之下不该有任何成员的投影落下")
	}
}

// Covers: ADR-0066 决定三——单成员业务终局（来源冲突）入账继续，其余成员照常派生。
func TestASingleMemberConflictDoesNotBlockTheRestOfTheExternalResult(t *testing.T) {
	fixture := newResultDeriveFixture(t, releaseOnlyMapping())
	conflicting := resultFactKey(t, "parcel-1")
	fixture.facts.byKey[conflicting] = veports.FactRecord{Key: conflicting, ContentDigest: "someone-else"}
	subject := newResultAdapter(t,
		&resultFinderDouble{record: layerRecord(t, ccdomain.ReleaseResultLayer, ccdomain.FullRelease), found: true},
		&unitFinderDouble{unit: scopeUnit(t, "parcel-1", "parcel-2"), found: true},
		fixture.handler)

	if err := subject.HandleReceivedExternalResult(t.Context(), receivedResultRef()); err != nil {
		t.Fatalf("单成员冲突不该拖垮整封：%v", err)
	}
	if _, found := fixture.projections.byKey[resultTenant+"/parcel-1"]; found {
		t.Fatal("冲突成员不该派生投影")
	}
	_ = onlyEntry(t, fixture.projections, "parcel-2")
}

// Covers: 缺件是续办——接收记录或范围单元读不动、读不回都落各自的未决哨兵，不走到派生。
func TestMissingDependenciesAreContinuableUndecided(t *testing.T) {
	full := layerRecord(t, ccdomain.ReleaseResultLayer, ccdomain.FullRelease)
	unit := scopeUnit(t, "parcel-1")
	for name, test := range map[string]struct {
		results *resultFinderDouble
		units   *unitFinderDouble
		want    error
	}{
		"接收记录读不动": {&resultFinderDouble{err: errors.New("store unavailable")},
			&unitFinderDouble{unit: unit, found: true}, adapter.ErrExternalResultNotVisible},
		"接收记录读不回": {&resultFinderDouble{},
			&unitFinderDouble{unit: unit, found: true}, adapter.ErrExternalResultNotVisible},
		"范围单元读不动": {&resultFinderDouble{record: full, found: true},
			&unitFinderDouble{err: errors.New("store unavailable")}, adapter.ErrResultScopeUnitNotVisible},
		"范围单元读不回": {&resultFinderDouble{record: full, found: true},
			&unitFinderDouble{}, adapter.ErrResultScopeUnitNotVisible},
	} {
		t.Run(name, func(t *testing.T) {
			derive := &caseCountingHandler{}
			subject := newResultAdapter(t, test.results, test.units, derive)
			if err := subject.HandleReceivedExternalResult(t.Context(), receivedResultRef()); !errors.Is(err, test.want) {
				t.Fatalf("err = %v, want %v", err, test.want)
			}
			if derive.calls != 0 {
				t.Fatal("缺件不该走到派生")
			}
		})
	}
}

// Covers: 仓储不变量已破的各格响亮报错、不进未决，也不走到派生。
func TestBrokenRecordsAreInconsistentNotUndecided(t *testing.T) {
	foreignUnit, err := ccdomain.FormDeclarationUnit(
		caseValue(t, ccdomain.NewDeclarationUnitID, "unit-other"),
		caseValue(t, ccdomain.NewCustomsCaseID, "case-1"),
		caseValue(t, ccdomain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		[]ccdomain.DeclaredParcelReference{caseValue(t, ccdomain.NewDeclaredParcelReference, "parcel-9")},
	)
	if err != nil {
		t.Fatalf("构造另一个申报单元：%v", err)
	}
	for name, test := range map[string]struct {
		mutate func(*ccports.ExternalResultRecord)
		unit   ccdomain.DeclarationUnit
	}{
		"记录指着另一个键": {mutate: func(record *ccports.ExternalResultRecord) {
			record.Key.SourceID = "result-other"
		}},
		"归属不上的留存": {mutate: func(record *ccports.ExternalResultRecord) {
			record.Unattributable = true
		}},
		"落记录时刻为零": {mutate: func(record *ccports.ExternalResultRecord) {
			record.RecordedAt = time.Time{}
		}},
		"放行层缺放行三件": {mutate: func(record *ccports.ExternalResultRecord) {
			record.Release = nil
		}},
		"非放行层带放行三件": {mutate: func(record *ccports.ExternalResultRecord) {
			release := record.Release
			*record = layerRecord(t, ccdomain.BusinessAcceptanceLayer, ccdomain.ReleaseKindInvalid)
			record.Release = release
		}},
		"放行范围与结果范围不符": {mutate: func(record *ccports.ExternalResultRecord) {
			other, err := ccdomain.ReceiveReleaseOutcome(ccdomain.FullRelease,
				caseValue(t, ccdomain.NewRegulatoryAuthorityReference, "US-CBP"),
				caseValue(t, ccdomain.NewDecisionScopeReference, "unit-other"), "", resultOccurredAt)
			if err != nil {
				t.Fatalf("构造放行事实：%v", err)
			}
			record.Release = &other
		}},
		"范围单元答非所问": {unit: foreignUnit},
	} {
		t.Run(name, func(t *testing.T) {
			record := layerRecord(t, ccdomain.ReleaseResultLayer, ccdomain.FullRelease)
			if test.mutate != nil {
				test.mutate(&record)
			}
			unit := test.unit
			if unit.ID().String() == "" {
				unit = scopeUnit(t, "parcel-1")
			}
			derive := &caseCountingHandler{}
			subject := newResultAdapter(t,
				&resultFinderDouble{record: record, found: true},
				&unitFinderDouble{unit: unit, found: true},
				derive)
			if err := subject.HandleReceivedExternalResult(
				t.Context(), receivedResultRef()); !errors.Is(err, adapter.ErrExternalResultRecordInconsistent) {
				t.Fatalf("err = %v, want ErrExternalResultRecordInconsistent", err)
			}
			if derive.calls != 0 {
				t.Fatal("不变量已破不该走到派生")
			}
		})
	}
}

func TestAnUntranslatableExternalResultReferenceKeepsItsSentinel(t *testing.T) {
	derive := &caseCountingHandler{}
	subject := newResultAdapter(t,
		&resultFinderDouble{record: layerRecord(t, ccdomain.ReleaseResultLayer, ccdomain.FullRelease), found: true},
		&unitFinderDouble{unit: scopeUnit(t, "parcel-1"), found: true},
		derive)
	for name, reference := range map[string]veinbox.ReceivedExternalResult{
		"空租户":   {SourceID: resultSource},
		"空来源标识": {TenantID: resultTenant},
	} {
		t.Run(name, func(t *testing.T) {
			if err := subject.HandleReceivedExternalResult(
				t.Context(), reference); !errors.Is(err, adapter.ErrExternalResultUntranslatableAnswer) {
				t.Fatalf("err = %v, want ErrExternalResultUntranslatableAnswer", err)
			}
		})
	}
	if derive.calls != 0 {
		t.Fatal("引用译不出来不该走到派生")
	}
}

// Covers: 缺依赖拒——接收记录读口、范围单元读口与派生编排缺一即拒绝构造。
func TestTheExternalResultAdapterRefusesMissingCollaborators(t *testing.T) {
	results := &resultFinderDouble{}
	units := &unitFinderDouble{}
	derive := &caseCountingHandler{}
	if _, err := adapter.NewDeriveOnExternalResultAdapter(nil, units, derive); err == nil {
		t.Fatal("缺接收记录读口必须拒绝构造")
	}
	if _, err := adapter.NewDeriveOnExternalResultAdapter(results, nil, derive); err == nil {
		t.Fatal("缺范围单元读口必须拒绝构造")
	}
	if _, err := adapter.NewDeriveOnExternalResultAdapter(results, units, nil); err == nil {
		t.Fatal("缺派生编排必须拒绝构造")
	}
}
