package application_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本文件用内存替身证入库预报的判定次序（UC-WF-003 不涉及接入面的部分）。真库上的锁与单据号另在适配器的用例里证。

type preAdviceWorld struct {
	store      *fakePreAdvices
	warehouses *fakePreAdviceWarehouses
	goods      *fakePreAdviceGoods
	numbers    *fakePreAdviceNumbers
	ids        *fakePreAdviceIDs
	owners     *fakeOwnerAccounts
}

func newPreAdviceWorld(t *testing.T) preAdviceWorld {
	t.Helper()
	world := preAdviceWorld{
		store:      newFakePreAdvices(),
		warehouses: newFakePreAdviceWarehouses(),
		goods:      newFakePreAdviceGoods(),
		numbers:    &fakePreAdviceNumbers{outcome: ports.PreAdviceNumberAssigned, number: "IB-20261120-00001"},
		ids:        &fakePreAdviceIDs{},
		owners:     registeredOwners(),
	}
	world.warehouses.put(adviceWarehouse(t, domain.WarehouseKindSelfOperated, false, "Asia/Shanghai"))
	world.goods.put(adviceGoods(t, "SYN-GOODS-01", false, false, "件", "箱"))
	return world
}

func (world preAdviceWorld) handler() *application.AcceptPreAdviceHandler {
	return application.NewAcceptPreAdviceHandler(
		world.store, world.warehouses, world.warehouses, world.goods, world.numbers, world.ids, world.owners,
		&fixedClock{at: registrationInstant},
	)
}

func TestAcceptPreAdviceReplaysBeforeTheWarehouseIsRechecked(t *testing.T) {
	world := newPreAdviceWorld(t)
	first := mustAccept(t, world, acceptCommand(t, "REQ-1", nil))
	advice, _ := first.PreAdvice()
	world.warehouses.put(adviceWarehouse(t, domain.WarehouseKindSelfOperated, true, "Asia/Shanghai"))
	replayCommand := acceptCommand(t, "REQ-1", func(command *application.AcceptPreAdviceCommand) {
		command.Registrant = valueOf(t, domain.NewRegistrant, "operator-2")
	})
	replay := mustAccept(t, world, replayCommand)
	replayed, has := replay.PreAdvice()
	number, numbered := replayed.DocumentNumber()
	original, _ := advice.DocumentNumber()
	if replay.Outcome() != application.PreAdviceOutcomeExistingResult || !has || replayed.ID() != advice.ID() || !numbered || number != original {
		t.Fatalf("重投 = %s %v", replay.Outcome(), has)
	}
	if world.ids.calls.Load() != 1 || world.numbers.calls.Load() != 1 || world.store.inserts.Load() != 1 {
		t.Fatalf("重投又签发或取号：标识 %d 取号 %d 插入 %d", world.ids.calls.Load(), world.numbers.calls.Load(), world.store.inserts.Load())
	}
	changed := acceptCommand(t, "REQ-1", func(command *application.AcceptPreAdviceCommand) {
		command.GoodsLines = []domain.GoodsInboundLine{adviceGoodsLine(t, "SYN-GOODS-01", "件", 9)}
	})
	if conflict := mustAccept(t, world, changed); conflict.Outcome() != application.PreAdviceOutcomeSourceConflict || world.store.inserts.Load() != 1 {
		t.Fatalf("换内容 = %s 插入 %d", conflict.Outcome(), world.store.inserts.Load())
	}
}

func TestAcceptPreAdviceCollectsEveryContentDefectAndStillAcceptsAMissingExpiry(t *testing.T) {
	world := newPreAdviceWorld(t)
	world.goods.put(adviceGoods(t, "SYN-GOODS-01", false, true, "件"))
	bad := acceptCommand(t, "REQ-BAD", func(command *application.AcceptPreAdviceCommand) {
		command.GoodsLines = []domain.GoodsInboundLine{
			adviceGoodsLineCounted(t, "SYN-MISSING", "件", 0, "", "", 0, true),
			adviceGoodsLineCounted(t, "SYN-GOODS-01", "托", 2, "LOT-1", "", 0, false),
			adviceGoodsLineCounted(t, "SYN-GOODS-01", "件", 2, "", "", 0, false),
		}
	})
	result := mustAccept(t, world, bad)
	if result.Outcome() != application.PreAdviceOutcomeContentInvalid || world.store.inserts.Load() != 0 || world.numbers.calls.Load() != 0 {
		t.Fatalf("不合 = %s 插入 %d 取号 %d", result.Outcome(), world.store.inserts.Load(), world.numbers.calls.Load())
	}
	want := []string{
		"0:-1:GOODS_NOT_REGISTERED",
		"0:-1:QUANTITY_NOT_POSITIVE",
		"0:-1:CARTON_COUNT_NOT_POSITIVE",
		"1:-1:UNIT_NOT_A_PACKAGING_LEVEL",
		"1:-1:BATCH_NOT_TRACKED",
	}
	if got := contentIssueTexts(result.Issues()); len(got) != len(want) {
		t.Fatalf("不合之处 = %v，want %v", got, want)
	} else {
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("不合之处 = %v，want %v", got, want)
			}
		}
	}
	if world.goods.loads.Load() != 2 {
		t.Fatalf("商品读了 %d 次，两行同一个已登记商品只该再读一次没登记的", world.goods.loads.Load())
	}

	missingExpiry := acceptCommand(t, "REQ-EXPIRY", func(command *application.AcceptPreAdviceCommand) {
		command.GoodsLines = []domain.GoodsInboundLine{adviceGoodsLine(t, "SYN-GOODS-01", "件", 1)}
	})
	accepted := mustAccept(t, world, missingExpiry)
	if accepted.Outcome() != application.PreAdviceOutcomeAdvised {
		t.Fatalf("跟踪效期却没带 = %s %v", accepted.Outcome(), contentIssueTexts(accepted.Issues()))
	}
}

