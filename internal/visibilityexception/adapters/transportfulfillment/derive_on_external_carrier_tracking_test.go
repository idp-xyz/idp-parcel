package transportfulfillment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件钉 TF 外部承运轨迹事实 → VE 已接受源事实的译装（label-channel/16）：三个时间各归各位
// 过桥、类型词是 external-carrier-tracking 而不含状态词、前版回指译进 Supersedes、待判断版本
// 到了这里算不一致而不是等谁、映射未配置时如实未归类。

var (
	trackingOccurredAtVE = time.Date(2026, 9, 3, 8, 30, 0, 0, time.UTC)
	trackingReceivedAtVE = time.Date(2026, 9, 3, 8, 31, 12, 0, time.UTC)
	trackingEffectiveVE  = time.Date(2026, 9, 3, 8, 45, 0, 0, time.UTC)
)

type externalTrackingFinderDouble struct {
	record tfports.ExternalTrackingFactRecord
	found  bool
	err    error
}

func (double *externalTrackingFinderDouble) FindByKey(
	_ context.Context, _ tfports.ExternalTrackingFactKey,
) (tfports.ExternalTrackingFactRecord, bool, error) {
	if double.err != nil {
		return tfports.ExternalTrackingFactRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

func judgedTrackingRecord(t *testing.T, version, supersedes string, judgment tfdomain.EffectiveTimeJudgment) tfports.ExternalTrackingFactRecord {
	t.Helper()
	spec := tfdomain.ExternalTrackingFactSpec{
		TenantID:    deliveryValue(t, tfdomain.NewTenantID, "tenant-1"),
		Fact:        deliveryValue(t, tfdomain.NewExternalTrackingFactReference, "EXTF-1"),
		Version:     deliveryValue(t, tfdomain.NewExternalTrackingFactVersion, version),
		Source:      deliveryValue(t, tfdomain.NewTrackingSourceReference, "aggregator-a"),
		Credential:  deliveryValue(t, tfdomain.NewExternalCarrierCredentialReference, "carrier-x/1Z999"),
		Object:      deliveryValue(t, tfdomain.NewCarriedObjectReference, "parcel-1"),
		SourceEvent: deliveryValue(t, tfdomain.NewSourceEventReference, "evt-"+version),
		Status:      deliveryValue(t, tfdomain.NewRawStatusReference, "DELIVERED"),
		OccurredAt:  trackingOccurredAtVE,
		ReceivedAt:  trackingReceivedAtVE,
		Effective:   judgment,
	}
	if supersedes != "" {
		spec.CorrectionOf = deliveryValue(t, tfdomain.NewSourceEventReference, "evt-prior")
		spec.Supersedes = deliveryValue(t, tfdomain.NewExternalTrackingFactVersion, supersedes)
	}
	fact, err := tfdomain.AdoptExternalCarrierTracking(spec)
	if err != nil {
		t.Fatalf("构造外部承运轨迹事实：%v", err)
	}
	return tfports.ExternalTrackingFactRecord{
		Key:        tfports.ExternalTrackingFactKey{TenantID: spec.TenantID, Fact: spec.Fact, Version: spec.Version},
		Fact:       fact,
		RecordedAt: trackingReceivedAtVE.Add(time.Second),
	}
}

func judgedTrackingRef(version string) veinbox.JudgedExternalTracking {
	return veinbox.JudgedExternalTracking{TenantID: "tenant-1", Fact: "EXTF-1", Version: version}
}

func TestAJudgedExternalTrackingFactLandsWithItsThreeTimesAndKind(t *testing.T) {
	judgment, _ := tfdomain.JudgeEffectiveTimeExplicitly(trackingEffectiveVE)
	derive, facts, projections := deliveryDeriveHandler(t, deliveryMappingViewDouble{configured: false}, deliveryProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnExternalCarrierTrackingAdapter(
		&externalTrackingFinderDouble{record: judgedTrackingRecord(t, "EXTV-1", "", judgment), found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := subject.HandleJudgedExternalTracking(t.Context(), judgedTrackingRef("EXTV-1")); err != nil {
		t.Fatalf("处理：%v", err)
	}

	var landed *veports.FactRecord
	for _, record := range facts.byKey {
		landed = &record
	}
	if landed == nil {
		t.Fatal("已接受源事实没有落库")
	}
	fact := landed.Fact
	if fact.Source() != vedomain.SourceTransportFulfillment {
		t.Fatalf("源上下文应仍是 TRANSPORT_FULFILLMENT——封闭五值不为外部源开口：%s", fact.Source())
	}
	if fact.Kind().String() != "external-carrier-tracking" {
		t.Fatalf("类型词应为 TF 命名的 external-carrier-tracking，且不含状态词：%q", fact.Kind())
	}
	if fact.Fact().String() != "external-carrier-tracking/EXTF-1" || fact.Version().String() != "EXTV-1" {
		t.Fatalf("事实引用与版本：%q / %q", fact.Fact(), fact.Version())
	}
	if fact.Parcel().String() != "parcel-1" {
		t.Fatalf("载运对象应原样进追踪包裹引用：%q", fact.Parcel())
	}
	if !fact.OccurredAt().Equal(trackingOccurredAtVE) || !fact.EffectiveAt().Equal(trackingEffectiveVE) || !fact.ReceivedAt().Equal(trackingReceivedAtVE) {
		t.Fatalf("三个时间没有各归各位：%s / %s / %s", fact.OccurredAt(), fact.EffectiveAt(), fact.ReceivedAt())
	}
	if _, has := fact.Supersedes(); has {
		t.Fatal("首版不该带前身")
	}
	projection, found := projections.byKey["tenant-1/parcel-1"]
	if !found || len(projection.Entries()) != 1 {
		t.Fatalf("投影应派生一条条目：found=%v", found)
	}
	if _, classified := projection.Entries()[0].Milestone(); classified {
		t.Fatal("状态词规则未登记，外部承运轨迹事实应如实未归类，不得强行映射")
	}
}

func TestASupersedingVersionCarriesItsPredecessor(t *testing.T) {
	rule := deliveryValue2(t, tfdomain.NewEffectiveTimeRuleReference, "aggregator-a/effective-time", "v3")
	judgment, _ := tfdomain.JudgeEffectiveTimeByRule(rule, trackingEffectiveVE)
	derive, facts, _ := deliveryDeriveHandler(t, deliveryMappingViewDouble{configured: false}, deliveryProjectionDownstreamDouble{})
	subject, _ := adapter.NewDeriveOnExternalCarrierTrackingAdapter(
		&externalTrackingFinderDouble{record: judgedTrackingRecord(t, "EXTV-2", "EXTV-1", judgment), found: true}, derive)

	if err := subject.HandleJudgedExternalTracking(t.Context(), judgedTrackingRef("EXTV-2")); err != nil {
		t.Fatalf("处理：%v", err)
	}
	for _, record := range facts.byKey {
		prior, has := record.Fact.Supersedes()
		if !has || prior.String() != "EXTV-1" {
			t.Fatalf("前版回指应译进 Supersedes：%q has=%v", prior, has)
		}
	}
}

func TestAPendingVersionArrivingHereIsInconsistentNotUndecided(t *testing.T) {
	derive := &countingProjectionHandler{}
	subject, _ := adapter.NewDeriveOnExternalCarrierTrackingAdapter(
		&externalTrackingFinderDouble{record: judgedTrackingRecord(t, "EXTV-1", "", tfdomain.PendingEffectiveTime()), found: true}, derive)

	err := subject.HandleJudgedExternalTracking(t.Context(), judgedTrackingRef("EXTV-1"))
	if !errors.Is(err, adapter.ErrExternalTrackingRecordInconsistent) {
		t.Fatalf("待判断的版本不该到这里，到了就是不一致而不是等谁：%v", err)
	}
	if derive.calls != 0 {
		t.Fatal("待判断的事实不得进投影")
	}
}

func TestAMissingOrUnreadableExternalTrackingFactIsContinuableUndecided(t *testing.T) {
	derive := &countingProjectionHandler{}
	missing, _ := adapter.NewDeriveOnExternalCarrierTrackingAdapter(&externalTrackingFinderDouble{}, derive)
	if err := missing.HandleJudgedExternalTracking(t.Context(), judgedTrackingRef("EXTV-1")); !errors.Is(err, adapter.ErrExternalTrackingNotVisible) {
		t.Fatalf("缺登记应为可续办未决：%v", err)
	}
	unreadable, _ := adapter.NewDeriveOnExternalCarrierTrackingAdapter(&externalTrackingFinderDouble{err: errors.New("store unavailable")}, derive)
	if err := unreadable.HandleJudgedExternalTracking(t.Context(), judgedTrackingRef("EXTV-1")); !errors.Is(err, adapter.ErrExternalTrackingNotVisible) {
		t.Fatalf("读失败应为可续办未决：%v", err)
	}
	if derive.calls != 0 {
		t.Fatal("没有记录不该走到派生")
	}
}

func TestAMismatchedExternalTrackingKeyIsInconsistent(t *testing.T) {
	judgment, _ := tfdomain.JudgeEffectiveTimeExplicitly(trackingEffectiveVE)
	record := judgedTrackingRecord(t, "EXTV-1", "", judgment)
	record.Key.Version = deliveryValue(t, tfdomain.NewExternalTrackingFactVersion, "EXTV-other")
	derive := &countingProjectionHandler{}
	subject, _ := adapter.NewDeriveOnExternalCarrierTrackingAdapter(&externalTrackingFinderDouble{record: record, found: true}, derive)

	if err := subject.HandleJudgedExternalTracking(t.Context(), judgedTrackingRef("EXTV-other")); !errors.Is(err, adapter.ErrExternalTrackingRecordInconsistent) {
		t.Fatalf("键与本体不符应为不一致：%v", err)
	}
}

func TestAnUntranslatableExternalTrackingReferenceIsRefused(t *testing.T) {
	derive := &countingProjectionHandler{}
	subject, _ := adapter.NewDeriveOnExternalCarrierTrackingAdapter(&externalTrackingFinderDouble{}, derive)
	if err := subject.HandleJudgedExternalTracking(t.Context(), veinbox.JudgedExternalTracking{TenantID: "tenant-1", Fact: "", Version: "v"}); !errors.Is(err, adapter.ErrExternalTrackingUntranslatableAnswer) {
		t.Fatalf("空事实引用应为不可译：%v", err)
	}
}

func deliveryValue2[T any](t *testing.T, construct func(string, string) (T, error), first, second string) T {
	t.Helper()
	built, err := construct(first, second)
	if err != nil {
		t.Fatalf("construct %q/%q: %v", first, second, err)
	}
	return built
}
