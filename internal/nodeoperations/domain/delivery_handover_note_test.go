package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证送站交接单的领域形状（ADR-0212 决定三；NO CONTEXT「送站交接单」）：声称至少一个客户账户、成员记形成方式、关闭与
// 绑定结论各是一份事实，范围成立是派生。夹具全为合成（S 级）。

var handoverOpenedAt = time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)

func handoverNote(t *testing.T, accounts ...string) domain.DeliveryHandoverNote {
	t.Helper()
	claimed := make([]domain.CustomerAccountReference, 0, len(accounts))
	for _, account := range accounts {
		claimed = append(claimed, mustBuild(t, domain.NewCustomerAccountReference, account))
	}
	note, err := domain.OpenDeliveryHandoverNote(domain.DeliveryHandoverNoteSpec{
		TenantID:        mustBuild(t, domain.NewTenantID, "SYN-TENANT-01"),
		ID:              mustBuild(t, domain.NewDeliveryHandoverNoteID, "SYN-HANDOVER-PSXM08-01"),
		Node:            mustBuild(t, domain.NewNodeReference, "SYN-NODE-SHA-HUB"),
		DeliveringParty: mustBuild(t, domain.NewDeliveringPartyReference, "SYN-COURIER-PSXM08"),
		ClaimedAccounts: claimed,
		OpenedAt:        handoverOpenedAt,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return note
}

func binding(t *testing.T, note domain.DeliveryHandoverNote, account string, conclusion domain.HandoverBindingConclusion) domain.DeliveryHandoverBinding {
	t.Helper()
	spec := domain.DeliveryHandoverBindingSpec{
		TenantID:         note.TenantID(),
		Note:             note.ID(),
		Account:          mustBuild(t, domain.NewCustomerAccountReference, account),
		Conclusion:       conclusion,
		Route:            domain.ConcludedViaCustomerChannel,
		Concluder:        "https://id.syn.example/dex#syn-customer-client-01",
		SourceRequestKey: "SYN-ACCESS-REQUEST-PSXM08-" + account,
		ConcludedAt:      handoverOpenedAt.Add(time.Hour),
	}
	formed, err := domain.FormDeliveryHandoverBinding(spec)
	if err != nil {
		t.Fatalf("binding %s %s: %v", account, conclusion, err)
	}
	return formed
}

// Covers: ADR-0212 决定三「开立固定节点、交付方线索、声称的客户账户（至少一个）与开立时刻」——一个都不声称、同一账户声称两次
// 都立不起来；声称按标识定序。
func TestAHandoverNoteClaimsAtLeastOneDistinctCustomerAccount(t *testing.T) {
	note := handoverNote(t, "SYN-ACCOUNT-02", "SYN-ACCOUNT-01")
	claimed := note.ClaimedAccounts()
	if len(claimed) != 2 || claimed[0].String() != "SYN-ACCOUNT-01" || claimed[1].String() != "SYN-ACCOUNT-02" {
		t.Fatalf("claimed = %v, want sorted A, B", claimed)
	}
	if !note.Claims(mustBuild(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-02")) ||
		note.Claims(mustBuild(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-03")) {
		t.Fatal("Claims 判错")
	}
	base := domain.DeliveryHandoverNoteSpec{
		TenantID:        note.TenantID(),
		ID:              note.ID(),
		Node:            note.Node(),
		DeliveringParty: note.DeliveringParty(),
		OpenedAt:        handoverOpenedAt,
	}
	duplicated := base
	duplicated.ClaimedAccounts = []domain.CustomerAccountReference{claimed[0], claimed[0]}
	missingParty := base
	missingParty.ClaimedAccounts = claimed
	missingParty.DeliveringParty = domain.DeliveringPartyReference{}
	for name, spec := range map[string]domain.DeliveryHandoverNoteSpec{"一个都不声称": base, "声称两次": duplicated, "缺交付方线索": missingParty} {
		if _, err := domain.OpenDeliveryHandoverNote(spec); !errors.Is(err, domain.ErrInvalidDeliveryHandoverNote) {
			t.Fatalf("%s: err = %v, want ErrInvalidDeliveryHandoverNote", name, err)
		}
	}
}

// Covers: ADR-0212 决定三「绑定确认有两路」——运营决定只能确认、不能否认，必带交付方参与方与依据引用；客户渠道两种结论都可以，
// 不带那两格。
func TestABindingConclusionCarriesItsRoute(t *testing.T) {
	note := handoverNote(t, "SYN-ACCOUNT-01")
	operation := domain.DeliveryHandoverBindingSpec{
		TenantID:              note.TenantID(),
		Note:                  note.ID(),
		Account:               mustBuild(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-01"),
		Conclusion:            domain.BindingConfirmed,
		Route:                 domain.ConcludedViaOperationDecision,
		Concluder:             "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-01",
		DeliveringParticipant: "SYN-PARTY-COURIER-01",
		Basis:                 "SYN-BASIS-AUTH-LETTER-01",
		ConcludedAt:           handoverOpenedAt.Add(time.Hour),
	}
	confirmed, err := domain.FormDeliveryHandoverBinding(operation)
	if err != nil {
		t.Fatalf("operation decision confirm: %v", err)
	}
	if participant, basis := confirmed.OperationDecisionEvidence(); participant != "SYN-PARTY-COURIER-01" || basis != "SYN-BASIS-AUTH-LETTER-01" {
		t.Fatalf("evidence = %q %q", participant, basis)
	}

	refused := map[string]func(*domain.DeliveryHandoverBindingSpec){
		"运营决定否认":      func(spec *domain.DeliveryHandoverBindingSpec) { spec.Conclusion = domain.BindingDenied },
		"运营决定缺交付方参与方": func(spec *domain.DeliveryHandoverBindingSpec) { spec.DeliveringParticipant = "" },
		"运营决定缺依据引用":   func(spec *domain.DeliveryHandoverBindingSpec) { spec.Basis = " " },
		"客户渠道带交付方参与方": func(spec *domain.DeliveryHandoverBindingSpec) {
			spec.Route = domain.ConcludedViaCustomerChannel
			spec.Basis = ""
		},
		"客户渠道带依据引用": func(spec *domain.DeliveryHandoverBindingSpec) {
			spec.Route = domain.ConcludedViaCustomerChannel
			spec.DeliveringParticipant = ""
		},
		"缺确认方": func(spec *domain.DeliveryHandoverBindingSpec) { spec.Concluder = "" },
		"结论不在词表": func(spec *domain.DeliveryHandoverBindingSpec) {
			spec.Conclusion = domain.HandoverBindingConclusionInvalid
		},
		"途径不在词表": func(spec *domain.DeliveryHandoverBindingSpec) { spec.Route = domain.HandoverBindingRouteInvalid },
		"缺结论时刻":  func(spec *domain.DeliveryHandoverBindingSpec) { spec.ConcludedAt = time.Time{} },
		"缺客户账户":  func(spec *domain.DeliveryHandoverBindingSpec) { spec.Account = domain.CustomerAccountReference{} },
		"缺交接单":   func(spec *domain.DeliveryHandoverBindingSpec) { spec.Note = domain.DeliveryHandoverNoteID{} },
	}
	for name, mutate := range refused {
		spec := operation
		mutate(&spec)
		if _, err := domain.FormDeliveryHandoverBinding(spec); !errors.Is(err, domain.ErrInvalidDeliveryHandoverNote) {
			t.Fatalf("%s: err = %v, want ErrInvalidDeliveryHandoverNote", name, err)
		}
	}
	denied := binding(t, note, "SYN-ACCOUNT-01", domain.BindingDenied)
	if participant, basis := denied.OperationDecisionEvidence(); participant != "" || basis != "" {
		t.Fatal("客户渠道的结论不该带运营决定那两格")
	}
}

// Covers: ADR-0212 决定三「成员……收寄来源带它标识的作业实物是成员……可以归入」与「关闭冻结成员」——成员记形成方式与事实身份；
// 关闭是一份事实，状态据它答已关闭。
func TestMembersRecordHowTheyJoinedAndClosureIsAFact(t *testing.T) {
	note := handoverNote(t, "SYN-ACCOUNT-01")
	for _, kind := range []domain.HandoverMembershipKind{domain.MembershipByReception, domain.MembershipByAssignment} {
		member, err := domain.FormDeliveryHandoverMember(domain.DeliveryHandoverMemberSpec{
			TenantID: note.TenantID(),
			Note:     note.ID(),
			Unit:     mustBuild(t, domain.NewHandlingUnitID, "SYN-UNIT-PSXM08-01"),
			Kind:     kind,
			FactID:   "SYN-FACT-PSXM08-" + kind.String(),
			FormedAt: handoverOpenedAt.Add(10 * time.Minute),
		})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if member.Kind() != kind {
			t.Fatalf("kind = %s, want %s", member.Kind(), kind)
		}
		parsed, err := domain.ParseHandoverMembershipKind(kind.String())
		if err != nil || parsed != kind {
			t.Fatalf("parse %s = %v %v", kind, parsed, err)
		}
	}
	if _, err := domain.FormDeliveryHandoverMember(domain.DeliveryHandoverMemberSpec{
		TenantID: note.TenantID(), Note: note.ID(), Unit: mustBuild(t, domain.NewHandlingUnitID, "SYN-UNIT-PSXM08-01"),
		FactID: "SYN-FACT", FormedAt: handoverOpenedAt,
	}); !errors.Is(err, domain.ErrInvalidDeliveryHandoverNote) {
		t.Fatalf("没有形成方式: err = %v", err)
	}

	open := domain.DeliveryHandoverNoteState{Note: note}
	if open.Closed() || !open.Registered() {
		t.Fatal("刚开立的交接单应在册、开放")
	}
	closure, err := domain.FormDeliveryHandoverClosure(domain.DeliveryHandoverClosureSpec{
		TenantID: note.TenantID(), Note: note.ID(), FactID: "SYN-FACT-PSXM08-CLOSE", ClosedAt: handoverOpenedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !(domain.DeliveryHandoverNoteState{Note: note, Closure: closure}).Closed() {
		t.Fatal("关闭之后应答已关闭")
	}
}

// Covers: ADR-0212 决定三「成立」与 AT-NO-074 领域半边——声称 A、B 只确认 A 时不成立；随后 B 否认仍不成立；两个都确认才成立，
// 范围是声称的整组。不在声称之列的结论不算数。
func TestAHandoverNoteEstablishesScopeOnlyWhenEveryClaimIsConfirmed(t *testing.T) {
	note := handoverNote(t, "SYN-ACCOUNT-01", "SYN-ACCOUNT-02")
	confirmedA := binding(t, note, "SYN-ACCOUNT-01", domain.BindingConfirmed)
	confirmedB := binding(t, note, "SYN-ACCOUNT-02", domain.BindingConfirmed)
	deniedB := binding(t, note, "SYN-ACCOUNT-02", domain.BindingDenied)
	stray := binding(t, note, "SYN-ACCOUNT-03", domain.BindingConfirmed)

	cases := map[string]struct {
		bindings    []domain.DeliveryHandoverBinding
		established bool
	}{
		"一个都没确认":      {nil, false},
		"只确认 A":       {[]domain.DeliveryHandoverBinding{confirmedA}, false},
		"A 确认、B 否认":   {[]domain.DeliveryHandoverBinding{confirmedA, deniedB}, false},
		"A 确认、外加不相干的": {[]domain.DeliveryHandoverBinding{confirmedA, stray}, false},
		"两个都确认":       {[]domain.DeliveryHandoverBinding{confirmedB, confirmedA}, true},
	}
	for name, testCase := range cases {
		state := domain.DeliveryHandoverNoteState{Note: note, Bindings: testCase.bindings}
		if got := state.ScopeEstablished(); got != testCase.established {
			t.Fatalf("%s: established = %v, want %v", name, got, testCase.established)
		}
	}
	if found, present := (domain.DeliveryHandoverNoteState{Note: note, Bindings: []domain.DeliveryHandoverBinding{deniedB}}).
		BindingFor(mustBuild(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-02")); !present || found.Conclusion() != domain.BindingDenied {
		t.Fatalf("BindingFor B = %v %v", found, present)
	}
}
