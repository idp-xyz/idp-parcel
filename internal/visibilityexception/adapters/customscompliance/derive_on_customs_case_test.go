package customscompliance_test

import (
	"context"
	"errors"
	"fmt"
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

var caseEstablishedAt = time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)

func caseValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type caseFinderDouble struct {
	customsCase ccdomain.CustomsCase
	found       bool
	err         error
	last        ccports.CustomsCaseKey
}

func (double *caseFinderDouble) FindByKey(
	_ context.Context, key ccports.CustomsCaseKey,
) (ccdomain.CustomsCase, bool, error) {
	double.last = key
	if double.err != nil {
		return ccdomain.CustomsCase{}, false, double.err
	}
	return double.customsCase, double.found, nil
}

type caseCountingHandler struct {
	calls int
}

func (double *caseCountingHandler) Handle(
	context.Context, veapplication.DeriveProjectionCommand,
) (veapplication.DeriveProjectionResult, error) {
	double.calls++
	return veapplication.DeriveProjectionResult{}, errors.New("derive should not be called")
}

type caseFactStoreDouble struct {
	byKey map[veports.FactKey]veports.FactRecord
}

func (double *caseFactStoreDouble) FindByKey(_ context.Context, key veports.FactKey) (veports.FactRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *caseFactStoreDouble) FindByParcel(
	_ context.Context,
	tenant vedomain.TenantID,
	parcel vedomain.TrackedParcelReference,
) ([]veports.FactRecord, error) {
	records := make([]veports.FactRecord, 0)
	for _, record := range double.byKey {
		if record.Key.Tenant == tenant && record.Fact.Parcel() == parcel {
			records = append(records, record)
		}
	}
	return records, nil
}

func (double *caseFactStoreDouble) Save(_ context.Context, record veports.FactRecord) (veports.FactSaveOutcome, error) {
	if _, exists := double.byKey[record.Key]; exists {
		return veports.FactAlreadyRecorded, nil
	}
	double.byKey[record.Key] = record
	return veports.FactSaved, nil
}

type caseMappingViewDouble struct {
	configured bool
	err        error
}

func (double *caseMappingViewDouble) ClassifyFact(
	_ context.Context,
	_ vedomain.AcceptedSourceFact,
) (veports.MilestoneAnswer, bool, error) {
	if double.err != nil {
		return veports.MilestoneAnswer{}, false, double.err
	}
	return veports.MilestoneAnswer{}, double.configured, nil
}

type caseProjectionStoreDouble struct {
	byKey map[string]vedomain.TrackingProjection
}

func (double *caseProjectionStoreDouble) FindCurrent(
	_ context.Context,
	tenant vedomain.TenantID,
	parcel vedomain.TrackedParcelReference,
) (vedomain.TrackingProjection, bool, error) {
	projection, found := double.byKey[tenant.String()+"/"+parcel.String()]
	return projection, found, nil
}

// 派生编排不消费按版本读回的口（那是审计与读侧的入口）；替身只存当前版，如实答未找到。
func (double *caseProjectionStoreDouble) FindByVersion(
	_ context.Context,
	_ vedomain.TenantID,
	_ vedomain.ProjectionVersionID,
) (vedomain.TrackingProjection, bool, error) {
	return vedomain.TrackingProjection{}, false, nil
}

func (double *caseProjectionStoreDouble) Save(
	_ context.Context, tenant vedomain.TenantID, projection vedomain.TrackingProjection,
) error {
	double.byKey[tenant.String()+"/"+projection.Parcel().String()] = projection
	return nil
}

// caseProjectionIdentityDouble 摹写真实签发器「每次一个不重的新标识」的性质：一封信
// N 个成员要 N 个投影版本，固定返回值会让第二个成员撞领域门。
type caseProjectionIdentityDouble struct{ n int }

func (double *caseProjectionIdentityDouble) NextProjectionVersionID(_ context.Context) (vedomain.ProjectionVersionID, error) {
	double.n++
	return vedomain.NewProjectionVersionID(fmt.Sprintf("projection-case-%d", double.n))
}

type caseProjectionDownstreamDouble struct {
	err error
}

func (double *caseProjectionDownstreamDouble) HandOffProjection(
	context.Context, veports.ProjectionHandoffIntent,
) error {
	return double.err
}

type caseClock struct{ at time.Time }

func (clock caseClock) Now() time.Time { return clock.at }

