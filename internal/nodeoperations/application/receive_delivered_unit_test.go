package application_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

var deliveredAt = time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)

func mustValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type identityViewDouble struct {
	candidates []domain.ParcelAssociationReference
	// ambiguous 是 PS 随候选答出的歧义信号（ADR-0195）。
	ambiguous bool
	err       error
}

func (double *identityViewDouble) ResolveParcelIdentity(
	_ context.Context,
	_ domain.TenantID,
	_ ports.ExternalMarkObservation,
) (ports.ParcelIdentityAnswer, error) {
	return ports.ParcelIdentityAnswer{Candidates: double.candidates, Ambiguous: double.ambiguous}, double.err
}

type receptionStoreDouble struct {
	byKey map[ports.ReceptionKey]ports.ReceptionRecord
	saved int
	// findErr 让每一次 FindByKey 报错；saveErr 让 Save 报错。
	findErr error
	saveErr error
	// lostRace 让 Save 答「已有记录」而不落册（另一方先提交了同键），readBackErr 让随后读回赢家那一次报错。
	lostRace    bool
	readBackErr error
	saveAsked   bool
	// othersErr 让按包裹读别的作业实物收寄那一次报错；chainErr 让读判断链当前有效版本那一次报错。
	othersErr error
	chainErr  error
}

func newReceptionStore() *receptionStoreDouble {
	return &receptionStoreDouble{byKey: map[ports.ReceptionKey]ports.ReceptionRecord{}}
}

func (double *receptionStoreDouble) FindByKey(
	_ context.Context,
	key ports.ReceptionKey,
) (ports.ReceptionRecord, bool, error) {
	if double.findErr != nil {
		return ports.ReceptionRecord{}, false, double.findErr
	}
	if double.saveAsked && double.readBackErr != nil {
		return ports.ReceptionRecord{}, false, double.readBackErr
	}
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *receptionStoreDouble) Save(
	_ context.Context,
	record ports.ReceptionRecord,
) (ports.ReceptionSaveOutcome, error) {
	double.saveAsked = true
	if double.saveErr != nil {
		return ports.ReceptionSaveOutcomeInvalid, double.saveErr
	}
	if _, exists := double.byKey[record.Key]; exists || double.lostRace {
		return ports.ReceptionAlreadyRecorded, nil
	}
	// 照库面的（租户，前版）唯一：一版至多一个后继。
	if record.Predecessor.String() != "" {
		if _, taken := double.successorOf(record.Key.TenantID, record.Predecessor); taken {
			return ports.ReceptionSaveOutcomeInvalid, ports.ErrPredecessorAlreadySucceeded
		}
	}
	double.byKey[record.Key] = record
	double.saved++
	return ports.ReceptionSaved, nil
}

// FindOtherUnitIntakes 照真库的问法在内存里筛：同租户、收寄关联到这件包裹、作业实物不是 unit，按来源标识定序。
func (double *receptionStoreDouble) FindOtherUnitIntakes(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelAssociationReference,
	unit domain.HandlingUnitID,
) ([]ports.ReceptionRecord, error) {
	if double.othersErr != nil {
		return nil, double.othersErr
	}
	found := []ports.ReceptionRecord{}
	for key, record := range double.byKey {
		association, present := record.Intake.Association()
		if key.TenantID == tenant && present && association == parcel && record.Intake.Unit() != unit {
			found = append(found, record)
		}
	}
	slices.SortFunc(found, func(left, right ports.ReceptionRecord) int {
		return strings.Compare(left.Key.SourceID, right.Key.SourceID)
	})
	return found, nil
}

// CurrentVersion 照真库的走法在内存里沿后继走：同租户里前版是它的那一行，一路走到没有后继的那一版。
func (double *receptionStoreDouble) CurrentVersion(
	_ context.Context,
	tenant domain.TenantID,
	version domain.IntakeResultVersion,
) (domain.IntakeResultVersion, error) {
	if double.chainErr != nil {
		return domain.IntakeResultVersion{}, double.chainErr
	}
	current := version
	for range len(double.byKey) + 1 {
		next, found := double.successorOf(tenant, current)
		if !found {
			return current, nil
		}
		current = next
	}
	return domain.IntakeResultVersion{}, errors.New("判断链没有尽头")
}

func (double *receptionStoreDouble) successorOf(
	tenant domain.TenantID,
	version domain.IntakeResultVersion,
) (domain.IntakeResultVersion, bool) {
	for key, record := range double.byKey {
		if key.TenantID == tenant && record.Predecessor == version {
			return record.JudgmentVersion, true
		}
	}
	return domain.IntakeResultVersion{}, false
}

// recordsOfUnit 交回收寄库里这件作业实物的各行带收寄的记录。
func (double *receptionStoreDouble) recordsOfUnit(unit string) []ports.ReceptionRecord {
	found := []ports.ReceptionRecord{}
	for _, record := range double.byKey {
		if !record.Intake.ReceivedAt().IsZero() && record.Intake.Unit().String() == unit {
			found = append(found, record)
		}
	}
	slices.SortFunc(found, func(left, right ports.ReceptionRecord) int {
		return strings.Compare(left.Key.SourceID, right.Key.SourceID)
	})
	return found
}

type versionFactoryDouble struct {
	next int
	err  error
}

func (double *versionFactoryDouble) NextIntakeResultVersion(_ context.Context) (domain.IntakeResultVersion, error) {
	if double.err != nil {
		return domain.IntakeResultVersion{}, double.err
	}
	double.next++
	return domain.NewIntakeResultVersion("intake-result/v" + string(rune('0'+double.next)))
}

type downstreamDouble struct {
	intents []ports.NodeIntakeHandoffIntent
	err     error
}

