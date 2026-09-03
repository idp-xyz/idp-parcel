package domain

import (
	"errors"
	"testing"
	"time"
)

// 本文件钉外部承运轨迹事实的构造门与有效时间判断（label-channel/16，落文 ADR-0102）。
// 判据取自 CONTEXT「外部承运轨迹事实」规则节：发生时间由源给且缺则不成立；有效时间是一次
// 显式判断，「待判断」与「判断过」在类型上分得开；源事件标识可缺席；更正只按源声明登记。

var (
	trackingOccurredAt = time.Date(2026, 9, 3, 8, 30, 0, 0, time.UTC)
	trackingReceivedAt = time.Date(2026, 9, 3, 8, 31, 12, 0, time.UTC)
)

func trackingFactSpec(t *testing.T) ExternalTrackingFactSpec {
	t.Helper()
	tenant, _ := NewTenantID("tenant-1")
	fact, _ := NewExternalTrackingFactReference("EXTF-000000000001")
	version, _ := NewExternalTrackingFactVersion("EXTV-000000000001")
	source, _ := NewTrackingSourceReference("aggregator-a")
	credential, _ := NewExternalCarrierCredentialReference("carrier-x/1Z999")
	object, _ := NewCarriedObjectReference("PCL-1")
	event, _ := NewSourceEventReference("evt-1")
	status, _ := NewRawStatusReference("IN_TRANSIT")
	return ExternalTrackingFactSpec{
		TenantID:    tenant,
		Fact:        fact,
		Version:     version,
		Source:      source,
		Credential:  credential,
		Object:      object,
		SourceEvent: event,
		Status:      status,
		OccurredAt:  trackingOccurredAt,
		ReceivedAt:  trackingReceivedAt,
		Effective:   PendingEffectiveTime(),
	}
}

func TestAnExternalTrackingFactIsAdoptedWithItsEffectiveTimePending(t *testing.T) {
	fact, err := AdoptExternalCarrierTracking(trackingFactSpec(t))
	if err != nil {
		t.Fatalf("认领：%v", err)
	}
	if _, judged := fact.EffectiveAt(); judged {
		t.Fatal("没有任何判断，有效时间却读成已判断")
	}
	if fact.Effective().Basis() != EffectiveTimePending {
		t.Fatalf("有效时间依据应为待判断：%s", fact.Effective().Basis())
	}
	if !fact.OccurredAt().Equal(trackingOccurredAt) || !fact.ReceivedAt().Equal(trackingReceivedAt) {
		t.Fatalf("发生/接收时间没有原样保存：%s / %s", fact.OccurredAt(), fact.ReceivedAt())
	}
	if fact.Status().String() != "IN_TRANSIT" {
		t.Fatalf("原始状态词应原样保存：%q", fact.Status())
	}
	if event, given := fact.SourceEvent(); !given || event.String() != "evt-1" {
		t.Fatalf("源事件标识没有原样保存：%q given=%v", event, given)
	}
	if _, has := fact.Supersedes(); has {
		t.Fatal("首版不该回指任何前版")
	}
	if _, declared := fact.CorrectionOf(); declared {
		t.Fatal("源没声明更正，事实上却带了更正声明")
	}
}

func TestAdoptionRefusesMaterialWhoseSourceGaveNoOccurrenceTime(t *testing.T) {
	spec := trackingFactSpec(t)
	spec.OccurredAt = time.Time{}
	_, err := AdoptExternalCarrierTracking(spec)
	if !errors.Is(err, ErrOccurredAtNotGivenBySource) {
		t.Fatalf("源未给发生时间应以专用理由拒绝，得到：%v", err)
	}
}

func TestAdoptionDoesNotRequireASourceEventIdentifier(t *testing.T) {
	spec := trackingFactSpec(t)
	spec.SourceEvent = SourceEventReference{}
	fact, err := AdoptExternalCarrierTracking(spec)
	if err != nil {
		t.Fatalf("源未给事件标识的素材也应能认领：%v", err)
	}
	if _, given := fact.SourceEvent(); given {
		t.Fatal("源未给事件标识，事实上却读出一个")
	}
}

func TestAdoptionRequiresTheSourceGivenStatusWordAndTheReceivedTime(t *testing.T) {
	withoutStatus := trackingFactSpec(t)
	withoutStatus.Status = RawStatusReference{}
	if _, err := AdoptExternalCarrierTracking(withoutStatus); !errors.Is(err, ErrInvalidExternalTrackingFact) {
		t.Fatalf("没有原始状态词不成事实，得到：%v", err)
	}
	withoutReceived := trackingFactSpec(t)
	withoutReceived.ReceivedAt = time.Time{}
	if _, err := AdoptExternalCarrierTracking(withoutReceived); !errors.Is(err, ErrInvalidExternalTrackingFact) {
		t.Fatalf("没有接收时间不成事实，得到：%v", err)
	}
}