func TestAcceptPreAdviceRefusesCartonDefectsAndWarehousesThatDoNotTakeAdvice(t *testing.T) {
	world := newPreAdviceWorld(t)
	cartons := acceptCommand(t, "REQ-CARTON", func(command *application.AcceptPreAdviceCommand) {
		command.Inbound = domain.InboundByCarton
		command.GoodsLines = nil
		command.Cartons = []domain.Carton{
			adviceCarton(t, "C-1", adviceCartonLine(t, "SYN-MISSING", 1)),
			adviceCarton(t, "C-2"),
			adviceCarton(t, "C-1", adviceCartonLine(t, "SYN-GOODS-01", 0)),
		}
	})
	refused := mustAccept(t, world, cartons)
	want := []string{
		"0:-1:DUPLICATE_CARTON_NUMBER",
		"0:0:GOODS_NOT_REGISTERED",
		"1:-1:EMPTY_CARTON",
		"2:-1:DUPLICATE_CARTON_NUMBER",
		"0:2:QUANTITY_NOT_POSITIVE",
	}
	if refused.Outcome() != application.PreAdviceOutcomeContentInvalid {
		t.Fatalf("整箱 = %s", refused.Outcome())
	}
	if got := contentIssueTexts(refused.Issues()); len(got) != len(want) {
		t.Fatalf("整箱不合 = %v，want %v", got, want)
	} else {
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("整箱不合 = %v，want %v", got, want)
			}
		}
	}
	if world.goods.loads.Load() != 2 {
		t.Fatalf("箱单里的商品读了 %d 次，两个编码各该读一次", world.goods.loads.Load())
	}

	world.warehouses.put(adviceWarehouse(t, domain.WarehouseKindAgent, false, "Asia/Shanghai"))
	agent := mustAccept(t, world, acceptCommand(t, "REQ-AGENT", nil))
	reason, hasReason := agent.NotAcceptingReason()
	if agent.Outcome() != application.PreAdviceOutcomeWarehouseNotAccepting || !hasReason || reason != application.WarehouseNotAcceptingAgent {
		t.Fatalf("代理仓 = %s %s", agent.Outcome(), reason)
	}
	world.warehouses.put(adviceWarehouse(t, domain.WarehouseKindSelfOperated, true, ""))
	deactivated := mustAccept(t, world, acceptCommand(t, "REQ-OFF", nil))
	reason, _ = deactivated.NotAcceptingReason()
	if deactivated.Outcome() != application.PreAdviceOutcomeWarehouseNotAccepting || reason != application.WarehouseNotAcceptingDeactivated {
		t.Fatalf("停用仓 = %s %s", deactivated.Outcome(), reason)
	}
	world.warehouses.rows = nil
	missing := mustAccept(t, world, acceptCommand(t, "REQ-NOWHERE", nil))
	if missing.Outcome() != application.PreAdviceOutcomeWarehouseNotFound || world.numbers.calls.Load() != 0 {
		t.Fatalf("没有仓 = %s 取号 %d", missing.Outcome(), world.numbers.calls.Load())
	}
}

func TestAcceptPreAdviceRecordsAMissingNumberingTimeZoneAndDropsACollision(t *testing.T) {
	world := newPreAdviceWorld(t)
	world.numbers.outcome = ports.PreAdviceNumberTimeZoneNotConfigured
	accepted := mustAccept(t, world, acceptCommand(t, "REQ-NO-ZONE", nil))
	advice, has := accepted.PreAdvice()
	_, numbered := advice.DocumentNumber()
	if accepted.Outcome() != application.PreAdviceOutcomeAdvised || !has || numbered ||
		advice.NumberingAbsence().String() != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" || advice.CurrentNotice().Version() != 1 {
		t.Fatalf("无时区 = %s 有号 %v 原因 %s", accepted.Outcome(), numbered, advice.NumberingAbsence())
	}

	colliding := newPreAdviceWorld(t)
	colliding.numbers.outcome = ports.PreAdviceNumberRuleCollision
	collision := mustAccept(t, colliding, acceptCommand(t, "REQ-HIT", nil))
	if collision.Outcome() != application.PreAdviceOutcomeNumberingRuleCollision || colliding.store.inserts.Load() != 0 || colliding.ids.calls.Load() != 0 {
		t.Fatalf("撞号 = %s 插入 %d 标识 %d", collision.Outcome(), colliding.store.inserts.Load(), colliding.ids.calls.Load())
	}
}

