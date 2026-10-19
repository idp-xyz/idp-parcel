package application_test

import (
	"context"
	"errors"
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
	err        error
}

func (double *identityViewDouble) ResolveParcelIdentity(
	_ context.Context,
	_ domain.TenantID,
	_ ports.ExternalMarkObservation,
) ([]domain.ParcelAssociationReference, error) {
	return double.candidates, double.err
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
	double.byKey[record.Key] = record
	double.saved++
	return ports.ReceptionSaved, nil
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

// Covers: ADR-0186「装配缺依赖，改为在构造期拒绝」——六个依赖缺任一个，构造就交回错误，不留到第一次收寄才暴露成一次
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
		"并发读回读不到赢家": func(fixture *receptionFixture) { fixture.receptions.lostRace = true },
		"版本签发报错":    func(fixture *receptionFixture) { fixture.versions.err = failure },
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
// 不创建正式包裹」与 `AT-NO-019`「同一外部条码出现在两件实物——保持独立，形成身份冲突
// 并暂停方向性作业」——两格都真实成立收寄与控制，都不交 PS 意图（没有可采用的关联）。
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
	fixture.downstream.err = errors.New("downstream unreachable")

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

	fixture.downstream.err = nil
	replay, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.ReceptionExistingResult {
		t.Fatalf("replay = %q", replay.Outcome())
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