func TestAdoptionRefusesAnUnjudgedZeroValueAsEffectiveTime(t *testing.T) {
	spec := trackingFactSpec(t)
	spec.Effective = EffectiveTimeJudgment{}
	if _, err := AdoptExternalCarrierTracking(spec); !errors.Is(err, ErrInvalidExternalTrackingFact) {
		t.Fatalf("零值判断既不是待判断也不是判断过，应拒绝，得到：%v", err)
	}
}

func TestAnExplicitJudgmentEqualToOccurrenceIsStillAJudgment(t *testing.T) {
	judgment, err := JudgeEffectiveTimeExplicitly(trackingOccurredAt)
	if err != nil {
		t.Fatalf("显式判断：%v", err)
	}
	spec := trackingFactSpec(t)
	spec.Effective = judgment
	fact, err := AdoptExternalCarrierTracking(spec)
	if err != nil {
		t.Fatalf("认领：%v", err)
	}
	at, judged := fact.EffectiveAt()
	if !judged || !at.Equal(trackingOccurredAt) {
		t.Fatalf("显式判断过的有效时间应读回：%s judged=%v", at, judged)
	}
	if fact.Effective().Basis() != EffectiveTimeJudgedExplicitly {
		t.Fatalf("依据应为显式判断：%s", fact.Effective().Basis())
	}
}

func TestAJudgmentByRuleRecordsWhichRuleVersionWasApplied(t *testing.T) {
	rule, err := NewEffectiveTimeRuleReference("aggregator-a/effective-time", "v3")
	if err != nil {
		t.Fatalf("规则引用：%v", err)
	}
	judgment, err := JudgeEffectiveTimeByRule(rule, trackingReceivedAt)
	if err != nil {
		t.Fatalf("按规则判断：%v", err)
	}
	applied, has := judgment.Rule()
	if !has || applied.Rule() != "aggregator-a/effective-time" || applied.Version() != "v3" {
		t.Fatalf("采用的规则与版本应记在判断上：%q/%q has=%v", applied.Rule(), applied.Version(), has)
	}
	if judgment.Basis() != EffectiveTimeJudgedByRule {
		t.Fatalf("依据应为按规则：%s", judgment.Basis())
	}
}

func TestAJudgmentNeedsATimeAndARuleNeedsAVersion(t *testing.T) {
	if _, err := JudgeEffectiveTimeExplicitly(time.Time{}); !errors.Is(err, ErrInvalidEffectiveTimeJudgment) {
		t.Fatalf("没有时间的显式判断应拒绝，得到：%v", err)
	}
	if _, err := NewEffectiveTimeRuleReference("aggregator-a/effective-time", ""); err == nil {
		t.Fatal("没有版本的规则引用应拒绝——不带版本就说不清历史事实按哪一版判过")
	}
	rule, _ := NewEffectiveTimeRuleReference("aggregator-a/effective-time", "v3")
	if _, err := JudgeEffectiveTimeByRule(rule, time.Time{}); !errors.Is(err, ErrInvalidEffectiveTimeJudgment) {
		t.Fatalf("没有时间的规则判断应拒绝，得到：%v", err)
	}
	if _, err := JudgeEffectiveTimeByRule(EffectiveTimeRuleReference{}, trackingReceivedAt); !errors.Is(err, ErrInvalidEffectiveTimeJudgment) {
		t.Fatalf("没有规则的规则判断应拒绝，得到：%v", err)
	}
}

func TestJudgingAPendingFactFormsANewVersionThatSupersedesThePendingOne(t *testing.T) {
	pending, err := AdoptExternalCarrierTracking(trackingFactSpec(t))
	if err != nil {
		t.Fatalf("认领：%v", err)
	}
	judgment, _ := JudgeEffectiveTimeExplicitly(trackingReceivedAt)
	next, _ := NewExternalTrackingFactVersion("EXTV-000000000002")

	judged, err := pending.JudgeEffectiveTime(judgment, next)
	if err != nil {
		t.Fatalf("判断：%v", err)
	}
	if judged.Version() != next {
		t.Fatalf("判断应落成新版本：%q", judged.Version())
	}
	prior, has := judged.Supersedes()
	if !has || prior != pending.Version() {
		t.Fatalf("新版本应回指待判断的那一版：%q has=%v", prior, has)
	}
	if at, ok := judged.EffectiveAt(); !ok || !at.Equal(trackingReceivedAt) {
		t.Fatalf("判断的有效时间没有落到新版本上：%s ok=%v", at, ok)
	}
	if judged.Fact() != pending.Fact() || judged.Status() != pending.Status() || !judged.OccurredAt().Equal(pending.OccurredAt()) {
		t.Fatal("判断不得改动源给的内容：事实身份、状态词与发生时间必须原样带过去")
	}
	if pending.Origin() != VersionFromMaterial || judged.Origin() != VersionFromJudgment {
		t.Fatalf("版本来路应分得开素材到达与判断：%s / %s", pending.Origin(), judged.Origin())
	}
	if _, stillJudged := pending.EffectiveAt(); stillJudged {
		t.Fatal("原版本应保持待判断——值语义，接收者不动")
	}
}

