package transportfulfillment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/finalconsume"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/labelfinal"
	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证收寄那一路的处理方（lc/25 做法 2–3）：按信封所指的（租户 + 事实 + 版本）取回 TF 首次有效收寄
// **指名那一代**、核键与本体一致、把（事实、版本、业务发生时间）折成 CarrierFirstEffectivePickupSpec 交
// 与面单交易那一路（lc/26）、关闭 / 重开那一路（lc/27）共用的处理方核；可见性滞后 / 不变量破坏 / 失效版本 /
// 译不出各自可识别。核与判断编排都是真的（labelfinal.ParcelJudgmentCore + psapplication.JudgeLabelServiceFinalHandler），
// 读口（TF 收寄登记册、包裹反查、面单交易册、继续尝试登记册、取消视图）与采用路径用替身——LabelServiceFinalOutcome
// 的每一格（LabelServiceFinalAdopted / LabelServiceNotFinalOutcome / LabelServiceCancellationStandsOutcome /
// LabelServiceJudgmentNotAccepted / LabelServiceJudgmentUndecided）要由真编排从夹具里判出来，替身直接吐结果就成了
// 对着自己写的翻译表打勾。

var (
	firstPickupAt   = time.Date(2026, 9, 11, 9, 30, 0, 0, time.UTC)
	pickupJudgedAt  = time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	pickupCorrected = time.Date(2026, 9, 11, 9, 5, 0, 0, time.UTC)
)

type carrierPickupFinderDouble struct {
	records map[tfports.CarrierFirstEffectivePickupKey]tfports.CarrierFirstEffectivePickupRecord
	err     error
	asked   []tfports.CarrierFirstEffectivePickupKey
}

func (double *carrierPickupFinderDouble) FindByKey(
	_ context.Context, key tfports.CarrierFirstEffectivePickupKey,
) (tfports.CarrierFirstEffectivePickupRecord, bool, error) {
	double.asked = append(double.asked, key)
	if double.err != nil {
		return tfports.CarrierFirstEffectivePickupRecord{}, false, double.err
	}
	record, found := double.records[key]
	return record, found, nil
}

type labelTargetViewDouble struct {
	target psdomain.CurrentAcceptedParcelTarget
	found  bool
	err    error
	tenant string
	parcel string
}

func (double *labelTargetViewDouble) FindCurrentAcceptedByParcel(
	_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.CurrentAcceptedParcelTarget, bool, error) {
	double.tenant = tenant.String()
	double.parcel = parcel.String()
	if double.err != nil {
		return psdomain.CurrentAcceptedParcelTarget{}, false, double.err
	}
	return double.target, double.found, nil
}

type labelTransactionsViewDouble struct{ err error }

func (double *labelTransactionsViewDouble) ListByCoveredParcel(
	context.Context, psdomain.TenantID, psdomain.DeclaredParcelID,
) ([]psdomain.LabelTransaction, error) {
	return nil, double.err
}

// openRegisterViewDouble 答「没开过册」——编排把它当空册，关闭路径不成立。
type openRegisterViewDouble struct{}

func (openRegisterViewDouble) FindByParcel(
	context.Context, psdomain.TenantID, psdomain.DeclaredParcelID,
) (psdomain.ContinuedAttemptRegister, bool, error) {
	return psdomain.ContinuedAttemptRegister{}, false, nil
}

type standingCancellationDouble struct{ cancelled bool }

func (double *standingCancellationDouble) FindCancellation(
	context.Context, psdomain.TenantID, psdomain.DeclaredParcelID,
) (psdomain.ParcelCancellation, bool, error) {
	return psdomain.ParcelCancellation{}, double.cancelled, nil
}

// labelFinalAdopterDouble 记下判断交给采用路径的命令，交回零值结果：采用各格怎么译归 finalconsume 自己的用例，
// 这里只证「判出终局的那一格确实交到了采用路径并经 finalconsume 收口」。
type labelFinalAdopterDouble struct {
	commands []psapplication.FormParcelFinalCommand
}