func (double *downstreamDouble) HandOffNodeIntake(
	_ context.Context,
	intent ports.NodeIntakeHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

// serviceResultViewDouble 记下被问的（租户，包裹），交回预设的答复或错误。
type serviceResultViewDouble struct {
	results ports.ParcelServiceResults
	err     error
	asked   []string
}

func (double *serviceResultViewDouble) FindParcelServiceResults(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelAssociationReference,
) (ports.ParcelServiceResults, error) {
	double.asked = append(double.asked, tenant.String()+"|"+parcel.String())
	return double.results, double.err
}

type receptionFixture struct {
	handler        *application.ReceiveDeliveredUnitHandler
	identity       *identityViewDouble
	serviceResults *serviceResultViewDouble
	receptions     *receptionStoreDouble
	versions       *versionFactoryDouble
	downstream     *downstreamDouble
}

func (fixture *receptionFixture) deps() application.ReceiveDeliveredUnitDeps {
	return application.ReceiveDeliveredUnitDeps{
		Identity:       fixture.identity,
		ServiceResults: fixture.serviceResults,
		Receptions:     fixture.receptions,
		Versions:       fixture.versions,
		Downstream:     fixture.downstream,
		Clock:          fixedClock{at: deliveredAt.Add(time.Minute)},
	}
}

func newReceptionFixture(t *testing.T) *receptionFixture {
	t.Helper()
	fixture := &receptionFixture{
		identity: &identityViewDouble{candidates: []domain.ParcelAssociationReference{
			mustValue(t, domain.NewParcelAssociationReference, "parcel-1"),
		}},
		serviceResults: &serviceResultViewDouble{},
		receptions:     newReceptionStore(),
		versions:       &versionFactoryDouble{},
		downstream:     &downstreamDouble{},
	}
	handler, err := application.NewReceiveDeliveredUnitHandler(fixture.deps())
	if err != nil {
		t.Fatalf("构造收寄编排：%v", err)
	}
	fixture.handler = handler
	return fixture
}

func receiveCommand(t *testing.T) application.ReceiveDeliveredUnitCommand {
	t.Helper()
	return application.ReceiveDeliveredUnitCommand{
		TenantID:    mustValue(t, domain.NewTenantID, "tenant-1"),
		SourceID:    "delivery-1",
		Node:        mustValue(t, domain.NewNodeReference, "node-origin"),
		DeliveredBy: mustValue(t, domain.NewDeliveringPartyReference, "customer-1"),
		Unit:        mustValue(t, domain.NewHandlingUnitID, "unit-1"),
		Mark:        ports.ExternalMarkObservation{Mark: "BARCODE-1"},
		Claim:       application.ExplicitReception,
		Evidence:    mustValue(t, domain.NewReceptionEvidenceReference, "SIGN-7"),
		OccurredAt:  deliveredAt,
	}
}

// Covers: `AT-NO-015`「已接受网络服务包裹由客户直接送达适用节点——形成节点收寄、节点
// 控制和来源证据；不直接形成正式承诺」——唯一身份关联收寄，控制以收寄为成立依据；
// 交给 PS 的只是发布意图，本编排不造承诺。PS 对这件包裹没有取消与终局时不带标记。
func TestAnExplicitReceptionFormsIntakeAndControl(t *testing.T) {
	fixture := newReceptionFixture(t)
	command := receiveCommand(t)

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.NodeIntakeFormed {
		t.Fatalf("outcome = %q, want INTAKE_FORMED", result.Outcome())
	}
	record, _ := result.Record()
	association, identified := record.Intake.Association()
	if !identified || association.String() != "parcel-1" {
		t.Fatalf("association = %v identified = %v", association, identified)
	}
	if !record.Control.Active() || record.Control.EstablishmentKind() != domain.EstablishedByNodeIntake {
		t.Fatalf("control = %#v; 控制必须以节点收寄为成立依据", record.Control)
	}
	if !record.Intake.ReceivedAt().Equal(deliveredAt) {
		t.Fatalf("received at = %s", record.Intake.ReceivedAt())
	}
	if len(record.ServiceMarkers) != 0 {
		t.Fatalf("PS 没有取消与终局，却带了标记：%v", record.ServiceMarkers)
	}
	if record.ServiceResultCheck != ports.ServiceResultsChecked {
		t.Fatalf("核对态 = %q, want CHECKED——读口答复了", record.ServiceResultCheck)
	}
	if len(fixture.serviceResults.asked) != 1 || fixture.serviceResults.asked[0] != "tenant-1|parcel-1" {
		t.Fatalf("服务结果读口被问 %v，want 恰一次、问的是身份核对出的那一件", fixture.serviceResults.asked)
	}
	if len(fixture.downstream.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.downstream.intents))
	}
}

// Covers: `AT-NO-020`「包裹已经取消但仍实际到站——如实保存接收和控制」与 `AT-NO-021`「已终局包裹仍实际到站——保留
// 终局并记录物理事实」（票 operator-channel/25）：身份唯一成立时编排向 PS 取这件包裹此刻的服务结果，取消落成
// `CANCELLED/<取消决定>`、终局落成 `FINAL/<种类>/<版本>`；标记不阻止接收——收寄与控制照常成立、意图照常交出。
func TestAUniquelyIdentifiedReceptionCarriesTheParcelShipmentServiceResults(t *testing.T) {
	fixture := newReceptionFixture(t)
	fixture.serviceResults.results = ports.ParcelServiceResults{Cancellation: "cancel-9", FinalKind: "SERVICE_FAILED", FinalVersion: "final-3"}

	result, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.NodeIntakeFormed {
		t.Fatalf("outcome = %q, want INTAKE_FORMED——标记不阻止接收", result.Outcome())
	}
	record, _ := result.Record()
	if len(record.ServiceMarkers) != 2 || record.ServiceMarkers[0] != "CANCELLED/cancel-9" || record.ServiceMarkers[1] != "FINAL/SERVICE_FAILED/final-3" {
		t.Fatalf("service markers = %v, want [CANCELLED/cancel-9 FINAL/SERVICE_FAILED/final-3]", record.ServiceMarkers)
	}
	if !record.Control.Active() || len(fixture.downstream.intents) != 1 {
		t.Fatal("带了标记就不建控制或不交意图——标记只限制后续方向性作业")
	}
}

// Covers: ADR-0186「身份唯一，但服务结果读不动：照常`节点收寄已形成`」——收寄、控制、关联、意图都照常，记录的核对态为
// 读不动、标记为空；读口答空时核对态为已查、标记也为空。两份记录的标记逐字相同，只有核对态把「没取得 PS 的答复」与
// 「查过了，没有取消与终局」分开。
func TestAnUnreadableServiceResultViewStillFormsTheIntakeAndSaysSo(t *testing.T) {
	unreadable := newReceptionFixture(t)
	unreadable.serviceResults.err = errors.New("parcel shipment unreachable")
	readable := newReceptionFixture(t)

	formed := map[string]ports.ReceptionRecord{}
	for name, fixture := range map[string]*receptionFixture{"读不动": unreadable, "答空": readable} {
		result, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
		if err != nil {
			t.Fatalf("%s：handle: %v", name, err)
		}
		if result.Outcome() != application.NodeIntakeFormed {
			t.Fatalf("%s：outcome = %q, want INTAKE_FORMED——服务结果读不动不挡已经发生的物理事实", name, result.Outcome())
		}
		record, recorded := result.Record()
		association, identified := record.Intake.Association()
		if !recorded || !identified || association.String() != "parcel-1" || !record.Control.Active() {
			t.Fatalf("%s：记录 = %+v；收寄、关联与控制都该照常成立", name, record)
		}
		if fixture.receptions.saved != 1 || len(fixture.downstream.intents) != 1 {
			t.Fatalf("%s：落册 %d、意图 %d，want 各 1", name, fixture.receptions.saved, len(fixture.downstream.intents))
		}
		if len(record.ServiceMarkers) != 0 {
			t.Fatalf("%s：标记 = %v, want 空", name, record.ServiceMarkers)
		}
		formed[name] = record
	}
	if got := formed["读不动"].ServiceResultCheck; got != ports.ServiceResultsUnreadable {
		t.Fatalf("读不动那条的核对态 = %q, want UNREADABLE", got)
	}
	if got := formed["答空"].ServiceResultCheck; got != ports.ServiceResultsChecked {
		t.Fatalf("答空那条的核对态 = %q, want CHECKED", got)
	}
}

// Covers: ADR-0186「装配缺依赖，改为在构造期拒绝」——依赖缺任一个，构造就交回错误，不留到第一次收寄才暴露成一次
// 依赖故障。
func TestAMissingDependencyIsRefusedAtConstruction(t *testing.T) {
	cases := map[string]func(*application.ReceiveDeliveredUnitDeps){
		"身份核对读口": func(deps *application.ReceiveDeliveredUnitDeps) { deps.Identity = nil },
		"服务结果读口": func(deps *application.ReceiveDeliveredUnitDeps) { deps.ServiceResults = nil },
		"收寄库":    func(deps *application.ReceiveDeliveredUnitDeps) { deps.Receptions = nil },
		"版本签发":   func(deps *application.ReceiveDeliveredUnitDeps) { deps.Versions = nil },
		"意图交付":   func(deps *application.ReceiveDeliveredUnitDeps) { deps.Downstream = nil },
		"时钟":     func(deps *application.ReceiveDeliveredUnitDeps) { deps.Clock = nil },
	}
	for name, drop := range cases {
		t.Run(name, func(t *testing.T) {
			deps := newReceptionFixture(t).deps()
			drop(&deps)
			if handler, err := application.NewReceiveDeliveredUnitHandler(deps); err == nil || handler != nil {
				t.Fatalf("缺%s却构造成功（handler=%v err=%v）", name, handler, err)
			}
		})
	}
}