func TestUpdatePreAdviceNoticeKeepsVersionsAndClearsWhatWasLeftOut(t *testing.T) {
	world := newPreAdviceWorld(t)
	accepted := mustAccept(t, world, acceptCommand(t, "REQ-1", func(command *application.AcceptPreAdviceCommand) {
		command.Clue = valueOf(t, domain.NewArrivalClue, "CONT-1")
		command.Reference = valueOf(t, domain.NewOwnerReference, "REF-1")
	}))
	advice, _ := accepted.PreAdvice()
	originalNumber, _ := advice.DocumentNumber()
	updated := mustUpdate(t, world, noticeCommand(t, advice.ID().String(), "REQ-N1", func(command *application.UpdatePreAdviceNoticeCommand) {
		command.ExpectedAt = registrationInstant.Add(48 * time.Hour)
	}))
	notice, hasNotice := updated.Notice()
	landed, _ := updated.PreAdvice()
	number, _ := landed.DocumentNumber()
	if updated.Outcome() != application.PreAdviceOutcomeNoticeUpdated || !hasNotice || notice.Version() != 2 ||
		notice.Clue().Given() || notice.Reference().Given() || number != originalNumber ||
		len(landed.GoodsLines()) != len(advice.GoodsLines()) || landed.Status() != domain.PreAdviceAdvised {
		t.Fatalf("清空线索 = %s 版本 %d 线索 %v 参考号 %v 状态 %s", updated.Outcome(), notice.Version(), notice.Clue().Given(), notice.Reference().Given(), landed.Status())
	}
	if !landed.Notices()[0].Clue().Given() || landed.Notices()[0].Clue().String() != "CONT-1" {
		t.Fatal("第 1 版告知被改掉了")
	}
	same := mustUpdate(t, world, noticeCommand(t, advice.ID().String(), "REQ-N2", func(command *application.UpdatePreAdviceNoticeCommand) {
		command.ExpectedAt = notice.ExpectedAt()
	}))
	if same.Outcome() != application.PreAdviceOutcomeNoticeUpdated || len(mustPreAdvice(t, same).Notices()) != 3 {
		t.Fatalf("与现行版相同也该留一版，得到 %s 共 %d 版", same.Outcome(), len(mustPreAdvice(t, same).Notices()))
	}
	replay := mustUpdate(t, world, noticeCommand(t, advice.ID().String(), "REQ-N2", func(command *application.UpdatePreAdviceNoticeCommand) {
		command.ExpectedAt = notice.ExpectedAt()
		command.Registrant = valueOf(t, domain.NewRegistrant, "operator-2")
	}))
	replayed, hasReplay := replay.Notice()
	if replay.Outcome() != application.PreAdviceOutcomeExistingResult || !hasReplay || replayed.Version() != 3 || len(mustPreAdvice(t, replay).Notices()) != 3 {
		t.Fatalf("告知重投 = %s 版本 %d", replay.Outcome(), replayed.Version())
	}
	changed := mustUpdate(t, world, noticeCommand(t, advice.ID().String(), "REQ-N2", func(command *application.UpdatePreAdviceNoticeCommand) {
		command.ExpectedAt = registrationInstant.Add(72 * time.Hour)
	}))
	if changed.Outcome() != application.PreAdviceOutcomeSourceConflict || len(mustStored(t, world, advice.ID()).Notices()) != 3 {
		t.Fatalf("告知换内容 = %s", changed.Outcome())
	}
}

func TestUpdatePreAdviceNoticeRefusesAFinishedAdviceAndAnotherOwners(t *testing.T) {
	world := newPreAdviceWorld(t)
	accepted := mustAccept(t, world, acceptCommand(t, "REQ-1", nil))
	advice, _ := accepted.PreAdvice()
	if started, err := world.handler().NoteReceiptStarted(t.Context(), application.NotePreAdviceCommand{Tenant: advice.Tenant(), ID: advice.ID()}); err != nil ||
		started.Outcome() != application.PreAdviceReceiptStarted {
		t.Fatalf("收货缝 = %s %v", started.Outcome(), err)
	}
	receiving := mustUpdate(t, world, noticeCommand(t, advice.ID().String(), "REQ-R", nil))
	if receiving.Outcome() != application.PreAdviceOutcomeNoticeUpdated {
		t.Fatalf("收货中更新 = %s", receiving.Outcome())
	}
	if closed, err := world.handler().NoteClosed(t.Context(), application.NotePreAdviceCommand{Tenant: advice.Tenant(), ID: advice.ID()}); err != nil ||
		closed.Outcome() != application.PreAdviceCloseClosed {
		t.Fatalf("完结缝 = %s %v", closed.Outcome(), err)
	}
	finished := mustUpdate(t, world, noticeCommand(t, advice.ID().String(), "REQ-C", nil))
	status, hasStatus := finished.Status()
	if finished.Outcome() != application.PreAdviceOutcomeStatusMismatch || !hasStatus || status != domain.PreAdviceClosed {
		t.Fatalf("已完结 = %s %s", finished.Outcome(), status)
	}

	cancelledWorld := newPreAdviceWorld(t)
	cancelledAdvice, _ := mustAccept(t, cancelledWorld, acceptCommand(t, "REQ-1", nil)).PreAdvice()
	if withdrawn := mustWithdraw(t, cancelledWorld, withdrawCommand(t, cancelledAdvice.ID().String(), "REQ-W", nil)); withdrawn.Outcome() != application.PreAdviceOutcomeCancelled {
		t.Fatal(withdrawn.Outcome())
	}
	cancelled := mustUpdate(t, cancelledWorld, noticeCommand(t, cancelledAdvice.ID().String(), "REQ-N", nil))
	status, _ = cancelled.Status()
	if cancelled.Outcome() != application.PreAdviceOutcomeStatusMismatch || status != domain.PreAdviceCancelled {
		t.Fatalf("已取消 = %s %s", cancelled.Outcome(), status)
	}

	other := mustUpdate(t, world, noticeCommand(t, advice.ID().String(), "REQ-OTHER", func(command *application.UpdatePreAdviceNoticeCommand) {
		command.Owner = valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-02")
	}))
	if other.Outcome() != application.PreAdviceOutcomeNotFound {
		t.Fatalf("别的货主 = %s", other.Outcome())
	}
}

