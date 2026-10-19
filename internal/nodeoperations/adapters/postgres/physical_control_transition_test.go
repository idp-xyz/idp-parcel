package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证节点控制转移册（ADR-0214 决定三，迁移 0017）：落行与读回、主键幂等、一份控制至多转出一次、读控制时
// 的折叠（按键读回收寄、认当前收寄、查阅目录三处都看得见转出，收寄行本身不改写）、在身控制的读法与实物锁、交接对象的认法。夹具全为
// 合成（S 级），断言一律在事务闭包外。

const controlTenant = "SYN-TENANT-61"

var controlHandedOverAt = receivedAt.Add(5 * time.Hour)

type controlFixture struct {
	db          *bentopg.DB
	transactor  bentoapp.Transactor
	receptions  *adapter.Receptions
	transitions *adapter.PhysicalControlTransitions
	catalogue   *adapter.ReviewCatalogue
}

func newControlFixture(t *testing.T) *controlFixture {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	fixture := &controlFixture{db: db, transactor: db.Transactor()}
	if fixture.receptions, err = adapter.NewReceptions(db); err != nil {
		t.Fatal(err)
	}
	if fixture.transitions, err = adapter.NewPhysicalControlTransitions(db); err != nil {
		t.Fatal(err)
	}
	if fixture.catalogue, err = adapter.NewReviewCatalogue(db); err != nil {
		t.Fatal(err)
	}
	return fixture
}

// intakeWithControl 拼一行带收寄与控制的判断：association 非空即收寄已形成、带那件包裹的关联，否则待识别。predecessor 非空即接续
// 那一版的追加版，控制照抄 basisVersion 那一版的（追加版不另起控制起点）。
func intakeWithControl(t *testing.T, sourceID, unit, node, version, association, predecessor, basisVersion string) ports.ReceptionRecord {
	t.Helper()
	tenant := ref(t, domain.NewTenantID, controlTenant)
	spec := domain.NodeIntakeSpec{
		TenantID:    tenant,
		Unit:        ref(t, domain.NewHandlingUnitID, unit),
		Node:        ref(t, domain.NewNodeReference, node),
		DeliveredBy: ref(t, domain.NewDeliveringPartyReference, "SYN-CUSTOMER-61"),
		Evidence:    ref(t, domain.NewReceptionEvidenceReference, "SYN-SIGN-61"),
		Version:     ref(t, domain.NewIntakeResultVersion, version),
		ReceivedAt:  receivedAt,
	}
	record := ports.ReceptionRecord{
		Key:             ports.ReceptionKey{TenantID: tenant, SourceID: sourceID},
		ContentDigest:   "SYN-DIGEST-" + sourceID,
		Kind:            ports.RecordPendingIdentification,
		JudgmentVersion: spec.Version,
		IdentityCheck:   ports.IdentityChecked,
		RecordedAt:      receivedAt.Add(time.Minute),
	}
	if association != "" {
		spec.Association = ref(t, domain.NewParcelAssociationReference, association)
		record.Kind, record.IdentityCheck, record.ServiceResultCheck = ports.RecordIntakeFormed, ports.IdentityCheckAbsent, ports.ServiceResultsChecked
	}
	if predecessor != "" {
		record.Predecessor = ref(t, domain.NewIntakeResultVersion, predecessor)
	}
	intake, err := domain.FormNodeIntake(spec)
	if err != nil {
		t.Fatalf("构造收寄：%v", err)
	}
	control, err := domain.EstablishPhysicalControl(domain.PhysicalControlSpec{
		TenantID: tenant, Unit: spec.Unit, Node: spec.Node, Kind: domain.EstablishedByNodeIntake,
		Basis: ref(t, domain.NewControlBasisReference, "NODE-INTAKE/"+basisVersion), EstablishedAt: receivedAt,
	})
	if err != nil {
		t.Fatalf("构造控制：%v", err)
	}
	record.Intake, record.Control = intake, control
	return record
}

