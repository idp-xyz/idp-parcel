package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件对真实 PostgreSQL 16 证节点控制转移册的转入一半（ADR-0214 决定七与文末票 62 补记，迁移 0018）：转入立起的控制落行、读回、
// 进在身控制，之后被另一次交接按同一份依据转出、经折叠读成已转出；转出的交出节点信息格落行与读回；查阅册照行列出；0018 的形状
// CHECK。夹具全为合成（S 级），断言一律在事务闭包外。

func handoverOn(t *testing.T, object, scope, version string) domain.AuthoritativeHandoverReference {
	t.Helper()
	reference, err := domain.NewAuthoritativeHandoverReference(object, scope, version)
	if err != nil {
		t.Fatal(err)
	}
	return reference
}

// advance 照编排的次序在一笔事务里跑一版：先读在身控制判转出，再读各份控制判转入，各落一行。receiving 为空即不转入。
func (fixture *controlFixture) advance(
	t *testing.T, unit string, handover domain.AuthoritativeHandoverReference, releasing, receiving string, at time.Time,
) (out, in ports.ControlTransitionRecord) {
	t.Helper()
	tenant, handlingUnit := ref(t, domain.NewTenantID, controlTenant), ref(t, domain.NewHandlingUnitID, unit)
	declared := domain.NodeReference{}
	if releasing != "" {
		declared = ref(t, domain.NewNodeReference, releasing)
	}
	inTx(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		active, err := fixture.transitions.FindActiveControls(txCtx, tenant, handlingUnit)
		if err != nil {
			return err
		}
		transition, err := domain.JudgeTransferOut(tenant, handlingUnit, active, handover,
			ref(t, domain.NewTransferOutReference, "TRANSPORT-HANDOVER/"+handover.Version()), declared, at)
		if err != nil {
			return err
		}
		out = ports.ControlTransitionRecord{Transition: transition, RecordedAt: at.Add(time.Minute)}
		if _, err := fixture.transitions.Save(txCtx, out); err != nil {
			return err
		}
		if receiving == "" {
			return nil
		}
		known, err := fixture.transitions.FindUnitControls(txCtx, tenant, handlingUnit)
		if err != nil {
			return err
		}
		transition, err = domain.JudgeTransferIn(tenant, handlingUnit, known, handover, ref(t, domain.NewNodeReference, receiving), at)
		if err != nil {
			return err
		}
		in = ports.ControlTransitionRecord{Transition: transition, RecordedAt: at.Add(time.Minute)}
		_, err = fixture.transitions.Save(txCtx, in)
		return err
	})
	return out, in
}

func (fixture *controlFixture) unitControls(t *testing.T, unit string) []domain.PhysicalControl {
	t.Helper()
	tenant, handlingUnit := ref(t, domain.NewTenantID, controlTenant), ref(t, domain.NewHandlingUnitID, unit)
	var known []domain.PhysicalControl
	inTx(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		known, err = fixture.transitions.FindUnitControls(txCtx, tenant, handlingUnit)
		return err
	})
	return known
}