func TestWithdrawPreAdviceCancelsOnceAndKeepsTheNumber(t *testing.T) {
	world := newPreAdviceWorld(t)
	first, _ := mustAccept(t, world, acceptCommand(t, "REQ-1", nil)).PreAdvice()
	second, _ := mustAccept(t, world, acceptCommand(t, "REQ-2", nil)).PreAdvice()
	original, _ := first.DocumentNumber()
	withdrawn := mustWithdraw(t, world, withdrawCommand(t, first.ID().String(), "REQ-W", nil))
	cancelled, has := withdrawn.PreAdvice()
	number, numbered := cancelled.DocumentNumber()
	if withdrawn.Outcome() != application.PreAdviceOutcomeCancelled || !has || cancelled.Status() != domain.PreAdviceCancelled || !numbered || number != original {
		t.Fatalf("撤回 = %s 状态 %s", withdrawn.Outcome(), cancelled.Status())
	}
	again := mustWithdraw(t, world, withdrawCommand(t, first.ID().String(), "REQ-W-2", nil))
	if again.Outcome() != application.PreAdviceOutcomeExistingResult {
		t.Fatalf("再撤 = %s", again.Outcome())
	}
	conflict := mustWithdraw(t, world, withdrawCommand(t, second.ID().String(), "REQ-W", nil))
	if conflict.Outcome() != application.PreAdviceOutcomeSourceConflict || mustStored(t, world, second.ID()).Status() != domain.PreAdviceAdvised {
		t.Fatalf("同一键撤另一份 = %s 状态 %s", conflict.Outcome(), mustStored(t, world, second.ID()).Status())
	}

	receivingWorld := newPreAdviceWorld(t)
	receiving, _ := mustAccept(t, receivingWorld, acceptCommand(t, "REQ-1", nil)).PreAdvice()
	if _, err := receivingWorld.handler().NoteReceiptStarted(t.Context(), application.NotePreAdviceCommand{Tenant: receiving.Tenant(), ID: receiving.ID()}); err != nil {
		t.Fatal(err)
	}
	started := mustWithdraw(t, receivingWorld, withdrawCommand(t, receiving.ID().String(), "REQ-W", nil))
	status, hasStatus := started.Status()
	if started.Outcome() != application.PreAdviceOutcomeReceiptStarted || !hasStatus || status != domain.PreAdviceReceiving {
		t.Fatalf("收货中撤回 = %s %s", started.Outcome(), status)
	}
	otherOwner := mustWithdraw(t, world, withdrawCommand(t, second.ID().String(), "REQ-OTHER", func(command *application.WithdrawPreAdviceCommand) {
		command.Owner = valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-02")
	}))
	if otherOwner.Outcome() != application.PreAdviceOutcomeNotFound || mustStored(t, world, second.ID()).Status() != domain.PreAdviceAdvised {
		t.Fatalf("别的货主撤回 = %s", otherOwner.Outcome())
	}
}

func TestNoticeUpdateAndWithdrawWhenNoWarehouseIsRegisteredDoNotReadTheAdvice(t *testing.T) {
	world := newPreAdviceWorld(t)
	world.warehouses.rows = map[string]domain.WarehouseRevision{}
	update := mustUpdate(t, world, noticeCommand(t, "IPA-MISSING", "REQ-N", nil))
	withdraw := mustWithdraw(t, world, withdrawCommand(t, "IPA-MISSING", "REQ-W", nil))
	if update.Outcome() != application.PreAdviceOutcomeNotConfigured || withdraw.Outcome() != application.PreAdviceOutcomeNotConfigured {
		t.Fatalf("更新 %s 撤回 %s", update.Outcome(), withdraw.Outcome())
	}
	if world.store.reads.Load() != 0 || world.store.holds.Load() != 0 {
		t.Fatalf("没登记仓仍查了预报：读 %d 锁 %d", world.store.reads.Load(), world.store.holds.Load())
	}
}