func TestJudgingRefusesToReuseTheVersionOrToLeaveItPending(t *testing.T) {
	pending, _ := AdoptExternalCarrierTracking(trackingFactSpec(t))
	judgment, _ := JudgeEffectiveTimeExplicitly(trackingReceivedAt)
	if _, err := pending.JudgeEffectiveTime(judgment, pending.Version()); !errors.Is(err, ErrInvalidExternalTrackingFact) {
		t.Fatalf("沿用原版本号就是覆盖，应拒绝，得到：%v", err)
	}
	next, _ := NewExternalTrackingFactVersion("EXTV-000000000002")
	if _, err := pending.JudgeEffectiveTime(PendingEffectiveTime(), next); !errors.Is(err, ErrInvalidEffectiveTimeJudgment) {
		t.Fatalf("「判断为待判断」不是判断，应拒绝，得到：%v", err)
	}
}

func TestASourceDeclaredCorrectionIsRegisteredAsGivenAndMayResolveToAPriorVersion(t *testing.T) {
	spec := trackingFactSpec(t)
	corrected, _ := NewSourceEventReference("evt-0")
	priorVersion, _ := NewExternalTrackingFactVersion("EXTV-000000000000")
	ownEvent, _ := NewSourceEventReference("evt-1b")
	spec.SourceEvent = ownEvent
	spec.CorrectionOf = corrected
	spec.Supersedes = priorVersion

	fact, err := AdoptExternalCarrierTracking(spec)
	if err != nil {
		t.Fatalf("认领更正：%v", err)
	}
	declared, has := fact.CorrectionOf()
	if !has || declared != corrected {
		t.Fatalf("源声明的被更正事件应原样登记：%q has=%v", declared, has)
	}
	prior, linked := fact.Supersedes()
	if !linked || prior != priorVersion {
		t.Fatalf("解析得到的前版应回指：%q linked=%v", prior, linked)
	}
}

func TestACorrectionDeclarationMayStayUnresolvedButAVersionLinkNeedsADeclaration(t *testing.T) {
	unresolved := trackingFactSpec(t)
	unresolved.CorrectionOf, _ = NewSourceEventReference("evt-unknown")
	if _, err := AdoptExternalCarrierTracking(unresolved); err != nil {
		t.Fatalf("源声明更正了一条本上下文不认识的事件，声明仍应原样登记：%v", err)
	}

	linkWithoutDeclaration := trackingFactSpec(t)
	linkWithoutDeclaration.Supersedes, _ = NewExternalTrackingFactVersion("EXTV-000000000000")
	if _, err := AdoptExternalCarrierTracking(linkWithoutDeclaration); !errors.Is(err, ErrInvalidExternalTrackingFact) {
		t.Fatalf("没有源声明却回指前版，等于本上下文自己推断了取代关系，应拒绝，得到：%v", err)
	}

	selfLink := trackingFactSpec(t)
	selfLink.CorrectionOf, _ = NewSourceEventReference("evt-0")
	selfLink.Supersedes = selfLink.Version
	if _, err := AdoptExternalCarrierTracking(selfLink); !errors.Is(err, ErrInvalidExternalTrackingFact) {
		t.Fatalf("前版引用指向自己是读不动的链，应拒绝，得到：%v", err)
	}
}

func TestEveryEffectiveTimeBasisHasAName(t *testing.T) {
	for _, basis := range []EffectiveTimeBasis{EffectiveTimePending, EffectiveTimeJudgedExplicitly, EffectiveTimeJudgedByRule} {
		if basis.String() == "" {
			t.Fatalf("取值 %d 没有名字", basis)
		}
	}
	if EffectiveTimeBasisInvalid.String() != "" {
		t.Fatal("零值不该有名字")
	}
	for _, origin := range []VersionOrigin{VersionFromMaterial, VersionFromJudgment} {
		if origin.String() == "" {
			t.Fatalf("来路 %d 没有名字", origin)
		}
	}
}
