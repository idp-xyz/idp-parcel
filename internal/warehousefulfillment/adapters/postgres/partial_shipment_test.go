package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func (world receiptWorld) registerShortageRules(t *testing.T, handler *application.OutboundHandler, revision int, kind domain.ShortagePolicyKind, partial domain.PartialShipment) application.OutboundResult {
	t.Helper()
	var result application.OutboundResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var registerErr error
		result, registerErr = handler.RegisterPolicy(ctx, application.RegisterShortagePolicyCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
			Revision: revision, Kind: kind, PartialShipment: partial, Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
		})
		return registerErr
	})
	if err != nil {
		t.Fatalf("缺货处置：%v", err)
	}
	return result
}

func (world receiptWorld) recordConsent(t *testing.T, handler *application.OutboundHandler, request domain.OutboundRequestID, rule domain.PartialShipment, basis string) application.OutboundResult {
	t.Helper()
	var result application.OutboundResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var recordErr error
		result, recordErr = handler.RecordConsent(ctx, application.RecordPartialShipmentConsentCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Request: request, Rule: rule, Basis: basis,
			Operator: valueOf(t, domain.NewRegistrant, "operator-2"),
		})
		return recordErr
	})
	if err != nil {
		t.Fatalf("代录部分发货同意：%v", err)
	}
	return result
}

func (world receiptWorld) partialShipmentOf(t *testing.T, request domain.OutboundRequestID) ports.PartialShipmentDetail {
	t.Helper()
	reads, err := adapter.NewWorkspaceReads(world.db)
	if err != nil {
		t.Fatal(err)
	}
	detail, found, err := reads.OutboundRequestDetail(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), request.String())
	if err != nil || !found {
		t.Fatalf("读出库请求 %s：found %v err %v", request, found, err)
	}
	return detail.PartialShipment
}

// Covers: 请求提交时带的部分发货规则落在请求上，是提交内容的一部分——同一个请求键换了规则再交是内容冲突。
// 没带的就是没写明，不替货主填一个。
func TestAnOutboundRequestKeepsThePartialShipmentItWasSubmittedWith(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	outbound := world.outbound(t)
	command := outboundCommand(t, "REQ-PS-ORDER", 4)
	command.PartialShipment = domain.PartialShipmentShortShip
	accepted := world.acceptOutbound(t, outbound, command)
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok || request.PartialShipment() != domain.PartialShipmentShortShip {
		t.Fatalf("受理 %s，请求上的规则 %s", accepted.Outcome(), request.PartialShipment())
	}
	if again := world.acceptOutbound(t, outbound, command); again.Outcome() != application.OutboundExisting {
		t.Fatalf("原样重交 = %s", again.Outcome())
	}
	command.PartialShipment = domain.PartialShipmentSplitShip
	if changed := world.acceptOutbound(t, outbound, command); changed.Outcome() != application.OutboundConflict {
		t.Fatalf("同一个请求键换了规则 = %s", changed.Outcome())
	}
	if got := world.partialShipmentOf(t, request.ID()); got.Rule != "SHORT_SHIP" || got.Source != "ORDER" || got.Order != "SHORT_SHIP" || got.Owner != "" || got.Consent != "" {
		t.Fatalf("读出来的规则 %+v", got)
	}

	plain := world.acceptOutbound(t, outbound, outboundCommand(t, "REQ-PS-PLAIN", 2))
	unset, _ := plain.Request()
	if unset.PartialShipment().Given() {
		t.Fatalf("没带规则的请求上有了 %s", unset.PartialShipment())
	}
	if got := world.partialShipmentOf(t, unset.ID()); got.Rule != "" || got.Source != "" {
		t.Fatalf("三层都没写明时读出了 %+v", got)
	}
}

