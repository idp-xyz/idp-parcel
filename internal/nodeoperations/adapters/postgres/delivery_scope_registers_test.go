package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证送站预报册、送站交接单册与预报有效时窗册（迁移 0013；ADR-0212 决定二、三；票
// ps-external-mark-relations/08）：每一份事实都只增，撞键回显册上那一行、不覆盖；（租户，交接单，客户账户）的结论唯一；同租户
// 时窗区间重叠在登记时拒。夹具全为合成（S 级）。

var registerTenant = "SYN-TENANT-PSXM08"

func newDeliveryScopeRegisters(t *testing.T) (*adapter.DeliveryPreAdvices, *adapter.DeliveryHandoverNotes, *adapter.PreAdviceWindows, bentoapp.Transactor) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	preAdvices, err := adapter.NewDeliveryPreAdvices(db)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := adapter.NewDeliveryHandoverNotes(db)
	if err != nil {
		t.Fatal(err)
	}
	windows, err := adapter.NewPreAdviceWindows(db)
	if err != nil {
		t.Fatal(err)
	}
	return preAdvices, notes, windows, db.Transactor()
}

// inTransaction 在一笔事务里跑 fn，错误原样交回：断言不进事务闭包（事务闭包门禁）。
func inTransaction(transactor bentoapp.Transactor, fn func(context.Context) error) error {
	return transactor.WithinTransaction(context.Background(), fn)
}

func must[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func registerPreAdvice(t *testing.T, id, account, requestKey, digest string, numbers ...string) domain.DeliveryPreAdvice {
	t.Helper()
	listed := make([]domain.PreAdvisedParcelNumber, 0, len(numbers))
	for _, number := range numbers {
		listed = append(listed, must(t, domain.NewPreAdvisedParcelNumber, number))
	}
	preAdvice, err := domain.FormDeliveryPreAdvice(domain.DeliveryPreAdviceSpec{
		TenantID:          must(t, domain.NewTenantID, registerTenant),
		ID:                must(t, domain.NewDeliveryPreAdviceID, id),
		CustomerAccount:   must(t, domain.NewCustomerAccountReference, account),
		Node:              must(t, domain.NewNodeReference, "SYN-NODE-SHA-HUB"),
		ExpectedArrivalAt: time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC),
		Numbers:           listed,
		DeliveringParty:   must(t, domain.NewDeliveringPartyReference, "SYN-COURIER-PSXM08"),
		Source:            domain.CustomerRequestSource{Channel: "https://id.syn.example/dex#syn-customer-client-01", RequestKey: requestKey, ContentDigest: digest},
		FormedAt:          time.Date(2026, 10, 19, 10, 0, 0, 123456000, time.UTC),
	})
	if err != nil {
		t.Fatalf("form pre-advice: %v", err)
	}
	return preAdvice
}

