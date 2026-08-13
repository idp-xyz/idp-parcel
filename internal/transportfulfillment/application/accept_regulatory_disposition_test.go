package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var dispositionDecidedAt = time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)

type dispositionAcceptanceStoreDouble struct {
	records map[ports.DispositionAcceptanceKey]ports.DispositionAcceptanceRecord
	findErr error
	saveErr error
	forced  ports.DispositionAcceptanceSaveOutcome
	saves   int
}

func newDispositionAcceptanceStore() *dispositionAcceptanceStoreDouble {
	return &dispositionAcceptanceStoreDouble{
		records: map[ports.DispositionAcceptanceKey]ports.DispositionAcceptanceRecord{},
	}
}

func (double *dispositionAcceptanceStoreDouble) FindByKey(
	_ context.Context,
	key ports.DispositionAcceptanceKey,
) (ports.DispositionAcceptanceRecord, bool, error) {
	if double.findErr != nil {
		return ports.DispositionAcceptanceRecord{}, false, double.findErr
	}
	record, found := double.records[key]
	return record, found, nil
}

func (double *dispositionAcceptanceStoreDouble) Save(
	_ context.Context,
	record ports.DispositionAcceptanceRecord,
) (ports.DispositionAcceptanceSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.DispositionAcceptanceSaveOutcomeInvalid, double.saveErr
	}
	if double.forced != ports.DispositionAcceptanceSaveOutcomeInvalid {
		return double.forced, nil
	}
	if _, exists := double.records[record.Key]; exists {
		return ports.DispositionAcceptanceAlreadyRecorded, nil
	}
	double.records[record.Key] = record
	double.saves++
	return ports.DispositionAcceptanceSaved, nil
}

type receiptHandoffDouble struct {
	intents []ports.RegulatoryAcceptanceHandoffIntent
	err     error
}

func (double *receiptHandoffDouble) HandOffRegulatoryAcceptance(
	_ context.Context,
	intent ports.RegulatoryAcceptanceHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type dispositionFixture2 struct {
	handler     *application.AcceptRegulatoryDispositionHandler
	acceptances *dispositionAcceptanceStoreDouble
	receipt     *receiptHandoffDouble
}

func newAcceptDispositionFixture(t *testing.T) *dispositionFixture2 {
	t.Helper()
	fixture := &dispositionFixture2{
		acceptances: newDispositionAcceptanceStore(),
		receipt:     &receiptHandoffDouble{},
	}
	fixture.handler = application.NewAcceptRegulatoryDispositionHandler(application.AcceptRegulatoryDispositionDeps{
		Acceptances: fixture.acceptances,
		Receipt:     fixture.receipt,
		Clock:       journeyClock{at: dispositionDecidedAt.Add(time.Minute)},
	})
	return fixture
}

func acceptDispositionCommand(t *testing.T) application.AcceptRegulatoryDispositionCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	item, err := domain.NewCollaborationItemReference("cc-collab-item-1")
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	basis, err := domain.NewDispositionBasisReference("regulatory-return-decision-1")
	if err != nil {
		t.Fatalf("basis: %v", err)
	}
	return application.AcceptRegulatoryDispositionCommand{
		TenantID:          tenant,
		Item:              item,
		Basis:             basis,
		MovementAction:    "RETURN_TO_BONDED_WAREHOUSE",
		Decision:          domain.DispositionAccepted,
		AcceptedObjects:   []string{"unit-1", "unit-2"},
		MovementAuthority: "movement-authority-1",
		DecidedAt:         dispositionDecidedAt,
	}
}

// Covers: UC-TF-001「逐载运对象形成已承接……结果」与一致性规则「相同事项版本、对象、
// 移动范围和请求身份重复到达时返回已有承接……相同身份内容不同形成冲突」——一事项
// 一决定：首决交回执意图，重放返原重发同一份，异内容冒名冲突不顶替。点名 `AT-TF-010`
// 的承接请求半边「请求……重复、冲突……幂等或形成冲突，不按最后到达覆盖」。
func TestOneDecisionPerDispositionItem(t *testing.T) {
	fixture := newAcceptDispositionFixture(t)
	ctx := context.Background()
	command := acceptDispositionCommand(t)

	first, err := fixture.handler.Handle(ctx, command)
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.DispositionDecided {
		t.Fatalf("outcome = %q, want DISPOSITION_DECIDED", first.Outcome())
	}
	record, present := first.Acceptance()
	if !present || record.Decision.Kind() != domain.DispositionAccepted {
		t.Fatal("a decided result carries no acceptance record")
	}
	if len(fixture.receipt.intents) != 1 {
		t.Fatalf("intents = %d, want 1（承接回执是 CC 协作链的输入）", len(fixture.receipt.intents))
	}

	replay, err := fixture.handler.Handle(ctx, command)
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.DispositionExistingDecision {
		t.Fatalf("outcome = %q, want EXISTING_DECISION", replay.Outcome())
	}
	if fixture.acceptances.saves != 1 || len(fixture.receipt.intents) != 2 {
		t.Fatalf("saves = %d intents = %d（重放不重存、重发同一份）", fixture.acceptances.saves, len(fixture.receipt.intents))
	}

	flipped := acceptDispositionCommand(t)
	flipped.Decision = domain.DispositionDeclined
	flipped.AcceptedObjects = nil
	flipped.MovementAuthority = ""
	flipped.DeclineBasis = "capacity-shortage"
	conflicted, err := fixture.handler.Handle(ctx, flipped)
	if err != nil {
		t.Fatalf("conflict handle: %v", err)
	}
	if conflicted.Outcome() != application.DispositionDecisionConflict {
		t.Fatalf("outcome = %q, want DECISION_CONFLICT（改决定走事项方更正，不顶替）", conflicted.Outcome())
	}
}