func (double *labelFinalAdopterDouble) Handle(
	_ context.Context, command psapplication.FormParcelFinalCommand,
) (psapplication.FormParcelFinalResult, error) {
	double.commands = append(double.commands, command)
	return psapplication.FormParcelFinalResult{}, nil
}

// recordingLabelJudge 包住真判断编排，记下核折出来的命令——收寄事实折进去的三件只能在这里看。
type recordingLabelJudge struct {
	inner    *psapplication.JudgeLabelServiceFinalHandler
	commands []psapplication.JudgeLabelServiceFinalCommand
}

func (judge *recordingLabelJudge) Handle(
	ctx context.Context, command psapplication.JudgeLabelServiceFinalCommand,
) (psapplication.LabelServiceFinalResult, error) {
	judge.commands = append(judge.commands, command)
	return judge.inner.Handle(ctx, command)
}

type carrierPickupFixture struct {
	handler      *adapter.JudgeOnCarrierFirstEffectivePickupAdapter
	pickups      *carrierPickupFinderDouble
	targets      *labelTargetViewDouble
	transactions *labelTransactionsViewDouble
	cancels      *standingCancellationDouble
	adopter      *labelFinalAdopterDouble
	judge        *recordingLabelJudge
}

func newCarrierPickupFixture(t *testing.T) *carrierPickupFixture {
	t.Helper()
	pickups := &carrierPickupFinderDouble{records: map[tfports.CarrierFirstEffectivePickupKey]tfports.CarrierFirstEffectivePickupRecord{}}
	targets := &labelTargetViewDouble{target: pickupTarget(t), found: true}
	transactions := &labelTransactionsViewDouble{}
	cancels := &standingCancellationDouble{}
	adopter := &labelFinalAdopterDouble{}
	judge := &recordingLabelJudge{inner: psapplication.NewJudgeLabelServiceFinalHandler(psapplication.JudgeLabelServiceFinalDeps{
		Transactions:  transactions,
		Registers:     openRegisterViewDouble{},
		Cancellations: cancels,
		Adoption:      adopter,
		Clock:         fixedClock{at: pickupJudgedAt},
	})}
	core, err := labelfinal.NewParcelJudgmentCore(targets, judge)
	if err != nil {
		t.Fatalf("new core: %v", err)
	}
	handler, err := adapter.NewJudgeOnCarrierFirstEffectivePickupAdapter(pickups, core)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	return &carrierPickupFixture{handler: handler, pickups: pickups, targets: targets,
		transactions: transactions, cancels: cancels, adopter: adopter, judge: judge}
}

func (f *carrierPickupFixture) put(record tfports.CarrierFirstEffectivePickupRecord) {
	f.pickups.records[record.Key] = record
}

func (f *carrierPickupFixture) handle(t *testing.T, registered psinbox.RegisteredCarrierFirstEffectivePickup) error {
	t.Helper()
	return f.handler.HandleRegisteredCarrierFirstEffectivePickup(context.Background(), registered)
}

func registeredCarrierPickupRef(version string) psinbox.RegisteredCarrierFirstEffectivePickup {
	return psinbox.RegisteredCarrierFirstEffectivePickup{
		TenantID: "tenant-1", Fact: "CFEP-1", Version: version, Object: "parcel-1",
	}
}

func carrierPickupKey(t *testing.T, tenant, fact, version string) tfports.CarrierFirstEffectivePickupKey {
	t.Helper()
	return tfports.CarrierFirstEffectivePickupKey{
		TenantID: value(t, tfdomain.NewTenantID, tenant),
		Fact:     value(t, tfdomain.NewCarrierFirstEffectivePickupReference, fact),
		Version:  value(t, tfdomain.NewCarrierFirstEffectivePickupVersion, version),
	}
}

func carrierPickupBases(t *testing.T, sourceVersion string) []tfdomain.CarrierPickupBasis {
	t.Helper()
	basis, err := tfdomain.NewCarrierPickupBasis(tfdomain.CarrierDirectPickupScan, "SCAN-1", sourceVersion)
	if err != nil {
		t.Fatalf("basis: %v", err)
	}
	return []tfdomain.CarrierPickupBasis{basis}
}

