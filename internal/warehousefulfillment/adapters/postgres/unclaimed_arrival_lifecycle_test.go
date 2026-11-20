package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

type unclaimedLifecycleWorld struct {
	receiptWorld
	rules *adapter.UnclaimedDisposalRules
}

func newUnclaimedLifecycleWorld(t *testing.T) unclaimedLifecycleWorld {
	t.Helper()
	rw := newReceiptWorld(t)
	rules, err := adapter.NewUnclaimedDisposalRules(rw.db)
	if err != nil {
		t.Fatal(err)
	}
	return unclaimedLifecycleWorld{receiptWorld: rw, rules: rules}
}

func (world unclaimedLifecycleWorld) registerDirect(
	t *testing.T,
	at time.Time,
	command application.RegisterUnclaimedArrivalCommand,
) application.RegisterUnclaimedArrivalResult {
	t.Helper()
	handler := application.NewRegisterUnclaimedArrivalHandler(world.arrivals, world.arrivalIDs, world.warehouses, fixedClock{at: at})
	var result application.RegisterUnclaimedArrivalResult
	err := world.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var runErr error
		result, runErr = handler.Register(txCtx, command)
		return runErr
	})
	if err != nil {
		t.Fatalf("直接登记待认领：%v", err)
	}
	return result
}

func (world unclaimedLifecycleWorld) registerRule(
	t *testing.T,
	at time.Time,
	command application.RegisterDisposalRuleCommand,
) application.RegisterDisposalRuleResult {
	t.Helper()
	handler := application.NewRegisterDisposalRuleHandler(world.rules, world.warehouses, fixedClock{at: at})
	var result application.RegisterDisposalRuleResult
	err := world.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var runErr error
		result, runErr = handler.Register(txCtx, command)
		return runErr
	})
	if err != nil {
		t.Fatalf("登记处置规则：%v", err)
	}
	return result
}

func (world unclaimedLifecycleWorld) manager(at time.Time, owners ports.OwnerAccountLookup) *application.ManageUnclaimedArrivalHandler {
	return application.NewManageUnclaimedArrivalHandler(
		world.arrivals, world.rules, world.preAdvices, world.warehouses, owners, fixedClock{at: at},
	)
}

func (world unclaimedLifecycleWorld) claim(
	t *testing.T,
	at time.Time,
	owners ports.OwnerAccountLookup,
	command application.ClaimUnclaimedArrivalCommand,
) (application.ClaimUnclaimedArrivalResult, error) {
	t.Helper()
	var result application.ClaimUnclaimedArrivalResult
	err := world.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var runErr error
		result, runErr = world.manager(at, owners).Claim(txCtx, command)
		return runErr
	})
	return result, err
}

func (world unclaimedLifecycleWorld) revoke(
	t *testing.T,
	at time.Time,
	command application.RevokeUnclaimedClaimCommand,
) application.RevokeUnclaimedClaimResult {
	t.Helper()
	var result application.RevokeUnclaimedClaimResult
	err := world.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var runErr error
		result, runErr = world.manager(at, presentOwnerAccounts{}).RevokeClaim(txCtx, command)
		return runErr
	})
	if err != nil {
		t.Fatalf("撤销认领：%v", err)
	}
	return result
}

func (world unclaimedLifecycleWorld) dispose(
	t *testing.T,
	at time.Time,
	command application.DisposeUnclaimedArrivalCommand,
) application.DisposeUnclaimedArrivalResult {
	t.Helper()
	var result application.DisposeUnclaimedArrivalResult
	err := world.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var runErr error
		result, runErr = world.manager(at, presentOwnerAccounts{}).Dispose(txCtx, command)
		return runErr
	})
	if err != nil {
		t.Fatalf("处置待认领：%v", err)
	}
	return result
}

type fixedOwnerAccounts map[string]bool

func (m fixedOwnerAccounts) OwnerAccountRegistered(_ context.Context, _ domain.TenantID, owner domain.OwnerAccountID) (bool, error) {
	return m[owner.String()], nil
}

type failingOwnerAccounts struct{ err error }

func (f failingOwnerAccounts) OwnerAccountRegistered(context.Context, domain.TenantID, domain.OwnerAccountID) (bool, error) {
	return false, f.err
}

func directArrivalCmd(
	t *testing.T,
	factID, warehouse string,
	kind domain.UnclaimedArrivalKind,
	occurredAt time.Time,
	observation string,
) application.RegisterUnclaimedArrivalCommand {
	t.Helper()
	return application.RegisterUnclaimedArrivalCommand{
		Tenant:         valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		FactID:         valueOf(t, domain.NewUnclaimedArrivalFactID, factID),
		Device:         valueOf(t, domain.NewReceiptDeviceID, "SYN-DEV-WH-01"),
		Operator:       valueOf(t, domain.NewReceiptOperatorID, "https://id.syn.example/dex#SYN-OP-1"),
		OccurredAt:     occurredAt,
		DeviceSequence: 1,
		Warehouse:      valueOf(t, domain.NewWarehouseCode, warehouse),
		Kind:           kind,
		Observation:    valueOf(t, domain.NewUnclaimedArrivalObservation, observation),
	}
}