// Covers: 缺货处置的一份修订说两件事，各说各的。只说拣不齐时怎么发、没说受理时不够怎么办的，受理时不够仍停在待定；
// 两件都没说的不收。货主的默认是请求没写明时用的那一层。
func TestAShortagePolicyStatesPartialShipmentIndependentlyOfItsKind(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 5)
	outbound := world.outbound(t)
	if got := world.registerShortageRules(t, outbound, 1, domain.ShortagePolicyInvalid, domain.PartialShipmentUnset).Outcome(); got != application.OutboundContentInvalid {
		t.Fatalf("两件都没说 = %s", got)
	}
	if got := world.registerShortageRules(t, outbound, 1, domain.ShortagePolicyInvalid, domain.PartialShipmentSplitShip).Outcome(); got != application.OutboundPolicyRegistered {
		t.Fatalf("只说拣不齐时怎么发 = %s", got)
	}
	if got := world.registerShortageRules(t, outbound, 1, domain.ShortagePolicyInvalid, domain.PartialShipmentSplitShip).Outcome(); got != application.OutboundExisting {
		t.Fatalf("原样重登 = %s", got)
	}
	short := world.acceptOutbound(t, outbound, outboundCommand(t, "REQ-PS-SHORT", 8))
	request, ok := short.Request()
	if short.Outcome() != application.OutboundPendingPolicy || !ok {
		t.Fatalf("受理时不够、那一件没说 = %s", short.Outcome())
	}
	if got := world.partialShipmentOf(t, request.ID()); got.Rule != "SPLIT_SHIP" || got.Source != "OWNER" || got.Owner != "SPLIT_SHIP" {
		t.Fatalf("请求没写明时读出来的规则 %+v", got)
	}
	if got := world.registerShortageRules(t, outbound, 2, domain.ShortagePolicyPartial, domain.PartialShipmentShipComplete).Outcome(); got != application.OutboundPolicyRegistered {
		t.Fatalf("第二版两件都说 = %s", got)
	}
	requests, err := adapter.NewOutboundRequests(world.db)
	if err != nil {
		t.Fatal(err)
	}
	policy, found, err := requests.LoadPolicy(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"))
	if err != nil || !found || policy.Revision != 2 || policy.Kind != domain.ShortagePolicyPartial || policy.PartialShipment != domain.PartialShipmentShipComplete {
		t.Fatalf("读回的缺货处置 %+v found %v err %v", policy, found, err)
	}
	if got := world.registerShortageRules(t, outbound, 3, domain.ShortagePolicyReject, domain.PartialShipmentUnset).Outcome(); got != application.OutboundPolicyRegistered {
		t.Fatalf("第三版只说受理时不够怎么办 = %s", got)
	}
	if got := world.partialShipmentOf(t, request.ID()); got.Rule != "" || got.Owner != "" {
		t.Fatalf("新一版没说拣不齐时怎么发，读出来仍有 %+v", got)
	}
}

// Covers: 受理之后货主对一张请求另外表示的意思由运营凭依据代录，盖过提交时带的与货主的默认；最晚的一句算数。
// 同一张请求同一个依据只记一次：原样重录答已有结果，换了规则是内容冲突。已经取消的请求不再收。
func TestARecordedConsentOverridesTheOrderAndTheOwnerDefault(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	outbound := world.outbound(t)
	if world.registerShortageRules(t, outbound, 1, domain.ShortagePolicyPartial, domain.PartialShipmentShortShip).Outcome() != application.OutboundPolicyRegistered {
		t.Fatal("货主默认没有登记")
	}
	command := outboundCommand(t, "REQ-PS-CONSENT", 4)
	command.PartialShipment = domain.PartialShipmentShipComplete
	accepted := world.acceptOutbound(t, outbound, command)
	request, ok := accepted.Request()
	if !ok {
		t.Fatalf("受理 %s", accepted.Outcome())
	}
	if got := world.partialShipmentOf(t, request.ID()); got.Rule != "SHIP_COMPLETE" || got.Source != "ORDER" || got.Owner != "SHORT_SHIP" {
		t.Fatalf("代录之前 %+v", got)
	}
	unknown := valueOf(t, domain.NewOutboundRequestID, "OBR-NOT-THERE")
	if got := world.recordConsent(t, outbound, unknown, domain.PartialShipmentSplitShip, "SYN-CONSENT-01").Outcome(); got != application.OutboundNotFound {
		t.Fatalf("没有的请求 = %s", got)
	}
	if got := world.recordConsent(t, outbound, request.ID(), domain.PartialShipmentSplitShip, "").Outcome(); got != application.OutboundContentInvalid {
		t.Fatalf("没有依据 = %s", got)
	}
	if got := world.recordConsent(t, outbound, request.ID(), domain.PartialShipmentUnset, "SYN-CONSENT-01").Outcome(); got != application.OutboundContentInvalid {
		t.Fatalf("没有规则 = %s", got)
	}
	if got := world.recordConsent(t, outbound, request.ID(), domain.PartialShipmentSplitShip, "SYN-CONSENT-01").Outcome(); got != application.OutboundConsentRecorded {
		t.Fatalf("代录 = %s", got)
	}
	if got := world.recordConsent(t, outbound, request.ID(), domain.PartialShipmentSplitShip, "SYN-CONSENT-01").Outcome(); got != application.OutboundExisting {
		t.Fatalf("原样重录 = %s", got)
	}
	if got := world.recordConsent(t, outbound, request.ID(), domain.PartialShipmentShortShip, "SYN-CONSENT-01").Outcome(); got != application.OutboundConflict {
		t.Fatalf("同一个依据换了规则 = %s", got)
	}
	detail := world.partialShipmentOf(t, request.ID())
	if detail.Rule != "SPLIT_SHIP" || detail.Source != "ORDER_CONSENT" || detail.Order != "SHIP_COMPLETE" || detail.Owner != "SHORT_SHIP" ||
		detail.Consent != "SPLIT_SHIP" || detail.ConsentBasis != "SYN-CONSENT-01" || detail.ConsentOperator != "operator-2" || detail.ConsentAt == nil {
		t.Fatalf("代录之后 %+v", detail)
	}

	world.clock.at = world.clock.at.Add(time.Hour)
	if got := world.recordConsent(t, outbound, request.ID(), domain.PartialShipmentShortShip, "SYN-CONSENT-02").Outcome(); got != application.OutboundConsentRecorded {
		t.Fatalf("货主后来改了主意 = %s", got)
	}
	if got := world.partialShipmentOf(t, request.ID()); got.Rule != "SHORT_SHIP" || got.ConsentBasis != "SYN-CONSENT-02" {
		t.Fatalf("最晚的一句没有算数 %+v", got)
	}

	if world.cutoff(t, world.waves(t), request.ID(), 0, "SYN-CUT-PS").Outcome() != application.CutoffCancelled {
		t.Fatal("截单没有成立")
	}
	if got := world.recordConsent(t, outbound, request.ID(), domain.PartialShipmentSplitShip, "SYN-CONSENT-03").Outcome(); got != application.OutboundStatusMismatch {
		t.Fatalf("已经取消的请求 = %s", got)
	}
}