// formedCarrierPickup 真经 TF 领域构造门造一版已形成的首登（对象 parcel-1，业务发生时间 firstPickupAt）。
func formedCarrierPickup(t *testing.T, object string) tfdomain.CarrierFirstEffectivePickup {
	t.Helper()
	carrier, err := tfdomain.NewCarrierSubject(tfdomain.ExternalCarrierParty, "PARTY-1")
	if err != nil {
		t.Fatalf("carrier subject: %v", err)
	}
	pickup, err := tfdomain.FormCarrierFirstEffectivePickup(tfdomain.CarrierFirstEffectivePickupSpec{
		TenantID:   value(t, tfdomain.NewTenantID, "tenant-1"),
		Object:     value(t, tfdomain.NewCarriedObjectReference, object),
		Fact:       value(t, tfdomain.NewCarrierFirstEffectivePickupReference, "CFEP-1"),
		Version:    value(t, tfdomain.NewCarrierFirstEffectivePickupVersion, "CFEV-1"),
		Carrier:    carrier,
		OccurredAt: firstPickupAt,
		JudgedAt:   pickupJudgedAt,
		Bases:      carrierPickupBases(t, "v1"),
	})
	if err != nil {
		t.Fatalf("form pickup: %v", err)
	}
	return pickup
}

func recordOf(t *testing.T, pickup tfdomain.CarrierFirstEffectivePickup) tfports.CarrierFirstEffectivePickupRecord {
	t.Helper()
	return tfports.CarrierFirstEffectivePickupRecord{
		Key: tfports.CarrierFirstEffectivePickupKey{
			TenantID: pickup.TenantID(), Fact: pickup.Fact(), Version: pickup.Version(),
		},
		Pickup:     pickup,
		RecordedAt: pickupJudgedAt.Add(time.Second),
	}
}

// Covers: 做法 2 的主干——两代事实同链在册（v2 替代 v1、业务时间不同），信封指 v1：取回的是 v1 不是链尾；
// 命令的委托来源身份与委托标识取自反查结果、包裹取自本体的载运对象、收寄三件取自 v1（EffectiveAt 是 TF
// 事实上的业务发生时间，ADR-0135 决定三）；租户串从信封带到反查。已形成的收寄 → FINAL_BY_FIRST_PICKUP →
// 交采用路径：执行证据 `事实@版本`、来源版本 = 收寄版本、生效时间 = 业务发生时间；结论由 finalconsume 收口
// （替身交回集合外的零值结果，finalconsume 响亮报错——这正证明结果走到了它手上）。
func TestARegisteredCarrierPickupIsFetchedAtTheEnvelopeVersionAndJudgedAsFinal(t *testing.T) {
	f := newCarrierPickupFixture(t)
	first := formedCarrierPickup(t, "parcel-1")
	second, err := first.Supersede(tfdomain.CarrierPickupSupersession{
		Version:    value(t, tfdomain.NewCarrierFirstEffectivePickupVersion, "CFEV-2"),
		Carrier:    mustCarrier(t, first),
		OccurredAt: pickupCorrected,
		JudgedAt:   pickupJudgedAt.Add(time.Minute),
		Bases:      carrierPickupBases(t, "v2"),
	})
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	f.put(recordOf(t, first))
	f.put(recordOf(t, second))

	err = f.handle(t, registeredCarrierPickupRef("CFEV-1"))
	if !errors.Is(err, finalconsume.ErrUnexpectedFinalOutcome) {
		t.Fatalf("采用结果应经 finalconsume 收口，实得：%v", err)
	}
	if len(f.pickups.asked) != 1 || f.pickups.asked[0] != carrierPickupKey(t, "tenant-1", "CFEP-1", "CFEV-1") {
		t.Fatalf("取回用的键 = %+v，want 信封所指的 v1", f.pickups.asked)
	}
	if f.targets.tenant != "tenant-1" || f.targets.parcel != "parcel-1" {
		t.Fatalf("反查用的键 = %s/%s", f.targets.tenant, f.targets.parcel)
	}
	if len(f.judge.commands) != 1 {
		t.Fatalf("判断次数 = %d, want 1", len(f.judge.commands))
	}
	command := f.judge.commands[0]
	if command.Identity != identity(t) || command.ShipmentRequestID.String() != "request-1" ||
		command.Parcel.String() != "parcel-1" {
		t.Fatalf("命令折错了：%#v", command)
	}
	want := psdomain.CarrierFirstEffectivePickupSpec{
		Fact:        value(t, psdomain.NewCarrierFirstEffectivePickupFactReference, "CFEP-1"),
		Version:     value(t, psdomain.NewCarrierFirstEffectivePickupFactVersion, "CFEV-1"),
		EffectiveAt: firstPickupAt,
	}
	if command.FirstEffectivePickup != want {
		t.Fatalf("收寄三件 = %#v，want %#v（v1 那一代，不是链尾 v2）", command.FirstEffectivePickup, want)
	}
	if len(f.adopter.commands) != 1 {
		t.Fatalf("采用路径调用次数 = %d, want 1", len(f.adopter.commands))
	}
	adoption := f.adopter.commands[0]
	if adoption.Identity != identity(t) || adoption.ShipmentRequestID.String() != "request-1" ||
		adoption.Outcome.Parcel.String() != "parcel-1" {
		t.Fatalf("交给采用路径的命令指错了对象：%#v", adoption)
	}
	if adoption.Outcome.Kind != psdomain.LabelServiceOutcome ||
		adoption.Outcome.Execution.String() != "CFEP-1@CFEV-1" ||
		adoption.Outcome.Version.String() != "CFEV-1" ||
		!adoption.Outcome.OccurredAt.Equal(firstPickupAt) {
		t.Fatalf("责任结果 = %#v，want 面单渠道服务非取消终局、证据 CFEP-1@CFEV-1、版本 CFEV-1、生效 %s",
			adoption.Outcome, firstPickupAt)
	}
}