func (fixture *controlFixture) save(t *testing.T, records ...ports.ReceptionRecord) {
	t.Helper()
	inTx(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		for _, record := range records {
			if _, err := fixture.receptions.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})
}

func controlHandover(t *testing.T, object, version string) domain.AuthoritativeHandoverReference {
	t.Helper()
	reference, err := domain.NewAuthoritativeHandoverReference(object, "SYN-SCOPE-61", version)
	if err != nil {
		t.Fatal(err)
	}
	return reference
}

// activeControls 在一笔事务里读一件实物此刻在身的各份控制。
func (fixture *controlFixture) activeControls(t *testing.T, unit string) []domain.PhysicalControl {
	t.Helper()
	tenant, handlingUnit := ref(t, domain.NewTenantID, controlTenant), ref(t, domain.NewHandlingUnitID, unit)
	var active []domain.PhysicalControl
	inTx(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		active, err = fixture.transitions.FindActiveControls(txCtx, tenant, handlingUnit)
		return err
	})
	return active
}

// transferOut 照编排的做法转出：取锁读在身控制，按领域判，落行。
func (fixture *controlFixture) transferOut(t *testing.T, unit, version string) ports.ControlTransitionRecord {
	t.Helper()
	tenant, handlingUnit := ref(t, domain.NewTenantID, controlTenant), ref(t, domain.NewHandlingUnitID, unit)
	handover := controlHandover(t, "SYN-PARCEL-61", version)
	reference := ref(t, domain.NewTransferOutReference, "TRANSPORT-HANDOVER/"+version)
	var record ports.ControlTransitionRecord
	inTx(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		active, err := fixture.transitions.FindActiveControls(txCtx, tenant, handlingUnit)
		if err != nil {
			return err
		}
		transition, err := domain.JudgeTransferOut(tenant, handlingUnit, active, handover, reference, controlHandedOverAt)
		if err != nil {
			return err
		}
		record = ports.ControlTransitionRecord{Transition: transition, RecordedAt: controlHandedOverAt.Add(time.Minute)}
		saved, err := fixture.transitions.Save(txCtx, record)
		if err == nil && saved != ports.ControlTransitionSaved {
			return errors.New("transition not saved")
		}
		return err
	})
	return record
}

// Covers: `AT-NO-101` 与 `AT-NO-111` 的存储面（ADR-0214 决定三）——转出落在转移册上，收寄行的 control 列不改写；按键读回收寄、认当前
// 收寄（收寄编排据它判并入）与查阅目录都经折叠读出已转出：controlReleasedBy 是交接给的引用、时刻取交接的业务时间。
func TestATransferOutIsFoldedIntoEveryReadOfTheIntakeControl(t *testing.T) {
	fixture := newControlFixture(t)
	formed := intakeWithControl(t, "SYN-SRC-61-A", "SYN-HU-61", "SYN-NODE-SHA-HUB", "SYN-INTAKEV-61-A", "SYN-PARCEL-61", "", "SYN-INTAKEV-61-A")
	fixture.save(t, formed)

	fixture.transferOut(t, "SYN-HU-61", "v1")

	read, _, err := fixture.receptions.FindByKey(t.Context(), formed.Key)
	if err != nil {
		t.Fatal(err)
	}
	reference, at, released := read.Control.Release()
	if !released || reference.String() != "TRANSPORT-HANDOVER/v1" || !at.Equal(controlHandedOverAt) {
		t.Fatalf("按键读回的控制 released=%v %q @ %s", released, reference, at)
	}
	tenant, unit, node := ref(t, domain.NewTenantID, controlTenant), ref(t, domain.NewHandlingUnitID, "SYN-HU-61"),
		ref(t, domain.NewNodeReference, "SYN-NODE-SHA-HUB")
	var heads []ports.ReceptionRecord
	inTx(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		heads, err = fixture.receptions.FindCurrentJudgmentsAtNode(txCtx, tenant, unit, node)
		return err
	})
	if len(heads) != 1 || heads[0].Control.Active() {
		t.Fatalf("认当前收寄读到 %d 条、控制在身=%v, want 一条且已转出（再送站不再并入它）", len(heads), len(heads) == 1 && heads[0].Control.Active())
	}
	rows, err := fixture.catalogue.ListReceptions(t.Context(), tenant, 10)
	if err != nil || len(rows) != 1 || rows[0].ControlReleasedBy != "TRANSPORT-HANDOVER/v1" ||
		rows[0].ControlReleasedAt == nil || !rows[0].ControlReleasedAt.Equal(controlHandedOverAt) {
		t.Fatalf("收寄册 = %+v err = %v", rows, err)
	}
	var raw *string
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := querier.QueryRow(t.Context(),
		`SELECT control ->> 'releasedBy' FROM node_operations.reception WHERE tenant_id = $1 AND source_id = $2`,
		controlTenant, formed.Key.SourceID).Scan(&raw); err != nil || raw != nil {
		t.Fatalf("收寄行的 control 列被改写了：%v err=%v", raw, err)
	}
}