func TestOnBehalfAcceptRefusesAnUnknownOwnerBeforeReadingAnythingElse(t *testing.T) {
	world := newPreAdviceWorld(t)
	world.owners = &fakeOwnerAccounts{}
	command := acceptCommand(t, "DEC-1", func(command *application.AcceptPreAdviceCommand) {
		command.Submission = onBehalfPreAdviceSubmission(t, "DEC-1")
	})
	_, err := world.handler().Accept(t.Context(), command)
	if !errors.Is(err, application.ErrOwnerAccountNotInTenant) {
		t.Fatalf("查不到货主 = %v", err)
	}
	if world.warehouses.loads.Load() != 0 || world.store.reads.Load() != 0 || world.store.holds.Load() != 0 {
		t.Fatalf("查不到货主仍往下读了：仓 %d 预报读 %d 锁 %d", world.warehouses.loads.Load(), world.store.reads.Load(), world.store.holds.Load())
	}
	world.owners.err = errors.New("party commercial is down")
	_, err = world.handler().Accept(t.Context(), command)
	if err == nil || errors.Is(err, application.ErrOwnerAccountNotInTenant) {
		t.Fatalf("读不动 = %v", err)
	}

	present := newPreAdviceWorld(t)
	accepted := mustAccept(t, present, acceptCommand(t, "REQ-1", nil))
	advice, _ := accepted.PreAdvice()
	onBehalf := noticeCommand(t, advice.ID().String(), "DEC-N", func(command *application.UpdatePreAdviceNoticeCommand) {
		command.Owner = domain.OwnerAccountID{}
		command.Submission = onBehalfPreAdviceSubmission(t, "DEC-N")
	})
	updated := mustUpdate(t, present, onBehalf)
	landed, _ := updated.PreAdvice()
	if updated.Outcome() != application.PreAdviceOutcomeNoticeUpdated || landed.Owner() != advice.Owner() {
		t.Fatalf("代录告知 = %s 货主 %s", updated.Outcome(), landed.Owner())
	}
}

func TestPreAdviceReceiptAndCloseSeams(t *testing.T) {
	world := newPreAdviceWorld(t)
	advice, _ := mustAccept(t, world, acceptCommand(t, "REQ-1", nil)).PreAdvice()
	note := application.NotePreAdviceCommand{Tenant: advice.Tenant(), ID: advice.ID()}
	started, err := world.handler().NoteReceiptStarted(t.Context(), note)
	if err != nil || started.Outcome() != application.PreAdviceReceiptStarted || mustNoted(t, started).Status() != domain.PreAdviceReceiving {
		t.Fatalf("开始 = %s %v", started.Outcome(), err)
	}
	again, err := world.handler().NoteReceiptStarted(t.Context(), note)
	if err != nil || again.Outcome() != application.PreAdviceReceiptAlreadyReceiving {
		t.Fatalf("再开始 = %s %v", again.Outcome(), err)
	}
	closed, err := world.handler().NoteClosed(t.Context(), note)
	if err != nil || closed.Outcome() != application.PreAdviceCloseClosed || mustClosed(t, closed).Status() != domain.PreAdviceClosed {
		t.Fatalf("完结 = %s %v", closed.Outcome(), err)
	}
	repeat, err := world.handler().NoteClosed(t.Context(), note)
	if err != nil || repeat.Outcome() != application.PreAdviceCloseAlreadyClosed {
		t.Fatalf("再完结 = %s %v", repeat.Outcome(), err)
	}
	if late, err := world.handler().NoteReceiptStarted(t.Context(), note); err != nil || late.Outcome() != application.PreAdviceReceiptNotReceivable {
		t.Fatalf("完结后收货 = %s %v", late.Outcome(), err)
	}

	cancelledWorld := newPreAdviceWorld(t)
	cancelled, _ := mustAccept(t, cancelledWorld, acceptCommand(t, "REQ-1", nil)).PreAdvice()
	if withdrawn := mustWithdraw(t, cancelledWorld, withdrawCommand(t, cancelled.ID().String(), "REQ-W", nil)); withdrawn.Outcome() != application.PreAdviceOutcomeCancelled {
		t.Fatal(withdrawn.Outcome())
	}
	refused, err := cancelledWorld.handler().NoteClosed(t.Context(), application.NotePreAdviceCommand{Tenant: cancelled.Tenant(), ID: cancelled.ID()})
	if err != nil || refused.Outcome() != application.PreAdviceCloseNotClosable {
		t.Fatalf("已取消完结 = %s %v", refused.Outcome(), err)
	}
}

type fakePreAdvices struct {
	mu      sync.Mutex
	byID    map[string]domain.PreAdvice
	reads   atomic.Int32
	holds   atomic.Int32
	inserts atomic.Int32
}

func newFakePreAdvices() *fakePreAdvices {
	return &fakePreAdvices{byID: map[string]domain.PreAdvice{}}
}

func (fake *fakePreAdvices) HoldSubmission(context.Context, domain.TenantID, domain.PreAdviceSubmission) error {
	fake.holds.Add(1)
	return nil
}

func (fake *fakePreAdvices) HoldPreAdvice(context.Context, domain.TenantID, domain.PreAdviceID) error {
	fake.holds.Add(1)
	return nil
}

func (fake *fakePreAdvices) FindBySubmission(_ context.Context, tenant domain.TenantID, submission domain.PreAdviceSubmission) (domain.PreAdvice, bool, error) {
	fake.reads.Add(1)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, advice := range fake.byID {
		if advice.Tenant() == tenant && advice.Submission().SameSlotAs(submission) {
			return advice, true, nil
		}
	}
	return domain.PreAdvice{}, false, nil
}

func (fake *fakePreAdvices) FindNoticeBySubmission(_ context.Context, tenant domain.TenantID, submission domain.PreAdviceSubmission) (domain.PreAdviceID, domain.PreAdviceNotice, bool, error) {
	fake.reads.Add(1)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, advice := range fake.byID {
		if advice.Tenant() != tenant {
			continue
		}
		for _, notice := range advice.Notices() {
			if notice.Submission().SameSlotAs(submission) {
				return advice.ID(), notice, true, nil
			}
		}
	}
	return domain.PreAdviceID{}, domain.PreAdviceNotice{}, false, nil
}

