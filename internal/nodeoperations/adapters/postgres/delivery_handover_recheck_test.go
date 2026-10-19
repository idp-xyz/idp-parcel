package postgres_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证交接单命令口落地时交接单册与收寄册多出的几问（票 ps-external-mark-relations/12；ADR-0212 决定三、
// 六）：按交接单找名下的重核对象、按事实身份找成员、客户渠道结论带来源请求键（迁移 node_operations 0016）、最后两个绑定并发确认时
// 在交接单那一行上串行。夹具全为合成（S 级）。

type handoverRecheckFixture struct {
	receptions *adapter.Receptions
	notes      *adapter.DeliveryHandoverNotes
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newHandoverRecheckFixture(t *testing.T) *handoverRecheckFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	receptions, err := adapter.NewReceptions(db)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := adapter.NewDeliveryHandoverNotes(db)
	if err != nil {
		t.Fatal(err)
	}
	return &handoverRecheckFixture{receptions: receptions, notes: notes, transactor: db.Transactor(), pool: pool}
}

// pendingUnitRecord 是一件在 node 上落成不记冲突、来源已保全的待识别实物 unit。
func pendingUnitRecord(t *testing.T, tenant, sourceID, version, unit, node string) ports.ReceptionRecord {
	t.Helper()
	record := recheckablePending(t, tenant, sourceID, version, "SYN-MARK-PSXM12")
	tenantID := record.Key.TenantID
	intake, err := domain.FormNodeIntake(domain.NodeIntakeSpec{
		TenantID:    tenantID,
		Unit:        ref(t, domain.NewHandlingUnitID, unit),
		Node:        ref(t, domain.NewNodeReference, node),
		DeliveredBy: record.Intake.DeliveredBy(),
		Evidence:    record.Intake.Evidence(),
		Version:     ref(t, domain.NewIntakeResultVersion, version),
		ReceivedAt:  record.Intake.ReceivedAt(),
	})
	if err != nil {
		t.Fatalf("构造待识别收寄：%v", err)
	}
	control, err := domain.EstablishPhysicalControl(domain.PhysicalControlSpec{
		TenantID:      tenantID,
		Unit:          intake.Unit(),
		Node:          intake.Node(),
		Kind:          domain.EstablishedByNodeIntake,
		Basis:         ref(t, domain.NewControlBasisReference, "NODE-INTAKE/"+version),
		EstablishedAt: intake.ReceivedAt(),
	})
	if err != nil {
		t.Fatalf("构造控制：%v", err)
	}
	record.Intake, record.Control = intake, control
	record.Source = deliverySource(t, unit, "SYN-MARK-PSXM12", "RECEIVED")
	record.Source.Node = intake.Node()
	return record
}