// Covers: ADR-0214 决定三——转移行原样读回（经领域读回门）；同一版同一方向再落答已在册、不覆盖；一份控制被另一版交接再转出撞部分
// 唯一索引报错，不形成答案。
func TestTransitionsAreKeyedByHandoverAndAControlIsReleasedOnce(t *testing.T) {
	fixture := newControlFixture(t)
	fixture.save(t, intakeWithControl(t, "SYN-SRC-61-B", "SYN-HU-61", "SYN-NODE-SHA-HUB", "SYN-INTAKEV-61-B", "SYN-PARCEL-61", "", "SYN-INTAKEV-61-B"))
	written := fixture.transferOut(t, "SYN-HU-61", "v1")
	tenant := ref(t, domain.NewTenantID, controlTenant)

	read, found, err := fixture.transitions.FindTransition(t.Context(), tenant, controlHandover(t, "SYN-PARCEL-61", "v1"), domain.ControlTransitionOut)
	if err != nil || !found || read.Transition != written.Transition || !read.RecordedAt.Equal(written.RecordedAt) {
		t.Fatalf("读回 = %+v found=%v err=%v", read, found, err)
	}
	chained, found, err := fixture.transitions.FindAdvancedInHandoverChain(t.Context(), tenant, controlHandover(t, "SYN-PARCEL-61", "v7"), domain.ControlTransitionOut)
	if err != nil || !found || chained.Transition != written.Transition {
		t.Fatalf("同链任一版都该认得出这次转出：found=%v err=%v", found, err)
	}

	var again ports.ControlTransitionSaveOutcome
	inTx(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		again, err = fixture.transitions.Save(txCtx, written)
		return err
	})
	if again != ports.ControlTransitionAlreadyRecorded {
		t.Fatalf("同键再落 = %d, want 已在册", again)
	}

	other, err := domain.RehydrateControlTransition(domain.ControlTransitionSpec{
		TenantID: tenant, Handover: controlHandover(t, "SYN-PARCEL-61-OTHER", "v1"), Direction: domain.ControlTransitionOut,
		Outcome: domain.ControlAdvanced, Unit: ref(t, domain.NewHandlingUnitID, "SYN-HU-61"),
		Control:    func() domain.PhysicalControl { control, _ := written.Transition.Control(); return control }(),
		OccurredAt: controlHandedOverAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	saveErr := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := fixture.transitions.Save(txCtx, ports.ControlTransitionRecord{Transition: other, RecordedAt: controlHandedOverAt})
		return err
	})
	var pgErr *pgconn.PgError
	if !errors.As(saveErr, &pgErr) || pgErr.ConstraintName != "physical_control_transition_released_once" {
		t.Fatalf("同一份控制再转出 err = %v, want 撞 physical_control_transition_released_once", saveErr)
	}
}