func withdrawalOf(t *testing.T, preAdvice, account, requestKey string) domain.DeliveryPreAdviceWithdrawal {
	t.Helper()
	withdrawal, err := domain.FormDeliveryPreAdviceWithdrawal(domain.DeliveryPreAdviceWithdrawalSpec{
		TenantID:        must(t, domain.NewTenantID, registerTenant),
		PreAdvice:       must(t, domain.NewDeliveryPreAdviceID, preAdvice),
		CustomerAccount: must(t, domain.NewCustomerAccountReference, account),
		Source:          domain.CustomerRequestSource{Channel: "https://id.syn.example/dex#syn-customer-client-01", RequestKey: requestKey, ContentDigest: "sha256:" + requestKey},
		WithdrawnAt:     time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return withdrawal
}

// Covers: 预报与撤回都只增——同一来源请求键再交一份（标识与号码都换了）不落第二行，回显的是册上那一份；撤回只认预报自己的客户
// 账户，别的账户与查无此预报同答；撤回是终态，第二份撤回不落、回显第一份；读回的预报逐格对得上（号码逐字、顺序照交来的）。
func TestPreAdvicesAndWithdrawalsOnlyAppend(t *testing.T) {
	preAdvices, _, _, transactor := newDeliveryScopeRegisters(t)
	ctx := context.Background()
	tenant := must(t, domain.NewTenantID, registerTenant)

	first := registerPreAdvice(t, "SYN-PREADV-PSXM08-01", "SYN-ACCOUNT-01", "SYN-REQ-01", "sha256:01", " SYN-CPN-A1", "SYN-CPN-A2")
	var recorded domain.DeliveryPreAdvice
	var outcome ports.DeliveryScopeAppendOutcome
	if err := inTransaction(transactor, func(txCtx context.Context) (err error) {
		recorded, outcome, err = preAdvices.RecordDeliveryPreAdvice(txCtx, first)
		return err
	}); err != nil || outcome != ports.DeliveryScopeAppended || recorded.ID() != first.ID() {
		t.Fatalf("first record = %v %s %v", recorded.ID(), outcome, err)
	}

	sameKey := registerPreAdvice(t, "SYN-PREADV-PSXM08-02", "SYN-ACCOUNT-01", "SYN-REQ-01", "sha256:other", "SYN-CPN-Z9")
	if err := inTransaction(transactor, func(txCtx context.Context) (err error) {
		recorded, outcome, err = preAdvices.RecordDeliveryPreAdvice(txCtx, sameKey)
		return err
	}); err != nil || outcome != ports.DeliveryScopeAlreadyRecorded {
		t.Fatalf("same request key = %s %v, want ALREADY_RECORDED", outcome, err)
	}
	if recorded.ID() != first.ID() || recorded.Source().ContentDigest != "sha256:01" {
		t.Fatalf("echo = %s %s, want the first pre-advice", recorded.ID(), recorded.Source().ContentDigest)
	}
	if _, found, err := preAdvices.FindDeliveryPreAdvice(ctx, tenant, sameKey.ID()); err != nil || found {
		t.Fatalf("the refused pre-advice was stored: found=%v err=%v", found, err)
	}

	state, found, err := preAdvices.FindDeliveryPreAdvice(ctx, tenant, first.ID())
	if err != nil || !found {
		t.Fatalf("find = %v %v", found, err)
	}
	numbers := state.PreAdvice.Numbers()
	if len(numbers) != 2 || numbers[0].String() != " SYN-CPN-A1" || numbers[1].String() != "SYN-CPN-A2" {
		t.Fatalf("numbers = %v", numbers)
	}
	if clue, present := state.PreAdvice.DeliveringParty(); !present || clue.String() != "SYN-COURIER-PSXM08" ||
		!state.PreAdvice.FormedAt().Equal(first.FormedAt()) || !state.PreAdvice.ExpectedArrivalAt().Equal(first.ExpectedArrivalAt()) ||
		state.PreAdvice.Node() != first.Node() || state.PreAdvice.CustomerAccount() != first.CustomerAccount() || state.Withdrawn() {
		t.Fatalf("read back = %+v", state)
	}

	var withdrawn domain.DeliveryPreAdviceWithdrawal
	if err := inTransaction(transactor, func(txCtx context.Context) (err error) {
		withdrawn, outcome, err = preAdvices.RecordDeliveryPreAdviceWithdrawal(txCtx, withdrawalOf(t, first.ID().String(), "SYN-ACCOUNT-02", "SYN-REQ-W0"))
		return err
	}); err != nil || outcome != ports.DeliveryScopeTargetNotFound {
		t.Fatalf("withdraw someone else's = %s %v, want TARGET_NOT_FOUND", outcome, err)
	}
	if err := inTransaction(transactor, func(txCtx context.Context) (err error) {
		withdrawn, outcome, err = preAdvices.RecordDeliveryPreAdviceWithdrawal(txCtx, withdrawalOf(t, "SYN-PREADV-PSXM08-NONE", "SYN-ACCOUNT-01", "SYN-REQ-W0"))
		return err
	}); err != nil || outcome != ports.DeliveryScopeTargetNotFound {
		t.Fatalf("withdraw an unknown pre-advice = %s %v, want TARGET_NOT_FOUND", outcome, err)
	}
	if err := inTransaction(transactor, func(txCtx context.Context) (err error) {
		withdrawn, outcome, err = preAdvices.RecordDeliveryPreAdviceWithdrawal(txCtx, withdrawalOf(t, first.ID().String(), "SYN-ACCOUNT-01", "SYN-REQ-W1"))
		return err
	}); err != nil || outcome != ports.DeliveryScopeAppended {
		t.Fatalf("withdraw = %s %v", outcome, err)
	}
	if err := inTransaction(transactor, func(txCtx context.Context) (err error) {
		withdrawn, outcome, err = preAdvices.RecordDeliveryPreAdviceWithdrawal(txCtx, withdrawalOf(t, first.ID().String(), "SYN-ACCOUNT-01", "SYN-REQ-W2"))
		return err
	}); err != nil || outcome != ports.DeliveryScopeAlreadyRecorded || withdrawn.Source().RequestKey != "SYN-REQ-W1" {
		t.Fatalf("second withdrawal = %s %s %v, want ALREADY_RECORDED echoing the first", outcome, withdrawn.Source().RequestKey, err)
	}
	state, _, err = preAdvices.FindDeliveryPreAdvice(ctx, tenant, first.ID())
	if err != nil || !state.Withdrawn() || state.Withdrawal.Source().RequestKey != "SYN-REQ-W1" {
		t.Fatalf("after withdrawal = %+v %v", state.Withdrawal, err)
	}
	if _, found, err := preAdvices.FindDeliveryPreAdvice(ctx, must(t, domain.NewTenantID, "SYN-TENANT-OTHER"), first.ID()); err != nil || found {
		t.Fatalf("another tenant sees the pre-advice: found=%v err=%v", found, err)
	}
}

func openNote(t *testing.T, id string, accounts ...string) domain.DeliveryHandoverNote {
	t.Helper()
	claimed := make([]domain.CustomerAccountReference, 0, len(accounts))
	for _, account := range accounts {
		claimed = append(claimed, must(t, domain.NewCustomerAccountReference, account))
	}
	note, err := domain.OpenDeliveryHandoverNote(domain.DeliveryHandoverNoteSpec{
		TenantID:        must(t, domain.NewTenantID, registerTenant),
		ID:              must(t, domain.NewDeliveryHandoverNoteID, id),
		Node:            must(t, domain.NewNodeReference, "SYN-NODE-SHA-HUB"),
		DeliveringParty: must(t, domain.NewDeliveringPartyReference, "SYN-COURIER-PSXM08"),
		ClaimedAccounts: claimed,
		OpenedAt:        time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return note
}

func memberOf(t *testing.T, note, unit string, kind domain.HandoverMembershipKind, fact string, at time.Time) domain.DeliveryHandoverMember {
	t.Helper()
	member, err := domain.FormDeliveryHandoverMember(domain.DeliveryHandoverMemberSpec{
		TenantID: must(t, domain.NewTenantID, registerTenant),
		Note:     must(t, domain.NewDeliveryHandoverNoteID, note),
		Unit:     must(t, domain.NewHandlingUnitID, unit),
		Kind:     kind,
		FactID:   fact,
		FormedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return member
}

func conclusionOf(t *testing.T, note, account string, conclusion domain.HandoverBindingConclusion) domain.DeliveryHandoverBinding {
	t.Helper()
	formed, err := domain.FormDeliveryHandoverBinding(domain.DeliveryHandoverBindingSpec{
		TenantID:         must(t, domain.NewTenantID, registerTenant),
		Note:             must(t, domain.NewDeliveryHandoverNoteID, note),
		Account:          must(t, domain.NewCustomerAccountReference, account),
		Conclusion:       conclusion,
		Route:            domain.ConcludedViaCustomerChannel,
		Concluder:        "https://id.syn.example/dex#syn-customer-client-01",
		SourceRequestKey: "SYN-ACCESS-REQUEST-PSXM08-" + account,
		ConcludedAt:      time.Date(2026, 10, 20, 11, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return formed
}

// Covers: 交接单、成员、关闭与绑定结论都只增——同一标识再开立回显原样；成员按事实身份认重放；关闭冻结成员（重放照旧答已在册），
// 第二份关闭回显第一份；（租户，交接单，客户账户）的结论唯一，先确认后否认撞键、回显已确认；不在声称之列与查无此单同答。
func TestHandoverNotesMembersClosuresAndBindingsOnlyAppend(t *testing.T) {
	_, notes, _, transactor := newDeliveryScopeRegisters(t)
	ctx := context.Background()
	tenant := must(t, domain.NewTenantID, registerTenant)
	const noteID = "SYN-HANDOVER-PSXM08-01"
	at := time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)

	var outcome ports.DeliveryScopeAppendOutcome
	var note domain.DeliveryHandoverNote
	if err := inTransaction(transactor, func(txCtx context.Context) (err error) {
		note, outcome, err = notes.OpenDeliveryHandoverNote(txCtx, openNote(t, noteID, "SYN-ACCOUNT-01", "SYN-ACCOUNT-02"))
		return err
	}); err != nil || outcome != ports.DeliveryScopeAppended {
		t.Fatalf("open = %s %v", outcome, err)
	}
	if err := inTransaction(transactor, func(txCtx context.Context) (err error) {
		note, outcome, err = notes.OpenDeliveryHandoverNote(txCtx, openNote(t, noteID, "SYN-ACCOUNT-09"))
		return err
	}); err != nil || outcome != ports.DeliveryScopeAlreadyRecorded || len(note.ClaimedAccounts()) != 2 {
		t.Fatalf("reopen = %s %v %v, want ALREADY_RECORDED echoing the two claims", outcome, note.ClaimedAccounts(), err)
	}

	byReception := memberOf(t, noteID, "SYN-UNIT-PSXM08-01", domain.MembershipByReception, "SYN-SRC-PSXM08-01", at)
	byAssignment := memberOf(t, noteID, "SYN-UNIT-PSXM08-02", domain.MembershipByAssignment, "SYN-FACT-ASSIGN-01", at.Add(time.Minute))
	appendMember := func(member domain.DeliveryHandoverMember) (domain.DeliveryHandoverMember, ports.DeliveryScopeAppendOutcome, error) {
		var recorded domain.DeliveryHandoverMember
		var appended ports.DeliveryScopeAppendOutcome
		err := inTransaction(transactor, func(txCtx context.Context) (err error) {
			recorded, appended, err = notes.AppendDeliveryHandoverMember(txCtx, member)
			return err
		})
		return recorded, appended, err
	}
	if _, got, err := appendMember(byReception); err != nil || got != ports.DeliveryScopeAppended {
		t.Fatalf("reception member = %s %v", got, err)
	}
	if _, got, err := appendMember(byAssignment); err != nil || got != ports.DeliveryScopeAppended {
		t.Fatalf("assignment member = %s %v", got, err)
	}
	changedReplay := memberOf(t, noteID, "SYN-UNIT-PSXM08-09", domain.MembershipByReception, "SYN-SRC-PSXM08-01", at)
	if echoed, got, err := appendMember(changedReplay); err != nil || got != ports.DeliveryScopeAlreadyRecorded || echoed.Unit().String() != "SYN-UNIT-PSXM08-01" {
		t.Fatalf("same fact again = %s %s %v, want ALREADY_RECORDED echoing the first unit", got, echoed.Unit(), err)
	}
	if _, got, err := appendMember(memberOf(t, "SYN-HANDOVER-PSXM08-NONE", "SYN-UNIT-PSXM08-03", domain.MembershipByAssignment, "SYN-FACT-ASSIGN-X", at)); err != nil ||
		got != ports.DeliveryScopeTargetNotFound {
		t.Fatalf("member of an unknown note = %s %v", got, err)
	}

	closure, err := domain.FormDeliveryHandoverClosure(domain.DeliveryHandoverClosureSpec{
		TenantID: tenant, Note: note.ID(), FactID: "SYN-FACT-CLOSE-01", ClosedAt: at.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	var closed domain.DeliveryHandoverClosure
	if err := inTransaction(transactor, func(txCtx context.Context) (err error) {
		closed, outcome, err = notes.CloseDeliveryHandoverNote(txCtx, closure)
		return err
	}); err != nil || outcome != ports.DeliveryScopeAppended {
		t.Fatalf("close = %s %v", outcome, err)
	}
	secondClosure, err := domain.FormDeliveryHandoverClosure(domain.DeliveryHandoverClosureSpec{
		TenantID: tenant, Note: note.ID(), FactID: "SYN-FACT-CLOSE-02", ClosedAt: at.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := inTransaction(transactor, func(txCtx context.Context) (err error) {
		closed, outcome, err = notes.CloseDeliveryHandoverNote(txCtx, secondClosure)
		return err
	}); err != nil || outcome != ports.DeliveryScopeAlreadyRecorded || closed.FactID() != "SYN-FACT-CLOSE-01" {
		t.Fatalf("second close = %s %s %v, want ALREADY_RECORDED echoing the first", outcome, closed.FactID(), err)
	}
	if _, got, err := appendMember(memberOf(t, noteID, "SYN-UNIT-PSXM08-04", domain.MembershipByAssignment, "SYN-FACT-ASSIGN-LATE", at.Add(3*time.Hour))); err != nil ||
		got != ports.DeliveryScopeNoteClosed {
		t.Fatalf("member after closure = %s %v, want NOTE_CLOSED", got, err)
	}
	if _, got, err := appendMember(byAssignment); err != nil || got != ports.DeliveryScopeAlreadyRecorded {
		t.Fatalf("replaying an earlier member after closure = %s %v, want ALREADY_RECORDED", got, err)
	}
	members, err := notes.ListDeliveryHandoverMembers(ctx, tenant, note.ID())
	if err != nil || len(members) != 2 {
		t.Fatalf("members = %v %v, want the two admitted before closure", members, err)
	}

	recordBinding := func(binding domain.DeliveryHandoverBinding) (domain.DeliveryHandoverBinding, ports.DeliveryScopeAppendOutcome, error) {
		var recorded domain.DeliveryHandoverBinding
		var appended ports.DeliveryScopeAppendOutcome
		err := inTransaction(transactor, func(txCtx context.Context) (err error) {
			recorded, appended, err = notes.RecordDeliveryHandoverBinding(txCtx, binding)
			return err
		})
		return recorded, appended, err
	}
	if _, got, err := recordBinding(conclusionOf(t, noteID, "SYN-ACCOUNT-01", domain.BindingConfirmed)); err != nil || got != ports.DeliveryScopeAppended {
		t.Fatalf("confirm A = %s %v", got, err)
	}
	if echoed, got, err := recordBinding(conclusionOf(t, noteID, "SYN-ACCOUNT-01", domain.BindingDenied)); err != nil ||
		got != ports.DeliveryScopeAlreadyRecorded || echoed.Conclusion() != domain.BindingConfirmed {
		t.Fatalf("deny A after confirming = %s %v %v, want ALREADY_RECORDED echoing CONFIRMED", got, echoed.Conclusion(), err)
	}
	if _, got, err := recordBinding(conclusionOf(t, noteID, "SYN-ACCOUNT-03", domain.BindingConfirmed)); err != nil || got != ports.DeliveryScopeTargetNotFound {
		t.Fatalf("an account not claimed = %s %v, want TARGET_NOT_FOUND", got, err)
	}
	if _, got, err := recordBinding(conclusionOf(t, "SYN-HANDOVER-PSXM08-NONE", "SYN-ACCOUNT-01", domain.BindingConfirmed)); err != nil || got != ports.DeliveryScopeTargetNotFound {
		t.Fatalf("an unknown note = %s %v, want TARGET_NOT_FOUND", got, err)
	}

	state, found, err := notes.FindDeliveryHandoverNote(ctx, tenant, note.ID())
	if err != nil || !found || !state.Closed() || len(state.Bindings) != 1 || state.ScopeEstablished() {
		t.Fatalf("state = %+v %v %v", state, found, err)
	}
	if _, got, err := recordBinding(conclusionOf(t, noteID, "SYN-ACCOUNT-02", domain.BindingConfirmed)); err != nil || got != ports.DeliveryScopeAppended {
		t.Fatalf("confirm B = %s %v", got, err)
	}
	state, _, err = notes.FindDeliveryHandoverNote(ctx, tenant, note.ID())
	if err != nil || !state.ScopeEstablished() {
		t.Fatalf("both confirmed: established = %v %v", state.ScopeEstablished(), err)
	}
}

// Covers: ADR-0212 决定三「一件实物的依据取它最后归入的交接单」的存储一问——按业务时间取最后一次归入；收寄带来的成员不算归入；
// 没归入过答没有。
func TestTheLastAssignmentOfAUnitNamesItsHandoverNote(t *testing.T) {
	_, notes, _, transactor := newDeliveryScopeRegisters(t)
	ctx := context.Background()
	tenant := must(t, domain.NewTenantID, registerTenant)
	at := time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)
	for _, id := range []string{"SYN-HANDOVER-PSXM08-A", "SYN-HANDOVER-PSXM08-B", "SYN-HANDOVER-PSXM08-C"} {
		if err := inTransaction(transactor, func(txCtx context.Context) error {
			_, _, err := notes.OpenDeliveryHandoverNote(txCtx, openNote(t, id, "SYN-ACCOUNT-01"))
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	unit := must(t, domain.NewHandlingUnitID, "SYN-UNIT-PSXM08-LAST")
	if _, found, err := notes.FindLastAssignedHandoverNote(ctx, tenant, unit); err != nil || found {
		t.Fatalf("before any assignment: found=%v err=%v", found, err)
	}
	// 后补传的那次归入业务时间更早：最后归入按业务时间认，不按落册先后。
	for _, member := range []domain.DeliveryHandoverMember{
		memberOf(t, "SYN-HANDOVER-PSXM08-C", "SYN-UNIT-PSXM08-LAST", domain.MembershipByReception, "SYN-SRC-LAST", at.Add(5*time.Hour)),
		memberOf(t, "SYN-HANDOVER-PSXM08-B", "SYN-UNIT-PSXM08-LAST", domain.MembershipByAssignment, "SYN-FACT-ASSIGN-B", at.Add(2*time.Hour)),
		memberOf(t, "SYN-HANDOVER-PSXM08-A", "SYN-UNIT-PSXM08-LAST", domain.MembershipByAssignment, "SYN-FACT-ASSIGN-A", at.Add(time.Hour)),
	} {
		if err := inTransaction(transactor, func(txCtx context.Context) error {
			_, _, err := notes.AppendDeliveryHandoverMember(txCtx, member)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	last, found, err := notes.FindLastAssignedHandoverNote(ctx, tenant, unit)
	if err != nil || !found || last.String() != "SYN-HANDOVER-PSXM08-B" {
		t.Fatalf("last assigned = %s %v %v, want SYN-HANDOVER-PSXM08-B", last, found, err)
	}
}

func windowOf(t *testing.T, from, until time.Time, grace time.Duration) domain.PreAdviceValidityWindow {
	t.Helper()
	window, err := domain.FormPreAdviceValidityWindow(domain.PreAdviceValidityWindowSpec{
		TenantID:       must(t, domain.NewTenantID, registerTenant),
		Lead:           24 * time.Hour,
		Grace:          grace,
		EffectiveFrom:  from,
		EffectiveUntil: until,
		Basis:          "SYN-BASIS-PSXM08-WINDOW",
		RegisteredAt:   time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return window
}

// Covers: ADR-0212 决定二末项——时窗行只增；同租户生效区间重叠在登记时拒（库面排他约束），撞上的那一行回显，调用方据此分重放
// 与重叠；首尾相接的下一段登得进；按时点读生效行，那一刻没有生效行答没有。
func TestValidityWindowsRefuseOverlapAtRegistration(t *testing.T) {
	_, _, windows, transactor := newDeliveryScopeRegisters(t)
	ctx := context.Background()
	tenant := must(t, domain.NewTenantID, registerTenant)
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	jul := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	register := func(window domain.PreAdviceValidityWindow) (domain.PreAdviceValidityWindow, ports.DeliveryScopeAppendOutcome, error) {
		var recorded domain.PreAdviceValidityWindow
		var outcome ports.DeliveryScopeAppendOutcome
		err := inTransaction(transactor, func(txCtx context.Context) (err error) {
			recorded, outcome, err = windows.RegisterPreAdviceWindow(txCtx, window)
			return err
		})
		return recorded, outcome, err
	}
	firstHalf := windowOf(t, jan, jul, 6*time.Hour)
	if _, outcome, err := register(firstHalf); err != nil || outcome != ports.DeliveryScopeAppended {
		t.Fatalf("first = %s %v", outcome, err)
	}
	if echoed, outcome, err := register(firstHalf); err != nil || outcome != ports.DeliveryScopeAlreadyRecorded || !echoed.SameContent(firstHalf) {
		t.Fatalf("replay = %s %v", outcome, err)
	}
	overlapping := windowOf(t, jan.AddDate(0, 3, 0), time.Time{}, 8*time.Hour)
	if echoed, outcome, err := register(overlapping); err != nil || outcome != ports.DeliveryScopeAlreadyRecorded || !echoed.SameContent(firstHalf) {
		t.Fatalf("overlapping = %s %v, want ALREADY_RECORDED echoing the first half", outcome, err)
	}
	secondHalf := windowOf(t, jul, time.Time{}, 8*time.Hour)
	if _, outcome, err := register(secondHalf); err != nil || outcome != ports.DeliveryScopeAppended {
		t.Fatalf("adjacent = %s %v", outcome, err)
	}

	for name, testCase := range map[string]struct {
		at    time.Time
		grace time.Duration
		found bool
	}{
		"上半年":   {jan.AddDate(0, 2, 0), 6 * time.Hour, true},
		"下半年起点": {jul, 8 * time.Hour, true},
		"登记之前":  {jan.Add(-time.Second), 0, false},
	} {
		window, found, err := windows.FindPreAdviceWindowAt(ctx, tenant, testCase.at)
		if err != nil || found != testCase.found || (found && window.Grace() != testCase.grace) {
			t.Fatalf("%s: window = %v %v %v", name, window.Grace(), found, err)
		}
	}
	if _, found, err := windows.FindPreAdviceWindowAt(ctx, must(t, domain.NewTenantID, "SYN-TENANT-OTHER"), jul); err != nil || found {
		t.Fatalf("another tenant: found=%v err=%v", found, err)
	}
}

// Covers: 重叠拒在库面——两笔并发登记各自查不到对方时，排他约束只放进一笔，另一笔撞上回显，不靠应用层先查后写。
func TestConcurrentOverlappingWindowsLandOnlyOnce(t *testing.T) {
	_, _, windows, transactor := newDeliveryScopeRegisters(t)
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	candidates := []domain.PreAdviceValidityWindow{
		windowOf(t, jan, time.Time{}, 6*time.Hour),
		windowOf(t, jan.AddDate(0, 1, 0), time.Time{}, 7*time.Hour),
	}
	outcomes := make([]ports.DeliveryScopeAppendOutcome, len(candidates))
	errs := make([]error, len(candidates))
	var wait sync.WaitGroup
	for index, window := range candidates {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs[index] = inTransaction(transactor, func(txCtx context.Context) (err error) {
				_, outcomes[index], err = windows.RegisterPreAdviceWindow(txCtx, window)
				return err
			})
		}()
	}
	wait.Wait()
	appended := 0
	for index := range candidates {
		if errs[index] != nil {
			t.Fatalf("candidate %d: %v", index, errs[index])
		}
		if outcomes[index] == ports.DeliveryScopeAppended {
			appended++
		}
	}
	if appended != 1 {
		t.Fatalf("appended = %d of two overlapping windows, want exactly one (%v)", appended, outcomes)
	}
}

// Covers: PBC-08 负向证据——各本册的每个写口在无事务上下文里一律拒，册上什么也不落。
func TestDeliveryScopeRegistersRefuseWritesOutsideATransaction(t *testing.T) {
	preAdvices, notes, windows, _ := newDeliveryScopeRegisters(t)
	ctx := context.Background()
	tenant := must(t, domain.NewTenantID, registerTenant)
	at := time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)

	preAdvice := registerPreAdvice(t, "SYN-PREADV-PSXM08-NOTX", "SYN-ACCOUNT-01", "SYN-REQ-NOTX", "sha256:notx", "SYN-CPN-A1")
	if _, _, err := preAdvices.RecordDeliveryPreAdvice(ctx, preAdvice); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("RecordDeliveryPreAdvice: err = %v", err)
	}
	if _, _, err := preAdvices.RecordDeliveryPreAdviceWithdrawal(ctx, withdrawalOf(t, "SYN-PREADV-PSXM08-NOTX", "SYN-ACCOUNT-01", "SYN-REQ-NOTX-W")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("RecordDeliveryPreAdviceWithdrawal: err = %v", err)
	}
	if _, found, err := preAdvices.FindDeliveryPreAdvice(ctx, tenant, preAdvice.ID()); err != nil || found {
		t.Errorf("the refused pre-advice landed: found=%v err=%v", found, err)
	}

	note := openNote(t, "SYN-HANDOVER-PSXM08-NOTX", "SYN-ACCOUNT-01")
	if _, _, err := notes.OpenDeliveryHandoverNote(ctx, note); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("OpenDeliveryHandoverNote: err = %v", err)
	}
	if _, _, err := notes.AppendDeliveryHandoverMember(ctx, memberOf(t, "SYN-HANDOVER-PSXM08-NOTX", "SYN-UNIT-NOTX", domain.MembershipByAssignment, "SYN-FACT-NOTX", at)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("AppendDeliveryHandoverMember: err = %v", err)
	}
	closure, err := domain.FormDeliveryHandoverClosure(domain.DeliveryHandoverClosureSpec{TenantID: tenant, Note: note.ID(), FactID: "SYN-FACT-CLOSE-NOTX", ClosedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := notes.CloseDeliveryHandoverNote(ctx, closure); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("CloseDeliveryHandoverNote: err = %v", err)
	}
	if _, _, err := notes.RecordDeliveryHandoverBinding(ctx, conclusionOf(t, "SYN-HANDOVER-PSXM08-NOTX", "SYN-ACCOUNT-01", domain.BindingConfirmed)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("RecordDeliveryHandoverBinding: err = %v", err)
	}
	if _, found, err := notes.FindDeliveryHandoverNote(ctx, tenant, note.ID()); err != nil || found {
		t.Errorf("the refused note landed: found=%v err=%v", found, err)
	}

	if _, _, err := windows.RegisterPreAdviceWindow(ctx, windowOf(t, at, time.Time{}, time.Hour)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("RegisterPreAdviceWindow: err = %v", err)
	}
	if _, found, err := windows.FindPreAdviceWindowAt(ctx, tenant, at); err != nil || found {
		t.Errorf("the refused window landed: found=%v err=%v", found, err)
	}
}