// Covers: UC-TF-001「本用例只处理需要真实移动的监管协作……原地查验、原地扣留、节点
// 开封、隔离、销毁或只需节点现场执行的动作由 UC-NO-001 承接，不建立运输履约」。点名
// `AT-TF-002`「协作只要求原地查验、原地扣留或节点销毁→不建立运输委托，交由 UC-NO-001
// 处理现场动作」。
func TestANodeOnlyCollaborationBuildsNoTransportObject(t *testing.T) {
	fixture := newAcceptDispositionFixture(t)
	command := acceptDispositionCommand(t)
	command.MovementAction = "  "

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.DispositionNodeOnly {
		t.Fatalf("outcome = %q, want NODE_ONLY_DISPOSITION", result.Outcome())
	}
	if fixture.acceptances.saves != 0 || len(fixture.receipt.intents) != 0 {
		t.Fatal("a node-only disposition still built a transport object or handed off a receipt")
	}
}

// Covers: UC-TF-001「不把扣留决定本身解释为移动授权」与结果契约`待补充`行「必要对象、
// 地点、授权、路由或责任不足→形成待补充，不创建占位旅程」。点名 `AT-TF-003`「扣留
// 范围没有明确移动授权→保持原控制，不因扣留本身创建运输任务」与 `AT-TF-004` 的授权
// 缺口半边「移动授权……缺失→形成待补充」。
func TestDetentionWithoutMovementAuthorityIsSupplementRequired(t *testing.T) {
	fixture := newAcceptDispositionFixture(t)
	command := acceptDispositionCommand(t)
	command.MovementAuthority = ""

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.DispositionSupplementRequired {
		t.Fatalf("outcome = %q, want SUPPLEMENT_REQUIRED", result.Outcome())
	}
	if fixture.acceptances.saves != 0 || len(fixture.receipt.intents) != 0 {
		t.Fatal("a supplement-required round still saved a decision or handed off a receipt")
	}
}

// Covers: 结果契约`已拒绝`行「拒绝范围、依据、决定方和业务时间」与三格形状（领域把门、
// 编排分格）——拒接必带拒因不带对象；拒接反带对象是形状错答未受理。
func TestADeclineCarriesItsBasisAndNoObjects(t *testing.T) {
	fixture := newAcceptDispositionFixture(t)
	ctx := context.Background()

	declined := acceptDispositionCommand(t)
	declined.Decision = domain.DispositionDeclined
	declined.AcceptedObjects = nil
	declined.MovementAuthority = ""
	declined.DeclineBasis = "no-lawful-route"
	result, err := fixture.handler.Handle(ctx, declined)
	if err != nil {
		t.Fatalf("decline handle: %v", err)
	}
	if result.Outcome() != application.DispositionDecided {
		t.Fatalf("outcome = %q, want DISPOSITION_DECIDED", result.Outcome())
	}
	record, _ := result.Acceptance()
	if record.Decision.Kind() != domain.DispositionDeclined || record.Decision.DeclineBasis() == "" {
		t.Fatal("the declined decision lost its basis")
	}

	// 干净夹具：同事项已有决定时幂等/冲突先答，形状错要在首决路上才走得到领域门。
	fresh := newAcceptDispositionFixture(t)
	malformed := acceptDispositionCommand(t)
	malformed.Decision = domain.DispositionDeclined
	malformed.DeclineBasis = "still-carrying-objects"
	broken, err := fresh.handler.Handle(ctx, malformed)
	if err != nil {
		t.Fatalf("malformed handle: %v", err)
	}
	if broken.Outcome() != application.AcceptDispositionNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED（拒接却带对象：领域把门）", broken.Outcome())
	}
}