// Covers: ADR-0186「身份核对读不动」与「NO 自身落不了册」——编排交回 error、不形成答案：零落册、零意图，也不拿空候选顶替
// （ADR-0089）。端点据此答 5xx，设备留队重投；前线导入记未决。
func TestDependencyFailuresFormNoAnswer(t *testing.T) {
	failure := errors.New("dependency down")
	cases := map[string]func(*receptionFixture){
		"身份核对读口报错":         func(fixture *receptionFixture) { fixture.identity.err = failure },
		"收寄库 FindByKey 报错": func(fixture *receptionFixture) { fixture.receptions.findErr = failure },
		"收寄库 Save 报错":      func(fixture *receptionFixture) { fixture.receptions.saveErr = failure },
		"并发读回赢家报错": func(fixture *receptionFixture) {
			fixture.receptions.lostRace, fixture.receptions.readBackErr = true, failure
		},
		"并发读回读不到赢家":    func(fixture *receptionFixture) { fixture.receptions.lostRace = true },
		"版本签发报错":       func(fixture *receptionFixture) { fixture.versions.err = failure },
		"按包裹读别的实物收寄报错": func(fixture *receptionFixture) { fixture.receptions.othersErr = failure },
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReceptionFixture(t)
			arrange(fixture)
			result, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
			if err == nil {
				t.Fatalf("outcome = %q, want error——没形成答案就不能答一格业务结果", result.Outcome())
			}
			if errors.Is(err, application.ErrReceptionCommandRejected) {
				t.Fatalf("依赖故障被当成了输入缺陷：%v", err)
			}
			if fixture.receptions.saved != 0 || len(fixture.downstream.intents) != 0 {
				t.Fatalf("落册 %d、意图 %d, want 都是 0", fixture.receptions.saved, len(fixture.downstream.intents))
			}
		})
	}
}

// Covers: ADR-0186 与 ADR-0014「规范化版本不同不是冲突」——已存摘要的形状版本认不出，无从判断是重放还是冲突，与收寄库
// 读不出同一种处理：交回 error，不答冲突、不另落一条。
func TestAnUnknownStoredDigestShapeFormsNoAnswer(t *testing.T) {
	fixture := newReceptionFixture(t)
	command := receiveCommand(t)
	if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
		t.Fatalf("首投：%v", err)
	}
	key := ports.ReceptionKey{TenantID: command.TenantID, SourceID: command.SourceID}
	record := fixture.receptions.byKey[key]
	record.ContentDigest = "NOC-9:9938edb9fffaab6f67866d4ed949ee826323c990a1a5ffc322c9b7f3ad19b430"
	fixture.receptions.byKey[key] = record

	result, err := fixture.handler.Handle(context.Background(), command)
	if err == nil {
		t.Fatalf("outcome = %q, want error", result.Outcome())
	}
	if fixture.receptions.saved != 1 || len(fixture.downstream.intents) != 1 {
		t.Fatalf("落册 %d、意图 %d, want 仍是首投那一份", fixture.receptions.saved, len(fixture.downstream.intents))
	}
}

// Covers: ADR-0186「输入缺陷两支，交回被拒类错误」——缺租户、来源、节点、实物或发生时间，与接收观察不在三态之内，重发同样
// 的内容不会变好：交回 ErrReceptionCommandRejected，端点据此答 400，前线导入记被拒；零落册、零意图。
func TestInputDefectsAreRejected(t *testing.T) {
	cases := map[string]func(*application.ReceiveDeliveredUnitCommand){
		"缺来源身份": func(command *application.ReceiveDeliveredUnitCommand) { command.SourceID = " " },
		"缺发生时间": func(command *application.ReceiveDeliveredUnitCommand) { command.OccurredAt = time.Time{} },
		"未知接收观察": func(command *application.ReceiveDeliveredUnitCommand) {
			command.Claim = application.ReceptionClaimKindInvalid
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReceptionFixture(t)
			command := receiveCommand(t)
			mutate(&command)
			result, err := fixture.handler.Handle(context.Background(), command)
			if !errors.Is(err, application.ErrReceptionCommandRejected) {
				t.Fatalf("outcome = %q err = %v, want ErrReceptionCommandRejected", result.Outcome(), err)
			}
			if fixture.receptions.saved != 0 || len(fixture.downstream.intents) != 0 || len(fixture.serviceResults.asked) != 0 {
				t.Fatal("被拒的输入不该落册、交意图或问读口")
			}
		})
	}
}

// Covers: ADR-0186「不补查，也不发续办引用」与 `AT-NO-016`——读不动那条落册之后，同一来源、同一内容重投答`已有结果`：不再
// 落一条、不重签版本、不再问读口，核对态照读回的那份答读不动，即使此刻读口已经读得动。
func TestReplayingAnUnreadableReceptionAnswersTheRecordedCheck(t *testing.T) {
	fixture := newReceptionFixture(t)
	fixture.serviceResults.err = errors.New("parcel shipment unreachable")
	if _, err := fixture.handler.Handle(context.Background(), receiveCommand(t)); err != nil {
		t.Fatalf("首投：%v", err)
	}
	fixture.serviceResults.err = nil
	fixture.serviceResults.results = ports.ParcelServiceResults{Cancellation: "cancel-9"}

	replay, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
	if err != nil {
		t.Fatalf("重投：%v", err)
	}
	if replay.Outcome() != application.ReceptionExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT", replay.Outcome())
	}
	record, _ := replay.Record()
	if record.ServiceResultCheck != ports.ServiceResultsUnreadable || len(record.ServiceMarkers) != 0 {
		t.Fatalf("重投读回 核对态 %q 标记 %v, want 读不动且无标记——不补查", record.ServiceResultCheck, record.ServiceMarkers)
	}
	if fixture.receptions.saved != 1 || fixture.versions.next != 1 || len(fixture.serviceResults.asked) != 1 {
		t.Fatalf("落册 %d、签发 %d、问读口 %d 次, want 各 1", fixture.receptions.saved, fixture.versions.next, len(fixture.serviceResults.asked))
	}
}