func handoverNoteAt(t *testing.T, tenant, id, node string, accounts ...string) domain.DeliveryHandoverNote {
	t.Helper()
	claimed := make([]domain.CustomerAccountReference, 0, len(accounts))
	for _, account := range accounts {
		claimed = append(claimed, ref(t, domain.NewCustomerAccountReference, account))
	}
	note, err := domain.OpenDeliveryHandoverNote(domain.DeliveryHandoverNoteSpec{
		TenantID:        ref(t, domain.NewTenantID, tenant),
		ID:              ref(t, domain.NewDeliveryHandoverNoteID, id),
		Node:            ref(t, domain.NewNodeReference, node),
		DeliveringParty: ref(t, domain.NewDeliveringPartyReference, "SYN-COURIER-PSXM12"),
		ClaimedAccounts: claimed,
		OpenedAt:        receivedAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return note
}

func handoverMemberAt(t *testing.T, tenant, note, unit string, kind domain.HandoverMembershipKind, fact string) domain.DeliveryHandoverMember {
	t.Helper()
	member, err := domain.FormDeliveryHandoverMember(domain.DeliveryHandoverMemberSpec{
		TenantID: ref(t, domain.NewTenantID, tenant),
		Note:     ref(t, domain.NewDeliveryHandoverNoteID, note),
		Unit:     ref(t, domain.NewHandlingUnitID, unit),
		Kind:     kind,
		FactID:   fact,
		FormedAt: receivedAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	return member
}

// Covers: `AT-NO-075`（存储面）、ADR-0212 决定六「重核对象只看成员里属于那一组的」的存储面——按交接单只找回名下（收寄或归入的成员）、在交接单开立的节点上
// 当前有效版本是待识别、不带关联、不记身份冲突或冲突来由是只因歧义信号、来源已保全的那几行，按来源标识定序：已关联的、记多候选
// 冲突的、已被接续的、在别的节点收寄的、不是成员的、别的租户同名交接单下的都不在其中；查无此单答空。
func TestRecheckableMembersAreFoundByTheirHandoverNote(t *testing.T) {
	fixture := newHandoverRecheckFixture(t)
	ctx := t.Context()
	const tenant, other, noteID = "SYN-TENANT-PSXM12", "SYN-TENANT-PSXM12-OTHER", "SYN-HANDOVER-PSXM12-01"

	assigned := pendingUnitRecord(t, tenant, "SYN-SCAN-PSXM12-B", "SYN-INTAKEV-PSXM12-B", "SYN-UNIT-ASSIGNED", "node-1")
	received := pendingUnitRecord(t, tenant, "SYN-SCAN-PSXM12-C", "SYN-INTAKEV-PSXM12-C", "SYN-UNIT-RECEIVED", "node-1")
	ambiguous := pendingUnitRecord(t, tenant, "SYN-SCAN-PSXM12-A", "SYN-INTAKEV-PSXM12-A", "SYN-UNIT-AMBIGUOUS", "node-1")
	ambiguous.IdentityConflict, ambiguous.IdentityConflictCause = true, ports.ConflictByAmbiguityOnly
	ambiguous.Candidates = []domain.ParcelAssociationReference{ref(t, domain.NewParcelAssociationReference, "SYN-PARCEL-PSXM12-1")}
	contested := pendingUnitRecord(t, tenant, "SYN-SCAN-PSXM12-D", "SYN-INTAKEV-PSXM12-D", "SYN-UNIT-CONTESTED", "node-1")
	contested.IdentityConflict, contested.IdentityConflictCause = true, ports.ConflictByMultipleCandidates
	contested.Candidates = []domain.ParcelAssociationReference{
		ref(t, domain.NewParcelAssociationReference, "SYN-PARCEL-PSXM12-1"), ref(t, domain.NewParcelAssociationReference, "SYN-PARCEL-PSXM12-2"),
	}
	superseded := pendingUnitRecord(t, tenant, "SYN-SCAN-PSXM12-E", "SYN-INTAKEV-PSXM12-E", "SYN-UNIT-SUPERSEDED", "node-1")
	successor := formedRecord(t, tenant, "IDENTITY-RECHECK/SYN-INTAKEV-PSXM12-E")
	successorIntake, err := domain.FormNodeIntake(domain.NodeIntakeSpec{
		TenantID: superseded.Key.TenantID, Unit: superseded.Intake.Unit(), Node: superseded.Intake.Node(),
		DeliveredBy: superseded.Intake.DeliveredBy(), Evidence: superseded.Intake.Evidence(),
		Version:     ref(t, domain.NewIntakeResultVersion, "SYN-INTAKEV-PSXM12-E2"),
		Association: ref(t, domain.NewParcelAssociationReference, "SYN-PARCEL-PSXM12-3"),
		ReceivedAt:  superseded.Intake.ReceivedAt(),
	})
	if err != nil {
		t.Fatal(err)
	}
	successor.Intake, successor.Control, successor.Source = successorIntake, superseded.Control, superseded.Source
	successor.JudgmentVersion, successor.Predecessor = successorIntake.Version(), superseded.JudgmentVersion
	elsewhere := pendingUnitRecord(t, tenant, "SYN-SCAN-PSXM12-F", "SYN-INTAKEV-PSXM12-F", "SYN-UNIT-ELSEWHERE", "node-2")
	stranger := pendingUnitRecord(t, tenant, "SYN-SCAN-PSXM12-G", "SYN-INTAKEV-PSXM12-G", "SYN-UNIT-STRANGER", "node-1")
	foreign := pendingUnitRecord(t, other, "SYN-SCAN-PSXM12-H", "SYN-INTAKEV-PSXM12-H", "SYN-UNIT-ASSIGNED", "node-1")

	notes := []domain.DeliveryHandoverNote{handoverNoteAt(t, tenant, noteID, "node-1", "SYN-ACCOUNT-01"), handoverNoteAt(t, other, noteID, "node-1", "SYN-ACCOUNT-01")}
	members := []domain.DeliveryHandoverMember{
		handoverMemberAt(t, tenant, noteID, "SYN-UNIT-ASSIGNED", domain.MembershipByAssignment, "SYN-FACT-PSXM12-1"),
		handoverMemberAt(t, tenant, noteID, "SYN-UNIT-RECEIVED", domain.MembershipByReception, "SYN-SCAN-PSXM12-C"),
		handoverMemberAt(t, tenant, noteID, "SYN-UNIT-AMBIGUOUS", domain.MembershipByAssignment, "SYN-FACT-PSXM12-2"),
		handoverMemberAt(t, tenant, noteID, "SYN-UNIT-CONTESTED", domain.MembershipByReception, "SYN-SCAN-PSXM12-D"),
		handoverMemberAt(t, tenant, noteID, "SYN-UNIT-SUPERSEDED", domain.MembershipByAssignment, "SYN-FACT-PSXM12-3"),
		handoverMemberAt(t, tenant, noteID, "SYN-UNIT-ELSEWHERE", domain.MembershipByAssignment, "SYN-FACT-PSXM12-4"),
	}
	inTx(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		for _, record := range []ports.ReceptionRecord{assigned, received, ambiguous, contested, superseded, successor, elsewhere, stranger, foreign} {
			if _, err := fixture.receptions.Save(txCtx, record); err != nil {
				return err
			}
		}
		for _, note := range notes {
			if _, _, err := fixture.notes.OpenDeliveryHandoverNote(txCtx, note); err != nil {
				return err
			}
		}
		for _, member := range members {
			if _, _, err := fixture.notes.AppendDeliveryHandoverMember(txCtx, member); err != nil {
				return err
			}
		}
		return nil
	})

	found, err := fixture.receptions.FindRecheckableInHandoverNote(ctx, ref(t, domain.NewTenantID, tenant), ref(t, domain.NewDeliveryHandoverNoteID, noteID))
	if err != nil {
		t.Fatalf("按交接单找重核对象：%v", err)
	}
	if got, want := sourceIDsOf(found), []string{"SYN-SCAN-PSXM12-A", "SYN-SCAN-PSXM12-B", "SYN-SCAN-PSXM12-C"}; !slices.Equal(got, want) {
		t.Fatalf("重核对象 = %v, want %v", got, want)
	}
	absent, err := fixture.receptions.FindRecheckableInHandoverNote(ctx, ref(t, domain.NewTenantID, tenant), ref(t, domain.NewDeliveryHandoverNoteID, "SYN-HANDOVER-PSXM12-NONE"))
	if err != nil || len(absent) != 0 {
		t.Fatalf("查无此单：%v %v", absent, err)
	}
}

// Covers: `AT-NO-071`（存储面）、票 12 做什么第 1 项「重放照该能力面的既有代数」的存储面——按（租户，形成方式，事实身份）找回那一个成员；同一事实身份在
// 另一种形成方式下不算；没有答 false。
func TestAMemberIsFoundByItsFactIdentity(t *testing.T) {
	fixture := newHandoverRecheckFixture(t)
	ctx := t.Context()
	const tenant, noteID = "SYN-TENANT-PSXM12", "SYN-HANDOVER-PSXM12-02"
	note := handoverNoteAt(t, tenant, noteID, "node-1", "SYN-ACCOUNT-01")
	member := handoverMemberAt(t, tenant, noteID, "SYN-UNIT-PSXM12-9", domain.MembershipByAssignment, "SYN-FACT-PSXM12-9")
	inTx(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		if _, _, err := fixture.notes.OpenDeliveryHandoverNote(txCtx, note); err != nil {
			return err
		}
		_, _, err := fixture.notes.AppendDeliveryHandoverMember(txCtx, member)
		return err
	})
	tenantID := ref(t, domain.NewTenantID, tenant)
	found9, found, err := fixture.notes.FindDeliveryHandoverMember(ctx, tenantID, domain.MembershipByAssignment, "SYN-FACT-PSXM12-9")
	if err != nil || !found || found9.Unit().String() != "SYN-UNIT-PSXM12-9" || found9.Note().String() != noteID || !found9.FormedAt().Equal(receivedAt.Add(time.Minute)) {
		t.Fatalf("按事实身份找成员：%+v %v %v", found9, found, err)
	}
	if _, found, err := fixture.notes.FindDeliveryHandoverMember(ctx, tenantID, domain.MembershipByReception, "SYN-FACT-PSXM12-9"); err != nil || found {
		t.Fatalf("换了形成方式还找得到：%v %v", found, err)
	}
	if _, found, err := fixture.notes.FindDeliveryHandoverMember(ctx, ref(t, domain.NewTenantID, "SYN-TENANT-PSXM12-OTHER"), domain.MembershipByAssignment, "SYN-FACT-PSXM12-9"); err != nil || found {
		t.Fatalf("别的租户找得到：%v %v", found, err)
	}
}

// Covers: UC-NO-006 结果语义契约「绑定已否认：否认时刻与来源请求键」的存储面——客户渠道的结论带来源请求键落册、原样读回；库面（迁移
// node_operations 0016）拒客户渠道缺来源请求键的一行，也拒运营决定带它的一行。
func TestACustomerConclusionKeepsItsSourceRequestKey(t *testing.T) {
	fixture := newHandoverRecheckFixture(t)
	ctx := t.Context()
	const tenant, noteID = "SYN-TENANT-PSXM12", "SYN-HANDOVER-PSXM12-03"
	note := handoverNoteAt(t, tenant, noteID, "node-1", "SYN-ACCOUNT-01", "SYN-ACCOUNT-02")
	denial, err := domain.FormDeliveryHandoverBinding(domain.DeliveryHandoverBindingSpec{
		TenantID:         note.TenantID(),
		Note:             note.ID(),
		Account:          ref(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-02"),
		Conclusion:       domain.BindingDenied,
		Route:            domain.ConcludedViaCustomerChannel,
		Concluder:        "https://id.syn.example/dex#syn-customer-client-01",
		SourceRequestKey: "SYN-ACCESS-REQUEST-PSXM12-1",
		ConcludedAt:      receivedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	inTx(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		if _, _, err := fixture.notes.OpenDeliveryHandoverNote(txCtx, note); err != nil {
			return err
		}
		_, _, err := fixture.notes.RecordDeliveryHandoverBinding(txCtx, denial)
		return err
	})
	state, found, err := fixture.notes.FindDeliveryHandoverNote(ctx, note.TenantID(), note.ID())
	if err != nil || !found {
		t.Fatalf("读交接单：%v %v", found, err)
	}
	recorded, present := state.BindingFor(denial.Account())
	if !present || recorded.SourceRequestKey() != "SYN-ACCESS-REQUEST-PSXM12-1" || recorded.Conclusion() != domain.BindingDenied {
		t.Fatalf("读回的结论 = %+v", recorded)
	}
	for name, statement := range map[string]string{
		"客户渠道缺来源请求键": `INSERT INTO node_operations.delivery_handover_binding
			(tenant_id, note_id, customer_account_id, conclusion, route, concluder, concluded_at)
			VALUES ($1, $2, 'SYN-ACCOUNT-01', 'CONFIRMED', 'CUSTOMER_CHANNEL', 'SYN-CLIENT', now())`,
		"运营决定带来源请求键": `INSERT INTO node_operations.delivery_handover_binding
			(tenant_id, note_id, customer_account_id, conclusion, route, concluder, delivering_participant, basis_ref, source_request_key, concluded_at)
			VALUES ($1, $2, 'SYN-ACCOUNT-01', 'CONFIRMED', 'OPERATION_DECISION', 'OPERATOR/SYN', 'SYN-PARTY', 'SYN-BASIS', 'SYN-KEY', now())`,
	} {
		if _, err := fixture.pool.Exec(ctx, statement, tenant, noteID); err == nil {
			t.Fatalf("%s 的一行落进了册", name)
		}
	}
}

// Covers: `AT-NO-074`（并发面）、ADR-0212 决定六「交接单最后一个绑定得到确认时，同一笔事务自投门铃」的并发面——声称 A、B 的交接单，A 与 B 的确认在两笔
// 并发事务里落：落结论取交接单那一行的排他锁，后一笔等前一笔提交之后才落、才读证据状态，于是只有后一笔看到范围成立。不锁时后一笔
// 不等，落下就读，读不到前一笔未提交的结论；两笔都在对方提交之前读，谁也看不到成立，那一封门铃就永远不响。本用例让前一笔落下之后
// 停一会儿再读：后一笔若不等锁，就抢在它前面读完，答出的先后是反的。
func TestTheLastTwoConfirmationsSerializeOnTheNote(t *testing.T) {
	fixture := newHandoverRecheckFixture(t)
	ctx := t.Context()
	const tenant, noteID = "SYN-TENANT-PSXM12", "SYN-HANDOVER-PSXM12-04"
	note := handoverNoteAt(t, tenant, noteID, "node-1", "SYN-ACCOUNT-01", "SYN-ACCOUNT-02")
	inTx(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		_, _, err := fixture.notes.OpenDeliveryHandoverNote(txCtx, note)
		return err
	})
	confirmation := func(account, key string) domain.DeliveryHandoverBinding {
		binding, err := domain.FormDeliveryHandoverBinding(domain.DeliveryHandoverBindingSpec{
			TenantID: note.TenantID(), Note: note.ID(), Account: ref(t, domain.NewCustomerAccountReference, account),
			Conclusion: domain.BindingConfirmed, Route: domain.ConcludedViaCustomerChannel,
			Concluder: "https://id.syn.example/dex#syn-customer-client-01", SourceRequestKey: key, ConcludedAt: receivedAt,
		})
		if err != nil {
			t.Fatal(err)
		}
		return binding
	}
	recordAndSee := func(binding domain.DeliveryHandoverBinding, recorded chan<- struct{}, hold time.Duration) (bool, error) {
		var established bool
		err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			if _, _, err := fixture.notes.RecordDeliveryHandoverBinding(txCtx, binding); err != nil {
				return err
			}
			if recorded != nil {
				close(recorded)
			}
			time.Sleep(hold)
			state, _, err := fixture.notes.FindDeliveryHandoverNote(txCtx, note.TenantID(), note.ID())
			established = state.ScopeEstablished()
			return err
		})
		return established, err
	}

	firstRecorded := make(chan struct{})
	type outcome struct {
		established bool
		err         error
	}
	firstDone := make(chan outcome, 1)
	go func() {
		established, err := recordAndSee(confirmation("SYN-ACCOUNT-01", "SYN-KEY-A"), firstRecorded, 300*time.Millisecond)
		firstDone <- outcome{established, err}
	}()
	<-firstRecorded
	secondEstablished, secondErr := recordAndSee(confirmation("SYN-ACCOUNT-02", "SYN-KEY-B"), nil, 0)
	first := <-firstDone
	if first.err != nil || secondErr != nil {
		t.Fatalf("两笔确认：%v / %v", first.err, secondErr)
	}
	if first.established || !secondEstablished {
		t.Fatalf("先一笔看到成立 %v、后一笔 %v, want 只有后一笔看到", first.established, secondEstablished)
	}
}

// Covers: 票 12 评审阻断的代决（ADR-0212 文末复核记录）的存储面——成员一问按（租户，交接单，作业实物）答，收寄带来的与归入的都算；
// 别的交接单、别的实物、别的租户都答否；范围证据读口答的是同一问。依据不适用的新细分 NOT_A_HANDOVER_MEMBER 过得了收寄册那条细分
// CHECK（迁移 node_operations 0016 重建），原样读回。
func TestMembershipIsAnsweredPerNoteAndUnit(t *testing.T) {
	fixture := newHandoverRecheckFixture(t)
	ctx := t.Context()
	const tenant, noteID = "SYN-TENANT-PSXM12", "SYN-HANDOVER-PSXM12-05"
	note := handoverNoteAt(t, tenant, noteID, "node-1", "SYN-ACCOUNT-01")
	other := handoverNoteAt(t, tenant, "SYN-HANDOVER-PSXM12-06", "node-1", "SYN-ACCOUNT-01")
	members := []domain.DeliveryHandoverMember{
		handoverMemberAt(t, tenant, noteID, "SYN-UNIT-PSXM12-ASSIGNED", domain.MembershipByAssignment, "SYN-FACT-PSXM12-5"),
		handoverMemberAt(t, tenant, noteID, "SYN-UNIT-PSXM12-RECEIVED", domain.MembershipByReception, "SYN-SCAN-PSXM12-5"),
	}
	stranger := recheckablePending(t, tenant, "SYN-SCAN-PSXM12-STRANGER", "SYN-INTAKEV-PSXM12-STRANGER", "SYN-MARK-PSXM12")
	check, err := domain.NewDeliveryScopeCheck(domain.DeliveryScopeUnverified, domain.ScopeBasisNotApplicable, domain.ScopeNotAHandoverMember)
	if err != nil {
		t.Fatal(err)
	}
	stranger.DeliveryScopeCheck = check
	stranger.IdentityCheck = ports.IdentityChecked
	inTx(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		for _, opened := range []domain.DeliveryHandoverNote{note, other} {
			if _, _, err := fixture.notes.OpenDeliveryHandoverNote(txCtx, opened); err != nil {
				return err
			}
		}
		for _, member := range members {
			if _, _, err := fixture.notes.AppendDeliveryHandoverMember(txCtx, member); err != nil {
				return err
			}
		}
		_, err := fixture.receptions.Save(txCtx, stranger)
		return err
	})
	db, err := bentopg.NewDB(fixture.pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := adapter.NewDeliveryScopeEvidence(db)
	if err != nil {
		t.Fatal(err)
	}
	tenantID := ref(t, domain.NewTenantID, tenant)
	for name, testCase := range map[string]struct {
		tenant domain.TenantID
		note   string
		unit   string
		want   bool
	}{
		"归入的成员":   {tenantID, noteID, "SYN-UNIT-PSXM12-ASSIGNED", true},
		"收寄带来的成员": {tenantID, noteID, "SYN-UNIT-PSXM12-RECEIVED", true},
		"不是成员的实物": {tenantID, noteID, "SYN-UNIT-PSXM12-STRANGER", false},
		"别的交接单":   {tenantID, "SYN-HANDOVER-PSXM12-06", "SYN-UNIT-PSXM12-ASSIGNED", false},
		"别的租户":    {ref(t, domain.NewTenantID, "SYN-TENANT-PSXM12-OTHER"), noteID, "SYN-UNIT-PSXM12-ASSIGNED", false},
	} {
		noteRef, unitRef := ref(t, domain.NewDeliveryHandoverNoteID, testCase.note), ref(t, domain.NewHandlingUnitID, testCase.unit)
		direct, err := fixture.notes.IsDeliveryHandoverMember(ctx, testCase.tenant, noteRef, unitRef)
		if err != nil || direct != testCase.want {
			t.Fatalf("%s：交接单册答 %v（err %v）, want %v", name, direct, err, testCase.want)
		}
		viaEvidence, err := evidence.IsDeliveryHandoverMember(ctx, testCase.tenant, noteRef, unitRef)
		if err != nil || viaEvidence != testCase.want {
			t.Fatalf("%s：范围证据读口答 %v（err %v）, want %v", name, viaEvidence, err, testCase.want)
		}
	}
	read, found, err := fixture.receptions.FindByKey(ctx, stranger.Key)
	if err != nil || !found || read.DeliveryScopeCheck != check {
		t.Fatalf("新细分读回 = %+v（found %v err %v）", read.DeliveryScopeCheck, found, err)
	}
}
