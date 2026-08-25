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
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var submissionFixedAt = time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)

type submissionFinderDouble struct {
	record      ccports.DeclarationSubmissionRecord
	found       bool
	err         error
	lastTenant  ccdomain.TenantID
	lastVersion ccdomain.SubmissionVersionID
}

func (double *submissionFinderDouble) FindByVersion(
	_ context.Context, tenant ccdomain.TenantID, version ccdomain.SubmissionVersionID,
) (ccports.DeclarationSubmissionRecord, bool, error) {
	double.lastTenant = tenant
	double.lastVersion = version
	if double.err != nil {
		return ccports.DeclarationSubmissionRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

// fixedSubmissionRecord 经领域真路径造一份提交记录：单元成形 → 就绪+授权双有效 →
// 成版 → 首次尝试。夹具不手搓版本——成版门（双有效、组成快照）是提供方的不变量，
// 绕开它等于造一个库里不可能出现的形状。
func fixedSubmissionRecord(t *testing.T, members ...string) ccports.DeclarationSubmissionRecord {
	t.Helper()
	unitID := caseValue(t, ccdomain.NewDeclarationUnitID, "unit-1")
	procedure := caseValue(t, ccdomain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86")
	declared := make([]ccdomain.DeclaredParcelReference, 0, len(members))
	for _, member := range members {
		declared = append(declared, caseValue(t, ccdomain.NewDeclaredParcelReference, member))
	}
	unit, err := ccdomain.FormDeclarationUnit(unitID,
		caseValue(t, ccdomain.NewCustomsCaseID, "case-1"), procedure, declared)
	if err != nil {
		t.Fatalf("构造申报单元：%v", err)
	}
	readiness, err := ccdomain.JudgeReady(
		unitID,
		caseValue(t, ccdomain.NewReadinessBasisReference, "readiness/check-1"),
		submissionFixedAt.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("构造就绪判断：%v", err)
	}
	authorization, err := ccdomain.GrantSubmissionAuthority(
		unitID,
		caseValue(t, ccdomain.NewSubmissionAuthorityReference, "authority/grant-1"),
		submissionFixedAt.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("构造提交授权：%v", err)
	}
	fixed, err := ccdomain.FixSubmissionVersion(ccdomain.CustomsSubmissionVersionSpec{
		ID:            caseValue(t, ccdomain.NewSubmissionVersionID, "version-1"),
		Unit:          unit,
		Dossier:       caseValue(t, ccdomain.NewDossierSnapshotReference, "dossier/snap-1"),
		Roles:         caseValue(t, ccdomain.NewRoleSnapshotReference, "roles/snap-1"),
		Readiness:     readiness,
		Authorization: authorization,
		FixedAt:       submissionFixedAt,
	})
	if err != nil {
		t.Fatalf("固定提交版本：%v", err)
	}
	attempt, err := ccdomain.InitialAttempt(
		fixed, "customs-gateway/prod", ccdomain.AttemptPendingConfirmation, submissionFixedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("构造首次尝试：%v", err)
	}
	return ccports.DeclarationSubmissionRecord{
		Key: ccports.DeclarationSubmissionKey{
			TenantID:  caseValue(t, ccdomain.NewTenantID, "tenant-1"),
			Unit:      unitID,
			Procedure: procedure,
		},
		ContentDigest: "digest-submission-1",
		Version:       fixed,
		Attempt:       attempt,
		RecordedAt:    submissionFixedAt.Add(2 * time.Minute),
	}
}

func formedSubmissionRef() veinbox.FormedDeclarationSubmission {
	return veinbox.FormedDeclarationSubmission{
		TenantID:  "tenant-1",
		UnitID:    "unit-1",
		Procedure: "US-IMPORT/TYPE-86",
		VersionID: "version-1",
	}
}

func submissionFactKey(t *testing.T, member string) veports.FactKey {
	t.Helper()
	return veports.FactKey{
		Tenant: caseValue(t, vedomain.NewTenantID, "tenant-1"),
		Source: vedomain.SourceCustomsCompliance,
		Fact: caseValue(t, vedomain.NewSourceFactReference,
			"declaration-submission/"+member+"/unit-1/US-IMPORT/TYPE-86"),
		Version: caseValue(t, vedomain.NewSourceFactVersion, "version-1"),
	}
}

func TestAMissingDeclarationSubmissionIsContinuableUndecided(t *testing.T) {
	derive := &caseCountingHandler{}
	subject, err := adapter.NewDeriveOnDeclarationSubmissionAdapter(&submissionFinderDouble{}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedDeclarationSubmission(t.Context(), formedSubmissionRef()); !errors.Is(err, adapter.ErrDeclarationSubmissionNotVisible) {
		t.Fatalf("err = %v, want ErrDeclarationSubmissionNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("缺提交记录不该走到派生")
	}
}

func TestAnUnreadableDeclarationSubmissionIsContinuableUndecided(t *testing.T) {
	derive := &caseCountingHandler{}
	subject, err := adapter.NewDeriveOnDeclarationSubmissionAdapter(
		&submissionFinderDouble{err: errors.New("store unavailable")}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedDeclarationSubmission(t.Context(), formedSubmissionRef()); !errors.Is(err, adapter.ErrDeclarationSubmissionNotVisible) {
		t.Fatalf("err = %v, want ErrDeclarationSubmissionNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("读失败不该走到派生")
	}
}

func TestAMismatchedDeclarationSubmissionKeyIsInconsistentNotUndecided(t *testing.T) {
	record := fixedSubmissionRecord(t, "parcel-1")
	record.Key.Unit = caseValue(t, ccdomain.NewDeclarationUnitID, "unit-other")
	derive := &caseCountingHandler{}
	subject, err := adapter.NewDeriveOnDeclarationSubmissionAdapter(
		&submissionFinderDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedDeclarationSubmission(t.Context(), formedSubmissionRef()); !errors.Is(err, adapter.ErrDeclarationSubmissionRecordInconsistent) {
		t.Fatalf("err = %v, want ErrDeclarationSubmissionRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("键本体不符不该走到派生")
	}
}

// Covers: 版本维核对——按版本取回的行必须就是所请求的那一版，读口答非所问即仓储
// 不变量已破，响亮报错，不静默派生另一代。
func TestAStoredVersionDifferentFromTheEnvelopeIsInconsistent(t *testing.T) {
	record := fixedSubmissionRecord(t, "parcel-1")
	derive := &caseCountingHandler{}
	subject, err := adapter.NewDeriveOnDeclarationSubmissionAdapter(
		&submissionFinderDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	reference := formedSubmissionRef()
	reference.VersionID = "version-2"
	if err := subject.HandleFormedDeclarationSubmission(t.Context(), reference); !errors.Is(err, adapter.ErrDeclarationSubmissionRecordInconsistent) {
		t.Fatalf("err = %v, want ErrDeclarationSubmissionRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("版本不符不该走到派生")
	}
}

func TestADeclarationSubmissionWithZeroRecordedAtIsInconsistentNotUndecided(t *testing.T) {
	record := fixedSubmissionRecord(t, "parcel-1")
	record.RecordedAt = time.Time{}
	derive := &caseCountingHandler{}
	subject, err := adapter.NewDeriveOnDeclarationSubmissionAdapter(
		&submissionFinderDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedDeclarationSubmission(t.Context(), formedSubmissionRef()); !errors.Is(err, adapter.ErrDeclarationSubmissionRecordInconsistent) {
		t.Fatalf("err = %v, want ErrDeclarationSubmissionRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("时间为零不该走到派生")
	}
}

func TestAnUntranslatableDeclarationSubmissionReferenceKeepsItsSentinel(t *testing.T) {
	derive := &caseCountingHandler{}
	subject, err := adapter.NewDeriveOnDeclarationSubmissionAdapter(
		&submissionFinderDouble{record: fixedSubmissionRecord(t, "parcel-1"), found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	for name, reference := range map[string]veinbox.FormedDeclarationSubmission{
		"空租户": {UnitID: "unit-1", Procedure: "US-IMPORT/TYPE-86", VersionID: "version-1"},
		"空单元": {TenantID: "tenant-1", Procedure: "US-IMPORT/TYPE-86", VersionID: "version-1"},
		"空程序": {TenantID: "tenant-1", UnitID: "unit-1", VersionID: "version-1"},
		"空版本": {TenantID: "tenant-1", UnitID: "unit-1", Procedure: "US-IMPORT/TYPE-86"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := subject.HandleFormedDeclarationSubmission(
				t.Context(), reference); !errors.Is(err, adapter.ErrDeclarationSubmissionUntranslatableAnswer) {
				t.Fatalf("err = %v, want ErrDeclarationSubmissionUntranslatableAnswer", err)
			}
		})
	}
	if derive.calls != 0 {
		t.Fatal("引用译不出来不该走到派生")
	}
}

// Covers: 取数走版本维——租户是隔离边界必须在签名上，版本是信封宣告的那一版；目标
// 三维键只作交叉核对，不再是取数键（多版本后按键只答当前版，迟到重放旧版会读串）。
func TestDeclarationSubmissionLookupCarriesTenantAndVersion(t *testing.T) {
	finder := &submissionFinderDouble{record: fixedSubmissionRecord(t, "parcel-1"), found: true}
	handler, _, _ := caseDeriveHandler(t, caseMappingViewDouble{configured: false}, caseProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnDeclarationSubmissionAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedDeclarationSubmission(t.Context(), formedSubmissionRef()); err != nil {
		t.Fatalf("处理提交：%v", err)
	}
	if finder.lastTenant.String() != "tenant-1" || finder.lastVersion.String() != "version-1" {
		t.Fatalf("FindByVersion 键 = %s/%s；租户与版本缺一不可",
			finder.lastTenant, finder.lastVersion)
	}
}

// Covers: 来源事实替代关系由源上下文随更正一并给出（VE CONTEXT 词条）——更正版记录
// 携带 CorrectedFrom，逐成员事实登记 Supersedes 指向前身版本；本适配器只转写不推断。
func TestACorrectedSubmissionCarriesItsSupersededVersion(t *testing.T) {
	record := fixedSubmissionRecord(t, "parcel-1")
	record.CorrectedFrom = caseValue(t, ccdomain.NewSubmissionVersionID, "version-0")
	finder := &submissionFinderDouble{record: record, found: true}
	handler, facts, _ := caseDeriveHandler(t,
		caseMappingViewDouble{configured: false}, caseProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnDeclarationSubmissionAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedDeclarationSubmission(t.Context(), formedSubmissionRef()); err != nil {
		t.Fatalf("处理更正版：%v", err)
	}
	stored, found := facts.byKey[submissionFactKey(t, "parcel-1")]
	if !found {
		t.Fatal("更正版成员事实没落库")
	}
	supersedes, superseding := stored.Fact.Supersedes()
	if !superseding || supersedes.String() != "version-0" {
		t.Fatalf("替代关系 = %q（在场 %v），want version-0；由源上下文给出的前身必须逐字登记",
			supersedes, superseding)
	}
}

// Covers: ADR-0066 决定一与二——一封信 N 个成员在消费侧逐成员派生，成员维编进事实
// 引用；引用跨版本稳定（单元+程序），版本走真版本维（提交版本标识）。
func TestAFormedSubmissionDerivesOneProjectionCommandPerMember(t *testing.T) {
	finder := &submissionFinderDouble{record: fixedSubmissionRecord(t, "parcel-1", "parcel-2"), found: true}
	handler, facts, projections := caseDeriveHandler(t,
		caseMappingViewDouble{configured: false}, caseProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnDeclarationSubmissionAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := subject.HandleFormedDeclarationSubmission(t.Context(), formedSubmissionRef()); err != nil {
		t.Fatalf("处理提交：%v", err)
	}

	for _, member := range []string{"parcel-1", "parcel-2"} {
		record, found := facts.byKey[submissionFactKey(t, member)]
		if !found {
			t.Fatalf("成员 %s 的事实没落库；成员维必须进事实引用，版本必须走版本维", member)
		}
		fact := record.Fact
		if fact.Parcel().String() != member {
			t.Fatalf("成员 %s 的事实包裹 = %s", member, fact.Parcel())
		}
		if fact.Kind().String() != "declaration-submission-formed" {
			t.Fatalf("成员 %s 的事实类型 = %s", member, fact.Kind())
		}
		if fact.Version().String() != "version-1" {
			t.Fatalf("成员 %s 的事实版本 = %s；提交版本标识是真版本维", member, fact.Version())
		}
		if !fact.OccurredAt().Equal(submissionFixedAt) ||
			!fact.ReceivedAt().Equal(submissionFixedAt.Add(2*time.Minute)) {
			t.Fatalf("成员 %s 的时间走样：occurred=%v received=%v",
				member, fact.OccurredAt(), fact.ReceivedAt())
		}
		if _, superseded := fact.Supersedes(); superseded {
			t.Fatalf("成员 %s 的首登事实长出了前身", member)
		}
		if _, found := projections.byKey["tenant-1/"+member]; !found {
			t.Fatalf("成员 %s 没派生出自己的投影", member)
		}
	}
}

// Covers: ADR-0066 决定三——单成员业务终局（来源冲突）入账继续，其余成员照常派生。
func TestASingleMemberConflictDoesNotBlockTheRestOfTheSubmission(t *testing.T) {
	finder := &submissionFinderDouble{record: fixedSubmissionRecord(t, "parcel-1", "parcel-2"), found: true}
	handler, facts, projections := caseDeriveHandler(t,
		caseMappingViewDouble{configured: false}, caseProjectionDownstreamDouble{})
	conflicting := submissionFactKey(t, "parcel-1")
	facts.byKey[conflicting] = veports.FactRecord{Key: conflicting, ContentDigest: "someone-else"}

	subject, err := adapter.NewDeriveOnDeclarationSubmissionAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedDeclarationSubmission(t.Context(), formedSubmissionRef()); err != nil {
		t.Fatalf("单成员冲突不该拖垮整封：%v", err)
	}

	if _, found := projections.byKey["tenant-1/parcel-1"]; found {
		t.Fatal("冲突成员不该派生投影")
	}
	if _, found := projections.byKey["tenant-1/parcel-2"]; !found {
		t.Fatal("其余成员该照常派生")
	}
}

// Covers: ADR-0066 决定三——某成员未决即整封报错回滚重投，不留部分状态在消费账上。
func TestAMemberUndecidedRollsBackTheWholeSubmissionEnvelope(t *testing.T) {
	finder := &submissionFinderDouble{record: fixedSubmissionRecord(t, "parcel-1", "parcel-2"), found: true}
	handler, _, projections := caseDeriveHandler(t,
		caseMappingViewDouble{err: errors.New("mapping view unavailable")},
		caseProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnDeclarationSubmissionAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := subject.HandleFormedDeclarationSubmission(t.Context(), formedSubmissionRef()); !errors.Is(err, adapter.ErrProjectionUndecided) {
		t.Fatalf("err = %v, want ErrProjectionUndecided", err)
	}
	if len(projections.byKey) != 0 {
		t.Fatal("未决之下不该有任何成员的投影落下")
	}
}