// Covers: 票 operator-channel/25 裁定「按什么键查 → 取 (b)，在编排里、身份唯一成立时按包裹查」——待识别（零候选）与身份冲突
// （多候选）没有可问的包裹，明确拒收与仅扫描不经身份核对，四支都不问服务结果读口，也不带标记与核对态。
func TestTheServiceResultViewIsAskedOnlyForAUniquelyIdentifiedParcel(t *testing.T) {
	cases := map[string]struct {
		candidates []string
		mutate     func(*application.ReceiveDeliveredUnitCommand)
	}{
		"待识别":  {candidates: nil},
		"身份冲突": {candidates: []string{"parcel-1", "parcel-2"}},
		"明确拒收": {candidates: []string{"parcel-1"}, mutate: func(command *application.ReceiveDeliveredUnitCommand) {
			command.Claim, command.Evidence, command.Refusal = application.ExplicitRefusal, domain.ReceptionEvidenceReference{}, "破损拒收"
		}},
		"仅扫描": {candidates: []string{"parcel-1"}, mutate: func(command *application.ReceiveDeliveredUnitCommand) {
			command.Claim, command.Evidence = application.ScanOnlyObservation, domain.ReceptionEvidenceReference{}
		}},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReceptionFixture(t)
			fixture.identity.candidates = nil
			for _, raw := range testCase.candidates {
				fixture.identity.candidates = append(fixture.identity.candidates, mustValue(t, domain.NewParcelAssociationReference, raw))
			}
			fixture.serviceResults.results = ports.ParcelServiceResults{Cancellation: "cancel-9"}
			command := receiveCommand(t)
			if testCase.mutate != nil {
				testCase.mutate(&command)
			}
			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if len(fixture.serviceResults.asked) != 0 {
				t.Fatalf("服务结果读口被问了 %v，这一支没有可问的包裹", fixture.serviceResults.asked)
			}
			record, recorded := result.Record()
			if !recorded {
				t.Fatal("这一支该落册")
			}
			if len(record.ServiceMarkers) != 0 || record.ServiceResultCheck != ports.ServiceResultCheckAbsent {
				t.Fatalf("这一支不该带标记与核对态：%v / %q", record.ServiceMarkers, record.ServiceResultCheck)
			}
		})
	}
}

// Covers: `AT-NO-018`「实物无法可靠识别——建立待识别实物，保存控制、位置、状况和候选，
// 不创建正式包裹」与 `AT-NO-019` 半边（只钉「形成身份冲突、不猜身份」的记法，场景是一件实物的号码在册里标识多件
// 包裹；两件实物贴同一个号的字面场景见 TestOneMarkOnTwoUnitsFormsAnIdentityConflict）——两格都真实成立收寄与控制，
// 都不交 PS 意图（没有可采用的关联）。
func TestUnknownAndConflictedIdentitiesStayPending(t *testing.T) {
	t.Run("unknown identity", func(t *testing.T) {
		fixture := newReceptionFixture(t)
		fixture.identity.candidates = nil

		result, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.UnitPendingIdentification {
			t.Fatalf("outcome = %q", result.Outcome())
		}
		record, _ := result.Record()
		if _, identified := record.Intake.Association(); identified {
			t.Fatal("未知身份凭空长出了关联")
		}
		if !record.Control.Active() {
			t.Fatal("待识别实物的控制没有成立")
		}
		if record.IdentityConflict {
			t.Fatal("未知不是冲突")
		}
		if len(fixture.downstream.intents) != 0 {
			t.Fatal("没有关联的收寄交了意图")
		}
	})

	t.Run("conflicting candidates", func(t *testing.T) {
		fixture := newReceptionFixture(t)
		fixture.identity.candidates = []domain.ParcelAssociationReference{
			mustValue(t, domain.NewParcelAssociationReference, "parcel-1"),
			mustValue(t, domain.NewParcelAssociationReference, "parcel-2"),
		}

		result, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.UnitPendingIdentification {
			t.Fatalf("outcome = %q", result.Outcome())
		}
		record, _ := result.Record()
		if !record.IdentityConflict || len(record.Candidates) != 2 {
			t.Fatalf("conflict = %v candidates = %d; 冲突必须带全部候选", record.IdentityConflict, len(record.Candidates))
		}
	})
}

// Covers: `AT-NO-052` 与 ADR-0195——身份核对答恰一个候选、同时答出歧义信号（号码另被一份未接受的委托报过），这不是唯一
// 命中：答待识别、记身份冲突、候选只记那一件，不问 PS 服务结果、不交意图。它也不走一码两物那一支，因为 ADR-0196 只管
// 唯一命中：那件包裹已关联到先到的另一件实物时，先到那件不追加冲突版。零候选带信号照旧是待识别、不记冲突——记了冲突，
// 那份委托接受之后的重核会把它跳过。多候选带信号与不带信号同答。
func TestAUniqueHitThatAnUnacceptedRequestAlsoDeclaresIsAnIdentityConflict(t *testing.T) {
	t.Run("唯一候选带信号", func(t *testing.T) {
		fixture := newReceptionFixture(t)
		parcel := fixture.identity.candidates[0]
		first, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
		if err != nil || first.Outcome() != application.NodeIntakeFormed {
			t.Fatalf("先到那件：outcome = %q err = %v, want INTAKE_FORMED——那时还没有歧义信号", first.Outcome(), err)
		}

		fixture.identity.ambiguous = true
		command := receiveCommand(t)
		command.SourceID = "delivery-2"
		command.Unit = mustValue(t, domain.NewHandlingUnitID, "unit-2")
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.UnitPendingIdentification {
			t.Fatalf("outcome = %q, want PENDING_IDENTIFICATION——带歧义信号的唯一命中不算唯一", result.Outcome())
		}
		record, _ := result.Record()
		if _, identified := record.Intake.Association(); identified {
			t.Fatal("带歧义信号的唯一命中被关联到了正式包裹")
		}
		if !record.IdentityConflict || len(record.Candidates) != 1 || record.Candidates[0] != parcel {
			t.Fatalf("conflict = %v candidates = %v, want 身份冲突、候选只记那一件", record.IdentityConflict, record.Candidates)
		}
		if !record.Control.Active() || record.ServiceResultCheck != ports.ServiceResultCheckAbsent {
			t.Fatalf("control = %v check = %q, want 控制成立、不带核对态", record.Control.Active(), record.ServiceResultCheck)
		}
		if len(fixture.serviceResults.asked) != 1 || len(fixture.downstream.intents) != 1 {
			t.Fatalf("问 PS %d 次、意图 %d 份, want 都只为先到那件", len(fixture.serviceResults.asked), len(fixture.downstream.intents))
		}
		if rows := fixture.receptions.recordsOfUnit("unit-1"); len(rows) != 1 || rows[0].IdentityConflict {
			t.Fatalf("先到那件在册 %+v, want 只有原判断一行——带信号的唯一命中不走一码两物那一支", rows)
		}
	})

	t.Run("零候选带信号", func(t *testing.T) {
		fixture := newReceptionFixture(t)
		fixture.identity.candidates = nil
		fixture.identity.ambiguous = true
		result, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		record, _ := result.Record()
		if result.Outcome() != application.UnitPendingIdentification || record.IdentityConflict || len(record.Candidates) != 0 {
			t.Fatalf("outcome = %q conflict = %v candidates = %v, want 待识别、不记冲突、零候选——声明不进候选",
				result.Outcome(), record.IdentityConflict, record.Candidates)
		}
	})

	t.Run("多候选带信号", func(t *testing.T) {
		fixture := newReceptionFixture(t)
		fixture.identity.candidates = append(fixture.identity.candidates,
			mustValue(t, domain.NewParcelAssociationReference, "parcel-2"))
		fixture.identity.ambiguous = true
		result, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		record, _ := result.Record()
		if result.Outcome() != application.UnitPendingIdentification || !record.IdentityConflict || len(record.Candidates) != 2 {
			t.Fatalf("outcome = %q conflict = %v candidates = %v, want 待识别、身份冲突、两个候选",
				result.Outcome(), record.IdentityConflict, record.Candidates)
		}
	})
}