// Covers: AT-WF-055、AT-WF-066、AT-WF-067——作业事实登记待认领到货（入库到货与退件、重放、内容冲突、停用自营仓照记、
// 代理仓答 WAREHOUSE_NOT_ACCEPTING/AGENT、无仓答 WAREHOUSE_NOT_FOUND）；带货主与预报线索也不进货主预报的 unclaimedArrival 标记。
func TestUnclaimedArrivalDirectRegistrationReplayConflictAndWarehouseRules(t *testing.T) {
	world := newUnclaimedLifecycleWorld(t)
	now := baseInstant.Add(10 * time.Minute)

	// 无仓租户：直接登记答 WAREHOUSE_NOT_FOUND，不占事实身份；认领、撤销、处置答 NOT_CONFIGURED；查阅答空。
	noWh := world.registerDirect(t, now, directArrivalCmd(t, "SYN-UCA-F1", "SYN-WH-01", domain.UnclaimedArrivalKindInbound, now, "无主三箱"))
	if noWh.Outcome() != application.RegisterUnclaimedWarehouseNotFound {
		t.Fatalf("无仓登记 = %s", noWh.Outcome())
	}
	noWhClaim, err := world.claim(t, now, presentOwnerAccounts{}, application.ClaimUnclaimedArrivalCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), DecisionID: valueOf(t, domain.NewRequestKey, "DEC-NO-WH"),
		ArrivalID: valueOf(t, domain.NewUnclaimedArrivalID, "UCA-NONE"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		Operator: valueOf(t, domain.NewRegistrant, "op-1"),
	})
	if err != nil || noWhClaim.Outcome() != application.ClaimUnclaimedNotConfigured || noWhClaim.Missing() != "WAREHOUSE" {
		t.Fatalf("无仓认领 = %s %q (%v)", noWhClaim.Outcome(), noWhClaim.Missing(), err)
	}
	noWhRevoke := world.revoke(t, now, application.RevokeUnclaimedClaimCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), DecisionID: valueOf(t, domain.NewRequestKey, "DEC-REV-NO-WH"),
		ArrivalID: valueOf(t, domain.NewUnclaimedArrivalID, "UCA-NONE"), Operator: valueOf(t, domain.NewRegistrant, "op-1"),
	})
	if noWhRevoke.Outcome() != application.RevokeClaimNotConfigured {
		t.Fatalf("无仓撤销 = %s", noWhRevoke.Outcome())
	}
	noWhDispose := world.dispose(t, now, application.DisposeUnclaimedArrivalCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ArrivalID: valueOf(t, domain.NewUnclaimedArrivalID, "UCA-NONE"),
		Method: valueOf(t, domain.NewUnclaimedDisposalMethod, "DONATE"), Operator: valueOf(t, domain.NewRegistrant, "op-1"),
	})
	if noWhDispose.Outcome() != application.DisposeUnclaimedNotConfigured || noWhDispose.Missing() != "WAREHOUSE" {
		t.Fatalf("无仓处置 = %s %q", noWhDispose.Outcome(), noWhDispose.Missing())
	}

	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-CLUE-ONLY", "SYN-GOODS-01", "件", 10)
	// 代理仓答 WAREHOUSE_NOT_ACCEPTING/AGENT，不占事实身份。
	mustSave(t, world.transactor, world.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-AGENT", 1, func(spec *domain.WarehouseContentSpec) {
		spec.Kind = domain.WarehouseKindAgent
	}))
	agent := world.registerDirect(t, now, directArrivalCmd(t, "SYN-UCA-F1", "SYN-WH-AGENT", domain.UnclaimedArrivalKindInbound, now, "无主三箱"))
	if reason, ok := agent.NotAcceptingReason(); agent.Outcome() != application.RegisterUnclaimedWarehouseNotAccepting || !ok || reason != application.WarehouseNotAcceptingAgent {
		t.Fatalf("代理仓 = %s %s (%v)", agent.Outcome(), reason, ok)
	}

	// 登记一件入库到货，带货主与预报线索、存放处。
	cmd := directArrivalCmd(t, "SYN-UCA-F1", "SYN-WH-01", domain.UnclaimedArrivalKindInbound, now, "外箱条码模糊，三箱")
	cmd.OwnerClue = valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	cmd.PreAdviceClue = advice.ID()
	cmd.StorageLocation = valueOf(t, domain.NewUnclaimedArrivalStorageLocation, "A区暂存架3号")
	before := world.ledgerNow(t)
	first := world.registerDirect(t, now.Add(time.Second), cmd)
	arrival, ok := first.Arrival()
	if first.Outcome() != application.RegisterUnclaimedRegistered || !ok || arrival.Status() != domain.UnclaimedArrivalStatusUnclaimed ||
		arrival.Origin() != domain.UnclaimedArrivalOriginWorkFact || arrival.Kind() != domain.UnclaimedArrivalKindInbound {
		t.Fatalf("首登 = %s %+v", first.Outcome(), arrival)
	}
	before.sameAs(t, world.ledgerNow(t))

	// 线索不是认定：货主读那份预报时不得显示 unclaimedArrival（只有收货转来才标）。
	marked, err := world.arrivals.PreAdviceIDsWithUnclaimedArrival(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), []domain.PreAdviceID{advice.ID()})
	if err != nil || len(marked) != 0 {
		t.Fatalf("直接登记的预报线索不应出现在预报读口标记里：%v (%v)", marked, err)
	}

	// 原样重投答 EXISTING_RESULT，不另建；换观察答 CONTENT_CONFLICT，在册不变。
	replay := world.registerDirect(t, now.Add(time.Minute), cmd)
	replayArrival, _ := replay.Arrival()
	if replay.Outcome() != application.RegisterUnclaimedExistingResult || replayArrival.ID() != arrival.ID() {
		t.Fatalf("重投 = %s %s", replay.Outcome(), replayArrival.ID())
	}
	conflictCmd := cmd
	conflictCmd.Observation = valueOf(t, domain.NewUnclaimedArrivalObservation, "改了观察")
	if conflict := world.registerDirect(t, now.Add(2*time.Minute), conflictCmd); conflict.Outcome() != application.RegisterUnclaimedContentConflict {
		t.Fatalf("冲突 = %s", conflict.Outcome())
	}

	// 停用的自营仓照记退件。
	mustSave(t, world.transactor, world.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 2, func(spec *domain.WarehouseContentSpec) {
		spec.Deactivated = true
	}))
	retRes := world.registerDirect(t, now.Add(3*time.Minute), directArrivalCmd(t, "SYN-UCA-RET", "SYN-WH-01", domain.UnclaimedArrivalKindReturn, now, "退件外箱无单号"))
	retArrival, ok := retRes.Arrival()
	if retRes.Outcome() != application.RegisterUnclaimedRegistered || !ok || retArrival.Kind() != domain.UnclaimedArrivalKindReturn {
		t.Fatalf("停用自营仓退件 = %s %+v", retRes.Outcome(), retArrival)
	}
}