// Covers: 信封指向**替代代** v2——TF `Supersede` 的产物仍是 `CarrierPickupFormed`（ADR-0135 决定六：更正沿来源更正
// 关系换替代版本，已形成的性质不变），到这里走已形成格、不走失效格：取回用的键是 v2 不是 v1，折进命令的
// 收寄三件与交给采用路径的执行证据 / 版本 / 生效时间全是 v2 那一代的（EffectiveAt = v2 的业务发生时间，
// 即更正后的时间）。与上一条合起来：三种入队版本里已形成 / 替代各有直接用例，失效见下文显式未决那一条。
func TestASupersedingCarrierPickupVersionIsJudgedAtItsOwnOccurrenceTime(t *testing.T) {
	f := newCarrierPickupFixture(t)
	first := formedCarrierPickup(t, "parcel-1")
	second, err := first.Supersede(tfdomain.CarrierPickupSupersession{
		Version:    value(t, tfdomain.NewCarrierFirstEffectivePickupVersion, "CFEV-2"),
		Carrier:    mustCarrier(t, first),
		OccurredAt: pickupCorrected,
		JudgedAt:   pickupJudgedAt.Add(time.Minute),
		Bases:      carrierPickupBases(t, "v2"),
	})
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	if second.Result() != tfdomain.CarrierPickupFormed {
		t.Fatalf("替代代的结果 = %q，want 已形成——前提不成立时下面证的就不是替代格", second.Result())
	}
	f.put(recordOf(t, first))
	f.put(recordOf(t, second))

	err = f.handle(t, registeredCarrierPickupRef("CFEV-2"))
	if !errors.Is(err, finalconsume.ErrUnexpectedFinalOutcome) {
		t.Fatalf("替代代应照已形成格判到采用路径并经 finalconsume 收口，实得：%v", err)
	}
	if len(f.pickups.asked) != 1 || f.pickups.asked[0] != carrierPickupKey(t, "tenant-1", "CFEP-1", "CFEV-2") {
		t.Fatalf("取回用的键 = %+v，want 信封所指的 v2", f.pickups.asked)
	}
	if len(f.judge.commands) != 1 {
		t.Fatalf("判断次数 = %d, want 1", len(f.judge.commands))
	}
	want := psdomain.CarrierFirstEffectivePickupSpec{
		Fact:        value(t, psdomain.NewCarrierFirstEffectivePickupFactReference, "CFEP-1"),
		Version:     value(t, psdomain.NewCarrierFirstEffectivePickupFactVersion, "CFEV-2"),
		EffectiveAt: pickupCorrected,
	}
	if got := f.judge.commands[0].FirstEffectivePickup; got != want {
		t.Fatalf("收寄三件 = %#v，want %#v（v2 那一代，生效时间是更正后的业务发生时间）", got, want)
	}
	if len(f.adopter.commands) != 1 {
		t.Fatalf("采用路径调用次数 = %d, want 1", len(f.adopter.commands))
	}
	outcome := f.adopter.commands[0].Outcome
	if outcome.Kind != psdomain.LabelServiceOutcome ||
		outcome.Execution.String() != "CFEP-1@CFEV-2" ||
		outcome.Version.String() != "CFEV-2" ||
		!outcome.OccurredAt.Equal(pickupCorrected) {
		t.Fatalf("责任结果 = %#v，want 证据 CFEP-1@CFEV-2、版本 CFEV-2、生效 %s", outcome, pickupCorrected)
	}
}

