package transportfulfillment_test

import (
	"errors"
	"testing"
	"time"

	"context"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var journeyStartedAt = time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)

type journeyFinderDouble struct {
	record tfports.AlternateJourneyRecord
	found  bool
	err    error
	last   tfports.AlternateJourneyKey
}

func (double *journeyFinderDouble) FindByKey(
	_ context.Context, key tfports.AlternateJourneyKey,
) (tfports.AlternateJourneyRecord, bool, error) {
	double.last = key
	if double.err != nil {
		return tfports.AlternateJourneyRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

func startedJourneyRecord(
	t *testing.T, purpose tfdomain.JourneyPurpose, members ...string,
) tfports.AlternateJourneyRecord {
	t.Helper()
	carried := make([]tfdomain.CarriedObjectReference, 0, len(members))
	for _, member := range members {
		carried = append(carried, handoverValue(t, tfdomain.NewCarriedObjectReference, member))
	}
	journey, err := tfdomain.FormAlternateJourney(tfdomain.AlternateJourneySpec{
		TenantID:        handoverValue(t, tfdomain.NewTenantID, "tenant-1"),
		Journey:         handoverValue(t, tfdomain.NewJourneyReference, "journey-alt-1"),
		Purpose:         purpose,
		OriginalJourney: handoverValue(t, tfdomain.NewJourneyReference, "journey-original-1"),
		BasisKind:       tfdomain.ServiceDispositionDecision,
		Basis:           handoverValue(t, tfdomain.NewDispositionBasisReference, "DISPOSITION/decision-1"),
		Members:         carried,
		StartedAt:       journeyStartedAt,
	})
	if err != nil {
		t.Fatalf("构造替代旅程：%v", err)
	}
	return tfports.AlternateJourneyRecord{
		Key: tfports.AlternateJourneyKey{
			TenantID: journey.TenantID(),
			Original: journey.OriginalJourney(),
			Purpose:  journey.Purpose(),
			Basis:    journey.Basis(),
		},
		ContentDigest: "digest-journey",
		Journey:       journey,
		RecordedAt:    journeyStartedAt.Add(time.Second),
	}
}

func recordedJourneyRef(purpose string) veinbox.RecordedExceptionJourney {
	return veinbox.RecordedExceptionJourney{
		TenantID: "tenant-1",
		Original: "journey-original-1",
		Purpose:  purpose,
		Basis:    "DISPOSITION/decision-1",
	}
}

func journeyFactKey(t *testing.T, member, purpose string) veports.FactKey {
	t.Helper()
	return veports.FactKey{
		Tenant: handoverValue(t, vedomain.NewTenantID, "tenant-1"),
		Source: vedomain.SourceTransportFulfillment,
		Fact: handoverValue(t, vedomain.NewSourceFactReference,
			"exception-journey/"+member+"/journey-original-1/"+purpose+"/DISPOSITION/decision-1"),
		Version: handoverValue(t, vedomain.NewSourceFactVersion, "journey-alt-1"),
	}
}

func TestAMissingExceptionJourneyIsContinuableUndecided(t *testing.T) {
	derive := &handoverCountingHandler{}
	subject, err := adapter.NewDeriveOnExceptionJourneyAdapter(&journeyFinderDouble{}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRecordedExceptionJourney(t.Context(), recordedJourneyRef("ALTERNATE")); !errors.Is(err, adapter.ErrExceptionJourneyNotVisible) {
		t.Fatalf("err = %v, want ErrExceptionJourneyNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("缺旅程记录不该走到派生")
	}
}

func TestAnUnreadableExceptionJourneyIsContinuableUndecided(t *testing.T) {
	derive := &handoverCountingHandler{}
	subject, err := adapter.NewDeriveOnExceptionJourneyAdapter(
		&journeyFinderDouble{err: errors.New("store unavailable")}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRecordedExceptionJourney(t.Context(), recordedJourneyRef("ALTERNATE")); !errors.Is(err, adapter.ErrExceptionJourneyNotVisible) {
		t.Fatalf("err = %v, want ErrExceptionJourneyNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("读失败不该走到派生")
	}
}

func TestAMismatchedExceptionJourneyKeyIsInconsistentNotUndecided(t *testing.T) {
	record := startedJourneyRecord(t, tfdomain.AlternateJourneyPurpose, "parcel-1")
	record.Key.Original = handoverValue(t, tfdomain.NewJourneyReference, "journey-other")
	derive := &handoverCountingHandler{}
	subject, err := adapter.NewDeriveOnExceptionJourneyAdapter(
		&journeyFinderDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRecordedExceptionJourney(t.Context(), recordedJourneyRef("ALTERNATE")); !errors.Is(err, adapter.ErrExceptionJourneyRecordInconsistent) {
		t.Fatalf("err = %v, want ErrExceptionJourneyRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("键本体不符不该走到派生")
	}
}

func TestAnExceptionJourneyWithZeroRecordedAtIsInconsistentNotUndecided(t *testing.T) {
	record := startedJourneyRecord(t, tfdomain.AlternateJourneyPurpose, "parcel-1")
	record.RecordedAt = time.Time{}
	derive := &handoverCountingHandler{}
	subject, err := adapter.NewDeriveOnExceptionJourneyAdapter(
		&journeyFinderDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRecordedExceptionJourney(t.Context(), recordedJourneyRef("ALTERNATE")); !errors.Is(err, adapter.ErrExceptionJourneyRecordInconsistent) {
		t.Fatalf("err = %v, want ErrExceptionJourneyRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("时间为零不该走到派生")
	}
}

func TestAnUntranslatableExceptionJourneyReferenceKeepsItsSentinel(t *testing.T) {
	derive := &handoverCountingHandler{}
	subject, err := adapter.NewDeriveOnExceptionJourneyAdapter(
		&journeyFinderDouble{record: startedJourneyRecord(t, tfdomain.AlternateJourneyPurpose, "parcel-1"), found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	for name, reference := range map[string]veinbox.RecordedExceptionJourney{
		"空租户":      {Original: "journey-original-1", Purpose: "ALTERNATE", Basis: "DISPOSITION/decision-1"},
		"空原旅程":     {TenantID: "tenant-1", Purpose: "ALTERNATE", Basis: "DISPOSITION/decision-1"},
		"空依据":      {TenantID: "tenant-1", Original: "journey-original-1", Purpose: "ALTERNATE"},
		"目的在封闭集合外": {TenantID: "tenant-1", Original: "journey-original-1", Purpose: "SIDEWAYS", Basis: "DISPOSITION/decision-1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := subject.HandleRecordedExceptionJourney(
				t.Context(), reference); !errors.Is(err, adapter.ErrExceptionJourneyUntranslatableAnswer) {
				t.Fatalf("err = %v, want ErrExceptionJourneyUntranslatableAnswer", err)
			}
		})
	}
	if derive.calls != 0 {
		t.Fatal("引用译不出来不该走到派生")
	}
}

func TestExceptionJourneyFindByKeyCarriesAllFourDimensions(t *testing.T) {
	finder := &journeyFinderDouble{record: startedJourneyRecord(t, tfdomain.AlternateJourneyPurpose, "parcel-1"), found: true}
	handler, _, _ := handoverDeriveHandler(t, handoverMappingViewDouble{configured: false}, handoverProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnExceptionJourneyAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRecordedExceptionJourney(t.Context(), recordedJourneyRef("ALTERNATE")); err != nil {
		t.Fatalf("处理旅程：%v", err)
	}
	if finder.last.TenantID.String() != "tenant-1" ||
		finder.last.Original.String() != "journey-original-1" ||
		finder.last.Purpose != tfdomain.AlternateJourneyPurpose ||
		finder.last.Basis.String() != "DISPOSITION/decision-1" {
		t.Fatalf("FindByKey 键 = %+v；四维缺一不可", finder.last)
	}
}

// Covers: ADR-0066 决定一与二——一封信 N 个成员在消费侧逐成员派生，成员维编进事实
// 引用；每个成员各是各的事实、各推各的投影链。
func TestAnExceptionJourneyDerivesOneProjectionCommandPerMember(t *testing.T) {
	finder := &journeyFinderDouble{
		record: startedJourneyRecord(t, tfdomain.AlternateJourneyPurpose, "parcel-1", "parcel-2"),
		found:  true,
	}
	handler, facts, projections := handoverDeriveHandler(t,
		handoverMappingViewDouble{configured: false}, handoverProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnExceptionJourneyAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := subject.HandleRecordedExceptionJourney(t.Context(), recordedJourneyRef("ALTERNATE")); err != nil {
		t.Fatalf("处理旅程：%v", err)
	}

	for _, member := range []string{"parcel-1", "parcel-2"} {
		record, found := facts.byKey[journeyFactKey(t, member, "ALTERNATE")]
		if !found {
			t.Fatalf("成员 %s 的事实没落库；成员维必须进事实引用", member)
		}
		fact := record.Fact
		if fact.Parcel().String() != member {
			t.Fatalf("成员 %s 的事实包裹 = %s", member, fact.Parcel())
		}
		if fact.Kind().String() != "exception-journey-alternate" {
			t.Fatalf("成员 %s 的事实类型 = %s", member, fact.Kind())
		}
		if !fact.OccurredAt().Equal(journeyStartedAt) ||
			!fact.ReceivedAt().Equal(journeyStartedAt.Add(time.Second)) {
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

// Covers: ADR-0066 决定四——RETURN 与 ALTERNATE 是两个映射类型字面量，目录才能把
// 退运与改送映成不同里程碑。
func TestAReturnJourneyIsItsOwnFactKind(t *testing.T) {
	finder := &journeyFinderDouble{
		record: startedJourneyRecord(t, tfdomain.ReturnJourneyPurpose, "parcel-1"),
		found:  true,
	}
	handler, facts, _ := handoverDeriveHandler(t,
		handoverMappingViewDouble{configured: false}, handoverProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnExceptionJourneyAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := subject.HandleRecordedExceptionJourney(t.Context(), recordedJourneyRef("RETURN")); err != nil {
		t.Fatalf("处理退运旅程：%v", err)
	}
	record, found := facts.byKey[journeyFactKey(t, "parcel-1", "RETURN")]
	if !found {
		t.Fatal("退运事实没落库")
	}
	if record.Fact.Kind().String() != "exception-journey-return" {
		t.Fatalf("退运事实类型 = %s", record.Fact.Kind())
	}
}

// Covers: ADR-0066 决定三——单成员业务终局（来源冲突）入账继续，其余成员照常派生；
// 冲突在事实库有行，不阻断整封。
func TestASingleMemberConflictDoesNotBlockTheRestOfTheEnvelope(t *testing.T) {
	finder := &journeyFinderDouble{
		record: startedJourneyRecord(t, tfdomain.AlternateJourneyPurpose, "parcel-1", "parcel-2"),
		found:  true,
	}
	handler, facts, projections := handoverDeriveHandler(t,
		handoverMappingViewDouble{configured: false}, handoverProjectionDownstreamDouble{})
	conflicting := journeyFactKey(t, "parcel-1", "ALTERNATE")
	facts.byKey[conflicting] = veports.FactRecord{Key: conflicting, ContentDigest: "someone-else"}

	subject, err := adapter.NewDeriveOnExceptionJourneyAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRecordedExceptionJourney(t.Context(), recordedJourneyRef("ALTERNATE")); err != nil {
		t.Fatalf("单成员冲突不该拖垮整封：%v", err)
	}

	if _, found := projections.byKey["tenant-1/parcel-1"]; found {
		t.Fatal("冲突成员不该派生投影")
	}
	if _, found := projections.byKey["tenant-1/parcel-2"]; !found {
		t.Fatal("其余成员该照常派生")
	}
	if _, found := facts.byKey[journeyFactKey(t, "parcel-2", "ALTERNATE")]; !found {
		t.Fatal("其余成员的事实该照常落库")
	}
}

// Covers: ADR-0066 决定三——某成员未决即整封报错回滚重投，不留部分状态在消费账上。
func TestAMemberUndecidedRollsBackTheWholeEnvelope(t *testing.T) {
	finder := &journeyFinderDouble{
		record: startedJourneyRecord(t, tfdomain.AlternateJourneyPurpose, "parcel-1", "parcel-2"),
		found:  true,
	}
	handler, _, projections := handoverDeriveHandler(t,
		handoverMappingViewDouble{err: errors.New("mapping view unavailable")},
		handoverProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnExceptionJourneyAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := subject.HandleRecordedExceptionJourney(t.Context(), recordedJourneyRef("ALTERNATE")); !errors.Is(err, adapter.ErrJourneyProjectionUndecided) {
		t.Fatalf("err = %v, want ErrJourneyProjectionUndecided", err)
	}
	if len(projections.byKey) != 0 {
		t.Fatal("未决之下不该有任何成员的投影落下")
	}
}