// Covers: AT-WF-057、AT-WF-058、AT-WF-059、AT-WF-060、AT-WF-061——认领（入库到货与退件）、重放、冲突、已有认领、
// 货主不可见/读不动、认领撤销、撤销后重认、收货确认引用已认领到货、引用待认领/错预报/已完结预报答 CONTENT_INVALID、
// 已有确认收货引用后撤销答 REVOCATION_NOT_ALLOWED。
func TestUnclaimedArrivalClaimRevokeAndReceiptReferenceLifecycle(t *testing.T) {
	world := newUnclaimedLifecycleWorld(t)
	world.seedReceiving(t)
	// 再登一个仓与一份预报，测跨仓与跨货主拒绝。
	mustSave(t, world.transactor, world.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-02", 1, nil))
	adviceA := world.adviseGoods(t, "REQ-CLAIM-A", "SYN-GOODS-01", "件", 20)
	adviceB := world.adviseGoods(t, "REQ-CLAIM-B", "SYN-GOODS-01", "件", 20)
	adviceClosed := world.adviseGoods(t, "REQ-CLAIM-CLOSED", "SYN-GOODS-01", "件", 10)
	world.withdrawAdvice(t, adviceClosed.ID(), "REQ-W-CLAIM")

	now := baseInstant.Add(15 * time.Minute)
	regRes := world.registerDirect(t, now, directArrivalCmd(t, "SYN-UCA-CLAIM-1", "SYN-WH-01", domain.UnclaimedArrivalKindInbound, now, "入库无主货"))
	inbound, _ := regRes.Arrival()
	retRes := world.registerDirect(t, now, directArrivalCmd(t, "SYN-UCA-CLAIM-RET", "SYN-WH-01", domain.UnclaimedArrivalKindReturn, now, "退件无主货"))
	retItem, _ := retRes.Arrival()

	// 1. 认领不成立（AT-WF-058）：入库到货没带预报、预报已取消、货主查不到、party-commercial 读不动；退件带了预报（AT-WF-061）。
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	op := valueOf(t, domain.NewRegistrant, "op-claim-1")
	ownerA := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	ownerB := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-02")
	owners := fixedOwnerAccounts{"SYN-OWNER-01": true, "SYN-OWNER-02": true}

	if res, err := world.claim(t, now, owners, application.ClaimUnclaimedArrivalCommand{
		Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, "DEC-BAD-1"), ArrivalID: inbound.ID(), Owner: ownerA, Operator: op,
	}); err != nil || res.Outcome() != application.ClaimUnclaimedClaimInvalid {
		t.Fatalf("入库缺预报 = %s (%v)", res.Outcome(), err)
	}
	if res, err := world.claim(t, now, owners, application.ClaimUnclaimedArrivalCommand{
		Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, "DEC-BAD-2"), ArrivalID: inbound.ID(), Owner: ownerB, PreAdvice: adviceA.ID(), Operator: op,
	}); err != nil || res.Outcome() != application.ClaimUnclaimedClaimInvalid {
		t.Fatalf("预报属另一货主 = %s (%v)", res.Outcome(), err)
	}
	if res, err := world.claim(t, now, owners, application.ClaimUnclaimedArrivalCommand{
		Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, "DEC-BAD-3"), ArrivalID: inbound.ID(), Owner: ownerA, PreAdvice: adviceClosed.ID(), Operator: op,
	}); err != nil || res.Outcome() != application.ClaimUnclaimedClaimInvalid {
		t.Fatalf("预报已取消 = %s (%v)", res.Outcome(), err)
	}
	if _, err := world.claim(t, now, owners, application.ClaimUnclaimedArrivalCommand{
		Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, "DEC-BAD-4"), ArrivalID: inbound.ID(), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-GHOST"), PreAdvice: adviceA.ID(), Operator: op,
	}); !errors.Is(err, application.ErrOwnerAccountNotInTenant) {
		t.Fatalf("货主查不到 = %v，want ErrOwnerAccountNotInTenant", err)
	}
	downErr := errors.New("party commercial unreachable")
	if _, err := world.claim(t, now, failingOwnerAccounts{err: downErr}, application.ClaimUnclaimedArrivalCommand{
		Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, "DEC-BAD-5"), ArrivalID: inbound.ID(), Owner: ownerA, PreAdvice: adviceA.ID(), Operator: op,
	}); !errors.Is(err, downErr) {
		t.Fatalf("货主读不动 = %v", err)
	}
	// 退件带预报答 CLAIM_INVALID；不带预报答 CLAIMED（AT-WF-061）。
	if res, err := world.claim(t, now, owners, application.ClaimUnclaimedArrivalCommand{
		Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, "DEC-RET-BAD"), ArrivalID: retItem.ID(), Owner: ownerA, PreAdvice: adviceA.ID(), Operator: op,
	}); err != nil || res.Outcome() != application.ClaimUnclaimedClaimInvalid {
		t.Fatalf("退件带预报 = %s (%v)", res.Outcome(), err)
	}
	if res, err := world.claim(t, now, owners, application.ClaimUnclaimedArrivalCommand{
		Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, "DEC-RET-OK"), ArrivalID: retItem.ID(), Owner: ownerA, Operator: op,
	}); err != nil || res.Outcome() != application.ClaimUnclaimedClaimed {
		t.Fatalf("退件认领 = %s (%v)", res.Outcome(), err)
	}

	// 收货确认引用还在待认领的到货答 CONTENT_INVALID（AT-WF-059）。
	refWhileUnclaimed := receiptCommand(t, "SYN-FACT-UNCLAIMED-REF", adviceA.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 5}))
	refWhileUnclaimed.UnclaimedArrival = inbound.ID()
	if r := world.confirm(t, refWhileUnclaimed); r.Outcome() != application.ConfirmReceiptContentInvalid {
		t.Fatalf("待认领时引用 = %s", r.Outcome())
	}

	// 对待认领的到货撤销答 STATUS_MISMATCH（AT-WF-060）。
	if r := world.revoke(t, now, application.RevokeUnclaimedClaimCommand{
		Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, "DEC-REV-EARLY"), ArrivalID: inbound.ID(), Operator: op,
	}); r.Outcome() != application.RevokeClaimStatusMismatch {
		t.Fatalf("未认领撤销 = %s", r.Outcome())
	}

	// 2. 正常认领入库到货、同决定重放、同决定换内容冲突、异决定同指名答 EXISTING_RESULT、异决定异指名答 ALREADY_CLAIMED（AT-WF-057）。
	before := world.ledgerNow(t)
	claim1Cmd := application.ClaimUnclaimedArrivalCommand{
		Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, "DEC-CLAIM-1"), ArrivalID: inbound.ID(), Owner: ownerA, PreAdvice: adviceA.ID(), Operator: op,
	}
	c1, err := world.claim(t, now, owners, claim1Cmd)
	if err != nil || c1.Outcome() != application.ClaimUnclaimedClaimed {
		t.Fatalf("首次认领 = %s (%v)", c1.Outcome(), err)
	}
	before.sameAs(t, world.ledgerNow(t))

	if replay, err := world.claim(t, now.Add(time.Second), owners, claim1Cmd); err != nil || replay.Outcome() != application.ClaimUnclaimedExistingResult {
		t.Fatalf("同决定重放 = %s (%v)", replay.Outcome(), err)
	}
	conflictClaim := claim1Cmd
	conflictClaim.PreAdvice = adviceB.ID()
	if conflict, err := world.claim(t, now.Add(2*time.Second), owners, conflictClaim); err != nil || conflict.Outcome() != application.ClaimUnclaimedSourceConflict {
		t.Fatalf("同决定换内容 = %s (%v)", conflict.Outcome(), err)
	}
	sameTargetDiffDec := claim1Cmd
	sameTargetDiffDec.DecisionID = valueOf(t, domain.NewRequestKey, "DEC-CLAIM-SAME")
	if sameRes, err := world.claim(t, now.Add(3*time.Second), owners, sameTargetDiffDec); err != nil || sameRes.Outcome() != application.ClaimUnclaimedExistingResult {
		t.Fatalf("异决定同指名 = %s (%v)", sameRes.Outcome(), err)
	}
	diffTarget := claim1Cmd
	diffTarget.DecisionID = valueOf(t, domain.NewRequestKey, "DEC-CLAIM-DIFF")
	diffTarget.PreAdvice = adviceB.ID()
	if already, err := world.claim(t, now.Add(4*time.Second), owners, diffTarget); err != nil || already.Outcome() != application.ClaimUnclaimedAlreadyClaimed {
		t.Fatalf("已有认领 = %s (%v)", already.Outcome(), err)
	}

	// 3. 撤销还没入账引用的认领、重投回显当初结果、再认给另一份预报 adviceB（AT-WF-060）。
	revoke1Cmd := application.RevokeUnclaimedClaimCommand{
		Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, "DEC-REV-1"), ArrivalID: inbound.ID(),
		Reason: valueOf(t, domain.NewUnclaimedArrivalNote, "指错预报"), Operator: op,
	}
	if rev := world.revoke(t, now.Add(5*time.Second), revoke1Cmd); rev.Outcome() != application.RevokeClaimRevoked {
		t.Fatalf("撤销 = %s", rev.Outcome())
	}
	// 撤销之后对原决定标识 DEC-CLAIM-1 重投，仍回显当初那一笔（不看到货此刻的状态）。
	if oldReplay, err := world.claim(t, now.Add(6*time.Second), owners, claim1Cmd); err != nil || oldReplay.Outcome() != application.ClaimUnclaimedExistingResult {
		t.Fatalf("撤销后原认领决定重投 = %s (%v)", oldReplay.Outcome(), err)
	}
	// 再认给 adviceB，然后重投撤销决定 DEC-REV-1 仍回显当初撤销结果。
	claim2Cmd := application.ClaimUnclaimedArrivalCommand{
		Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, "DEC-CLAIM-2"), ArrivalID: inbound.ID(), Owner: ownerA, PreAdvice: adviceB.ID(), Operator: op,
	}
	if c2, err := world.claim(t, now.Add(7*time.Second), owners, claim2Cmd); err != nil || c2.Outcome() != application.ClaimUnclaimedClaimed {
		t.Fatalf("再认领 = %s (%v)", c2.Outcome(), err)
	}
	if revReplay := world.revoke(t, now.Add(8*time.Second), revoke1Cmd); revReplay.Outcome() != application.RevokeClaimExistingResult {
		t.Fatalf("撤销重投 = %s", revReplay.Outcome())
	}

	// 4. 收货确认引用这件到货（AT-WF-059）：对着 adviceA（错预报）答 CONTENT_INVALID；对着 adviceB 成功入账并留下入账引用；
	// 之后撤销答 REVOCATION_NOT_ALLOWED。
	wrongAdviceReceipt := receiptCommand(t, "SYN-FACT-WRONG-ADV", adviceA.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 5}))
	wrongAdviceReceipt.UnclaimedArrival = inbound.ID()
	if r := world.confirm(t, wrongAdviceReceipt); r.Outcome() != application.ConfirmReceiptContentInvalid {
		t.Fatalf("认给 B 却收向 A = %s", r.Outcome())
	}
	rightReceipt := receiptCommand(t, "SYN-FACT-RIGHT-ADV", adviceB.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 5}))
	rightReceipt.UnclaimedArrival = inbound.ID()
	if r := world.confirm(t, rightReceipt); r.Outcome() != application.ConfirmReceiptConfirmed || world.stockOf(t, "SYN-GOODS-01") != 5 {
		t.Fatalf("引用已认领到货收货 = %s 库存 %d", r.Outcome(), world.stockOf(t, "SYN-GOODS-01"))
	}
	if blocked := world.revoke(t, now.Add(9*time.Second), application.RevokeUnclaimedClaimCommand{
		Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, "DEC-REV-BLOCKED"), ArrivalID: inbound.ID(), Operator: op,
	}); blocked.Outcome() != application.RevokeClaimNotAllowed || world.stockOf(t, "SYN-GOODS-01") != 5 {
		t.Fatalf("已有入账引用再撤销 = %s 库存 %d", blocked.Outcome(), world.stockOf(t, "SYN-GOODS-01"))
	}

	// 查详情：保留全部两笔认领（首笔带撤销记录）、入账引用 SYN-FACT-RIGHT-ADV（AT-WF-066）。
	detail, found, err := world.arrivals.LoadDetail(t.Context(), tenant, inbound.ID())
	if err != nil || !found || len(detail.Claims) != 2 || detail.Claims[0].Revocation == nil || detail.Claims[1].Revocation != nil ||
		len(detail.PostingReferences) != 1 || detail.PostingReferences[0].String() != "SYN-FACT-RIGHT-ADV" {
		t.Fatalf("详情 = %+v (%v, %v)", detail, found, err)
	}

	// 认领所指的预报被撤回之后，引用它的收货事实答 CONTENT_INVALID、不另建新到货，运营撤销后可重新认领（AT-WF-059 最后一种）。
	adviceToCancel := world.adviseGoods(t, "REQ-CLAIM-CANCEL-AFTER", "SYN-GOODS-01", "件", 10)
	regCancelRes := world.registerDirect(t, now, directArrivalCmd(t, "SYN-UCA-CANCEL-AFTER", "SYN-WH-01", domain.UnclaimedArrivalKindInbound, now, "预报将撤回的到货"))
	arrCancel, _ := regCancelRes.Arrival()
	if c, err := world.claim(t, now, owners, application.ClaimUnclaimedArrivalCommand{
		Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, "DEC-CLAIM-CA"), ArrivalID: arrCancel.ID(), Owner: ownerA, PreAdvice: adviceToCancel.ID(), Operator: op,
	}); err != nil || c.Outcome() != application.ClaimUnclaimedClaimed {
		t.Fatalf("认领 = %s (%v)", c.Outcome(), err)
	}
	world.withdrawAdvice(t, adviceToCancel.ID(), "REQ-W-CA")
	countBefore := world.unclaimedCount(t)
	afterCancelReceipt := receiptCommand(t, "SYN-FACT-CA", adviceToCancel.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 2}))
	afterCancelReceipt.UnclaimedArrival = arrCancel.ID()
	if r := world.confirm(t, afterCancelReceipt); r.Outcome() != application.ConfirmReceiptContentInvalid || world.unclaimedCount(t) != countBefore {
		t.Fatalf("预报已取消且引用了到货 = %s 到货数 %d/%d", r.Outcome(), world.unclaimedCount(t), countBefore)
	}
	if rev := world.revoke(t, now.Add(time.Minute), application.RevokeUnclaimedClaimCommand{
		Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, "DEC-REV-CA"), ArrivalID: arrCancel.ID(), Operator: op,
	}); rev.Outcome() != application.RevokeClaimRevoked {
		t.Fatalf("撤销后重认准备 = %s", rev.Outcome())
	}
}