func mustCarrier(t *testing.T, pickup tfdomain.CarrierFirstEffectivePickup) tfdomain.CarrierSubject {
	t.Helper()
	carrier, ok := pickup.Carrier()
	if !ok {
		t.Fatal("formed pickup carries no carrier")
	}
	return carrier
}

// Covers: 信封不带载运对象时从本体读——对象不是键的一维，本体才是权威。
func TestAnEnvelopeWithoutAnObjectReadsItFromThePickup(t *testing.T) {
	f := newCarrierPickupFixture(t)
	f.put(recordOf(t, formedCarrierPickup(t, "parcel-1")))
	registered := registeredCarrierPickupRef("CFEV-1")
	registered.Object = ""

	if err := f.handle(t, registered); !errors.Is(err, finalconsume.ErrUnexpectedFinalOutcome) {
		t.Fatalf("应照常判到采用路径，实得：%v", err)
	}
	if f.targets.parcel != "parcel-1" {
		t.Fatalf("反查包裹 = %q，want 本体上的载运对象", f.targets.parcel)
	}
}

// Covers: CANCELLATION_STANDS → 入账，不交采用路径——取消在先时收寄不盖非取消终局：PS CONTEXT
// 「除已经形成的有效取消结果外，实际承运商首次有效收寄即形成终局」那句的前半。
func TestAStandingCancellationOutranksTheFirstPickup(t *testing.T) {
	f := newCarrierPickupFixture(t)
	f.put(recordOf(t, formedCarrierPickup(t, "parcel-1")))
	f.cancels.cancelled = true

	if err := f.handle(t, registeredCarrierPickupRef("CFEV-1")); err != nil {
		t.Fatalf("CANCELLATION_STANDS 应入账，实得：%v", err)
	}
	if len(f.judge.commands) != 1 || len(f.adopter.commands) != 0 {
		t.Fatalf("判断 %d 次、采用 %d 次，want 1 / 0", len(f.judge.commands), len(f.adopter.commands))
	}
}

// Covers: LABEL_SERVICE_NOT_FINAL 在本路**结构上到不了**：夹具是面单交易那一路判 NOT_FINAL 的输入（没开过册、
// 无交易），而本路的命令总带一份已形成的收寄，PS CONTEXT「实际承运商首次有效收寄即形成终局」先于
// 「否则，只有在当前受控关闭已经生效且未被重开」那条关闭路径成立。这条用例钉的是这一点，
// LabelServiceNotFinalOutcome 那一格在翻译表里的译法由 labelfinal 自己的用例守。
func TestNotFinalIsStructurallyUnreachableOnThePickupRoute(t *testing.T) {
	f := newCarrierPickupFixture(t)
	f.put(recordOf(t, formedCarrierPickup(t, "parcel-1")))

	if err := f.handle(t, registeredCarrierPickupRef("CFEV-1")); !errors.Is(err, finalconsume.ErrUnexpectedFinalOutcome) {
		t.Fatalf("同样的册与交易在交易那一路判 NOT_FINAL，本路必须判出终局并交采用，实得：%v", err)
	}
	if len(f.adopter.commands) != 1 {
		t.Fatalf("采用路径调用次数 = %d, want 1", len(f.adopter.commands))
	}
}