func TestSavingAPartialShipmentConsentRefusesToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	requests, err := adapter.NewOutboundRequests(world.db)
	if err != nil {
		t.Fatal(err)
	}
	err = requests.SaveConsent(context.Background(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ports.PartialShipmentConsent{
		Request: valueOf(t, domain.NewOutboundRequestID, "OBR-1"), Basis: "SYN-CONSENT-01", Rule: domain.PartialShipmentShortShip,
		Operator: "operator-1", RecordedAt: preAdviceNumberingInstant,
	})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveConsent：%v", err)
	}
}

// Covers: 货主对一张请求说过的话跟着拆出的每一份走——受理之后代录的同意盖过提交时带的，子请求按它记；
// 货主的默认不抄到子请求上，没写明的子请求各自按当时的默认读。
func TestSplitChildrenCarryWhatTheOwnerSaidAboutTheOrder(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	outbound := world.outbound(t)
	if world.registerShortageRules(t, outbound, 1, domain.ShortagePolicyPartial, domain.PartialShipmentShortShip).Outcome() != application.OutboundPolicyRegistered {
		t.Fatal("货主默认没有登记")
	}
	command := outboundCommand(t, "REQ-PS-SPLIT", 4)
	command.PartialShipment = domain.PartialShipmentSplitShip
	accepted := world.acceptOutbound(t, outbound, command)
	request, ok := accepted.Request()
	if !ok {
		t.Fatalf("受理 %s", accepted.Outcome())
	}
	if got := world.recordConsent(t, outbound, request.ID(), domain.PartialShipmentShipComplete, "SYN-CONSENT-11").Outcome(); got != application.OutboundConsentRecorded {
		t.Fatalf("代录 = %s", got)
	}
	split := world.splitOutbound(t, outbound, request.ID(), 1, 3)
	if split.Outcome() != application.OutboundSplitResult || len(split.Children()) != 2 {
		t.Fatalf("拆单 = %s，子请求 %d 张", split.Outcome(), len(split.Children()))
	}
	for _, child := range split.Children() {
		if child.PartialShipment() != domain.PartialShipmentShipComplete {
			t.Fatalf("子请求 %s 上记的是 %q", child.ID(), child.PartialShipment())
		}
		if got := world.partialShipmentOf(t, child.ID()); got.Rule != "SHIP_COMPLETE" || got.Source != "ORDER" || got.Owner != "SHORT_SHIP" {
			t.Fatalf("子请求 %s 读出来 %+v", child.ID(), got)
		}
	}

	plain := world.acceptOutbound(t, outbound, outboundCommand(t, "REQ-PS-SPLIT-PLAIN", 4))
	unset, ok := plain.Request()
	if !ok {
		t.Fatalf("受理 %s", plain.Outcome())
	}
	split = world.splitOutbound(t, outbound, unset.ID(), 2, 2)
	if split.Outcome() != application.OutboundSplitResult || len(split.Children()) != 2 {
		t.Fatalf("没写明的那张拆单 = %s", split.Outcome())
	}
	for _, child := range split.Children() {
		if child.PartialShipment().Given() {
			t.Fatalf("货主的默认被抄到了子请求 %s 上：%s", child.ID(), child.PartialShipment())
		}
		if got := world.partialShipmentOf(t, child.ID()); got.Rule != "SHORT_SHIP" || got.Source != "OWNER" {
			t.Fatalf("没写明的子请求 %s 读出来 %+v", child.ID(), got)
		}
	}
}
