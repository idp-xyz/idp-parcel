package parcelshipment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 VE 终局投影适配器只翻译不判断：按四维键重读、键本体不符响亮、两支裁决各
// 有字面量、两支时间语义各自诚实。派生编排本身的行为在编排自己的测试里，这里只证交
// 给它的命令长得对。

var (
	finalOccurredAt = time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	finalAdoptedAt  = time.Date(2026, 8, 19, 11, 0, 0, 0, time.UTC)
)

var errDeriveReached = errors.New("derive reached")

func finalValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type finalFinderDouble struct {
	record psports.FinalOutcomeRecord
	found  bool
	err    error
	last   psports.FinalAdoptionKey

	// 按终局版本那一法的答复与留痕：前一代登记、在不在、读不读得通，以及被问过几次、
	// 问的是哪一格。
	predecessor      psports.FinalOutcomeRecord
	predecessorFound bool
	predecessorErr   error
	byVersionCalls   int
	byVersionTenant  psdomain.TenantID
	byVersionParcel  psdomain.DeclaredParcelID
	byVersionVersion psdomain.FinalOutcomeVersionID
}

func (double *finalFinderDouble) FindByKey(
	_ context.Context, key psports.FinalAdoptionKey,
) (psports.FinalOutcomeRecord, bool, error) {
	double.last = key
	if double.err != nil {
		return psports.FinalOutcomeRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

func (double *finalFinderDouble) FindByFinalVersion(
	_ context.Context,
	tenant psdomain.TenantID,
	parcel psdomain.DeclaredParcelID,
	version psdomain.FinalOutcomeVersionID,
) (psports.FinalOutcomeRecord, bool, error) {
	double.byVersionCalls++
	double.byVersionTenant = tenant
	double.byVersionParcel = parcel
	double.byVersionVersion = version
	if double.predecessorErr != nil {
		return psports.FinalOutcomeRecord{}, false, double.predecessorErr
	}
	return double.predecessor, double.predecessorFound, nil
}

// capturingHandler 留下命令再交回一个哨兵错误：本适配器的职责就是把记录译成命令，
// 派生结果怎么落成消费两格由 veconsume 与编排的测试各自负责。
type capturingHandler struct {
	calls   int
	command veapplication.DeriveProjectionCommand
}

func (double *capturingHandler) Handle(
	_ context.Context, command veapplication.DeriveProjectionCommand,
) (veapplication.DeriveProjectionResult, error) {
	double.calls++
	double.command = command
	return veapplication.DeriveProjectionResult{}, errDeriveReached
}

func finalKey(t *testing.T) psports.FinalAdoptionKey {
	t.Helper()
	return psports.FinalAdoptionKey{
		TenantID: finalValue(t, psdomain.NewTenantID, "tenant-1"),
		Parcel:   finalValue(t, psdomain.NewDeclaredParcelID, "parcel-1"),
		Kind:     psdomain.EffectiveDeliveryOutcome,
		Version:  finalValue(t, psdomain.NewResponsibilityOutcomeVersion, "responsibility-outcome/v1"),
	}
}

func adoptedFinalRecord(t *testing.T) psports.FinalOutcomeRecord {
	t.Helper()
	source, err := psdomain.NewResponsibilityOutcome(psdomain.ResponsibilityOutcomeSpec{
		Kind:       psdomain.EffectiveDeliveryOutcome,
		Parcel:     finalValue(t, psdomain.NewDeclaredParcelID, "parcel-1"),
		Decision:   finalValue(t, psdomain.NewResponsibilityDecisionReference, "delivery-judgment-1"),
		Execution:  finalValue(t, psdomain.NewExecutionEvidenceReference, "pod-1"),
		Version:    finalValue(t, psdomain.NewResponsibilityOutcomeVersion, "responsibility-outcome/v1"),
		OccurredAt: finalOccurredAt,
	})
	if err != nil {
		t.Fatalf("构造责任结果：%v", err)
	}
	final, err := psdomain.FormParcelFinalOutcome(psdomain.ParcelFinalOutcomeSpec{
		Version:     finalValue(t, psdomain.NewFinalOutcomeVersionID, "final/v1"),
		Parcel:      finalValue(t, psdomain.NewDeclaredParcelID, "parcel-1"),
		Kind:        finalValue(t, psdomain.NewFinalKindReference, "DELIVERED"),
		Source:      source,
		RuleVersion: finalValue(t, psdomain.NewFinalRuleVersionReference, "final-rule/v1"),
	})
	if err != nil {
		t.Fatalf("构造终局：%v", err)
	}
	return psports.FinalOutcomeRecord{
		Key:           finalKey(t),
		ContentDigest: "digest-final",
		Finalized:     true,
		Final:         final,
		AdoptedAt:     finalAdoptedAt,
	}
}

func notAdoptedFinalRecord(t *testing.T) psports.FinalOutcomeRecord {
	t.Helper()
	return psports.FinalOutcomeRecord{
		Key:           finalKey(t),
		ContentDigest: "digest-refusal",
		Finalized:     false,
		RefusalBasis:  finalValue(t, psdomain.NewCheckReason, "FINAL_RULE_NOT_CONFIGURED"),
		AdoptedAt:     finalAdoptedAt,
	}
}

func formedFinalRef() veinbox.FormedFinalOutcome {
	return veinbox.FormedFinalOutcome{
		TenantID: "tenant-1",
		Parcel:   "parcel-1",
		Kind:     "EFFECTIVE_DELIVERY",
		Version:  "responsibility-outcome/v1",
	}
}

func newFinalSubject(t *testing.T, finder *finalFinderDouble) (
	*adapter.DeriveOnFinalOutcomeAdapter, *capturingHandler,
) {
	t.Helper()
	derive := &capturingHandler{}
	subject, err := adapter.NewDeriveOnFinalOutcomeAdapter(finder, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	return subject, derive
}

func TestAMissingFinalOutcomeIsContinuableUndecided(t *testing.T) {
	subject, derive := newFinalSubject(t, &finalFinderDouble{})

	err := subject.HandleFormedFinalOutcome(t.Context(), formedFinalRef())
	if !errors.Is(err, adapter.ErrFinalNotVisible) {
		t.Fatalf("err = %v, want ErrFinalNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("缺终局登记不该走到派生")
	}
}

func TestAnUnreadableFinalOutcomeIsContinuableUndecided(t *testing.T) {
	subject, derive := newFinalSubject(t, &finalFinderDouble{err: errors.New("store unavailable")})

	err := subject.HandleFormedFinalOutcome(t.Context(), formedFinalRef())
	if !errors.Is(err, adapter.ErrFinalNotVisible) {
		t.Fatalf("err = %v, want ErrFinalNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("读失败不该走到派生")
	}
}

// 版本必须进 FindByKey：更正走新版本新登记，按三维读「当前版」会把更正与原判断叠成
// 一次查找，重派生场景下静默拿错版本。
func TestTheFinalOutcomeIsReadByAllFourKeyDimensions(t *testing.T) {
	finder := &finalFinderDouble{record: adoptedFinalRecord(t), found: true}
	subject, _ := newFinalSubject(t, finder)

	_ = subject.HandleFormedFinalOutcome(t.Context(), formedFinalRef())

	want := finalKey(t)
	if finder.last != want {
		t.Fatalf("取回用的键 = %+v, want %+v", finder.last, want)
	}
}

func TestAMismatchedFinalOutcomeKeyIsInconsistentNotUndecided(t *testing.T) {
	record := adoptedFinalRecord(t)
	record.Key.Version = finalValue(t, psdomain.NewResponsibilityOutcomeVersion, "responsibility-outcome/v2")
	subject, derive := newFinalSubject(t, &finalFinderDouble{record: record, found: true})

	err := subject.HandleFormedFinalOutcome(t.Context(), formedFinalRef())
	if !errors.Is(err, adapter.ErrFinalRecordInconsistent) {
		t.Fatalf("err = %v, want ErrFinalRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("键本体不符不该走到派生")
	}
}

func TestAFinalOutcomeWithZeroAdoptedAtIsInconsistentNotUndecided(t *testing.T) {
	record := adoptedFinalRecord(t)
	record.AdoptedAt = time.Time{}
	subject, derive := newFinalSubject(t, &finalFinderDouble{record: record, found: true})

	err := subject.HandleFormedFinalOutcome(t.Context(), formedFinalRef())
	if !errors.Is(err, adapter.ErrFinalRecordInconsistent) {
		t.Fatalf("err = %v, want ErrFinalRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("采认时刻为零不该走到派生——它同时是本事实的业务时间")
	}
}

func TestAnUntranslatableFinalOutcomeReferenceKeepsItsSentinel(t *testing.T) {
	subject, derive := newFinalSubject(t,
		&finalFinderDouble{record: adoptedFinalRecord(t), found: true})

	for name, reference := range map[string]veinbox.FormedFinalOutcome{
		"空租户":     {Parcel: "parcel-1", Kind: "EFFECTIVE_DELIVERY", Version: "responsibility-outcome/v1"},
		"空包裹":     {TenantID: "tenant-1", Kind: "EFFECTIVE_DELIVERY", Version: "responsibility-outcome/v1"},
		"空版本":     {TenantID: "tenant-1", Parcel: "parcel-1", Kind: "EFFECTIVE_DELIVERY"},
		"认不得的种类":  {TenantID: "tenant-1", Parcel: "parcel-1", Kind: "PARCEL_CANCELLED", Version: "responsibility-outcome/v1"},
		"种类大小写不符": {TenantID: "tenant-1", Parcel: "parcel-1", Kind: "effective_delivery", Version: "responsibility-outcome/v1"},
	} {
		t.Run(name, func(t *testing.T) {
			err := subject.HandleFormedFinalOutcome(t.Context(), reference)
			if !errors.Is(err, adapter.ErrFinalUntranslatableAnswer) {
				t.Fatalf("err = %v, want ErrFinalUntranslatableAnswer", err)
			}
		})
	}
	if derive.calls != 0 {
		t.Fatal("引用译不出来不该走到派生")
	}
}

// 已采认支：EffectiveAt 取被采用责任结果的业务时间，与 OccurredAt（采认时刻）**不等**。
// 这是四路里第一条两者可分的，故意如此——源侧给得出第三时间，翻译一步不该丢掉它。
func TestAnAdoptedFinalOutcomeTranslatesToItsOwnKindAndKeepsBothTimes(t *testing.T) {
	subject, derive := newFinalSubject(t,
		&finalFinderDouble{record: adoptedFinalRecord(t), found: true})

	err := subject.HandleFormedFinalOutcome(t.Context(), formedFinalRef())
	if !errors.Is(err, errDeriveReached) {
		t.Fatalf("err = %v, want 派生被调用", err)
	}

	fact := derive.command.Fact
	if derive.command.TenantID.String() != "tenant-1" {
		t.Fatalf("租户 = %q", derive.command.TenantID.String())
	}
	if fact.Source != vedomain.SourceParcelShipment {
		t.Fatalf("来源 = %v, want SourceParcelShipment——终局是 PS 自家事实", fact.Source)
	}
	if fact.Parcel.String() != "parcel-1" {
		t.Fatalf("包裹 = %q", fact.Parcel.String())
	}
	if fact.Kind.String() != "final-outcome-adopted" {
		t.Fatalf("字面量 = %q, want final-outcome-adopted", fact.Kind.String())
	}
	if fact.Fact.String() != "final-outcome/parcel-1/EFFECTIVE_DELIVERY" {
		t.Fatalf("事实引用 = %q；前缀防同源撞键，种类进引用因同包裹可有多条独立终局流", fact.Fact.String())
	}
	if fact.Version.String() != "responsibility-outcome/v1" {
		t.Fatalf("版本 = %q, want 责任结果版本", fact.Version.String())
	}
	if !fact.OccurredAt.Equal(finalAdoptedAt) {
		t.Fatalf("OccurredAt = %v, want 采认时刻 %v", fact.OccurredAt, finalAdoptedAt)
	}
	if !fact.EffectiveAt.Equal(finalOccurredAt) {
		t.Fatalf("EffectiveAt = %v, want 责任结果业务时间 %v——不得塌成采认时刻", fact.EffectiveAt, finalOccurredAt)
	}
	if !fact.ReceivedAt.Equal(finalAdoptedAt) {
		t.Fatalf("ReceivedAt = %v, want %v", fact.ReceivedAt, finalAdoptedAt)
	}
	if fact.Supersedes.String() != "" {
		t.Fatalf("首派生指名了前身 %q", fact.Supersedes.String())
	}
}

// 不采用支必须派生：追踪要知道不采用的迟到来源（PS 侧 handOff 注释）。它没有 Final
// 对象因而没有第三时间，EffectiveAt 只能同 OccurredAt。
func TestANotAdoptedFinalOutcomeStillDerivesUnderItsOwnKind(t *testing.T) {
	subject, derive := newFinalSubject(t,
		&finalFinderDouble{record: notAdoptedFinalRecord(t), found: true})

	err := subject.HandleFormedFinalOutcome(t.Context(), formedFinalRef())
	if !errors.Is(err, errDeriveReached) {
		t.Fatalf("err = %v, want 不采用支也要派生", err)
	}

	fact := derive.command.Fact
	if fact.Kind.String() != "final-outcome-not-adopted" {
		t.Fatalf("字面量 = %q, want final-outcome-not-adopted——与已采认共用一行映射会归进同一里程碑", fact.Kind.String())
	}
	if !fact.OccurredAt.Equal(finalAdoptedAt) || !fact.EffectiveAt.Equal(finalAdoptedAt) {
		t.Fatalf("不采用支两时间都取采认时刻：OccurredAt = %v EffectiveAt = %v", fact.OccurredAt, fact.EffectiveAt)
	}
	if fact.Fact.String() != "final-outcome/parcel-1/EFFECTIVE_DELIVERY" {
		t.Fatalf("事实引用 = %q；两支同键不同字面量，冲突由派生编排裁", fact.Fact.String())
	}
	if fact.Supersedes.String() != "" {
		t.Fatalf("不采用支指名了前身 %q——它没有终局，也就没有前一代", fact.Supersedes.String())
	}
}

// 首派生与不采用都不问前一代：没有 PriorVersion 就没有可译的关系，多问一次只会让一个与
// 本代无关的读失败把它拖进未决。
func TestOnlyARederivedFinalOutcomeAsksForItsPredecessor(t *testing.T) {
	for name, record := range map[string]psports.FinalOutcomeRecord{
		"首派生": adoptedFinalRecord(t),
		"不采用": notAdoptedFinalRecord(t),
	} {
		t.Run(name, func(t *testing.T) {
			finder := &finalFinderDouble{record: record, found: true,
				predecessorErr: errors.New("不该被问到")}
			subject, _ := newFinalSubject(t, finder)

			if err := subject.HandleFormedFinalOutcome(t.Context(), formedFinalRef()); !errors.Is(err, errDeriveReached) {
				t.Fatalf("err = %v, want 派生被调用", err)
			}
			if finder.byVersionCalls != 0 {
				t.Fatalf("按终局版本问了 %d 次", finder.byVersionCalls)
			}
		})
	}
}

// rederivedFinalRecord 从 adoptedFinalRecord 经 PS 领域的 Rederive 造出同种更正后的下一代：
// 责任结果换到 v2、终局版本换到 final/v2、PriorVersion 指回 final/v1——与 FormParcelFinalHandler
// 的 rederive 同一走法。
func rederivedFinalRecord(t *testing.T) psports.FinalOutcomeRecord {
	t.Helper()
	first := adoptedFinalRecord(t)
	key := first.Key
	key.Version = finalValue(t, psdomain.NewResponsibilityOutcomeVersion, "responsibility-outcome/v2")
	source, err := psdomain.NewResponsibilityOutcome(psdomain.ResponsibilityOutcomeSpec{
		Kind:       key.Kind,
		Parcel:     key.Parcel,
		Decision:   finalValue(t, psdomain.NewResponsibilityDecisionReference, "delivery-judgment-1"),
		Execution:  finalValue(t, psdomain.NewExecutionEvidenceReference, "pod-1-corrected"),
		Version:    key.Version,
		OccurredAt: finalOccurredAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("构造更正后的责任结果：%v", err)
	}
	rederived, err := first.Final.Rederive(
		finalValue(t, psdomain.NewFinalOutcomeVersionID, "final/v2"),
		source,
		finalValue(t, psdomain.NewRederivationReason, "SOURCE_REVISED/responsibility-outcome/v2"),
	)
	if err != nil {
		t.Fatalf("重派生终局：%v", err)
	}
	return psports.FinalOutcomeRecord{
		Key:           key,
		ContentDigest: "digest-final-v2",
		Finalized:     true,
		Final:         rederived,
		AdoptedAt:     finalAdoptedAt.Add(2 * time.Hour),
	}
}

func formedRederivedFinalRef() veinbox.FormedFinalOutcome {
	formed := formedFinalRef()
	formed.Version = "responsibility-outcome/v2"
	return formed
}

// Covers: AT-VE-174（译装半边）——PS 给的前一代是终局版本，投影要的前身是同一事实引用下
// 的事实版本即前一代的采用版本：按终局版本找回前一代登记，把它键上的版本译进 Supersedes。
func TestARederivedFinalOutcomeSupersedesItsPredecessorsAdoptionVersion(t *testing.T) {
	finder := &finalFinderDouble{
		record: rederivedFinalRecord(t), found: true,
		predecessor: adoptedFinalRecord(t), predecessorFound: true,
	}
	subject, derive := newFinalSubject(t, finder)

	if err := subject.HandleFormedFinalOutcome(t.Context(), formedRederivedFinalRef()); !errors.Is(err, errDeriveReached) {
		t.Fatalf("err = %v, want 派生被调用", err)
	}

	if finder.byVersionCalls != 1 ||
		finder.byVersionTenant.String() != "tenant-1" ||
		finder.byVersionParcel.String() != "parcel-1" ||
		finder.byVersionVersion.String() != "final/v1" {
		t.Fatalf("按终局版本问的是 %d 次（%q, %q, %q），want 一次问本包裹的 final/v1",
			finder.byVersionCalls, finder.byVersionTenant.String(),
			finder.byVersionParcel.String(), finder.byVersionVersion.String())
	}
	fact := derive.command.Fact
	if fact.Fact.String() != "final-outcome/parcel-1/EFFECTIVE_DELIVERY" {
		t.Fatalf("事实引用 = %q；前后两代必须在同一引用下替代才成立", fact.Fact.String())
	}
	if fact.Version.String() != "responsibility-outcome/v2" {
		t.Fatalf("版本 = %q, want 本代的采用版本", fact.Version.String())
	}
	if fact.Supersedes.String() != "responsibility-outcome/v1" {
		t.Fatalf("前身 = %q, want 前一代的采用版本 responsibility-outcome/v1——不是终局版本 final/v1", fact.Supersedes.String())
	}
}

// Covers: AT-VE-175——前一代读时依赖调不通是续办：不派生，更不把本代当无前身的首登派生
// （那样两代终局又会同时当前有效，而重投时前身一格还会变，派生编排会把它判成同版本冲突）。
func TestAnUnreadablePredecessorIsContinuableUndecided(t *testing.T) {
	finder := &finalFinderDouble{
		record: rederivedFinalRecord(t), found: true,
		predecessorErr: errors.New("store unavailable"),
	}
	subject, derive := newFinalSubject(t, finder)

	err := subject.HandleFormedFinalOutcome(t.Context(), formedRederivedFinalRef())
	if !errors.Is(err, adapter.ErrFinalNotVisible) {
		t.Fatalf("err = %v, want ErrFinalNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("前一代读不通不该走到派生")
	}
}

// Covers: AT-VE-176——前一代找不到，或找回的不是本包裹同一责任结果种类的已采用终局：账已
// 坏，确定性拒收不进未决。换种类那一格今天 PS 不会给出（异种一律不采用）；ADR-0138 若让它
// 成为正常路径，要在那时另裁，不在这里跨事实引用指名前身。
func TestAMissingOrForeignPredecessorIsInconsistentNotUndecided(t *testing.T) {
	for name, reshape := range map[string]func(double *finalFinderDouble){
		"前一代不在账上": func(double *finalFinderDouble) {
			double.predecessorFound = false
		},
		"前一代是不采用记录": func(double *finalFinderDouble) {
			double.predecessor = notAdoptedFinalRecord(t)
		},
		"前一代在另一个包裹": func(double *finalFinderDouble) {
			double.predecessor.Key.Parcel = finalValue(t, psdomain.NewDeclaredParcelID, "parcel-2")
		},
		"前一代是另一种责任结果": func(double *finalFinderDouble) {
			double.predecessor.Key.Kind = psdomain.ReturnCompletedOutcome
		},
		"找回的终局版本不是所问那一版": func(double *finalFinderDouble) {
			double.predecessor = rederivedFinalRecord(t)
			double.predecessor.Key.Version = finalValue(t, psdomain.NewResponsibilityOutcomeVersion, "responsibility-outcome/v0")
		},
		"前一代与本代同一采用版本": func(double *finalFinderDouble) {
			double.predecessor.Key.Version = double.record.Key.Version
		},
	} {
		t.Run(name, func(t *testing.T) {
			finder := &finalFinderDouble{
				record: rederivedFinalRecord(t), found: true,
				predecessor: adoptedFinalRecord(t), predecessorFound: true,
			}
			reshape(finder)
			subject, derive := newFinalSubject(t, finder)

			err := subject.HandleFormedFinalOutcome(t.Context(), formedRederivedFinalRef())
			if !errors.Is(err, adapter.ErrFinalRecordInconsistent) {
				t.Fatalf("err = %v, want ErrFinalRecordInconsistent", err)
			}
			if derive.calls != 0 {
				t.Fatal("前一代对不上不该走到派生")
			}
		})
	}
}