// Covers: `AT-NO-107` 的存储面（ADR-0214 决定四）——在身控制：各条判断链照抄的同一份按控制依据去重；转出之后不在身；转入成立的控制
// （转移册上的 IN 行）同样算、同样折叠；别的实物、别的租户不算。
func TestActiveControlsComeFromIntakesAndHandoversIn(t *testing.T) {
	fixture := newControlFixture(t)
	original := intakeWithControl(t, "SYN-SRC-61-C", "SYN-HU-61", "SYN-NODE-SHA-HUB", "SYN-INTAKEV-61-C", "SYN-PARCEL-61", "", "SYN-INTAKEV-61-C")
	appended := intakeWithControl(t, "SYN-SRC-61-C2", "SYN-HU-61", "SYN-NODE-SHA-HUB", "SYN-INTAKEV-61-C2", "", "SYN-INTAKEV-61-C", "SYN-INTAKEV-61-C")
	elsewhere := intakeWithControl(t, "SYN-SRC-61-D", "SYN-HU-61-OTHER", "SYN-NODE-SHA-HUB", "SYN-INTAKEV-61-D", "", "", "SYN-INTAKEV-61-D")
	fixture.save(t, original, appended, elsewhere)

	if active := fixture.activeControls(t, "SYN-HU-61"); len(active) != 1 || active[0].Basis().String() != "NODE-INTAKE/SYN-INTAKEV-61-C" {
		t.Fatalf("在身控制 = %v, want 照抄的那一份只算一次", active)
	}
	fixture.transferOut(t, "SYN-HU-61", "v1")
	if active := fixture.activeControls(t, "SYN-HU-61"); len(active) != 0 {
		t.Fatalf("转出之后仍有 %d 份在身", len(active))
	}

	executor := func(txCtx context.Context) error {
		writer, err := fixture.db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		_, err = writer.Exec(txCtx,
			`INSERT INTO node_operations.physical_control_transition
			     (tenant_id, handover_object, handover_scope, handover_version, direction, outcome,
			      unit, node, control_kind, control_basis, control_established_at, occurred_at, recorded_at)
			 VALUES ($1, 'SYN-PARCEL-61', 'SYN-SCOPE-61-IN', 'v1', 'IN', 'ADVANCED',
			         'SYN-HU-61', 'SYN-NODE-SZX-GATE', 'HANDOVER_IN', 'TRANSPORT-HANDOVER/IN-v1', $2, $2, $2)`,
			controlTenant, controlHandedOverAt.Add(time.Hour))
		return err
	}
	inTx(t, fixture.transactor, t.Context(), executor)
	active := fixture.activeControls(t, "SYN-HU-61")
	if len(active) != 1 || active[0].EstablishmentKind() != domain.EstablishedByHandoverIn || active[0].Node().String() != "SYN-NODE-SZX-GATE" {
		t.Fatalf("转入成立的控制 = %v, want 在 SZX-GATE 的那一份", active)
	}
	if other := fixture.activeControls(t, "SYN-HU-61-OTHER"); len(other) != 1 {
		t.Fatalf("另一件实物的控制 = %d 份, want 1", len(other))
	}
}