// Covers: AT-WF-062、AT-WF-063、AT-WF-064、AT-WF-065——处置规则修订（重放、冲突、错位、无仓、内容不合、新修订）、
// 没规则答 NOT_CONFIGURED 且不借用别的仓、认领期限按到货发生时刻经过时间算（无业务时区照判；收货转来取原事实发生时刻）、
// 期限未到/方式不在规则答 DISPOSITION_NOT_ALLOWED、已处置同方式重放答 EXISTING_RESULT、换方式答 STATUS_MISMATCH 并回显处置。
func TestUnclaimedDisposalRuleAndDispositionLifecycle(t *testing.T) {
	world := newUnclaimedLifecycleWorld(t)
	world.seedReceiving(t)
	// 登一个不带业务时区的仓 SYN-WH-NO-TZ，证 AT-WF-065 不依赖业务时区。
	mustSave(t, world.transactor, world.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-NO-TZ", 1, func(spec *domain.WarehouseContentSpec) {
		spec.BusinessTimeZone = domain.BusinessTimeZone{}
	}))

	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	op := valueOf(t, domain.NewRegistrant, "op-rule-1")
	whNoTZ := valueOf(t, domain.NewWarehouseCode, "SYN-WH-NO-TZ")
	wh1 := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	now := baseInstant.Add(time.Hour)

	// 1. 处置规则登记全路径（AT-WF-062）。
	if r := world.registerRule(t, now, application.RegisterDisposalRuleCommand{
		Tenant: tenant, Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-MISSING"), Revision: 1,
		Content: domain.NewDisposalRuleContent(3600, []domain.UnclaimedDisposalMethod{valueOf(t, domain.NewUnclaimedDisposalMethod, "DONATE")}), Registrant: op,
	}); r.Outcome() != application.RegistrationNotAccepted || r.Refusal() != application.RegistrationRefusalReferenceNotFound {
		t.Fatalf("无仓 = %s/%s", r.Outcome(), r.Refusal())
	}
	if r := world.registerRule(t, now, application.RegisterDisposalRuleCommand{
		Tenant: tenant, Warehouse: whNoTZ, Revision: 2,
		Content: domain.NewDisposalRuleContent(3600, []domain.UnclaimedDisposalMethod{valueOf(t, domain.NewUnclaimedDisposalMethod, "DONATE")}), Registrant: op,
	}); r.Outcome() != application.RegistrationNotAccepted || r.Refusal() != application.RegistrationRefusalRevisionOutOfSequence {
		t.Fatalf("跳号 = %s/%s", r.Outcome(), r.Refusal())
	}
	if r := world.registerRule(t, now, application.RegisterDisposalRuleCommand{
		Tenant: tenant, Warehouse: whNoTZ, Revision: 1,
		Content: domain.NewDisposalRuleContent(0, []domain.UnclaimedDisposalMethod{valueOf(t, domain.NewUnclaimedDisposalMethod, "DONATE")}), Registrant: op,
	}); r.Outcome() != application.RegistrationNotAccepted || r.Refusal() != application.RegistrationRefusalContentInvalid || r.Field() != domain.DisposalRuleFieldClaimWindowSeconds {
		t.Fatalf("期限为零 = %s/%s/%s", r.Outcome(), r.Refusal(), r.Field())
	}
	if r := world.registerRule(t, now, application.RegisterDisposalRuleCommand{
		Tenant: tenant, Warehouse: whNoTZ, Revision: 1,
		Content: domain.NewDisposalRuleContent(math.MaxInt64/int64(time.Second)+1, []domain.UnclaimedDisposalMethod{valueOf(t, domain.NewUnclaimedDisposalMethod, "DONATE")}), Registrant: op,
	}); r.Outcome() != application.RegistrationNotAccepted || r.Refusal() != application.RegistrationRefusalContentInvalid || r.Field() != domain.DisposalRuleFieldClaimWindowSeconds {
		t.Fatalf("期限大到会回绕 = %s/%s/%s", r.Outcome(), r.Refusal(), r.Field())
	}
	if r := world.registerRule(t, now, application.RegisterDisposalRuleCommand{
		Tenant: tenant, Warehouse: whNoTZ, Revision: 1,
		Content: domain.NewDisposalRuleContent(3600, nil), Registrant: op,
	}); r.Outcome() != application.RegistrationNotAccepted || r.Refusal() != application.RegistrationRefusalContentInvalid || r.Field() != domain.DisposalRuleFieldDisposalMethods {
		t.Fatalf("空方式 = %s/%s/%s", r.Outcome(), r.Refusal(), r.Field())
	}

	// 只给 SYN-WH-01 登规则，不给 SYN-WH-NO-TZ 登：SYN-WH-NO-TZ 处置答 NOT_CONFIGURED，不借用 SYN-WH-01（AT-WF-064）。
	ruleWh1 := application.RegisterDisposalRuleCommand{
		Tenant: tenant, Warehouse: wh1, Revision: 1,
		Content: domain.NewDisposalRuleContent(3600, []domain.UnclaimedDisposalMethod{
			valueOf(t, domain.NewUnclaimedDisposalMethod, "DONATE"),
			valueOf(t, domain.NewUnclaimedDisposalMethod, "DESTROY"),
		}), Registrant: op,
	}
	if r := world.registerRule(t, now, ruleWh1); r.Outcome() != application.RegistrationRegistered {
		t.Fatalf("登规则 = %s", r.Outcome())
	}
	if r := world.registerRule(t, now, ruleWh1); r.Outcome() != application.RegistrationAlreadyRegistered {
		t.Fatalf("规则重放 = %s", r.Outcome())
	}
	conflictRule := ruleWh1
	conflictRule.Content = domain.NewDisposalRuleContent(7200, ruleWh1.Content.Methods())
	if r := world.registerRule(t, now, conflictRule); r.Outcome() != application.RegistrationContentConflict {
		t.Fatalf("规则冲突 = %s", r.Outcome())
	}

	// 在无时区仓登记一件到货：此时该仓无规则答 NOT_CONFIGURED；登完规则后期限未到答 CLAIM_WINDOW_NOT_ELAPSED；
	// 期限到达后不在规则的方式答 METHOD_NOT_IN_RULE；在规则内答 DISPOSED。
	occurred := baseInstant
	regNoTZ := world.registerDirect(t, occurred.Add(10*time.Minute), directArrivalCmd(t, "SYN-UCA-DISP-1", "SYN-WH-NO-TZ", domain.UnclaimedArrivalKindReturn, occurred, "无时区仓待认领"))
	arrNoTZ, _ := regNoTZ.Arrival()

	if r := world.dispose(t, occurred.Add(2*time.Hour), application.DisposeUnclaimedArrivalCommand{
		Tenant: tenant, ArrivalID: arrNoTZ.ID(), Method: valueOf(t, domain.NewUnclaimedDisposalMethod, "DONATE"), Operator: op,
	}); r.Outcome() != application.DisposeUnclaimedNotConfigured || r.Missing() != "DISPOSAL_RULE" {
		t.Fatalf("无规则处置 = %s %q", r.Outcome(), r.Missing())
	}
	if revs, err := world.rules.ListDisposalRuleRevisions(t.Context(), tenant, whNoTZ); err != nil || len(revs) != 0 {
		t.Fatalf("未登规则读口 = %+v (%v)", revs, err)
	}

	if r := world.registerRule(t, now, application.RegisterDisposalRuleCommand{
		Tenant: tenant, Warehouse: whNoTZ, Revision: 1,
		Content: domain.NewDisposalRuleContent(3600, []domain.UnclaimedDisposalMethod{
			valueOf(t, domain.NewUnclaimedDisposalMethod, "DONATE"),
			valueOf(t, domain.NewUnclaimedDisposalMethod, "AUCTION"),
		}), Registrant: op,
	}); r.Outcome() != application.RegistrationRegistered {
		t.Fatalf("无时区仓登规则 = %s", r.Outcome())
	}

	// 期限未到（occurred + 3599s < occurred + 3600s）。
	earlyDisp := world.dispose(t, occurred.Add(3599*time.Second), application.DisposeUnclaimedArrivalCommand{
		Tenant: tenant, ArrivalID: arrNoTZ.ID(), Method: valueOf(t, domain.NewUnclaimedDisposalMethod, "DONATE"), Operator: op,
	})
	if reason, ok := earlyDisp.Reason(); earlyDisp.Outcome() != application.DisposeUnclaimedNotAllowed || !ok || reason != application.DispositionClaimWindowNotElapsed {
		t.Fatalf("期限未到 = %s %s", earlyDisp.Outcome(), reason)
	}
	// 方式不在规则里（区分大小写逐字比）。
	wrongMethodDisp := world.dispose(t, occurred.Add(3600*time.Second), application.DisposeUnclaimedArrivalCommand{
		Tenant: tenant, ArrivalID: arrNoTZ.ID(), Method: valueOf(t, domain.NewUnclaimedDisposalMethod, "donate"), Operator: op,
	})
	if reason, ok := wrongMethodDisp.Reason(); wrongMethodDisp.Outcome() != application.DisposeUnclaimedNotAllowed || !ok || reason != application.DispositionMethodNotInRule {
		t.Fatalf("方式不在规则 = %s %s", wrongMethodDisp.Outcome(), reason)
	}

	before := world.ledgerNow(t)
	disposed := world.dispose(t, occurred.Add(3600*time.Second), application.DisposeUnclaimedArrivalCommand{
		Tenant: tenant, ArrivalID: arrNoTZ.ID(), Method: valueOf(t, domain.NewUnclaimedDisposalMethod, "DONATE"),
		Note: valueOf(t, domain.NewUnclaimedArrivalNote, "满一小时无人认领"), Operator: op,
	})
	disp, has := disposed.Disposition()
	if disposed.Outcome() != application.DisposeUnclaimedDisposed || !has || disp.RuleRevision() != 1 {
		t.Fatalf("处置 = %s %+v", disposed.Outcome(), disp)
	}
	before.sameAs(t, world.ledgerNow(t))

	// 同方式再处置答 EXISTING_RESULT；换方式答 STATUS_MISMATCH 并回显在册处置；已处置再认领答 STATUS_MISMATCH（AT-WF-063）。
	if replay := world.dispose(t, occurred.Add(4000*time.Second), application.DisposeUnclaimedArrivalCommand{
		Tenant: tenant, ArrivalID: arrNoTZ.ID(), Method: valueOf(t, domain.NewUnclaimedDisposalMethod, "DONATE"), Operator: op,
	}); replay.Outcome() != application.DisposeUnclaimedExistingResult {
		t.Fatalf("同方式再处置 = %s", replay.Outcome())
	}
	diffMethod := world.dispose(t, occurred.Add(4000*time.Second), application.DisposeUnclaimedArrivalCommand{
		Tenant: tenant, ArrivalID: arrNoTZ.ID(), Method: valueOf(t, domain.NewUnclaimedDisposalMethod, "AUCTION"), Operator: op,
	})
	if echoed, ok := diffMethod.Disposition(); diffMethod.Outcome() != application.DisposeUnclaimedStatusMismatch || !ok || echoed.Method().String() != "DONATE" {
		t.Fatalf("换方式 = %s %+v", diffMethod.Outcome(), echoed)
	}
	if claimAfterDispose, err := world.claim(t, occurred.Add(4000*time.Second), presentOwnerAccounts{}, application.ClaimUnclaimedArrivalCommand{
		Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, "DEC-AFTER-DISP"), ArrivalID: arrNoTZ.ID(),
		Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), Operator: op,
	}); err != nil || claimAfterDispose.Outcome() != application.ClaimUnclaimedStatusMismatch {
		t.Fatalf("已处置再认领 = %s (%v)", claimAfterDispose.Outcome(), err)
	}

	// 新修订改长认领期限不影响已作出的处置，但约束此后的处置（AT-WF-062）。
	if r := world.registerRule(t, occurred.Add(4100*time.Second), application.RegisterDisposalRuleCommand{
		Tenant: tenant, Warehouse: whNoTZ, Revision: 2,
		Content: domain.NewDisposalRuleContent(86400, []domain.UnclaimedDisposalMethod{
			valueOf(t, domain.NewUnclaimedDisposalMethod, "DONATE"),
		}), Registrant: op,
	}); r.Outcome() != application.RegistrationRegistered {
		t.Fatalf("规则修订 2 = %s", r.Outcome())
	}

	// 收货事实转来的待认领：认领期限从原收货事实的 OccurredAt 起算（AT-WF-056、AT-WF-065）。
	advice := world.adviseGoods(t, "REQ-DIVERT-DISP", "SYN-GOODS-01", "件", 10)
	world.withdrawAdvice(t, advice.ID(), "REQ-W-DIVERT-DISP")
	lateCmd := lateGoodsCommand(t, "SYN-FACT-DIVERT-DISP", advice.ID().String(), unclaimedScan, unclaimedQuantity)
	if r := world.confirm(t, lateCmd); r.Outcome() != application.ConfirmReceiptDivertedToUnclaimed {
		t.Fatalf("转待认领 = %s", r.Outcome())
	}
	diverted, found, err := world.arrivals.LoadByFact(t.Context(), tenant, valueOf(t, domain.NewUnclaimedArrivalFactID, "SYN-FACT-DIVERT-DISP"))
	if err != nil || !found || diverted.Origin() != domain.UnclaimedArrivalOriginDivertedReceipt {
		t.Fatalf("读回转来待认领 = %+v (%v, %v)", diverted, found, err)
	}
	// lateCmd.OccurredAt 是 preAdviceNumberingInstant - 2h，规则 1 期限是 1h，在 preAdviceNumberingInstant 处置应通过。
	if r := world.dispose(t, preAdviceNumberingInstant, application.DisposeUnclaimedArrivalCommand{
		Tenant: tenant, ArrivalID: diverted.ID(),
		Method: valueOf(t, domain.NewUnclaimedDisposalMethod, "DESTROY"), Operator: op,
	}); r.Outcome() != application.DisposeUnclaimedDisposed {
		t.Fatalf("收货转来按原发生时刻起算 = %s", r.Outcome())
	}

	// 列表筛选查阅（AT-WF-066）：按仓、种类、状态筛。
	q, err := ports.UnclaimedArrivalPages.Decode(url.Values{"warehouseCode": {"SYN-WH-NO-TZ"}, "status": {"DISPOSED"}})
	if err != nil {
		t.Fatal(err)
	}
	page, err := world.arrivals.ListUnclaimedArrivals(t.Context(), tenant, 10, q, time.Time{}, time.Time{})
	if err != nil || page.Total != 1 || len(page.Rows) != 1 || page.Rows[0].Arrival.ID() != arrNoTZ.ID() {
		t.Fatalf("筛选列表 = %+v (%v)", page, err)
	}
}