// Covers: `AT-NO-023`「只有到站扫描——判断待确认，不建立控制」与 `AT-NO-024`「节点明确
// 拒收且未取得实物控制——形成未收寄结果和原因，不形成委托拒绝或取消」——两格都不建
// 控制、不交意图；拒收原因随记录保全。
func TestScanOnlyAndRefusalDoNotEstablishControl(t *testing.T) {
	t.Run("scan only", func(t *testing.T) {
		fixture := newReceptionFixture(t)
		command := receiveCommand(t)
		command.Claim = application.ScanOnlyObservation
		command.Evidence = domain.ReceptionEvidenceReference{}

		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ReceptionUndecided {
			t.Fatalf("outcome = %q", result.Outcome())
		}
		record, _ := result.Record()
		if record.Control.Active() {
			t.Fatal("扫描建立了控制")
		}
		if fixture.receptions.saved != 1 {
			t.Fatal("扫描来源没有随记录保全")
		}
	})

	t.Run("explicit refusal", func(t *testing.T) {
		fixture := newReceptionFixture(t)
		command := receiveCommand(t)
		command.Claim = application.ExplicitRefusal
		command.Refusal = "PACKAGING_UNSAFE"
		command.Evidence = domain.ReceptionEvidenceReference{}

		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.NodeIntakeNotFormed {
			t.Fatalf("outcome = %q", result.Outcome())
		}
		record, _ := result.Record()
		if record.RefusalReason != "PACKAGING_UNSAFE" {
			t.Fatal("拒收原因没有随记录保全")
		}
		if record.Control.Active() {
			t.Fatal("拒收建立了控制")
		}
		if len(fixture.downstream.intents) != 0 {
			t.Fatal("未收寄交了意图")
		}
	})
}

// Covers: `AT-NO-016`「同一交付来源和内容重复到达——返回原结果，不重复建立作业实物、
// 控制起点或事件」、`AT-NO-017`「同一来源身份携带不同包裹范围——形成来源冲突，保留原
// 结果」与 `AT-NO-027`「收寄结果提交成功但事件投递失败——保留业务结果，只重试同一发布
// 意图」。
func TestReplayConflictAndFailedIntentStayDisciplined(t *testing.T) {
	fixture := newReceptionFixture(t)
	unreachable := errors.New("downstream unreachable")
	fixture.downstream.err = unreachable

	first, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.NodeIntakeFormed {
		t.Fatalf("outcome = %q; 投递失败不得翻收寄结果", first.Outcome())
	}
	if first.IntakeHandoffReference() == "" {
		t.Fatal("首投失败没有留发布续办引用")
	}
	// 入队的错随结果交出：不读续办的调用方据它整笔回滚（票 operator-channel/45）。
	if !errors.Is(first.IntakeHandoffError(), unreachable) {
		t.Fatalf("handoff error = %v; 续办引用在场时入队的错也该在场", first.IntakeHandoffError())
	}

	fixture.downstream.err = nil
	replay, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.ReceptionExistingResult {
		t.Fatalf("replay = %q", replay.Outcome())
	}
	if replay.IntakeHandoffReference() != "" || replay.IntakeHandoffError() != nil {
		t.Fatalf("重发成功仍留着续办：%q / %v", replay.IntakeHandoffReference(), replay.IntakeHandoffError())
	}
	if fixture.receptions.saved != 1 {
		t.Fatal("重放重复建立了记录")
	}
	if len(fixture.downstream.intents) != 1 {
		t.Fatalf("intents = %d; 重发的必须是同一份", len(fixture.downstream.intents))
	}

	conflicting := receiveCommand(t)
	conflicting.Unit = mustValue(t, domain.NewHandlingUnitID, "unit-9")
	conflict, err := fixture.handler.Handle(context.Background(), conflicting)
	if err != nil {
		t.Fatalf("conflict handle: %v", err)
	}
	if conflict.Outcome() != application.ReceptionSourceConflict {
		t.Fatalf("outcome = %q, want SOURCE_CONFLICT", conflict.Outcome())
	}
	if fixture.receptions.saved != 1 {
		t.Fatal("冲突覆盖了原结果")
	}
}

// Covers: `AT-NO-025`「批量交付部分收寄、部分待识别、部分未收寄——逐件结果并存，成功
// 成员不因其他成员失败回滚」——批量由调用方逐件分发，三件三种走向各自成立，先成的
// 记录纹丝不动。
func TestABatchKeepsPerUnitResultsIndependent(t *testing.T) {
	fixture := newReceptionFixture(t)

	formed, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
	if err != nil {
		t.Fatalf("formed handle: %v", err)
	}
	if formed.Outcome() != application.NodeIntakeFormed {
		t.Fatalf("formed = %q", formed.Outcome())
	}

	fixture.identity.candidates = nil
	pending := receiveCommand(t)
	pending.SourceID = "delivery-2"
	pending.Unit = mustValue(t, domain.NewHandlingUnitID, "unit-2")
	pendingResult, err := fixture.handler.Handle(context.Background(), pending)
	if err != nil {
		t.Fatalf("pending handle: %v", err)
	}
	if pendingResult.Outcome() != application.UnitPendingIdentification {
		t.Fatalf("pending = %q", pendingResult.Outcome())
	}

	refused := receiveCommand(t)
	refused.SourceID = "delivery-3"
	refused.Unit = mustValue(t, domain.NewHandlingUnitID, "unit-3")
	refused.Claim = application.ExplicitRefusal
	refused.Refusal = "LEAKING"
	refused.Evidence = domain.ReceptionEvidenceReference{}
	refusedResult, err := fixture.handler.Handle(context.Background(), refused)
	if err != nil {
		t.Fatalf("refused handle: %v", err)
	}
	if refusedResult.Outcome() != application.NodeIntakeNotFormed {
		t.Fatalf("refused = %q", refusedResult.Outcome())
	}

	if fixture.receptions.saved != 3 {
		t.Fatalf("saved = %d; 三件该有三份互不相扰的记录", fixture.receptions.saved)
	}
	survivor, found, err := fixture.receptions.FindByKey(context.Background(),
		ports.ReceptionKey{TenantID: mustValue(t, domain.NewTenantID, "tenant-1"), SourceID: "delivery-1"})
	if err != nil || !found || survivor.Kind != ports.RecordIntakeFormed {
		t.Fatalf("survivor = %#v; 先成的收寄被回滚了", survivor)
	}
}

func oneMarkCommand(t *testing.T, sourceID, unit string) application.ReceiveDeliveredUnitCommand {
	t.Helper()
	return application.ReceiveDeliveredUnitCommand{
		TenantID:    mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		SourceID:    sourceID,
		Node:        mustValue(t, domain.NewNodeReference, "SYN-NODE-NRI01"),
		DeliveredBy: mustValue(t, domain.NewDeliveringPartyReference, "SYN-CUSTOMER-NRI01"),
		Unit:        mustValue(t, domain.NewHandlingUnitID, unit),
		Mark:        ports.ExternalMarkObservation{Mark: "SYN-BARCODE-NRI01"},
		Claim:       application.ExplicitReception,
		Evidence:    mustValue(t, domain.NewReceptionEvidenceReference, "SYN-SIGN-"+unit),
		OccurredAt:  deliveredAt,
	}
}

func oneMarkFixture(t *testing.T) (*receptionFixture, domain.ParcelAssociationReference) {
	t.Helper()
	fixture := newReceptionFixture(t)
	parcel := mustValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI01")
	fixture.identity.candidates = []domain.ParcelAssociationReference{parcel}
	return fixture, parcel
}