// Covers: ADR-0214 决定三（迁移 0017 的形状 CHECK）——领域读回门守的几条时刻与种类，库面也守：转入推进的控制必须是交接转入成立的；
// 推进的转出时刻必须就是这一行的业务时间；转出不早于控制成立；交接早于控制的入账，业务时间不晚于控制成立。绕过领域直插的坏行断在
// 23514 与那一条约束上，不留在册上等读回时才响。
func TestTheTransitionRegisterRefusesRowsOfAShapeTheDomainWouldRefuse(t *testing.T) {
	fixture := newControlFixture(t)
	established, handedOver := controlHandedOverAt, controlHandedOverAt.Add(time.Hour)
	for name, row := range map[string]struct {
		direction, outcome, reason, kind string
		controlEstablishedAt             time.Time
		transferredAt                    *time.Time
		occurredAt                       time.Time
		constraint                       string
	}{
		"转入推进带收寄成立的控制": {"IN", "ADVANCED", "", "NODE_INTAKE", established, nil, handedOver,
			"physical_control_transition_in_carries_handover_in"},
		"推进的转出时刻不是业务时间": {"OUT", "ADVANCED", "", "NODE_INTAKE", established, &established, handedOver,
			"physical_control_transition_out_released_at_occurrence"},
		"转出早于控制成立": {"OUT", "ADVANCED", "", "NODE_INTAKE", handedOver, &established, established,
			"physical_control_transition_transfer_whole"},
		"交接早于控制的入账却晚于它": {"OUT", "NOT_ADVANCED", "HANDOVER_PRECEDES_CONTROL", "NODE_INTAKE", established, nil, handedOver,
			"physical_control_transition_reason_shape"},
	} {
		t.Run(name, func(t *testing.T) {
			transferReference := (*string)(nil)
			if row.transferredAt != nil {
				reference := "TRANSPORT-HANDOVER/SHAPE"
				transferReference = &reference
			}
			reason := (*string)(nil)
			if row.reason != "" {
				reason = &row.reason
			}
			err := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
				writer, err := fixture.db.RequireExecutor(txCtx)
				if err != nil {
					return err
				}
				_, err = writer.Exec(txCtx,
					`INSERT INTO node_operations.physical_control_transition
					     (tenant_id, handover_object, handover_scope, handover_version, direction, outcome, reason,
					      unit, node, control_kind, control_basis, control_established_at, transfer_reference, transferred_at,
					      occurred_at, recorded_at)
					 VALUES ($1, 'SYN-PARCEL-61-SHAPE', 'SYN-SCOPE-61', 'v1', $2, $3, $4,
					         'SYN-HU-61-SHAPE', 'SYN-NODE-SHA-HUB', $5, 'NODE-INTAKE/SHAPE', $6, $7, $8, $9, $9)`,
					controlTenant, row.direction, row.outcome, reason, row.kind, row.controlEstablishedAt,
					transferReference, row.transferredAt, row.occurredAt)
				return err
			})
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != row.constraint {
				t.Fatalf("直插坏行 err = %v, want 23514 撞 %s", err, row.constraint)
			}
		})
	}
}