// Covers: `AT-NO-113`、`AT-NO-114`、`AT-NO-115`、`AT-NO-120`、`AT-NO-123` 的存储面（ADR-0214 决定三、四末条、七）——上海枢纽收寄在身，
// 节点之间直接交接进深圳口岸：同一笔先落转出、再落转入，转入行原样读回，在身控制只剩深圳口岸那一份，有过的各份连已转出的一起读得到；
// 之后另一次交接声明的交出节点不是深圳口岸，照样按同一份依据转出它、信息格落行并读回；查阅册列出转入行与信息格。
func TestATransferInIsStoredFoldedAndTransferredOutAgain(t *testing.T) {
	fixture := newControlFixture(t)
	tenant := ref(t, domain.NewTenantID, controlTenant)
	fixture.save(t, intakeWithControl(t, "SYN-SRC-62-A", "SYN-HU-62", "SYN-NODE-SHA-HUB", "SYN-INTAKEV-62-A", "SYN-PARCEL-62", "", "SYN-INTAKEV-62-A"))
	intoGateway := handoverOn(t, "SYN-PARCEL-62", "SYN-SCOPE-62-SZX", "SYN-HO-62-A/v1")

	out, in := fixture.advance(t, "SYN-HU-62", intoGateway, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE", controlHandedOverAt)

	if out.Transition.Outcome() != domain.ControlAdvanced || in.Transition.Outcome() != domain.ControlAdvanced {
		t.Fatalf("转出 %s、转入 %s %s", out.Transition.Outcome(), in.Transition.Outcome(), in.Transition.Reason())
	}
	read, found, err := fixture.transitions.FindTransition(t.Context(), tenant, intoGateway, domain.ControlTransitionIn)
	if err != nil || !found || read.Transition != in.Transition {
		t.Fatalf("转入行读回 = %+v found=%v err=%v", read, found, err)
	}
	inbound, _ := in.Transition.Control()
	if active := fixture.activeControls(t, "SYN-HU-62"); len(active) != 1 || active[0] != inbound {
		t.Fatalf("在身控制 = %v, want 只剩深圳口岸那一份", active)
	}
	if known := fixture.unitControls(t, "SYN-HU-62"); len(known) != 2 {
		t.Fatalf("有过的控制 = %v, want 上海那一份（已转出）与深圳那一份", known)
	}

	onward := handoverOn(t, "SYN-PARCEL-62", "SYN-SCOPE-62-TRUNK", "SYN-HO-62-B/v1")
	later := controlHandedOverAt.Add(4 * time.Hour)
	again, _ := fixture.advance(t, "SYN-HU-62", onward, "SYN-NODE-SIN-HUB", "", later)

	released, _ := again.Transition.Control()
	if node, noted := again.Transition.ReleasingNodeMismatch(); again.Transition.Outcome() != domain.ControlAdvanced ||
		released.Basis() != inbound.Basis() || !noted || node.String() != "SYN-NODE-SIN-HUB" {
		t.Fatalf("再转出 = %s，转出 %q，信息格 %q noted=%v", again.Transition.Outcome(), released.Basis(), node, noted)
	}
	readAgain, _, err := fixture.transitions.FindTransition(t.Context(), tenant, onward, domain.ControlTransitionOut)
	if err != nil || readAgain.Transition != again.Transition {
		t.Fatalf("带信息格的转出读回 = %+v err = %v", readAgain, err)
	}
	if active := fixture.activeControls(t, "SYN-HU-62"); len(active) != 0 {
		t.Fatalf("转入立起的那一份被转出之后仍有 %d 份在身", len(active))
	}

	rows, err := fixture.catalogue.ListPhysicalControlTransitions(t.Context(), tenant, 10)
	if err != nil || len(rows) != 3 {
		t.Fatalf("上列 %d 行 err=%v", len(rows), err)
	}
	if rows[0].MismatchedReleasingNode != "SYN-NODE-SIN-HUB" || rows[0].ControlKind != "HANDOVER_IN" {
		t.Fatalf("再转出那一行 = %+v", rows[0])
	}
	var inRow ports.PhysicalControlTransitionCatalogueRow
	for _, row := range rows[1:] {
		if row.Direction == "IN" {
			inRow = row
		} else if row.MismatchedReleasingNode != "" {
			t.Fatalf("声明相符的转出不该带信息格：%+v", row)
		}
	}
	if inRow.Outcome != "ADVANCED" || inRow.Node != "SYN-NODE-SZX-GATE" || inRow.ControlKind != "HANDOVER_IN" ||
		inRow.ControlBasis != inbound.Basis().String() || inRow.TransferReference != "" || inRow.TransferredAt != nil {
		t.Fatalf("转入那一行 = %+v", inRow)
	}
}

// Covers: `AT-NO-118` 的存储面与 PBC-08 负向证据（ADR-0214 决定七落地补记）——读各份控制取的是同一把实物锁，不在事务里调直接拒；
// 实物仍有在身控制时转入入账「仍有在身控制」，只带实物，读回同形。
func TestATransferInOverAnActiveControlIsRecordedAndUnitControlsNeedATransaction(t *testing.T) {
	fixture := newControlFixture(t)
	tenant := ref(t, domain.NewTenantID, controlTenant)
	if _, err := fixture.transitions.FindUnitControls(t.Context(), tenant, ref(t, domain.NewHandlingUnitID, "SYN-HU-62")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务读各份控制 err = %v, want ErrTransactionRequired", err)
	}
	fixture.save(t,
		intakeWithControl(t, "SYN-SRC-62-B", "SYN-HU-62", "SYN-NODE-SHA-HUB", "SYN-INTAKEV-62-B", "SYN-PARCEL-62", "", "SYN-INTAKEV-62-B"),
		intakeWithControl(t, "SYN-SRC-62-C", "SYN-HU-62", "SYN-NODE-SZX-GATE", "SYN-INTAKEV-62-C", "SYN-PARCEL-62", "", "SYN-INTAKEV-62-C"),
	)
	handover := handoverOn(t, "SYN-PARCEL-62", "SYN-SCOPE-62-SIN", "SYN-HO-62-C/v1")

	out, in := fixture.advance(t, "SYN-HU-62", handover, "", "SYN-NODE-SIN-HUB", controlHandedOverAt)

	if out.Transition.Reason() != domain.SeveralActiveControls || in.Transition.Reason() != domain.ControlStillActive {
		t.Fatalf("转出 %s、转入 %s", out.Transition.Reason(), in.Transition.Reason())
	}
	read, found, err := fixture.transitions.FindTransition(t.Context(), tenant, handover, domain.ControlTransitionIn)
	if err != nil || !found || read.Transition != in.Transition {
		t.Fatalf("读回 = %+v found=%v err=%v", read, found, err)
	}
}

// Covers: 迁移 0018 的形状 CHECK（ADR-0214 决定四末条、决定七落地补记）——领域读回门守的几条库面也守：信息格只在推进了的转出上、且
// 不等于控制的节点；转入立起的控制成立时刻就是业务时间；只属转入的原因不落在转出上；转入一侧的待重算带的是交接转入成立的控制。绕过
// 领域直插的坏行断在 23514 与那一条约束上。
func TestTheTransitionRegisterRefusesMisshapenTransfersIn(t *testing.T) {
	fixture := newControlFixture(t)
	at := controlHandedOverAt
	later := at.Add(time.Hour)
	for name, row := range map[string]struct {
		direction, outcome, reason, kind, mismatched string
		establishedAt                                time.Time
		constraint                                   string
	}{
		"信息格在转入上": {"IN", "ADVANCED", "", "HANDOVER_IN", "SYN-NODE-SIN-HUB", at,
			"physical_control_transition_mismatched_releasing_node_shape"},
		"信息格就是控制的节点": {"OUT", "ADVANCED", "", "NODE_INTAKE", "SYN-NODE-SZX-GATE", at,
			"physical_control_transition_mismatched_releasing_node_shape"},
		"转入成立时刻不是业务时间": {"IN", "ADVANCED", "", "HANDOVER_IN", "", later,
			"physical_control_transition_in_established_at_occurrence"},
		"仍有在身控制落在转出上": {"OUT", "NOT_ADVANCED", "CONTROL_STILL_ACTIVE", "", "", time.Time{},
			"physical_control_transition_reason_shape"},
		"转入待重算带收寄成立的控制": {"IN", "NOT_ADVANCED", "CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE", "NODE_INTAKE", "", at,
			"physical_control_transition_reason_shape"},
	} {
		t.Run(name, func(t *testing.T) {
			optional := func(value string) *string {
				if value == "" {
					return nil
				}
				return &value
			}
			var node, basis *string
			var establishedAt *time.Time
			if row.kind != "" {
				node, basis, establishedAt = optional("SYN-NODE-SZX-GATE"), optional("HANDOVER-IN/SHAPE"), &row.establishedAt
			}
			var transferReference *string
			var transferredAt *time.Time
			if row.direction == "OUT" && row.outcome == "ADVANCED" {
				transferReference, transferredAt = optional("TRANSPORT-HANDOVER/SHAPE"), &at
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
					      occurred_at, recorded_at, mismatched_releasing_node)
					 VALUES ($1, 'SYN-PARCEL-62-SHAPE', 'SYN-SCOPE-62', 'v1', $2, $3, $4,
					         'SYN-HU-62-SHAPE', $5, $6, $7, $8, $9, $10, $11, $11, $12)`,
					controlTenant, row.direction, row.outcome, optional(row.reason), node, optional(row.kind), basis,
					establishedAt, transferReference, transferredAt, at, optional(row.mismatched))
				return err
			})
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != row.constraint {
				t.Fatalf("直插坏行 err = %v, want 23514 撞 %s", err, row.constraint)
			}
		})
	}
}