func (fake *fakePreAdvices) FindWithdrawalBySubmission(_ context.Context, tenant domain.TenantID, submission domain.PreAdviceSubmission) (domain.PreAdviceID, bool, error) {
	fake.reads.Add(1)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, advice := range fake.byID {
		withdrawal, recorded := advice.Withdrawal()
		if advice.Tenant() == tenant && recorded && withdrawal.Submission().SameSlotAs(submission) {
			return advice.ID(), true, nil
		}
	}
	return domain.PreAdviceID{}, false, nil
}

func (fake *fakePreAdvices) FindForOwner(_ context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, id domain.PreAdviceID) (domain.PreAdvice, bool, error) {
	fake.reads.Add(1)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	advice, found := fake.byID[id.String()]
	if !found || advice.Tenant() != tenant || advice.Owner() != owner {
		return domain.PreAdvice{}, false, nil
	}
	return advice, true, nil
}

func (fake *fakePreAdvices) Find(_ context.Context, tenant domain.TenantID, id domain.PreAdviceID) (domain.PreAdvice, bool, error) {
	fake.reads.Add(1)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	advice, found := fake.byID[id.String()]
	if !found || advice.Tenant() != tenant {
		return domain.PreAdvice{}, false, nil
	}
	return advice, true, nil
}

func (fake *fakePreAdvices) Insert(_ context.Context, advice domain.PreAdvice) (ports.PreAdviceWriteOutcome, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, existing := range fake.byID {
		if existing.Tenant() == advice.Tenant() && existing.Submission().SameSlotAs(advice.Submission()) {
			return ports.PreAdviceWriteKeyTaken, nil
		}
	}
	fake.byID[advice.ID().String()] = advice
	fake.inserts.Add(1)
	return ports.PreAdviceWritten, nil
}

func (fake *fakePreAdvices) AppendNotice(_ context.Context, tenant domain.TenantID, id domain.PreAdviceID, notice domain.PreAdviceNotice) (ports.PreAdviceWriteOutcome, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, advice := range fake.byID {
		for _, existing := range advice.Notices() {
			if advice.Tenant() == tenant && existing.Submission().SameSlotAs(notice.Submission()) {
				return ports.PreAdviceWriteKeyTaken, nil
			}
		}
	}
	current, found := fake.byID[id.String()]
	if !found {
		return ports.PreAdviceWriteOutcomeInvalid, errors.New("missing pre-advice")
	}
	updated, err := current.AppendNotice(notice)
	if err != nil {
		return ports.PreAdviceWriteOutcomeInvalid, err
	}
	fake.byID[id.String()] = updated
	return ports.PreAdviceWritten, nil
}

func (fake *fakePreAdvices) Cancel(_ context.Context, advice domain.PreAdvice) (ports.PreAdviceWriteOutcome, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	withdrawal, recorded := advice.Withdrawal()
	if recorded {
		for _, existing := range fake.byID {
			other, has := existing.Withdrawal()
			if has && existing.Tenant() == advice.Tenant() && other.Submission().SameSlotAs(withdrawal.Submission()) && existing.ID() != advice.ID() {
				return ports.PreAdviceWriteKeyTaken, nil
			}
		}
	}
	fake.byID[advice.ID().String()] = advice
	return ports.PreAdviceWritten, nil
}

func (fake *fakePreAdvices) MarkReceiving(_ context.Context, tenant domain.TenantID, id domain.PreAdviceID) error {
	return fake.mark(tenant, id, true)
}

func (fake *fakePreAdvices) MarkClosed(_ context.Context, tenant domain.TenantID, id domain.PreAdviceID) error {
	return fake.mark(tenant, id, false)
}

func (fake *fakePreAdvices) mark(tenant domain.TenantID, id domain.PreAdviceID, receiving bool) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	current, found := fake.byID[id.String()]
	if !found || current.Tenant() != tenant {
		return errors.New("missing pre-advice")
	}
	var next domain.PreAdvice
	var err error
	if receiving {
		next, err = current.StartReceiving()
	} else {
		next, err = current.CloseOut()
	}
	if err != nil {
		return err
	}
	fake.byID[id.String()] = next
	return nil
}

type fakePreAdviceWarehouses struct {
	mu    sync.Mutex
	rows  map[string]domain.WarehouseRevision
	loads atomic.Int32
}

func newFakePreAdviceWarehouses() *fakePreAdviceWarehouses {
	return &fakePreAdviceWarehouses{rows: map[string]domain.WarehouseRevision{}}
}

func (fake *fakePreAdviceWarehouses) put(revision domain.WarehouseRevision) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.rows == nil {
		fake.rows = map[string]domain.WarehouseRevision{}
	}
	fake.rows[revision.Code().String()] = revision
}

func (fake *fakePreAdviceWarehouses) LoadLatestWarehouse(_ context.Context, tenant domain.TenantID, code domain.WarehouseCode) (domain.WarehouseRevision, bool, error) {
	fake.loads.Add(1)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	revision, found := fake.rows[code.String()]
	if !found || revision.Tenant() != tenant {
		return domain.WarehouseRevision{}, false, nil
	}
	return revision, true, nil
}

func (fake *fakePreAdviceWarehouses) HasAnyWarehouse(context.Context, domain.TenantID) (bool, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return len(fake.rows) > 0, nil
}

type fakePreAdviceGoods struct {
	mu    sync.Mutex
	rows  map[string]domain.GoodsRevision
	loads atomic.Int32
}