// Covers: JUDGMENT_UNDECIDED（读口答不出）→ 未决哨兵，重投会改变结果；不交采用。
func TestAnUndecidedJudgmentOnThePickupRouteIsASentinel(t *testing.T) {
	f := newCarrierPickupFixture(t)
	f.put(recordOf(t, formedCarrierPickup(t, "parcel-1")))
	f.transactions.err = errors.New("label transactions unavailable")

	err := f.handle(t, registeredCarrierPickupRef("CFEV-1"))
	if !errors.Is(err, labelfinal.ErrJudgmentUndecided) {
		t.Fatalf("err = %v, want labelfinal.ErrJudgmentUndecided", err)
	}
	if len(f.adopter.commands) != 0 {
		t.Fatal("未决却交了采用路径")
	}
}

// Covers: REQUEST_NOT_ACCEPTED（反查交回一份立不起的目标）→ 适配器缺陷，响亮报错、不进未决。
func TestARejectedCommandOnThePickupRouteIsLoud(t *testing.T) {
	f := newCarrierPickupFixture(t)
	f.put(recordOf(t, formedCarrierPickup(t, "parcel-1")))
	f.targets.target = psdomain.CurrentAcceptedParcelTarget{}

	err := f.handle(t, registeredCarrierPickupRef("CFEV-1"))
	if !errors.Is(err, labelfinal.ErrJudgmentNotAccepted) {
		t.Fatalf("err = %v, want labelfinal.ErrJudgmentNotAccepted", err)
	}
	if errors.Is(err, labelfinal.ErrJudgmentUndecided) {
		t.Fatal("命令立不起被登成了未决")
	}
}

// Covers: 反查不中 → labelfinal.ErrParcelTargetNotFound（不猜委托；载运对象可能是集运单元）；歧义原样上抛。
func TestAMissingParcelTargetForACarrierPickupIsContinuableUndecided(t *testing.T) {
	f := newCarrierPickupFixture(t)
	f.put(recordOf(t, formedCarrierPickup(t, "parcel-1")))
	f.targets.found = false

	if err := f.handle(t, registeredCarrierPickupRef("CFEV-1")); !errors.Is(err, labelfinal.ErrParcelTargetNotFound) {
		t.Fatalf("反查不中 err = %v, want labelfinal.ErrParcelTargetNotFound", err)
	}
	if len(f.judge.commands) != 0 {
		t.Fatal("反查不中仍去判了")
	}

	f = newCarrierPickupFixture(t)
	f.put(recordOf(t, formedCarrierPickup(t, "parcel-1")))
	f.targets.err = psdomain.ErrAmbiguousParcelTarget
	if err := f.handle(t, registeredCarrierPickupRef("CFEV-1")); !errors.Is(err, psdomain.ErrAmbiguousParcelTarget) {
		t.Fatalf("歧义 err = %v, want ErrAmbiguousParcelTarget 原样上抛", err)
	}
}

// Covers: 按键取不回 → 可见性滞后（续办，重投会改变结果）；登记册读口报错同一格。都不去判。
func TestAMissingCarrierPickupIsContinuableUndecided(t *testing.T) {
	f := newCarrierPickupFixture(t)

	err := f.handle(t, registeredCarrierPickupRef("CFEV-1"))
	if !errors.Is(err, adapter.ErrCarrierPickupNotVisible) {
		t.Fatalf("err = %v, want ErrCarrierPickupNotVisible", err)
	}

	f = newCarrierPickupFixture(t)
	f.pickups.err = errors.New("registry unavailable")
	err = f.handle(t, registeredCarrierPickupRef("CFEV-1"))
	if !errors.Is(err, adapter.ErrCarrierPickupNotVisible) {
		t.Fatalf("读口报错 err = %v, want ErrCarrierPickupNotVisible", err)
	}
	if len(f.judge.commands) != 0 {
		t.Fatal("取不回仍去判了")
	}
}