// Covers: UC-WF-005 并发——同一件到货上两个不同认领并发，恰一个 CLAIMED，另一个 ALREADY_CLAIMED。
func TestUnclaimedArrivalConcurrentClaimsSerializeOnTheArrival(t *testing.T) {
	world := newUnclaimedLifecycleWorld(t)
	world.seedReceiving(t)
	advice1 := world.adviseGoods(t, "REQ-CONC-1", "SYN-GOODS-01", "件", 10)
	advice2 := world.adviseGoods(t, "REQ-CONC-2", "SYN-GOODS-01", "件", 10)
	now := baseInstant.Add(20 * time.Minute)
	reg := world.registerDirect(t, now, directArrivalCmd(t, "SYN-UCA-CONC", "SYN-WH-01", domain.UnclaimedArrivalKindInbound, now, "并发争认"))
	arrival, _ := reg.Arrival()

	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	owner := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	op := valueOf(t, domain.NewRegistrant, "op-conc")
	start := make(chan struct{})
	var wg sync.WaitGroup
	outcomes := make([]application.ClaimUnclaimedArrivalOutcome, 2)
	errs := make([]error, 2)
	advices := []domain.PreAdviceID{advice1.ID(), advice2.ID()}
	decisions := []string{"DEC-CONC-A", "DEC-CONC-B"}

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			res, err := world.claim(t, now.Add(time.Second), presentOwnerAccounts{}, application.ClaimUnclaimedArrivalCommand{
				Tenant: tenant, DecisionID: valueOf(t, domain.NewRequestKey, decisions[idx]),
				ArrivalID: arrival.ID(), Owner: owner, PreAdvice: advices[idx], Operator: op,
			})
			outcomes[idx] = res.Outcome()
			errs[idx] = err
		}(i)
	}
	close(start)
	wg.Wait()

	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("并发认领报错：%v %v", errs[0], errs[1])
	}
	claimed, already := 0, 0
	for _, o := range outcomes {
		switch o {
		case application.ClaimUnclaimedClaimed:
			claimed++
		case application.ClaimUnclaimedAlreadyClaimed:
			already++
		}
	}
	if claimed != 1 || already != 1 {
		t.Fatalf("并发认领结果 = %v，want 1 CLAIMED + 1 ALREADY_CLAIMED", outcomes)
	}
}