func identityReasonsOf(t *testing.T, records []ports.ReceptionRecord) []domain.DirectionalBasisReason {
	t.Helper()
	observations := make([]domain.IntakeIdentityObservation, 0, len(records))
	for _, record := range records {
		observation := domain.IntakeIdentityObservation{
			SourceID:         record.Key.SourceID,
			Version:          record.Intake.Version(),
			IdentityConflict: record.IdentityConflict,
			ReceivedAt:       record.Intake.ReceivedAt(),
		}
		if association, present := record.Intake.Association(); present {
			observation.Association = association
		}
		observations = append(observations, observation)
	}
	basis, err := domain.JudgeIdentityBasis(observations)
	if err != nil {
		t.Fatalf("判身份：%v", err)
	}
	return basis.Cell().Reasons()
}

// Covers: `AT-NO-019`「同一外部条码出现在两件实物——两件实物保持独立，形成身份冲突并暂停方向性作业」的字面场景
// （票 no-reception-identity/01、ADR-0196）：号码在册里只标识一件包裹，两件作业实物先后贴着它送站。后到那件答待识别、
// 记身份冲突、候选是那件包裹，不向 PS 问服务结果、不交意图；先到那件的原判断照留，另追加一版身份冲突判断；两件的控制
// 各自成立；两件在方向依据的身份一格里都落成身份冲突。
func TestOneMarkOnTwoUnitsFormsAnIdentityConflict(t *testing.T) {
	fixture, parcel := oneMarkFixture(t)
	first := oneMarkCommand(t, "SYN-DELIVERY-NRI01-A", "SYN-UNIT-NRI01-A")
	formed, err := fixture.handler.Handle(context.Background(), first)
	if err != nil || formed.Outcome() != application.NodeIntakeFormed {
		t.Fatalf("先到那件：outcome = %q err = %v, want INTAKE_FORMED", formed.Outcome(), err)
	}
	original, _ := formed.Record()

	second := oneMarkCommand(t, "SYN-DELIVERY-NRI01-B", "SYN-UNIT-NRI01-B")
	result, err := fixture.handler.Handle(context.Background(), second)
	if err != nil {
		t.Fatalf("后到那件：%v", err)
	}
	if result.Outcome() != application.UnitPendingIdentification {
		t.Fatalf("后到那件 outcome = %q, want PENDING_IDENTIFICATION——同一个号已关联到另一件实物", result.Outcome())
	}
	late, _ := result.Record()
	if _, identified := late.Intake.Association(); identified {
		t.Fatal("后到那件被关联到了正式包裹")
	}
	if !late.IdentityConflict || len(late.Candidates) != 1 || late.Candidates[0] != parcel {
		t.Fatalf("后到那件 conflict = %v candidates = %v, want 身份冲突且候选是那件包裹", late.IdentityConflict, late.Candidates)
	}
	if !late.Control.Active() || late.ServiceResultCheck != ports.ServiceResultCheckAbsent {
		t.Fatalf("后到那件 control = %v check = %q, want 控制成立、不带核对态", late.Control.Active(), late.ServiceResultCheck)
	}
	if len(fixture.serviceResults.asked) != 1 || len(fixture.downstream.intents) != 1 {
		t.Fatalf("问 PS %d 次、意图 %d 份, want 都只为先到那件", len(fixture.serviceResults.asked), len(fixture.downstream.intents))
	}

	firstRows := fixture.receptions.recordsOfUnit("SYN-UNIT-NRI01-A")
	if len(firstRows) != 2 {
		t.Fatalf("先到那件在册 %d 行, want 原判断加追加的一版", len(firstRows))
	}
	kept := fixture.receptions.byKey[ports.ReceptionKey{TenantID: first.TenantID, SourceID: first.SourceID}]
	if kept.Kind != ports.RecordIntakeFormed || kept.IdentityConflict || kept.Intake != original.Intake {
		t.Fatalf("先到那件的原判断被改了：%+v", kept)
	}
	var appended ports.ReceptionRecord
	for _, row := range firstRows {
		if row.Key != kept.Key {
			appended = row
		}
	}
	if !appended.IdentityConflict || appended.Kind != ports.RecordPendingIdentification {
		t.Fatalf("追加的一版 = %+v, want 待识别且记身份冲突", appended)
	}
	if _, identified := appended.Intake.Association(); identified || appended.Intake.Version() == original.Intake.Version() {
		t.Fatalf("追加的一版带了关联或沿用了原版本：%+v", appended.Intake)
	}
	if len(appended.Candidates) != 1 || appended.Candidates[0] != parcel || appended.Control != original.Control {
		t.Fatalf("追加的一版 candidates = %v control = %+v, want 候选是那件包裹、控制照原判断", appended.Candidates, appended.Control)
	}

	for _, unit := range []string{"SYN-UNIT-NRI01-A", "SYN-UNIT-NRI01-B"} {
		reasons := identityReasonsOf(t, fixture.receptions.recordsOfUnit(unit))
		if !slices.Contains(reasons, domain.ReasonIdentityConflict) {
			t.Fatalf("%s 的身份一格 = %v, want 含身份冲突", unit, reasons)
		}
	}
}

// Covers: 票 no-reception-identity/01 裁定「同一作业实物的重投仍是重放」——同一份送站重投走已有结果，同一件实物换个来源
// 再收一次照常形成收寄，都不认成一码多物；冲突形成之后重放后到那件，也不再追加。
func TestTheSameUnitDeliveredAgainIsNotAnIdentityConflict(t *testing.T) {
	fixture, _ := oneMarkFixture(t)
	first := oneMarkCommand(t, "SYN-DELIVERY-NRI01-A", "SYN-UNIT-NRI01-A")
	if _, err := fixture.handler.Handle(context.Background(), first); err != nil {
		t.Fatalf("首投：%v", err)
	}
	replay, err := fixture.handler.Handle(context.Background(), first)
	if err != nil || replay.Outcome() != application.ReceptionExistingResult {
		t.Fatalf("重投 outcome = %q err = %v, want EXISTING_RESULT", replay.Outcome(), err)
	}

	again := oneMarkCommand(t, "SYN-DELIVERY-NRI01-A2", "SYN-UNIT-NRI01-A")
	again.OccurredAt = deliveredAt.Add(time.Hour)
	result, err := fixture.handler.Handle(context.Background(), again)
	if err != nil || result.Outcome() != application.NodeIntakeFormed {
		t.Fatalf("同一件实物再收 outcome = %q err = %v, want INTAKE_FORMED", result.Outcome(), err)
	}
	if record, _ := result.Record(); record.IdentityConflict {
		t.Fatal("同一件实物再收被记成了身份冲突")
	}

	second := oneMarkCommand(t, "SYN-DELIVERY-NRI01-B", "SYN-UNIT-NRI01-B")
	if _, err := fixture.handler.Handle(context.Background(), second); err != nil {
		t.Fatalf("后到那件：%v", err)
	}
	saved := fixture.receptions.saved
	replayed, err := fixture.handler.Handle(context.Background(), second)
	if err != nil || replayed.Outcome() != application.ReceptionExistingResult {
		t.Fatalf("重放后到那件 outcome = %q err = %v, want EXISTING_RESULT", replayed.Outcome(), err)
	}
	if fixture.receptions.saved != saved {
		t.Fatalf("重放又落了 %d 行", fixture.receptions.saved-saved)
	}
}