func newFakePreAdviceGoods() *fakePreAdviceGoods {
	return &fakePreAdviceGoods{rows: map[string]domain.GoodsRevision{}}
}

func (fake *fakePreAdviceGoods) put(revision domain.GoodsRevision) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.rows[revision.Code().String()] = revision
}

func (fake *fakePreAdviceGoods) LoadLatestGoods(_ context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, code domain.GoodsCode) (domain.GoodsRevision, bool, error) {
	fake.loads.Add(1)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	revision, found := fake.rows[code.String()]
	if !found || revision.Tenant() != tenant || revision.Owner() != owner {
		return domain.GoodsRevision{}, false, nil
	}
	return revision, true, nil
}

type fakePreAdviceNumbers struct {
	outcome ports.PreAdviceNumberOutcome
	number  string
	calls   atomic.Int32
}

func (fake *fakePreAdviceNumbers) IssuePreAdviceNumber(context.Context, domain.TenantID, domain.BusinessTimeZone) (ports.PreAdviceNumberIssue, error) {
	fake.calls.Add(1)
	switch fake.outcome {
	case ports.PreAdviceNumberTimeZoneNotConfigured:
		return ports.PreAdviceNumberIssue{Outcome: ports.PreAdviceNumberTimeZoneNotConfigured}, nil
	case ports.PreAdviceNumberRuleCollision:
		return ports.PreAdviceNumberIssue{Outcome: ports.PreAdviceNumberRuleCollision}, nil
	default:
		number, err := domain.NewDocumentNumber(fake.number)
		if err != nil {
			return ports.PreAdviceNumberIssue{}, err
		}
		return ports.PreAdviceNumberIssue{Outcome: ports.PreAdviceNumberAssigned, Number: number}, nil
	}
}

type fakePreAdviceIDs struct {
	calls atomic.Int32
}

func (fake *fakePreAdviceIDs) NextPreAdviceID() (domain.PreAdviceID, error) {
	next := fake.calls.Add(1)
	return domain.NewPreAdviceID("IPA-" + time.Unix(0, int64(next)).Format("150405.000000000"))
}