// Covers: ADR-0214 决定二——交接对象先认集运单元（与包裹号同串时也按单元），再按正式包裹身份读各条判断链当前有效版本上的关联：被
// 接续了、接续版不带关联的那件不算；多件各算一件。
func TestHandoverObjectsResolveByConsolidationUnitThenCurrentAssociation(t *testing.T) {
	fixture := newControlFixture(t)
	tenant := ref(t, domain.NewTenantID, controlTenant)
	fixture.save(t,
		intakeWithControl(t, "SYN-SRC-61-E", "SYN-HU-61-E", "SYN-NODE-SHA-HUB", "SYN-INTAKEV-61-E", "SYN-PARCEL-61-ONE", "", "SYN-INTAKEV-61-E"),
		intakeWithControl(t, "SYN-SRC-61-F", "SYN-HU-61-F", "SYN-NODE-SHA-HUB", "SYN-INTAKEV-61-F", "SYN-PARCEL-61-TWO", "", "SYN-INTAKEV-61-F"),
		intakeWithControl(t, "SYN-SRC-61-G", "SYN-HU-61-G", "SYN-NODE-SHA-HUB", "SYN-INTAKEV-61-G", "SYN-PARCEL-61-TWO", "", "SYN-INTAKEV-61-G"),
		intakeWithControl(t, "SYN-SRC-61-H", "SYN-HU-61-H", "SYN-NODE-SHA-HUB", "SYN-INTAKEV-61-H", "SYN-PARCEL-61-CONTESTED", "", "SYN-INTAKEV-61-H"),
		intakeWithControl(t, "SYN-SRC-61-H2", "SYN-HU-61-H", "SYN-NODE-SHA-HUB", "SYN-INTAKEV-61-H2", "", "SYN-INTAKEV-61-H", "SYN-INTAKEV-61-H"),
	)
	inTx(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		writer, err := fixture.db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		_, err = writer.Exec(txCtx,
			`INSERT INTO node_operations.consolidation_unit (tenant_id, unit_id, asset_ref, phase, opened_source)
			 VALUES ($1, 'SYN-PARCEL-61-ONE', 'SYN-BAG-61', 'OPEN',
			         '{"sourceId":"SYN-OPEN-61","performedBy":"SYN-OPERATOR-61","evidence":"SYN-EVIDENCE-61","occurredAt":"2026-08-13T14:00:00Z"}')`,
			controlTenant)
		return err
	})

	for object, want := range map[string]struct {
		kind  ports.HandoverObjectKind
		units int
	}{
		"SYN-PARCEL-61-ONE":       {kind: ports.HandoverObjectConsolidationUnit},
		"SYN-PARCEL-61-TWO":       {kind: ports.HandoverObjectParcel, units: 2},
		"SYN-PARCEL-61-CONTESTED": {kind: ports.HandoverObjectParcel},
		"SYN-PARCEL-61-NONE":      {kind: ports.HandoverObjectParcel},
	} {
		resolution, err := fixture.transitions.ResolveHandoverObject(t.Context(), tenant, controlHandover(t, object, "v1"))
		if err != nil || resolution.Kind != want.kind || len(resolution.Units) != want.units {
			t.Fatalf("%s 认成 %s %v err=%v, want %s 与 %d 件", object, resolution.Kind, resolution.Units, err, want.kind, want.units)
		}
	}
}

// Covers: `AT-NO-110` 的锁面（ADR-0214 决定四）——读在身控制取的实物锁与收寄认当前收寄是同一把：一方握着 FindCurrentJudgmentsAtNode
// 的锁时，同一件实物的转出等在锁上（lock_timeout 量出 55P03），另一件实物不等。不在事务里调直接拒。
func TestTheTransferOutWaitsOnTheReceptionUnitLock(t *testing.T) {
	fixture := newControlFixture(t)
	tenant := ref(t, domain.NewTenantID, controlTenant)
	unit := ref(t, domain.NewHandlingUnitID, "SYN-HU-61")
	node := ref(t, domain.NewNodeReference, "SYN-NODE-SHA-HUB")
	otherUnit := ref(t, domain.NewHandlingUnitID, "SYN-HU-61-OTHER")

	if _, err := fixture.transitions.FindActiveControls(t.Context(), tenant, unit); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务读在身控制 err = %v, want ErrTransactionRequired", err)
	}

	holding, release, held := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		held <- fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
			if _, err := fixture.receptions.FindCurrentJudgmentsAtNode(txCtx, tenant, unit, node); err != nil {
				return err
			}
			close(holding)
			<-release
			return nil
		})
	}()
	var once sync.Once
	letGo := func() { once.Do(func() { close(release) }) }
	t.Cleanup(letGo)
	select {
	case <-holding:
	case err := <-held:
		t.Fatalf("握锁的一方没握住就结束了：%v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("握锁的一方没在时限内取到锁")
	}

	probe := func(asked domain.HandlingUnitID) error {
		return fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
			executor, err := fixture.db.RequireExecutor(txCtx)
			if err != nil {
				return err
			}
			if _, err := executor.Exec(txCtx, `SET LOCAL lock_timeout = '300ms'`); err != nil {
				return err
			}
			_, err = fixture.transitions.FindActiveControls(txCtx, tenant, asked)
			return err
		})
	}
	var pgErr *pgconn.PgError
	if err := probe(unit); !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
		t.Errorf("同一件实物 err = %v, want 等在收寄那把锁上（55P03）", err)
	}
	if err := probe(otherUnit); err != nil {
		t.Errorf("另一件实物被挡住了：%v", err)
	}
	letGo()
	if err := <-held; err != nil {
		t.Fatalf("握锁的一方：%v", err)
	}
}