// sourceOf 是一条命令应留下的来源文档：逐格取自命令，外部标识观察不改写。
func sourceOf(command application.ReceiveDeliveredUnitCommand, claim string) domain.DeliverySource {
	return domain.DeliverySource{
		Unit:        command.Unit,
		Mark:        command.Mark.Mark,
		Claim:       claim,
		Evidence:    command.Evidence,
		Refusal:     command.Refusal,
		DeliveredBy: command.DeliveredBy,
		Node:        command.Node,
		OccurredAt:  command.OccurredAt.UTC(),
	}
}

// Covers: `AT-NO-047`「明确接收、明确拒收、只有扫描各来一次交付——每一次的来源都随判断保全，外部标识观察原样；此后的重投
// 与追加版本都不改写它」的编排面（ADR-0199 决定一、决定二）：三支各落一行，记录带来源文档，外部标识观察逐字等于入参
// （两端带空白的号原样），某格在这种观察里本就缺席时是空串；三支都签判断版本，形成格的就是收寄结果版本。同一来源只在
// 证据、交付方或拒收原因上不同的重投答已有结果，首存的来源文档不被覆盖。
func TestEveryObservationKeepsItsDeliverySource(t *testing.T) {
	const mark = "  SYN-BARCODE-NRS02 \t"
	customer2 := mustValue(t, domain.NewDeliveringPartyReference, "SYN-CUSTOMER-NRS02-2")
	cases := map[string]struct {
		arrange func(*application.ReceiveDeliveredUnitCommand)
		resend  func(*application.ReceiveDeliveredUnitCommand)
		claim   string
		kind    ports.ReceptionRecordKind
	}{
		"明确接收": {
			resend: func(command *application.ReceiveDeliveredUnitCommand) {
				command.Evidence = mustValue(t, domain.NewReceptionEvidenceReference, "SYN-SIGN-NRS02-OTHER")
				command.DeliveredBy = customer2
			},
			claim: "RECEIVED",
			kind:  ports.RecordIntakeFormed,
		},
		"明确拒收": {
			arrange: func(command *application.ReceiveDeliveredUnitCommand) {
				command.Claim, command.Evidence, command.Refusal = application.ExplicitRefusal, domain.ReceptionEvidenceReference{}, "SYN-REFUSED-DAMAGED"
			},
			resend: func(command *application.ReceiveDeliveredUnitCommand) {
				command.Refusal = "SYN-REFUSED-OTHER"
				command.DeliveredBy = customer2
			},
			claim: "REFUSED",
			kind:  ports.RecordIntakeNotFormed,
		},
		"仅扫描": {
			arrange: func(command *application.ReceiveDeliveredUnitCommand) {
				command.Claim, command.Evidence = application.ScanOnlyObservation, domain.ReceptionEvidenceReference{}
			},
			resend: func(command *application.ReceiveDeliveredUnitCommand) { command.DeliveredBy = customer2 },
			claim:  "SCAN_ONLY",
			kind:   ports.RecordReceptionUndecided,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReceptionFixture(t)
			command := receiveCommand(t)
			command.Mark = ports.ExternalMarkObservation{Mark: mark}
			if testCase.arrange != nil {
				testCase.arrange(&command)
			}
			want := sourceOf(command, testCase.claim)

			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("首投：%v", err)
			}
			record, _ := result.Record()
			if record.Kind != testCase.kind {
				t.Fatalf("种类 = %s, want %s", record.Kind, testCase.kind)
			}
			if record.Source != want || record.Source.Mark != mark {
				t.Fatalf("来源文档 = %+v, want %+v", record.Source, want)
			}
			if record.JudgmentVersion.String() == "" {
				t.Fatal("这一支的判断没有签判断版本")
			}
			if testCase.kind == ports.RecordIntakeFormed && record.JudgmentVersion != record.Intake.Version() {
				t.Fatalf("形成格的判断版本 %q 不是收寄结果版本 %q", record.JudgmentVersion, record.Intake.Version())
			}
			if record.Predecessor.String() != "" {
				t.Fatalf("首投指名了前版 %q", record.Predecessor)
			}

			resent := command
			testCase.resend(&resent)
			replay, err := fixture.handler.Handle(context.Background(), resent)
			if err != nil || replay.Outcome() != application.ReceptionExistingResult {
				t.Fatalf("只在证据、交付方或原因上不同的重投 outcome = %q err = %v, want EXISTING_RESULT", replay.Outcome(), err)
			}
			key := ports.ReceptionKey{TenantID: command.TenantID, SourceID: command.SourceID}
			if kept := fixture.receptions.byKey[key]; kept.Source != want || kept.JudgmentVersion != record.JudgmentVersion {
				t.Fatalf("重投之后在册的来源 %+v 版本 %q, want 首存的 %+v %q", kept.Source, kept.JudgmentVersion, want, record.JudgmentVersion)
			}
			if fixture.receptions.saved != 1 {
				t.Fatalf("落册 %d 行, want 1", fixture.receptions.saved)
			}
		})
	}
}

// conflictVersionsOf 交回这件作业实物在册上指名了前版的各行，即追加的冲突版。
func conflictVersionsOf(fixture *receptionFixture, unit string) map[domain.IntakeResultVersion]ports.ReceptionRecord {
	byPredecessor := map[domain.IntakeResultVersion]ports.ReceptionRecord{}
	for _, row := range fixture.receptions.recordsOfUnit(unit) {
		if row.Predecessor.String() != "" {
			byPredecessor[row.Predecessor] = row
		}
	}
	return byPredecessor
}