// Covers: 无事务负向证据门禁——待认领到货与处置规则的全部加锁/写入方法在无环境事务时返回 ErrTransactionRequired。
func TestUnclaimedArrivalsAndDisposalRulesRefuseToRunOutsideATransaction(t *testing.T) {
	world := newUnclaimedLifecycleWorld(t)
	ctx := context.Background()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	arrivals, err := adapter.NewUnclaimedArrivals(world.db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := adapter.NewUnclaimedDisposalRules(world.db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := arrivals.Save(ctx, domain.UnclaimedArrival{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Save：%v", err)
	}
	if err := arrivals.HoldFact(ctx, tenant, valueOf(t, domain.NewUnclaimedArrivalFactID, "F1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldFact：%v", err)
	}
	if _, err := arrivals.SaveDirect(ctx, domain.RecordedUnclaimedArrival{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveDirect：%v", err)
	}
	if err := arrivals.HoldArrival(ctx, tenant, valueOf(t, domain.NewUnclaimedArrivalID, "UCA-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldArrival：%v", err)
	}
	if err := arrivals.HoldClaimDecision(ctx, tenant, valueOf(t, domain.NewRequestKey, "DEC-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldClaimDecision：%v", err)
	}
	if err := arrivals.HoldRevokeDecision(ctx, tenant, valueOf(t, domain.NewRequestKey, "DEC-2")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldRevokeDecision：%v", err)
	}
	if err := arrivals.RecordClaim(ctx, domain.UnclaimedArrivalClaim{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("RecordClaim：%v", err)
	}
	if err := arrivals.RecordRevocation(ctx, domain.UnclaimedArrivalClaimRevocation{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("RecordRevocation：%v", err)
	}
	if err := arrivals.RecordDisposition(ctx, domain.UnclaimedArrivalDisposition{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("RecordDisposition：%v", err)
	}
	if _, err := rules.SaveDisposalRuleRevision(ctx, domain.DisposalRuleRevision{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveDisposalRuleRevision：%v", err)
	}
}

// Covers: HTTP 认领口查无货主答 404 OWNER_ACCOUNT_NOT_VISIBLE、直接登记口坏种类答 400。
func TestUnclaimedArrivalHTTPStatusesAndBadPayloads(t *testing.T) {
	world := newUnclaimedLifecycleWorld(t)
	world.seedReceiving(t)
	now := baseInstant.Add(time.Hour)

	claimAuth := fakeUnclaimedClaimAuth{tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), op: valueOf(t, domain.NewRegistrant, "op-http")}
	claimIntake, err := warehousehttp.NewOperatorUnclaimedClaimIntake(claimAuth)
	if err != nil {
		t.Fatal(err)
	}
	claimEndpoint := warehousehttp.NewClaimUnclaimedArrivalEndpoint(claimIntake, txUnclaimedManager{
		transactor: world.transactor,
		inner:      world.manager(now, fixedOwnerAccounts{}),
	})
	req := httptest.NewRequest(http.MethodPost, "/warehouse-fulfillment-unclaimed-arrival-claims",
		strings.NewReader(`{"decisionId":"DEC-HTTP-1","arrivalId":"UCA-1","ownerAccount":"SYN-GHOST"}`))
	req.Header.Set("Authorization", "Bearer presented")
	res := httptest.NewRecorder()
	claimEndpoint.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound || !strings.Contains(res.Body.String(), "OWNER_ACCOUNT_NOT_VISIBLE") {
		t.Fatalf("货主不可见 = %d %s", res.Code, res.Body.String())
	}

	wfAuth := fakeUnclaimedWorkFactAuth{tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01")}
	wfIntake, err := warehousehttp.NewUnclaimedArrivalWorkFactIntake(wfAuth)
	if err != nil {
		t.Fatal(err)
	}
	regEndpoint := warehousehttp.NewRegisterUnclaimedArrivalEndpoint(wfIntake, txUnclaimedRegistrar{
		transactor: world.transactor,
		inner:      application.NewRegisterUnclaimedArrivalHandler(world.arrivals, world.arrivalIDs, world.warehouses, fixedClock{at: now}),
	})
	badReq := httptest.NewRequest(http.MethodPost, "/warehouse-fulfillment-unclaimed-arrival-registrations",
		strings.NewReader(`{"factId":"F-BAD","occurredAt":"2026-11-20T01:00:00Z","deviceSequence":1,"warehouse":"SYN-WH-01","kind":"UNKNOWN","observation":"坏种类"}`))
	badReq.Header.Set("Parcel-Work-Scope-Kind", "WAREHOUSE")
	badReq.Header.Set("Parcel-Work-Scope-Reference", "SYN-WH-01")
	badReq.Header.Set("Parcel-Device-Id", "SYN-DEV-1")
	badReq.Header.Set("Authorization", "Bearer presented")
	badRes := httptest.NewRecorder()
	regEndpoint.ServeHTTP(badRes, badReq)
	if badRes.Code != http.StatusBadRequest {
		t.Fatalf("坏种类 = %d %s", badRes.Code, badRes.Body.String())
	}
	var problem map[string]any
	if err := json.Unmarshal(badRes.Body.Bytes(), &problem); err != nil || problem["code"] != "MALFORMED_REQUEST" {
		t.Fatalf("问题体 = %s (%v)", badRes.Body.String(), err)
	}
}

type fakeUnclaimedClaimAuth struct {
	tenant domain.TenantID
	op     domain.Registrant
}

func (a fakeUnclaimedClaimAuth) AuthenticateUnclaimedArrivalClaim(context.Context, string) (warehousehttp.OperatorIdentity, error) {
	return warehousehttp.OperatorIdentity{Tenant: a.tenant, Registrant: a.op}, nil
}

type fakeUnclaimedWorkFactAuth struct{ tenant domain.TenantID }

func (a fakeUnclaimedWorkFactAuth) AuthenticateUnclaimedArrival(_ context.Context, _, deviceID, _, _ string) (domain.TenantID, domain.ReceiptOperatorID, domain.ReceiptDeviceID, error) {
	op, _ := domain.NewReceiptOperatorID("https://id.syn.example/dex#op-1")
	dev, _ := domain.NewReceiptDeviceID(deviceID)
	return a.tenant, op, dev, nil
}

type txUnclaimedManager struct {
	transactor bentoapp.Transactor
	inner      *application.ManageUnclaimedArrivalHandler
}

func (m txUnclaimedManager) Claim(ctx context.Context, command application.ClaimUnclaimedArrivalCommand) (application.ClaimUnclaimedArrivalResult, error) {
	var res application.ClaimUnclaimedArrivalResult
	err := m.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var runErr error
		res, runErr = m.inner.Claim(txCtx, command)
		return runErr
	})
	return res, err
}

func (m txUnclaimedManager) RevokeClaim(ctx context.Context, command application.RevokeUnclaimedClaimCommand) (application.RevokeUnclaimedClaimResult, error) {
	var res application.RevokeUnclaimedClaimResult
	err := m.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var runErr error
		res, runErr = m.inner.RevokeClaim(txCtx, command)
		return runErr
	})
	return res, err
}

func (m txUnclaimedManager) Dispose(ctx context.Context, command application.DisposeUnclaimedArrivalCommand) (application.DisposeUnclaimedArrivalResult, error) {
	var res application.DisposeUnclaimedArrivalResult
	err := m.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var runErr error
		res, runErr = m.inner.Dispose(txCtx, command)
		return runErr
	})
	return res, err
}

type txUnclaimedRegistrar struct {
	transactor bentoapp.Transactor
	inner      *application.RegisterUnclaimedArrivalHandler
}

// Covers: 来源形状与认领期限的库上门。转出来源把事实标识留空时，NULL 比较是 UNKNOWN，CHECK 会放行；
// 认领期限大到乘进 time.Duration 会回绕时，直插也须被拒。两条都不走应用层。
func TestUnclaimedArrivalShapeAndClaimWindowRejectDirectInserts(t *testing.T) {
	world := newUnclaimedLifecycleWorld(t)
	ctx := t.Context()
	pgCode := func(err error) string {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return pgErr.Code
		}
		return ""
	}

	_, err := world.pool.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.unclaimed_arrival
		    (tenant_id, arrival_id, fact_id, origin, kind, warehouse_code, device_id, operator_id,
		     occurred_at, device_sequence, received_at, observation, status)
		 VALUES ('SYN-TENANT-01', 'ARR-WORK', 'FACT-WORK', 'WORK_FACT', 'RETURN', 'SYN-WH-01', 'DEV-1', 'OP-1',
		         now(), 1, now(), '一线观察', 'UNCLAIMED')`)
	if err != nil {
		t.Fatalf("作业事实来源应能插入：%v", err)
	}

	_, err = world.pool.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.unclaimed_arrival
		    (tenant_id, arrival_id, fact_id, pre_advice_id, owner_account_id, origin, kind, warehouse_code,
		     device_id, operator_id, occurred_at, received_at, status)
		 VALUES ('SYN-TENANT-01', 'ARR-DIVERT', 'FACT-DIVERT', 'ADV-1', 'SYN-OWNER-01', 'DIVERTED_RECEIPT', 'INBOUND',
		         'SYN-WH-01', 'DEV-1', 'OP-1', now(), now(), 'UNCLAIMED')`)
	if pgCode(err) != "23514" {
		t.Fatalf("转出来源缺事实标识应被 CHECK 拒（23514），实得 %v", err)
	}

	_, err = world.pool.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.unclaimed_disposal_rule_revision
		    (tenant_id, warehouse_code, revision, claim_window_seconds, registrant, registered_at)
		 VALUES ('SYN-TENANT-01', 'SYN-WH-01', 1, 9223372036, 'op-1', now())`)
	if err != nil {
		t.Fatalf("最大可表示整秒应能插入：%v", err)
	}
	_, err = world.pool.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.unclaimed_disposal_rule_revision
		    (tenant_id, warehouse_code, revision, claim_window_seconds, registrant, registered_at)
		 VALUES ('SYN-TENANT-01', 'SYN-WH-01', 2, 9223372037, 'op-1', now())`)
	if pgCode(err) != "23514" {
		t.Fatalf("再大一秒应被 CHECK 拒（23514），实得 %v", err)
	}
}

func (r txUnclaimedRegistrar) Register(ctx context.Context, command application.RegisterUnclaimedArrivalCommand) (application.RegisterUnclaimedArrivalResult, error) {
	var res application.RegisterUnclaimedArrivalResult
	err := r.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var runErr error
		res, runErr = r.inner.Register(txCtx, command)
		return runErr
	})
	return res, err
}