// Covers: 结果契约`部分承接`行「已承接、未承接范围及逐项原因……不得用整批结果扩大
// 范围」。点名 `AT-TF-006` 的承接半边「逐对象保存……批次不显示整体完成」——部分承接
// 两半都要，缺未承接原因即立不起来。
func TestAPartialAcceptanceKeepsBothSides(t *testing.T) {
	fixture := newAcceptDispositionFixture(t)
	ctx := context.Background()

	partial := acceptDispositionCommand(t)
	partial.Decision = domain.DispositionPartiallyAccepted
	partial.AcceptedObjects = []string{"unit-1"}
	partial.DeclineBasis = "unit-2-not-under-node-control"
	result, err := fixture.handler.Handle(ctx, partial)
	if err != nil {
		t.Fatalf("partial handle: %v", err)
	}
	if result.Outcome() != application.DispositionDecided {
		t.Fatalf("outcome = %q, want DISPOSITION_DECIDED", result.Outcome())
	}
	record, _ := result.Acceptance()
	if len(record.Decision.AcceptedObjects()) != 1 || record.Decision.DeclineBasis() == "" {
		t.Fatal("the partial acceptance lost one of its two halves")
	}

	// 干净夹具：理由同拒接变体——形状错只在首决路上走得到领域门。
	fresh := newAcceptDispositionFixture(t)
	lopsided := acceptDispositionCommand(t)
	lopsided.Decision = domain.DispositionPartiallyAccepted
	lopsided.AcceptedObjects = []string{"unit-1"}
	lopsided.DeclineBasis = ""
	broken, err := fresh.handler.Handle(ctx, lopsided)
	if err != nil {
		t.Fatalf("lopsided handle: %v", err)
	}
	if broken.Outcome() != application.AcceptDispositionNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED（部分承接缺未承接原因）", broken.Outcome())
	}
}

// Covers: 六层分离「监管运输承接决定……不等于建立旅程或开始移动」与派工③——承接
// 决定产出旅程启动消费的监管依据引用：BasisKind 恒为监管格，正好构造得出
// StartAlternateJourney 的监管来路命令；类型上没有旅程、交接或移动字段。
func TestAcceptanceFeedsTheJourneySeamWithItsRegulatoryBasis(t *testing.T) {
	fixture := newAcceptDispositionFixture(t)

	result, err := fixture.handler.Handle(context.Background(), acceptDispositionCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	record, _ := result.Acceptance()
	if record.Decision.BasisKind() != domain.RegulatoryDispositionDecision {
		t.Fatalf("basis kind = %q; 旅程启动按监管格分流双链，普通格冒充不了", record.Decision.BasisKind())
	}
	if record.Decision.Basis().String() != "regulatory-return-decision-1" {
		t.Fatal("the acceptance lost the regulatory basis reference the journey start consumes")
	}
	if _, authorized := record.Decision.MovementAuthority(); !authorized {
		t.Fatal("an accepted disposition carries no movement authority")
	}
}

// Covers: 一致性与恢复——库读不回未决；并发输家读回赢家（同内容等价重放）；回执首发
// 失败决定不翻留续办（ADR-0043「首次交付失败不改写业务结果……另留发布续办引用」）。
func TestDispositionRecoveryDiscipline(t *testing.T) {
	t.Run("a store failure is undecided", func(t *testing.T) {
		fixture := newAcceptDispositionFixture(t)
		fixture.acceptances.findErr = errors.New("store down")
		result, err := fixture.handler.Handle(context.Background(), acceptDispositionCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AcceptDispositionUndecided ||
			result.UndecidedReason() != application.DispositionAcceptanceStoreUnavailable {
			t.Fatalf("result = %q/%q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("a concurrent loser reads back the winner", func(t *testing.T) {
		fixture := newAcceptDispositionFixture(t)
		ctx := context.Background()
		command := acceptDispositionCommand(t)
		if _, err := fixture.handler.Handle(ctx, command); err != nil {
			t.Fatalf("seed: %v", err)
		}
		// 复现竞争窗口：FindByKey 未见、Save 撞已在册。双儿按已存在作答即可。
		result, err := fixture.handler.Handle(ctx, command)
		if err != nil {
			t.Fatalf("loser handle: %v", err)
		}
		if result.Outcome() != application.DispositionExistingDecision {
			t.Fatalf("outcome = %q, want EXISTING_DECISION", result.Outcome())
		}
	})

	t.Run("a failed receipt handoff keeps the decision with a reference", func(t *testing.T) {
		fixture := newAcceptDispositionFixture(t)
		fixture.receipt.err = errors.New("receipt seam unavailable")
		result, err := fixture.handler.Handle(context.Background(), acceptDispositionCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.DispositionDecided {
			t.Fatalf("outcome = %q, want DISPOSITION_DECIDED", result.Outcome())
		}
		if result.HandoffReference() == "" {
			t.Fatal("a failed receipt handoff left no resumable reference")
		}
	})

	t.Run("an unexpected save outcome is a programming error", func(t *testing.T) {
		fixture := newAcceptDispositionFixture(t)
		fixture.acceptances.forced = ports.DispositionAcceptanceSaveOutcome(99)
		if _, err := fixture.handler.Handle(context.Background(), acceptDispositionCommand(t)); err == nil {
			t.Fatal("an out-of-set save outcome was swallowed instead of raised")
		}
	})
}