// Covers: `AT-NO-048`「原判断已被后续版本接续之后，同一来源重投——返回已有结果，带原判断种类与当前有效版本；不重复控制
// 起点或事件」（ADR-0199 决定二）：ADR-0196 一码两物之后，先到那件的原判断由追加的冲突版接续——冲突版指名原判断为
// 前版、带后到那次送站的来源文档。重投先到那件的来源答已有结果：原判断种类是 INTAKE_FORMED，当前有效版本是冲突版；
// 不另落一行、不另签版本，意图只重发原来那一份。第三件再来，追加的冲突版指名上一版冲突版，当前有效版本随之走到它。
func TestAReplayAfterAnIdentityConflictAnswersTheCurrentVersion(t *testing.T) {
	fixture, _ := oneMarkFixture(t)
	first := oneMarkCommand(t, "SYN-DELIVERY-NRS02-A", "SYN-UNIT-NRS02-A")
	formed, err := fixture.handler.Handle(context.Background(), first)
	if err != nil || formed.Outcome() != application.NodeIntakeFormed {
		t.Fatalf("先到那件 outcome = %q err = %v, want INTAKE_FORMED", formed.Outcome(), err)
	}
	if formed.RecordedOutcome() != application.ReceptionOutcomeInvalid || formed.CurrentVersion().String() != "" {
		t.Fatalf("首答带了原判断种类 %q 或当前有效版本 %q——两格只随已有结果", formed.RecordedOutcome(), formed.CurrentVersion())
	}
	original, _ := formed.Record()

	second := oneMarkCommand(t, "SYN-DELIVERY-NRS02-B", "SYN-UNIT-NRS02-B")
	conflicted, err := fixture.handler.Handle(context.Background(), second)
	if err != nil || conflicted.Outcome() != application.UnitPendingIdentification {
		t.Fatalf("后到那件 outcome = %q err = %v, want PENDING_IDENTIFICATION", conflicted.Outcome(), err)
	}
	late, _ := conflicted.Record()
	appended, found := conflictVersionsOf(fixture, "SYN-UNIT-NRS02-A")[original.JudgmentVersion]
	if !found {
		t.Fatalf("先到那件的冲突版没有指名原判断 %q 为前版：%+v", original.JudgmentVersion, conflictVersionsOf(fixture, "SYN-UNIT-NRS02-A"))
	}
	if appended.JudgmentVersion.String() == "" || appended.JudgmentVersion != appended.Intake.Version() {
		t.Fatalf("冲突版的判断版本 %q 不是它的收寄结果版本 %q", appended.JudgmentVersion, appended.Intake.Version())
	}
	if appended.Source != late.Source || appended.Source.Unit.String() != "SYN-UNIT-NRS02-B" {
		t.Fatalf("冲突版的来源文档 = %+v, want 后到那次送站的 %+v", appended.Source, late.Source)
	}

	saved, signed, intents := fixture.receptions.saved, fixture.versions.next, len(fixture.downstream.intents)
	replay, err := fixture.handler.Handle(context.Background(), first)
	if err != nil || replay.Outcome() != application.ReceptionExistingResult {
		t.Fatalf("重投先到那件 outcome = %q err = %v, want EXISTING_RESULT", replay.Outcome(), err)
	}
	if replay.RecordedOutcome() != application.NodeIntakeFormed {
		t.Fatalf("原判断种类 = %q, want INTAKE_FORMED", replay.RecordedOutcome())
	}
	if replay.CurrentVersion() != appended.JudgmentVersion {
		t.Fatalf("当前有效版本 = %q, want 冲突版 %q", replay.CurrentVersion(), appended.JudgmentVersion)
	}
	if fixture.receptions.saved != saved || fixture.versions.next != signed {
		t.Fatalf("重投又落了 %d 行、签了 %d 个版本", fixture.receptions.saved-saved, fixture.versions.next-signed)
	}
	if got := fixture.downstream.intents[len(fixture.downstream.intents)-1]; len(fixture.downstream.intents) != intents+1 || got.Record.Key != original.Key {
		t.Fatalf("意图 %d → %d 份，末一份是 %q, want 只重发原来那一份", intents, len(fixture.downstream.intents), got.Record.Key.SourceID)
	}

	third := oneMarkCommand(t, "SYN-DELIVERY-NRS02-C", "SYN-UNIT-NRS02-C")
	if _, err := fixture.handler.Handle(context.Background(), third); err != nil {
		t.Fatalf("第三件：%v", err)
	}
	next, found := conflictVersionsOf(fixture, "SYN-UNIT-NRS02-A")[appended.JudgmentVersion]
	if !found {
		t.Fatalf("第三件引起的冲突版没有指名上一版冲突版 %q：%+v", appended.JudgmentVersion, conflictVersionsOf(fixture, "SYN-UNIT-NRS02-A"))
	}
	again, err := fixture.handler.Handle(context.Background(), first)
	if err != nil || again.CurrentVersion() != next.JudgmentVersion || again.RecordedOutcome() != application.NodeIntakeFormed {
		t.Fatalf("再重投 当前有效版本 = %q 原判断种类 = %q err = %v, want %q INTAKE_FORMED",
			again.CurrentVersion(), again.RecordedOutcome(), err, next.JudgmentVersion)
	}
}

// Covers: `AT-NO-048` 的另两支（ADR-0199 决定二）——仅扫描与明确拒收重投答已有结果，原判断种类各是 RECEPTION_UNDECIDED 与
// INTAKE_NOT_FORMED，当前有效版本是它们自己签下的判断版本（没有后继）。
func TestAReplayOfAnUndecidedOrRefusedJudgmentAnswersItsOwnVersion(t *testing.T) {
	cases := map[string]struct {
		arrange func(*application.ReceiveDeliveredUnitCommand)
		want    application.ReceptionOutcome
	}{
		"仅扫描": {
			arrange: func(command *application.ReceiveDeliveredUnitCommand) {
				command.Claim, command.Evidence = application.ScanOnlyObservation, domain.ReceptionEvidenceReference{}
			},
			want: application.ReceptionUndecided,
		},
		"明确拒收": {
			arrange: func(command *application.ReceiveDeliveredUnitCommand) {
				command.Claim, command.Evidence, command.Refusal = application.ExplicitRefusal, domain.ReceptionEvidenceReference{}, "SYN-REFUSED-DAMAGED"
			},
			want: application.NodeIntakeNotFormed,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReceptionFixture(t)
			command := receiveCommand(t)
			testCase.arrange(&command)
			first, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("首投：%v", err)
			}
			record, _ := first.Record()
			replay, err := fixture.handler.Handle(context.Background(), command)
			if err != nil || replay.Outcome() != application.ReceptionExistingResult {
				t.Fatalf("重投 outcome = %q err = %v, want EXISTING_RESULT", replay.Outcome(), err)
			}
			if replay.RecordedOutcome() != testCase.want {
				t.Fatalf("原判断种类 = %q, want %q", replay.RecordedOutcome(), testCase.want)
			}
			if replay.CurrentVersion().String() == "" || replay.CurrentVersion() != record.JudgmentVersion {
				t.Fatalf("当前有效版本 = %q, want 它自己的判断版本 %q", replay.CurrentVersion(), record.JudgmentVersion)
			}
		})
	}
}

// Covers: ADR-0186「没形成答案一律交回 error」的两处新读写——拒收与仅扫描签不出判断版本时不落册；重投时读不出判断链的当前
// 有效版本，不答一格缺了当前有效版本的已有结果。两处都零落册、零意图。
func TestJudgmentVersionFailuresFormNoAnswer(t *testing.T) {
	failure := errors.New("dependency down")
	for name, claim := range map[string]application.ReceptionClaimKind{"明确拒收": application.ExplicitRefusal, "仅扫描": application.ScanOnlyObservation} {
		t.Run(name+"签不出判断版本", func(t *testing.T) {
			fixture := newReceptionFixture(t)
			fixture.versions.err = failure
			command := receiveCommand(t)
			command.Claim, command.Evidence, command.Refusal = claim, domain.ReceptionEvidenceReference{}, "SYN-REFUSED-DAMAGED"
			result, err := fixture.handler.Handle(context.Background(), command)
			if err == nil || errors.Is(err, application.ErrReceptionCommandRejected) {
				t.Fatalf("outcome = %q err = %v, want 依赖故障类 error", result.Outcome(), err)
			}
			if fixture.receptions.saved != 0 {
				t.Fatalf("落册 %d 行, want 0", fixture.receptions.saved)
			}
		})
	}

	t.Run("重投读不出当前有效版本", func(t *testing.T) {
		fixture := newReceptionFixture(t)
		if _, err := fixture.handler.Handle(context.Background(), receiveCommand(t)); err != nil {
			t.Fatalf("首投：%v", err)
		}
		fixture.receptions.chainErr = failure
		intents := len(fixture.downstream.intents)
		result, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
		if err == nil {
			t.Fatalf("outcome = %q, want error", result.Outcome())
		}
		if fixture.receptions.saved != 1 || len(fixture.downstream.intents) != intents {
			t.Fatalf("落册 %d、意图 %d → %d, want 仍是首投那一份", fixture.receptions.saved, intents, len(fixture.downstream.intents))
		}
	})
}