// Covers: `AT-NO-112` 的存储面（ADR-0214 决定三、五）——转移册照行上列：转出与入账不推进同册，新近记录在前；不在场的格读成空、时刻
// 读成 nil，不编造。
func TestTheControlTransitionRegisterIsListed(t *testing.T) {
	fixture := newControlFixture(t)
	fixture.save(t, intakeWithControl(t, "SYN-SRC-61-J", "SYN-HU-61", "SYN-NODE-SHA-HUB", "SYN-INTAKEV-61-J", "SYN-PARCEL-61", "", "SYN-INTAKEV-61-J"))
	advanced := fixture.transferOut(t, "SYN-HU-61", "v1")
	tenant := ref(t, domain.NewTenantID, controlTenant)
	pending, err := domain.PendingControlSourceRecompute(advanced.Transition, controlHandover(t, "SYN-PARCEL-61", "v2"), controlHandedOverAt)
	if err != nil {
		t.Fatal(err)
	}
	unresolved, err := domain.ObjectNotResolved(tenant, controlHandover(t, "SYN-PARCEL-61-NONE", "v1"), domain.ControlTransitionOut,
		domain.NoAssociatedUnit, controlHandedOverAt)
	if err != nil {
		t.Fatal(err)
	}
	inTx(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		for index, transition := range []domain.ControlTransition{pending, unresolved} {
			record := ports.ControlTransitionRecord{Transition: transition, RecordedAt: controlHandedOverAt.Add(time.Duration(index+2) * time.Minute)}
			if _, err := fixture.transitions.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})

	rows, err := fixture.catalogue.ListPhysicalControlTransitions(t.Context(), tenant, 10)
	if err != nil || len(rows) != 3 {
		t.Fatalf("上列 %d 行 err=%v", len(rows), err)
	}
	if rows[0].Reason != "NO_ASSOCIATED_UNIT" || rows[0].Unit != "" || rows[0].ControlEstablishedAt != nil || rows[0].TransferredAt != nil {
		t.Fatalf("对不上实物那一行 = %+v", rows[0])
	}
	if rows[1].Reason != "CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE" || rows[1].HandoverVersion != "v2" ||
		rows[1].TransferReference != "TRANSPORT-HANDOVER/v1" || rows[1].Node != "SYN-NODE-SHA-HUB" {
		t.Fatalf("待重算那一行 = %+v", rows[1])
	}
	if rows[2].Outcome != "ADVANCED" || rows[2].Direction != "OUT" || rows[2].ControlKind != "NODE_INTAKE" ||
		rows[2].TransferredAt == nil || !rows[2].TransferredAt.Equal(controlHandedOverAt) {
		t.Fatalf("转出那一行 = %+v", rows[2])
	}
	if _, err := fixture.catalogue.ListPhysicalControlTransitions(t.Context(), tenant, 0); err == nil {
		t.Fatal("页大小为零该拒")
	}
}

// Covers: 转移落册只在事务里成立——转出与消费门的已处理、与实物锁同生共死；没有事务时拒，不改用连接池。
func TestControlTransitionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	fixture := newControlFixture(t)
	tenant := ref(t, domain.NewTenantID, controlTenant)
	transition, err := domain.ObjectNotResolved(tenant, controlHandover(t, "SYN-PARCEL-61", "v1"), domain.ControlTransitionOut,
		domain.NoAssociatedUnit, controlHandedOverAt)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.transitions.Save(t.Context(), ports.ControlTransitionRecord{Transition: transition, RecordedAt: controlHandedOverAt})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务落转移 err = %v, want ErrTransactionRequired", err)
	}
}