// Covers: 键与本体不符 → 不一致格（仓储 / 数据不变量已破，ADR-0029），与可见性滞后分开、不进未决哨兵；
// 三种不符各一格：登记交回另一个键、本体的租户与键不符、信封的载运对象与本体不符。都不去判。
func TestACarrierPickupRecordThatDisagreesWithItsKeyIsInconsistentNotInvisible(t *testing.T) {
	envelopeKey := carrierPickupKey(t, "tenant-1", "CFEP-1", "CFEV-1")
	cases := map[string]struct {
		record     tfports.CarrierFirstEffectivePickupRecord
		registered psinbox.RegisteredCarrierFirstEffectivePickup
	}{
		"登记交回另一版": {
			record: func() tfports.CarrierFirstEffectivePickupRecord {
				record := recordOf(t, formedCarrierPickup(t, "parcel-1"))
				record.Key = carrierPickupKey(t, "tenant-1", "CFEP-1", "CFEV-9")
				return record
			}(),
			registered: registeredCarrierPickupRef("CFEV-1"),
		},
		"本体租户与键不符": {
			record: func() tfports.CarrierFirstEffectivePickupRecord {
				record := recordOf(t, formedCarrierPickup(t, "parcel-1"))
				record.Key.TenantID = value(t, tfdomain.NewTenantID, "tenant-2")
				return record
			}(),
			registered: psinbox.RegisteredCarrierFirstEffectivePickup{
				TenantID: "tenant-2", Fact: "CFEP-1", Version: "CFEV-1", Object: "parcel-1",
			},
		},
		"信封对象与本体不符": {
			record:     recordOf(t, formedCarrierPickup(t, "parcel-1")),
			registered: psinbox.RegisteredCarrierFirstEffectivePickup{TenantID: "tenant-1", Fact: "CFEP-1", Version: "CFEV-1", Object: "parcel-2"},
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			f := newCarrierPickupFixture(t)
			f.pickups.records[envelopeKey] = test.record
			f.pickups.records[test.record.Key] = test.record

			err := f.handle(t, test.registered)
			if !errors.Is(err, adapter.ErrCarrierPickupRecordInconsistent) {
				t.Fatalf("err = %v, want ErrCarrierPickupRecordInconsistent", err)
			}
			if errors.Is(err, adapter.ErrCarrierPickupNotVisible) {
				t.Fatal("不变量破坏被登成了可见性滞后")
			}
			if len(f.judge.commands) != 0 {
				t.Fatal("键与本体不符仍去判了")
			}
		})
	}
}

// Covers: 取回的是待确认版本 → 不一致格。TF 侧对待确认版本响亮拒绝入队（ADR-0135 决定七），到这里说明
// 提供方交接口的装配错了，不是等谁；它没有业务发生时间可作终局生效时间，也不得据它判。
func TestAPendingCarrierPickupVersionIsInconsistentNotUndecided(t *testing.T) {
	f := newCarrierPickupFixture(t)
	pending, err := tfdomain.HoldCarrierFirstEffectivePickupPending(tfdomain.PendingCarrierFirstEffectivePickupSpec{
		TenantID: value(t, tfdomain.NewTenantID, "tenant-1"),
		Object:   value(t, tfdomain.NewCarriedObjectReference, "parcel-1"),
		Fact:     value(t, tfdomain.NewCarrierFirstEffectivePickupReference, "CFEP-1"),
		Version:  value(t, tfdomain.NewCarrierFirstEffectivePickupVersion, "CFEV-1"),
		Reason:   tfdomain.PickupCarrierIdentityNotRegistered,
		Material: "ACME EXPRESS",
		JudgedAt: pickupJudgedAt,
		Bases:    carrierPickupBases(t, "v1"),
	})
	if err != nil {
		t.Fatalf("hold pending: %v", err)
	}
	f.put(recordOf(t, pending))

	err = f.handle(t, registeredCarrierPickupRef("CFEV-1"))
	if !errors.Is(err, adapter.ErrCarrierPickupRecordInconsistent) {
		t.Fatalf("err = %v, want ErrCarrierPickupRecordInconsistent", err)
	}
	if len(f.judge.commands) != 0 {
		t.Fatal("待确认版本被拿去判了")
	}
}

