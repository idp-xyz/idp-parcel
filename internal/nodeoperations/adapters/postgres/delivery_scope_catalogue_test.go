package postgres_test

import (
	"context"
	"slices"
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

// 本文件对真实 PostgreSQL 证送站预报与送站交接单的运营查阅读（票 ps-external-mark-relations/13；UC-NO-006「权限、隔离与审计」）：
// 两本册按节点与业务时刻（预计送达、开立）筛、新近在前，页大小由读口执行；详情读一份，带撤回、关闭、各账户的结论与成员；别的租户的
// 同名对象不出现在列表里，按标识读与查无此单同答。夹具全为合成（S 级）。

const (
	catalogueTenant      = "SYN-TENANT-PSXM13"
	catalogueOtherTenant = "SYN-TENANT-PSXM13-OTHER"
	catalogueHub         = "SYN-NODE-PSXM13-HUB"
	catalogueEdge        = "SYN-NODE-PSXM13-EDGE"
)

type deliveryScopeCatalogueFixture struct {
	catalogue  *adapter.DeliveryScopeCatalogue
	preAdvices *adapter.DeliveryPreAdvices
	notes      *adapter.DeliveryHandoverNotes
	transactor bentoapp.Transactor
}

func newDeliveryScopeCatalogueFixture(t *testing.T) *deliveryScopeCatalogueFixture {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalogue, err := adapter.NewDeliveryScopeCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	preAdvices, err := adapter.NewDeliveryPreAdvices(db)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := adapter.NewDeliveryHandoverNotes(db)
	if err != nil {
		t.Fatal(err)
	}
	return &deliveryScopeCatalogueFixture{catalogue: catalogue, preAdvices: preAdvices, notes: notes, transactor: db.Transactor()}
}

func catalogueInstant(day, hour int) time.Time {
	return time.Date(2026, 10, day, hour, 0, 0, 0, time.UTC)
}

func preAdviceForCatalogue(t *testing.T, tenant, id, node string, expected time.Time, clue string, numbers ...string) domain.DeliveryPreAdvice {
	t.Helper()
	listed := make([]domain.PreAdvisedParcelNumber, 0, len(numbers))
	for _, number := range numbers {
		listed = append(listed, must(t, domain.NewPreAdvisedParcelNumber, number))
	}
	spec := domain.DeliveryPreAdviceSpec{
		TenantID:          must(t, domain.NewTenantID, tenant),
		ID:                must(t, domain.NewDeliveryPreAdviceID, id),
		CustomerAccount:   must(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-PSXM13-1"),
		Node:              must(t, domain.NewNodeReference, node),
		ExpectedArrivalAt: expected,
		Numbers:           listed,
		Source: domain.CustomerRequestSource{
			Channel: "https://id.syn.example/dex#syn-customer-client-psxm13", RequestKey: "SYN-REQUEST-" + id, ContentDigest: "sha256:" + id,
		},
		FormedAt: catalogueInstant(19, 8),
	}
	if clue != "" {
		spec.DeliveringParty = must(t, domain.NewDeliveringPartyReference, clue)
	}
	preAdvice, err := domain.FormDeliveryPreAdvice(spec)
	if err != nil {
		t.Fatalf("form pre-advice %s: %v", id, err)
	}
	return preAdvice
}

func preAdviceIDsOf(states []domain.DeliveryPreAdviceState) []string {
	ids := make([]string, 0, len(states))
	for _, state := range states {
		ids = append(ids, state.PreAdvice.ID().String())
	}
	return ids
}

// Covers: 票 ps-external-mark-relations/13「做什么」的「查阅读口」（预报）——按节点与预计送达时刻筛，区间左闭右开，新近在前；撤回的那份带撤回；
// 页大小由读口执行；别的租户同标识的预报不出现，按标识读交回本租户那一份，别的租户才有的标识与查无此预报同答（found=false）。
func TestPreAdvicesAreListedByNodeAndExpectedArrivalWithinTheTenant(t *testing.T) {
	fixture := newDeliveryScopeCatalogueFixture(t)
	ctx := t.Context()
	first := preAdviceForCatalogue(t, catalogueTenant, "SYN-PREADV-PSXM13-1", catalogueHub, catalogueInstant(20, 10), "SYN-COURIER-PSXM13",
		"SYN-NUMBER-PSXM13-B", "SYN-NUMBER-PSXM13-A")
	withdrawn := preAdviceForCatalogue(t, catalogueTenant, "SYN-PREADV-PSXM13-2", catalogueHub, catalogueInstant(21, 8), "", "SYN-NUMBER-PSXM13-C")
	edge := preAdviceForCatalogue(t, catalogueTenant, "SYN-PREADV-PSXM13-3", catalogueEdge, catalogueInstant(20, 12), "", "SYN-NUMBER-PSXM13-D")
	midnight := preAdviceForCatalogue(t, catalogueTenant, "SYN-PREADV-PSXM13-4", catalogueHub, catalogueInstant(21, 0), "", "SYN-NUMBER-PSXM13-E")
	sameIDElsewhere := preAdviceForCatalogue(t, catalogueOtherTenant, "SYN-PREADV-PSXM13-1", catalogueHub, catalogueInstant(20, 11), "", "SYN-NUMBER-PSXM13-X")
	onlyElsewhere := preAdviceForCatalogue(t, catalogueOtherTenant, "SYN-PREADV-PSXM13-OTHER", catalogueHub, catalogueInstant(20, 11), "", "SYN-NUMBER-PSXM13-Y")
	withdrawal, err := domain.FormDeliveryPreAdviceWithdrawal(domain.DeliveryPreAdviceWithdrawalSpec{
		TenantID:        must(t, domain.NewTenantID, catalogueTenant),
		PreAdvice:       withdrawn.ID(),
		CustomerAccount: withdrawn.CustomerAccount(),
		Source: domain.CustomerRequestSource{
			Channel: "https://id.syn.example/dex#syn-customer-client-psxm13", RequestKey: "SYN-REQUEST-WITHDRAW-PSXM13", ContentDigest: "sha256:withdraw",
		},
		WithdrawnAt: catalogueInstant(19, 18),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := inTransaction(fixture.transactor, func(txCtx context.Context) error {
		for _, preAdvice := range []domain.DeliveryPreAdvice{first, withdrawn, edge, midnight, sameIDElsewhere, onlyElsewhere} {
			if _, _, err := fixture.preAdvices.RecordDeliveryPreAdvice(txCtx, preAdvice); err != nil {
				return err
			}
		}
		_, _, err := fixture.preAdvices.RecordDeliveryPreAdviceWithdrawal(txCtx, withdrawal)
		return err
	}); err != nil {
		t.Fatalf("落预报：%v", err)
	}
	tenant := must(t, domain.NewTenantID, catalogueTenant)

	all, err := fixture.catalogue.ListDeliveryPreAdvices(ctx, tenant, ports.DeliveryScopeCatalogueFilter{}, 10)
	if err != nil {
		t.Fatalf("上列预报：%v", err)
	}
	if got, want := preAdviceIDsOf(all), []string{"SYN-PREADV-PSXM13-2", "SYN-PREADV-PSXM13-4", "SYN-PREADV-PSXM13-3", "SYN-PREADV-PSXM13-1"}; !slices.Equal(got, want) {
		t.Fatalf("不筛 = %v, want %v（新近在前、别的租户不在）", got, want)
	}
	for _, state := range all {
		if state.Withdrawn() != (state.PreAdvice.ID() == withdrawn.ID()) {
			t.Errorf("%s 撤回 = %v", state.PreAdvice.ID(), state.Withdrawn())
		}
	}
	if !all[0].Withdrawal.WithdrawnAt().Equal(catalogueInstant(19, 18)) {
		t.Errorf("撤回时刻 = %v", all[0].Withdrawal.WithdrawnAt())
	}
	listedFirst := all[3].PreAdvice
	if clue, present := listedFirst.DeliveringParty(); !present || clue.String() != "SYN-COURIER-PSXM13" ||
		!slices.Equal(numberStrings(listedFirst.Numbers()), []string{"SYN-NUMBER-PSXM13-B", "SYN-NUMBER-PSXM13-A"}) ||
		!listedFirst.ExpectedArrivalAt().Equal(catalogueInstant(20, 10)) || listedFirst.Node().String() != catalogueHub {
		t.Errorf("列出的预报走样：%+v", listedFirst)
	}

	hub, err := fixture.catalogue.ListDeliveryPreAdvices(ctx, tenant, ports.DeliveryScopeCatalogueFilter{Node: must(t, domain.NewNodeReference, catalogueHub)}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := preAdviceIDsOf(hub), []string{"SYN-PREADV-PSXM13-2", "SYN-PREADV-PSXM13-4", "SYN-PREADV-PSXM13-1"}; !slices.Equal(got, want) {
		t.Fatalf("按节点 = %v, want %v", got, want)
	}
	day, err := fixture.catalogue.ListDeliveryPreAdvices(ctx, tenant, ports.DeliveryScopeCatalogueFilter{
		Node: must(t, domain.NewNodeReference, catalogueHub), From: catalogueInstant(20, 0), Until: catalogueInstant(21, 0),
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := preAdviceIDsOf(day), []string{"SYN-PREADV-PSXM13-1"}; !slices.Equal(got, want) {
		t.Fatalf("按节点与预计送达日 = %v, want %v（右端开：零点那一份归下一日）", got, want)
	}
	from, err := fixture.catalogue.ListDeliveryPreAdvices(ctx, tenant, ports.DeliveryScopeCatalogueFilter{From: catalogueInstant(21, 0)}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := preAdviceIDsOf(from), []string{"SYN-PREADV-PSXM13-2", "SYN-PREADV-PSXM13-4"}; !slices.Equal(got, want) {
		t.Fatalf("只给起点 = %v, want %v（左端闭）", got, want)
	}
	page, err := fixture.catalogue.ListDeliveryPreAdvices(ctx, tenant, ports.DeliveryScopeCatalogueFilter{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 {
		t.Fatalf("页大小 2 读回 %d 行", len(page))
	}
	if _, err := fixture.catalogue.ListDeliveryPreAdvices(ctx, tenant, ports.DeliveryScopeCatalogueFilter{}, 0); err == nil {
		t.Fatal("页大小 0 未被拒")
	}
	other, err := fixture.catalogue.ListDeliveryPreAdvices(ctx, must(t, domain.NewTenantID, catalogueOtherTenant), ports.DeliveryScopeCatalogueFilter{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := preAdviceIDsOf(other), []string{"SYN-PREADV-PSXM13-1", "SYN-PREADV-PSXM13-OTHER"}; !slices.Equal(got, want) {
		t.Fatalf("别的租户 = %v, want %v", got, want)
	}

	own, found, err := fixture.catalogue.FindDeliveryPreAdvice(ctx, tenant, first.ID())
	if err != nil || !found || !own.PreAdvice.ExpectedArrivalAt().Equal(catalogueInstant(20, 10)) || own.Withdrawn() {
		t.Fatalf("按标识读本租户那一份 = %+v found=%v err=%v", own, found, err)
	}
	gone, found, err := fixture.catalogue.FindDeliveryPreAdvice(ctx, tenant, withdrawn.ID())
	if err != nil || !found || !gone.Withdrawn() {
		t.Fatalf("按标识读撤回的那一份 = %+v found=%v err=%v", gone, found, err)
	}
	for _, id := range []string{"SYN-PREADV-PSXM13-OTHER", "SYN-PREADV-PSXM13-UNKNOWN"} {
		if _, found, err := fixture.catalogue.FindDeliveryPreAdvice(ctx, tenant, must(t, domain.NewDeliveryPreAdviceID, id)); err != nil || found {
			t.Errorf("%s 在本租户 found=%v err=%v, want 查无", id, found, err)
		}
	}
}

func numberStrings(numbers []domain.PreAdvisedParcelNumber) []string {
	values := make([]string, 0, len(numbers))
	for _, number := range numbers {
		values = append(values, number.String())
	}
	return values
}

func noteForCatalogue(t *testing.T, tenant, id, node string, opened time.Time, accounts ...string) domain.DeliveryHandoverNote {
	t.Helper()
	claimed := make([]domain.CustomerAccountReference, 0, len(accounts))
	for _, account := range accounts {
		claimed = append(claimed, must(t, domain.NewCustomerAccountReference, account))
	}
	note, err := domain.OpenDeliveryHandoverNote(domain.DeliveryHandoverNoteSpec{
		TenantID:        must(t, domain.NewTenantID, tenant),
		ID:              must(t, domain.NewDeliveryHandoverNoteID, id),
		Node:            must(t, domain.NewNodeReference, node),
		DeliveringParty: must(t, domain.NewDeliveringPartyReference, "SYN-DRIVER-PSXM13"),
		ClaimedAccounts: claimed,
		OpenedAt:        opened,
	})
	if err != nil {
		t.Fatal(err)
	}
	return note
}

func catalogueMember(t *testing.T, tenant, note, unit string, kind domain.HandoverMembershipKind, fact string, at time.Time) domain.DeliveryHandoverMember {
	t.Helper()
	member, err := domain.FormDeliveryHandoverMember(domain.DeliveryHandoverMemberSpec{
		TenantID: must(t, domain.NewTenantID, tenant),
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

func catalogueBinding(t *testing.T, tenant, note, account string, conclusion domain.HandoverBindingConclusion, route domain.HandoverBindingRoute, at time.Time) domain.DeliveryHandoverBinding {
	t.Helper()
	spec := domain.DeliveryHandoverBindingSpec{
		TenantID:    must(t, domain.NewTenantID, tenant),
		Note:        must(t, domain.NewDeliveryHandoverNoteID, note),
		Account:     must(t, domain.NewCustomerAccountReference, account),
		Conclusion:  conclusion,
		Route:       route,
		ConcludedAt: at,
	}
	if route == domain.ConcludedViaOperationDecision {
		spec.Concluder = "OPERATOR/https://id.syn.example/dex#syn-operator-psxm13"
		spec.DeliveringParticipant = "SYN-PARTY-DELIVERER-PSXM13"
		spec.Basis = "SYN-BASIS-PSXM13-COUNTER"
	} else {
		spec.Concluder = "https://id.syn.example/dex#syn-customer-client-psxm13"
		spec.SourceRequestKey = "SYN-ACCESS-REQUEST-PSXM13-" + account
	}
	binding, err := domain.FormDeliveryHandoverBinding(spec)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func noteIDsOf(rows []ports.DeliveryHandoverNoteCatalogueRow) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.State.Note.ID().String())
	}
	return ids
}

// Covers: 票 ps-external-mark-relations/13「做什么」的「查阅读口」（交接单）——按节点与开立时刻筛、新近在前；每行带关闭、成员数与各账户的结论，
// 范围成立与否由领域按结论派生；详情另带成员与各自的形成方式，按落册先后；别的租户的同名交接单不出现，按标识读与查无此单同答。
func TestHandoverNotesAreListedByNodeAndOpeningWithClaimsAndMembers(t *testing.T) {
	fixture := newDeliveryScopeCatalogueFixture(t)
	ctx := t.Context()
	const established, denied, edgeNote = "SYN-HN-PSXM13-1", "SYN-HN-PSXM13-2", "SYN-HN-PSXM13-3"
	first := noteForCatalogue(t, catalogueTenant, established, catalogueHub, catalogueInstant(20, 9), "SYN-ACCOUNT-PSXM13-1", "SYN-ACCOUNT-PSXM13-2")
	second := noteForCatalogue(t, catalogueTenant, denied, catalogueHub, catalogueInstant(20, 15), "SYN-ACCOUNT-PSXM13-1", "SYN-ACCOUNT-PSXM13-3")
	third := noteForCatalogue(t, catalogueTenant, edgeNote, catalogueEdge, catalogueInstant(21, 9), "SYN-ACCOUNT-PSXM13-3")
	sameIDElsewhere := noteForCatalogue(t, catalogueOtherTenant, established, catalogueHub, catalogueInstant(20, 9), "SYN-ACCOUNT-PSXM13-9")
	onlyElsewhere := noteForCatalogue(t, catalogueOtherTenant, "SYN-HN-PSXM13-OTHER", catalogueHub, catalogueInstant(20, 9), "SYN-ACCOUNT-PSXM13-9")
	closure, err := domain.FormDeliveryHandoverClosure(domain.DeliveryHandoverClosureSpec{
		TenantID: must(t, domain.NewTenantID, catalogueTenant), Note: first.ID(), FactID: "SYN-CLOSE-PSXM13-1", ClosedAt: catalogueInstant(20, 10),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := inTransaction(fixture.transactor, func(txCtx context.Context) error {
		for _, note := range []domain.DeliveryHandoverNote{first, second, third, sameIDElsewhere, onlyElsewhere} {
			if _, _, err := fixture.notes.OpenDeliveryHandoverNote(txCtx, note); err != nil {
				return err
			}
		}
		for _, member := range []domain.DeliveryHandoverMember{
			catalogueMember(t, catalogueTenant, established, "SYN-UNIT-PSXM13-1", domain.MembershipByReception, "SYN-SCAN-PSXM13-1", catalogueInstant(20, 9).Add(5*time.Minute)),
			catalogueMember(t, catalogueTenant, established, "SYN-UNIT-PSXM13-2", domain.MembershipByAssignment, "SYN-ASSIGN-PSXM13-2", catalogueInstant(20, 9).Add(7*time.Minute)),
			catalogueMember(t, catalogueOtherTenant, established, "SYN-UNIT-PSXM13-9", domain.MembershipByAssignment, "SYN-ASSIGN-PSXM13-9", catalogueInstant(20, 9).Add(time.Minute)),
		} {
			if _, _, err := fixture.notes.AppendDeliveryHandoverMember(txCtx, member); err != nil {
				return err
			}
		}
		if _, _, err := fixture.notes.CloseDeliveryHandoverNote(txCtx, closure); err != nil {
			return err
		}
		for _, binding := range []domain.DeliveryHandoverBinding{
			catalogueBinding(t, catalogueTenant, established, "SYN-ACCOUNT-PSXM13-1", domain.BindingConfirmed, domain.ConcludedViaOperationDecision, catalogueInstant(20, 11)),
			catalogueBinding(t, catalogueTenant, established, "SYN-ACCOUNT-PSXM13-2", domain.BindingConfirmed, domain.ConcludedViaCustomerChannel, catalogueInstant(20, 12)),
			catalogueBinding(t, catalogueTenant, denied, "SYN-ACCOUNT-PSXM13-3", domain.BindingDenied, domain.ConcludedViaCustomerChannel, catalogueInstant(20, 16)),
		} {
			if _, _, err := fixture.notes.RecordDeliveryHandoverBinding(txCtx, binding); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("落交接单：%v", err)
	}
	tenant := must(t, domain.NewTenantID, catalogueTenant)

	all, err := fixture.catalogue.ListDeliveryHandoverNotes(ctx, tenant, ports.DeliveryScopeCatalogueFilter{}, 10)
	if err != nil {
		t.Fatalf("上列交接单：%v", err)
	}
	if got, want := noteIDsOf(all), []string{edgeNote, denied, established}; !slices.Equal(got, want) {
		t.Fatalf("不筛 = %v, want %v（新近在前、别的租户不在）", got, want)
	}
	if all[2].MemberCount != 2 || all[1].MemberCount != 0 || all[0].MemberCount != 0 {
		t.Errorf("成员数 = %d/%d/%d, want 0/0/2（别的租户同名交接单的成员不算）", all[0].MemberCount, all[1].MemberCount, all[2].MemberCount)
	}
	if !all[2].State.Closed() || all[2].State.Closure.FactID() != "SYN-CLOSE-PSXM13-1" || all[1].State.Closed() || all[0].State.Closed() {
		t.Errorf("关闭走样：%+v", all)
	}
	if at, ok := all[2].State.ScopeEstablishedAt(); !ok || !at.Equal(catalogueInstant(20, 12)) {
		t.Errorf("范围成立时刻 = %v %v, want 最后一个确认的时刻", at, ok)
	}
	if all[1].State.ScopeEstablished() || all[0].State.ScopeEstablished() {
		t.Error("有否认或待确认的交接单读成了范围成立")
	}
	if binding, ok := all[1].State.BindingFor(must(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-PSXM13-3")); !ok || binding.Conclusion() != domain.BindingDenied {
		t.Errorf("否认那一格 = %+v %v", binding, ok)
	}
	if _, ok := all[1].State.BindingFor(must(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-PSXM13-1")); ok {
		t.Error("没下结论的账户读出了结论")
	}
	operated, _ := all[2].State.BindingFor(must(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-PSXM13-1"))
	if participant, basis := operated.OperationDecisionEvidence(); operated.Route() != domain.ConcludedViaOperationDecision ||
		participant != "SYN-PARTY-DELIVERER-PSXM13" || basis != "SYN-BASIS-PSXM13-COUNTER" {
		t.Errorf("运营决定那一格 = %+v", operated)
	}

	day, err := fixture.catalogue.ListDeliveryHandoverNotes(ctx, tenant, ports.DeliveryScopeCatalogueFilter{
		Node: must(t, domain.NewNodeReference, catalogueHub), From: catalogueInstant(20, 0), Until: catalogueInstant(21, 0),
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := noteIDsOf(day), []string{denied, established}; !slices.Equal(got, want) {
		t.Fatalf("按节点与开立日 = %v, want %v", got, want)
	}
	page, err := fixture.catalogue.ListDeliveryHandoverNotes(ctx, tenant, ports.DeliveryScopeCatalogueFilter{}, 1)
	if err != nil || len(page) != 1 {
		t.Fatalf("页大小 1 读回 %d 行 err=%v", len(page), err)
	}
	if _, err := fixture.catalogue.ListDeliveryHandoverNotes(ctx, tenant, ports.DeliveryScopeCatalogueFilter{}, 0); err == nil {
		t.Fatal("页大小 0 未被拒")
	}
	other, err := fixture.catalogue.ListDeliveryHandoverNotes(ctx, must(t, domain.NewTenantID, catalogueOtherTenant), ports.DeliveryScopeCatalogueFilter{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 2 || other[0].State.Note.ClaimedAccounts()[0].String() != "SYN-ACCOUNT-PSXM13-9" {
		t.Fatalf("别的租户 = %+v", other)
	}

	detail, found, err := fixture.catalogue.FindDeliveryHandoverNoteDetail(ctx, tenant, first.ID())
	if err != nil || !found {
		t.Fatalf("按标识读 found=%v err=%v", found, err)
	}
	if !detail.State.ScopeEstablished() || !detail.State.Closed() || len(detail.State.Bindings) != 2 {
		t.Errorf("详情的交接单状态走样：%+v", detail.State)
	}
	if len(detail.Members) != 2 ||
		detail.Members[0].Unit().String() != "SYN-UNIT-PSXM13-1" || detail.Members[0].Kind() != domain.MembershipByReception ||
		detail.Members[1].Unit().String() != "SYN-UNIT-PSXM13-2" || detail.Members[1].Kind() != domain.MembershipByAssignment ||
		detail.Members[1].FactID() != "SYN-ASSIGN-PSXM13-2" {
		t.Errorf("成员走样：%+v", detail.Members)
	}
	empty, found, err := fixture.catalogue.FindDeliveryHandoverNoteDetail(ctx, tenant, third.ID())
	if err != nil || !found || len(empty.Members) != 0 || len(empty.State.Bindings) != 0 {
		t.Errorf("没有成员与结论的交接单 = %+v found=%v err=%v", empty, found, err)
	}
	for _, id := range []string{"SYN-HN-PSXM13-OTHER", "SYN-HN-PSXM13-UNKNOWN"} {
		if _, found, err := fixture.catalogue.FindDeliveryHandoverNoteDetail(ctx, tenant, must(t, domain.NewDeliveryHandoverNoteID, id)); err != nil || found {
			t.Errorf("%s 在本租户 found=%v err=%v, want 查无", id, found, err)
		}
	}
}