func adviceWarehouse(t *testing.T, kind domain.WarehouseKind, deactivated bool, zone string) domain.WarehouseRevision {
	t.Helper()
	spec := domain.WarehouseContentSpec{
		Name: valueOf(t, domain.NewWarehouseName, "合成一号仓"),
		Kind: kind,
		Address: domain.NewWarehouseAddress(
			valueOf(t, domain.NewCountryCode, "CN"), valueOf(t, domain.NewAddressText, "合成路 1 号"),
		),
		Deactivated: deactivated,
	}
	if zone != "" {
		spec.BusinessTimeZone = valueOf(t, domain.NewBusinessTimeZone, zone)
	}
	content, err := domain.NewWarehouseContent(spec)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := domain.NewWarehouseRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		1, content, valueOf(t, domain.NewRegistrant, "operator-1"), registrationInstant,
	)
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func adviceGoods(t *testing.T, code string, trackBatch, trackExpiry bool, units ...string) domain.GoodsRevision {
	t.Helper()
	levels := make([]domain.PackagingLevel, len(units))
	for index, unit := range units {
		factor := int64(1)
		if index > 0 {
			factor = 24
		}
		levels[index] = domain.NewPackagingLevel(valueOf(t, domain.NewGoodsUnitName, unit), factor)
	}
	content, err := domain.NewGoodsContent(domain.GoodsContentSpec{
		Name: valueOf(t, domain.NewGoodsName, "合成商品"), BaseUnit: valueOf(t, domain.NewGoodsUnitName, "件"),
		PackagingLevels: levels, TrackBatch: trackBatch, TrackExpiry: trackExpiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	submission, err := domain.NewOwnerSubmission(valueOf(t, domain.NewSubmissionSource, "SYN-CHANNEL"), valueOf(t, domain.NewRequestKey, "GOODS-"+code))
	if err != nil {
		t.Fatal(err)
	}
	revision, err := domain.NewGoodsRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		valueOf(t, domain.NewGoodsCode, code), 1, content, submission, valueOf(t, domain.NewRegistrant, "operator-1"), registrationInstant,
	)
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func acceptCommand(t *testing.T, key string, edit func(*application.AcceptPreAdviceCommand)) application.AcceptPreAdviceCommand {
	t.Helper()
	command := application.AcceptPreAdviceCommand{
		Tenant:     valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		Owner:      valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		Warehouse:  valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		Arrival:    domain.ArrivalFullContainer,
		Inbound:    domain.InboundByGoods,
		GoodsLines: []domain.GoodsInboundLine{adviceGoodsLine(t, "SYN-GOODS-01", "件", 2)},
		ExpectedAt: registrationInstant.Add(24 * time.Hour),
		Submission: ownerPreAdviceSubmission(t, key),
		Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
	}
	if edit != nil {
		edit(&command)
	}
	return command
}

func noticeCommand(t *testing.T, id, key string, edit func(*application.UpdatePreAdviceNoticeCommand)) application.UpdatePreAdviceNoticeCommand {
	t.Helper()
	command := application.UpdatePreAdviceNoticeCommand{
		Tenant:     valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		Owner:      valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		ID:         valueOf(t, domain.NewPreAdviceID, id),
		ExpectedAt: registrationInstant.Add(36 * time.Hour),
		Submission: ownerPreAdviceSubmission(t, key),
		Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
	}
	if edit != nil {
		edit(&command)
	}
	return command
}

func withdrawCommand(t *testing.T, id, key string, edit func(*application.WithdrawPreAdviceCommand)) application.WithdrawPreAdviceCommand {
	t.Helper()
	command := application.WithdrawPreAdviceCommand{
		Tenant:     valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		Owner:      valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		ID:         valueOf(t, domain.NewPreAdviceID, id),
		Submission: ownerPreAdviceSubmission(t, key),
		Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
	}
	if edit != nil {
		edit(&command)
	}
	return command
}

func ownerPreAdviceSubmission(t *testing.T, key string) domain.PreAdviceSubmission {
	t.Helper()
	submission, err := domain.NewOwnerPreAdviceSubmission(valueOf(t, domain.NewSubmissionSource, "SYN-CHANNEL"), valueOf(t, domain.NewRequestKey, key))
	if err != nil {
		t.Fatal(err)
	}
	return submission
}

func onBehalfPreAdviceSubmission(t *testing.T, key string) domain.PreAdviceSubmission {
	t.Helper()
	submission, err := domain.NewOnBehalfPreAdviceSubmission(valueOf(t, domain.NewRequestKey, key), valueOf(t, domain.NewEvidenceReference, "AUTH-1"))
	if err != nil {
		t.Fatal(err)
	}
	return submission
}

func adviceGoodsLine(t *testing.T, code, unit string, quantity int64) domain.GoodsInboundLine {
	t.Helper()
	return adviceGoodsLineCounted(t, code, unit, quantity, "", "", 0, false)
}

func adviceGoodsLineCounted(t *testing.T, code, unit string, quantity int64, batch, expiry string, cartons int64, cartonsGiven bool) domain.GoodsInboundLine {
	t.Helper()
	var batchNumber domain.BatchNumber
	var expiryDate domain.ExpiryDate
	var err error
	if batch != "" {
		batchNumber, err = domain.NewBatchNumber(batch)
		if err != nil {
			t.Fatal(err)
		}
	}
	if expiry != "" {
		expiryDate, err = domain.NewExpiryDate(expiry)
		if err != nil {
			t.Fatal(err)
		}
	}
	var count domain.CartonCount
	if cartonsGiven {
		count = domain.GivenCartonCount(cartons)
	}
	line, err := domain.NewGoodsInboundLine(valueOf(t, domain.NewGoodsCode, code), valueOf(t, domain.NewGoodsUnitName, unit), quantity, batchNumber, expiryDate, count)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func adviceCarton(t *testing.T, number string, lines ...domain.CartonContentLine) domain.Carton {
	t.Helper()
	carton, err := domain.NewCarton(valueOf(t, domain.NewCartonNumber, number), lines)
	if err != nil {
		t.Fatal(err)
	}
	return carton
}

func adviceCartonLine(t *testing.T, code string, quantity int64) domain.CartonContentLine {
	t.Helper()
	line, err := domain.NewCartonContentLine(valueOf(t, domain.NewGoodsCode, code), quantity)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func mustAccept(t *testing.T, world preAdviceWorld, command application.AcceptPreAdviceCommand) application.AcceptPreAdviceResult {
	t.Helper()
	result, err := world.handler().Accept(t.Context(), command)
	if err != nil {
		t.Fatalf("受理 %s：%v", command.Submission.Key(), err)
	}
	return result
}

func mustUpdate(t *testing.T, world preAdviceWorld, command application.UpdatePreAdviceNoticeCommand) application.AcceptPreAdviceResult {
	t.Helper()
	result, err := world.handler().UpdateNotice(t.Context(), command)
	if err != nil {
		t.Fatalf("告知 %s：%v", command.Submission.Key(), err)
	}
	return result
}

func mustWithdraw(t *testing.T, world preAdviceWorld, command application.WithdrawPreAdviceCommand) application.AcceptPreAdviceResult {
	t.Helper()
	result, err := world.handler().Withdraw(t.Context(), command)
	if err != nil {
		t.Fatalf("撤回 %s：%v", command.Submission.Key(), err)
	}
	return result
}

func mustPreAdvice(t *testing.T, result application.AcceptPreAdviceResult) domain.PreAdvice {
	t.Helper()
	advice, has := result.PreAdvice()
	if !has {
		t.Fatal("结果里没有预报")
	}
	return advice
}

func mustStored(t *testing.T, world preAdviceWorld, id domain.PreAdviceID) domain.PreAdvice {
	t.Helper()
	advice, found, err := world.store.Find(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), id)
	if err != nil || !found {
		t.Fatalf("读回 %s：%v", id, err)
	}
	return advice
}

func mustNoted(t *testing.T, result application.NotePreAdviceReceiptResult) domain.PreAdvice {
	t.Helper()
	advice, has := result.PreAdvice()
	if !has {
		t.Fatal("收货缝没有回显预报")
	}
	return advice
}

func mustClosed(t *testing.T, result application.NotePreAdviceCloseResult) domain.PreAdvice {
	t.Helper()
	advice, has := result.PreAdvice()
	if !has {
		t.Fatal("完结缝没有回显预报")
	}
	return advice
}

func contentIssueTexts(issues []domain.PreAdviceContentIssue) []string {
	texts := make([]string, len(issues))
	for index, issue := range issues {
		texts[index] = issue.String()
	}
	return texts
}