func establishedCase(t *testing.T, parcels ...string) ccdomain.CustomsCase {
	t.Helper()
	associations := make([]ccdomain.CaseParcelAssociation, 0, len(parcels))
	for _, parcel := range parcels {
		associations = append(associations, ccdomain.CaseParcelAssociation{
			Parcel:    parcel,
			Customer:  "customer-1",
			SourceRef: "shipment-request/" + parcel,
		})
	}
	customsCase, err := ccdomain.EstablishCustomsCase(ccdomain.CustomsCaseSpec{
		ID:            caseValue(t, ccdomain.NewCustomsCaseID, "case-1"),
		Jurisdiction:  caseValue(t, ccdomain.NewRegulatoryJurisdictionReference, "US-CBP"),
		Direction:     ccdomain.ImportManifest,
		Procedure:     caseValue(t, ccdomain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		Obligation:    caseValue(t, ccdomain.NewObligationScopeReference, "obligation-1"),
		Parcels:       associations,
		EstablishedAt: caseEstablishedAt,
	})
	if err != nil {
		t.Fatalf("构造关务案件：%v", err)
	}
	return customsCase
}

func establishedCaseRef() veinbox.EstablishedCustomsCase {
	return veinbox.EstablishedCustomsCase{
		TenantID:     "tenant-1",
		Jurisdiction: "US-CBP",
		Direction:    "IMPORT",
		Procedure:    "US-IMPORT/TYPE-86",
		Obligation:   "obligation-1",
	}
}

func caseFactKey(t *testing.T, member string) veports.FactKey {
	t.Helper()
	return veports.FactKey{
		Tenant: caseValue(t, vedomain.NewTenantID, "tenant-1"),
		Source: vedomain.SourceCustomsCompliance,
		Fact: caseValue(t, vedomain.NewSourceFactReference,
			"customs-case/"+member+"/case-1"),
		Version: caseValue(t, vedomain.NewSourceFactVersion, "case-1"),
	}
}

func caseDeriveHandler(t *testing.T, mapping caseMappingViewDouble, downstream caseProjectionDownstreamDouble) (
	*veapplication.DeriveProjectionHandler,
	*caseFactStoreDouble,
	*caseProjectionStoreDouble,
) {
	t.Helper()
	facts := &caseFactStoreDouble{byKey: map[veports.FactKey]veports.FactRecord{}}
	projections := &caseProjectionStoreDouble{byKey: map[string]vedomain.TrackingProjection{}}
	handler := veapplication.NewDeriveProjectionHandler(veapplication.DeriveProjectionDeps{
		Facts:       facts,
		Mapping:     &mapping,
		Projections: projections,
		Identities:  &caseProjectionIdentityDouble{},
		Downstream:  &downstream,
		Clock:       caseClock{at: caseEstablishedAt.Add(2 * time.Hour)},
	})
	return handler, facts, projections
}

func TestAMissingCustomsCaseIsContinuableUndecided(t *testing.T) {
	derive := &caseCountingHandler{}
	subject, err := adapter.NewDeriveOnCustomsCaseAdapter(&caseFinderDouble{}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleEstablishedCustomsCase(t.Context(), establishedCaseRef()); !errors.Is(err, adapter.ErrCustomsCaseNotVisible) {
		t.Fatalf("err = %v, want ErrCustomsCaseNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("缺案件记录不该走到派生")
	}
}

func TestAnUnreadableCustomsCaseIsContinuableUndecided(t *testing.T) {
	derive := &caseCountingHandler{}
	subject, err := adapter.NewDeriveOnCustomsCaseAdapter(
		&caseFinderDouble{err: errors.New("store unavailable")}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleEstablishedCustomsCase(t.Context(), establishedCaseRef()); !errors.Is(err, adapter.ErrCustomsCaseNotVisible) {
		t.Fatalf("err = %v, want ErrCustomsCaseNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("读失败不该走到派生")
	}
}

func TestAMismatchedCustomsCaseIsInconsistentNotUndecided(t *testing.T) {
	derive := &caseCountingHandler{}
	subject, err := adapter.NewDeriveOnCustomsCaseAdapter(
		&caseFinderDouble{customsCase: establishedCase(t, "parcel-1"), found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	reference := establishedCaseRef()
	reference.Obligation = "obligation-other"
	if err := subject.HandleEstablishedCustomsCase(t.Context(), reference); !errors.Is(err, adapter.ErrCustomsCaseRecordInconsistent) {
		t.Fatalf("err = %v, want ErrCustomsCaseRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("键本体不符不该走到派生")
	}
}

func TestAnUntranslatableCustomsCaseReferenceKeepsItsSentinel(t *testing.T) {
	derive := &caseCountingHandler{}
	subject, err := adapter.NewDeriveOnCustomsCaseAdapter(
		&caseFinderDouble{customsCase: establishedCase(t, "parcel-1"), found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	for name, reference := range map[string]veinbox.EstablishedCustomsCase{
		"空租户":      {Jurisdiction: "US-CBP", Direction: "IMPORT", Procedure: "US-IMPORT/TYPE-86", Obligation: "obligation-1"},
		"空辖区":      {TenantID: "tenant-1", Direction: "IMPORT", Procedure: "US-IMPORT/TYPE-86", Obligation: "obligation-1"},
		"空程序":      {TenantID: "tenant-1", Jurisdiction: "US-CBP", Direction: "IMPORT", Obligation: "obligation-1"},
		"空义务范围":    {TenantID: "tenant-1", Jurisdiction: "US-CBP", Direction: "IMPORT", Procedure: "US-IMPORT/TYPE-86"},
		"方向在封闭集合外": {TenantID: "tenant-1", Jurisdiction: "US-CBP", Direction: "SIDEWAYS", Procedure: "US-IMPORT/TYPE-86", Obligation: "obligation-1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := subject.HandleEstablishedCustomsCase(
				t.Context(), reference); !errors.Is(err, adapter.ErrCustomsCaseUntranslatableAnswer) {
				t.Fatalf("err = %v, want ErrCustomsCaseUntranslatableAnswer", err)
			}
		})
	}
	if derive.calls != 0 {
		t.Fatal("引用译不出来不该走到派生")
	}
}

func TestCustomsCaseFindByKeyCarriesAllFiveDimensions(t *testing.T) {
	finder := &caseFinderDouble{customsCase: establishedCase(t, "parcel-1"), found: true}
	handler, _, _ := caseDeriveHandler(t, caseMappingViewDouble{configured: false}, caseProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnCustomsCaseAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleEstablishedCustomsCase(t.Context(), establishedCaseRef()); err != nil {
		t.Fatalf("处理案件：%v", err)
	}
	if finder.last.TenantID.String() != "tenant-1" ||
		finder.last.Jurisdiction.String() != "US-CBP" ||
		finder.last.Direction != ccdomain.ImportManifest ||
		finder.last.Procedure.String() != "US-IMPORT/TYPE-86" ||
		finder.last.Obligation.String() != "obligation-1" {
		t.Fatalf("FindByKey 键 = %+v；五维缺一不可", finder.last)
	}
}

// Covers: ADR-0066 决定一、二与四——案件建立在消费侧逐成员派生，成员维与案件维一并
// 进事实引用（同一包裹可关联多个彼此独立的案件，缺案件维两条案件事实互撞）。
func TestAnEstablishedCaseDerivesOneProjectionCommandPerAssociatedParcel(t *testing.T) {
	finder := &caseFinderDouble{customsCase: establishedCase(t, "parcel-1", "parcel-2"), found: true}
	handler, facts, projections := caseDeriveHandler(t,
		caseMappingViewDouble{configured: false}, caseProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnCustomsCaseAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := subject.HandleEstablishedCustomsCase(t.Context(), establishedCaseRef()); err != nil {
		t.Fatalf("处理案件：%v", err)
	}

	for _, member := range []string{"parcel-1", "parcel-2"} {
		record, found := facts.byKey[caseFactKey(t, member)]
		if !found {
			t.Fatalf("成员 %s 的事实没落库；成员维与案件维必须进事实引用", member)
		}
		fact := record.Fact
		if fact.Parcel().String() != member {
			t.Fatalf("成员 %s 的事实包裹 = %s", member, fact.Parcel())
		}
		if fact.Kind().String() != "customs-case-established" {
			t.Fatalf("成员 %s 的事实类型 = %s", member, fact.Kind())
		}
		if !fact.OccurredAt().Equal(caseEstablishedAt) ||
			!fact.ReceivedAt().Equal(caseEstablishedAt) {
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
func TestASingleParcelConflictDoesNotBlockTheRestOfTheCase(t *testing.T) {
	finder := &caseFinderDouble{customsCase: establishedCase(t, "parcel-1", "parcel-2"), found: true}
	handler, facts, projections := caseDeriveHandler(t,
		caseMappingViewDouble{configured: false}, caseProjectionDownstreamDouble{})
	conflicting := caseFactKey(t, "parcel-1")
	facts.byKey[conflicting] = veports.FactRecord{Key: conflicting, ContentDigest: "someone-else"}

	subject, err := adapter.NewDeriveOnCustomsCaseAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleEstablishedCustomsCase(t.Context(), establishedCaseRef()); err != nil {
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
func TestAParcelUndecidedRollsBackTheWholeCaseEnvelope(t *testing.T) {
	finder := &caseFinderDouble{customsCase: establishedCase(t, "parcel-1", "parcel-2"), found: true}
	handler, _, projections := caseDeriveHandler(t,
		caseMappingViewDouble{err: errors.New("mapping view unavailable")},
		caseProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnCustomsCaseAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := subject.HandleEstablishedCustomsCase(t.Context(), establishedCaseRef()); !errors.Is(err, adapter.ErrProjectionUndecided) {
		t.Fatalf("err = %v, want ErrProjectionUndecided", err)
	}
	if len(projections.byKey) != 0 {
		t.Fatal("未决之下不该有任何成员的投影落下")
	}
}