// Covers: 票面「要裁的」2——失效版本到达（依据被源更正为不再表达收寄，ADR-0135 决定六）：已据前版形成的非取消
// 终局怎么重派生归 PS owner，未裁之前按红线「未确认规则保持显式未决」停在自己的哨兵上：不吸收、不重派生、
// 不判、零写入。它与可见性滞后 / 不一致都分开——恢复动作是「等 owner 裁定并接上重派生」，不是等依赖也不是查行。
func TestAVoidedCarrierPickupVersionStopsAsExplicitUndecidedWithZeroWrites(t *testing.T) {
	f := newCarrierPickupFixture(t)
	first := formedCarrierPickup(t, "parcel-1")
	voided, err := first.Void(tfdomain.CarrierPickupVoiding{
		Version:  value(t, tfdomain.NewCarrierFirstEffectivePickupVersion, "CFEV-2"),
		JudgedAt: pickupJudgedAt.Add(time.Minute),
		Bases:    carrierPickupBases(t, "v2"),
	})
	if err != nil {
		t.Fatalf("void: %v", err)
	}
	f.put(recordOf(t, first))
	f.put(recordOf(t, voided))

	err = f.handle(t, registeredCarrierPickupRef("CFEV-2"))
	if !errors.Is(err, adapter.ErrVoidedCarrierPickupRederivationUndecided) {
		t.Fatalf("err = %v, want ErrVoidedCarrierPickupRederivationUndecided", err)
	}
	if errors.Is(err, adapter.ErrCarrierPickupNotVisible) || errors.Is(err, adapter.ErrCarrierPickupRecordInconsistent) {
		t.Fatal("失效版本的显式未决与可见性滞后 / 不一致混成了一格")
	}
	if len(f.judge.commands) != 0 || len(f.adopter.commands) != 0 {
		t.Fatalf("失效版本判了 %d 次、采用 %d 次，want 零写入", len(f.judge.commands), len(f.adopter.commands))
	}
}

// Covers: 信封三维译不成 TF 的键 → ErrUntranslatableAnswer（引用坏了，不是等谁），不去取回。
func TestAnUntranslatableCarrierPickupReferenceKeepsItsSentinel(t *testing.T) {
	for name, registered := range map[string]psinbox.RegisteredCarrierFirstEffectivePickup{
		"空租户": {Fact: "CFEP-1", Version: "CFEV-1", Object: "parcel-1"},
		"空事实": {TenantID: "tenant-1", Version: "CFEV-1", Object: "parcel-1"},
		"空版本": {TenantID: "tenant-1", Fact: "CFEP-1", Object: "parcel-1"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCarrierPickupFixture(t)
			err := f.handle(t, registered)
			if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
				t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
			}
			if len(f.pickups.asked) != 0 {
				t.Fatal("译不出仍去取回了")
			}
		})
	}
}

func TestTheCarrierPickupAdapterRefusesNilDependencies(t *testing.T) {
	f := newCarrierPickupFixture(t)
	core, err := labelfinal.NewParcelJudgmentCore(f.targets, f.judge)
	if err != nil {
		t.Fatalf("new core: %v", err)
	}
	if _, err := adapter.NewJudgeOnCarrierFirstEffectivePickupAdapter(nil, core); err == nil {
		t.Fatal("nil 登记册读口被接受了")
	}
	if _, err := adapter.NewJudgeOnCarrierFirstEffectivePickupAdapter(f.pickups, nil); err == nil {
		t.Fatal("nil 处理方核被接受了")
	}
}
