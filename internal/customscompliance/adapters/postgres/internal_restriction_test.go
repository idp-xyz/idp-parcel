package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 内部合规限制册在真库上（隔离合成 S）：建立与决定经生产适配器和编排落 0028 的四张表，准入判断读监管限制与内部合规限制
// 两本册。门禁与放行用显式 SQL 种好，看清准入读到的是库里的什么。

const irSource = "SYN-SOURCE-TRADE-COMPLIANCE"

type internalRestrictionDB struct {
	*admissionDB
	store   *adapter.InternalRestrictions
	handler *application.ManageInternalRestrictionHandler
}

func newInternalRestrictionDB(t *testing.T) *internalRestrictionDB {
	t.Helper()
	db := newAdmissionDB(t)
	store, err := adapter.NewInternalRestrictions(db.fixture.db)
	if err != nil {
		t.Fatalf("构造适配器：%v", err)
	}
	handler, err := application.NewManageInternalRestrictionHandler(application.ManageInternalRestrictionDeps{Store: store})
	if err != nil {
		t.Fatalf("构造编排：%v", err)
	}
	return &internalRestrictionDB{admissionDB: db, store: store, handler: handler}
}

func (db *internalRestrictionDB) inTransaction(call func(context.Context) error) {
	db.t.Helper()
	if err := db.fixture.db.Transactor().WithinTransaction(db.t.Context(), call); err != nil {
		db.t.Fatalf("事务：%v", err)
	}
}

func (db *internalRestrictionDB) establish(id string, standing domain.CoverageStanding, actions ...domain.GuardedAction) application.InternalRestrictionOutcome {
	db.t.Helper()
	spec := domain.InternalRestrictionSpec{
		ID:        viewValue(db.t, domain.NewInternalRestrictionID, id),
		Source:    viewValue(db.t, domain.NewRestrictionSource, irSource),
		Basis:     viewValue(db.t, domain.NewRestrictionBasis, "SYN-RULE-DUAL-USE@v3"),
		Mode:      domain.ManualDecision,
		DecidedBy: viewValue(db.t, domain.NewRestrictionDecider, "SYN-ROLE-REVIEWER"),
		Condition: viewValue(db.t, domain.NewRestrictionReleaseCondition, "SYN-COND-LICENCE"),
		StartedAt: admissionDBAt.Add(-3 * time.Hour),
	}
	for _, action := range actions {
		spec.Coverage = append(spec.Coverage, domain.CoverageSpec{
			Scope:    viewValue(db.t, domain.NewDecisionScopeReference, admissionParcel),
			Action:   action,
			Standing: standing,
		})
	}
	var outcome application.InternalRestrictionOutcome
	db.inTransaction(func(ctx context.Context) error {
		result, err := db.handler.Establish(ctx, application.EstablishInternalRestrictionCommand{
			TenantID: viewValue(db.t, domain.NewTenantID, admissionTenant), Spec: spec,
		})
		outcome = result.Outcome()
		return err
	})
	return outcome
}

func (db *internalRestrictionDB) decide(restriction, decision, source string, kind domain.RestrictionDecisionKind, at time.Time, action domain.GuardedAction) application.InternalRestrictionOutcome {
	db.t.Helper()
	var outcome application.InternalRestrictionOutcome
	db.inTransaction(func(ctx context.Context) error {
		result, err := db.handler.Decide(ctx, application.DecideInternalRestrictionCommand{
			TenantID:      viewValue(db.t, domain.NewTenantID, admissionTenant),
			RestrictionID: viewValue(db.t, domain.NewInternalRestrictionID, restriction),
			Spec: domain.RestrictionDecisionSpec{
				ID:        viewValue(db.t, domain.NewRestrictionDecisionID, decision),
				Kind:      kind,
				Source:    viewValue(db.t, domain.NewRestrictionSource, source),
				Evidence:  viewValue(db.t, domain.NewRestrictionEvidence, "SYN-EVIDENCE-LICENCE-4711"),
				DecidedBy: viewValue(db.t, domain.NewRestrictionDecider, "SYN-ROLE-REVIEWER"),
				Authority: viewValue(db.t, domain.NewRestrictionAuthority, "SYN-DELEGATION-2026-07"),
				At:        at,
				Covers: []domain.CoverageKey{{
					Scope:  viewValue(db.t, domain.NewDecisionScopeReference, admissionParcel),
					Action: action,
				}},
			},
		})
		outcome = result.Outcome()
		return err
	})
	return outcome
}

func expectIROutcome(t *testing.T, got, want application.InternalRestrictionOutcome) {
	t.Helper()
	if got != want {
		t.Fatalf("结果 = %s，要 %s", got, want)
	}
}

// 准入读两本册：内部合规限制待确认答未决并指名那条限制，确认晚于动作时点仍待确认；另一条有效的内部合规限制答阻断；监管限制加进来
// 两条依据各列各的；只解除内部合规限制后监管限制仍阻断。
// Covers: AT-CC-449、AT-CC-451（真库半边）。
func TestAdmissionOnRealPostgresReadsBothRestrictionRegisters(t *testing.T) {
	db := newInternalRestrictionDB(t)
	db.demoZones()
	db.inUnit()
	db.gate("LOADING_DEPARTURE", true, map[string]string{"SYN-PRE-EXPORT-DECL-RELEASED": "MET"})
	db.release("SYN-SRC-FULL", "FULL", admissionDBAt.Add(-time.Hour), false)
	loading := func() domain.ActionAdmissionFinding {
		_, judgement, _ := db.judge("SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE")
		return admissionFinding(t, judgement, domain.LoadingDeparture)
	}

	expectAdmission(t, loading(), domain.AdmissionAdmitted, domain.CauseReleasedInFull)

	expectIROutcome(t, db.establish("SYN-IR-1", domain.CoverageUnconfirmed, domain.LoadingDeparture), application.InternalRestrictionEstablished)
	pending := loading()
	expectAdmission(t, pending, domain.AdmissionUndecided, domain.CauseInternalRestrictionUnconfirmed)
	if pending.Bases[0].Sources[0] != "SYN-IR-1" {
		t.Fatalf("待确认要指名那条限制：%+v", pending.Bases)
	}

	expectIROutcome(t, db.decide("SYN-IR-1", "D-1", irSource, domain.ConfirmCoverage, admissionDBAt.Add(time.Hour), domain.LoadingDeparture),
		application.InternalRestrictionDecided)
	expectAdmission(t, loading(), domain.AdmissionUndecided, domain.CauseInternalRestrictionUnconfirmed)

	expectIROutcome(t, db.establish("SYN-IR-2", domain.CoverageInForce, domain.LoadingDeparture), application.InternalRestrictionEstablished)
	blocked := loading()
	expectAdmission(t, blocked, domain.AdmissionBlocked, domain.CauseInternalRestrictionInForce)
	if len(blocked.Bases) != 1 || blocked.Bases[0].Sources[0] != "SYN-IR-2" {
		t.Fatalf("阻断只列挡住它的那条：%+v", blocked.Bases)
	}

	db.restrictParcel("SYN-RESTRICTION-IR-01", "LOADING_DEPARTURE")
	both := loading()
	expectAdmission(t, both, domain.AdmissionBlocked, domain.CauseRestrictionInForce)
	if len(both.Bases) != 2 || both.Bases[1].Cause != domain.CauseInternalRestrictionInForce {
		t.Fatalf("两种限制的依据各列各的：%+v", both.Bases)
	}

	expectIROutcome(t, db.decide("SYN-IR-2", "D-2", "SYN-SOURCE-ADMIN", domain.ReleaseCoverage, admissionDBAt.Add(-30*time.Minute), domain.LoadingDeparture),
		application.InternalRestrictionNotResponsibleSource)
	expectIROutcome(t, db.decide("SYN-IR-2", "D-2", irSource, domain.ReleaseCoverage, admissionDBAt.Add(-30*time.Minute), domain.LoadingDeparture),
		application.InternalRestrictionDecided)
	afterRelease := loading()
	expectAdmission(t, afterRelease, domain.AdmissionBlocked, domain.CauseRestrictionInForce)
	if len(afterRelease.Bases) != 1 {
		t.Fatalf("只解除内部合规限制后监管限制仍阻断：%+v", afterRelease.Bases)
	}
}

// 册的读回与并发护栏：限制与决定经领域重演读回；同标识同内容重放答已有、异内容答冲突；同一覆盖项第二次解除被拒；按范围盘出时
// 别的范围读空册。
// Covers: AT-CC-452（真库半边）。
func TestInternalRestrictionRegisterRoundTripsDecisionsAndGuardsReplay(t *testing.T) {
	db := newInternalRestrictionDB(t)
	expectIROutcome(t, db.establish("SYN-IR-RT", domain.CoverageConflicting, domain.LoadingDeparture, domain.FinalDelivery), application.InternalRestrictionEstablished)
	expectIROutcome(t, db.establish("SYN-IR-RT", domain.CoverageConflicting, domain.LoadingDeparture, domain.FinalDelivery), application.InternalRestrictionExisting)
	expectIROutcome(t, db.establish("SYN-IR-RT", domain.CoverageInForce, domain.LoadingDeparture, domain.FinalDelivery), application.InternalRestrictionContentConflict)

	at := admissionDBAt.Add(-time.Hour)
	expectIROutcome(t, db.decide("SYN-IR-RT", "D-1", irSource, domain.ConfirmCoverage, at, domain.LoadingDeparture), application.InternalRestrictionDecided)
	expectIROutcome(t, db.decide("SYN-IR-RT", "D-1", irSource, domain.ConfirmCoverage, at, domain.LoadingDeparture), application.InternalRestrictionDecisionExisting)
	expectIROutcome(t, db.decide("SYN-IR-RT", "D-1", irSource, domain.ReleaseCoverage, at, domain.LoadingDeparture), application.InternalRestrictionDecisionContentConflict)
	expectIROutcome(t, db.decide("SYN-IR-RT", "D-2", irSource, domain.ReleaseCoverage, at.Add(time.Minute), domain.LoadingDeparture), application.InternalRestrictionDecided)
	expectIROutcome(t, db.decide("SYN-IR-RT", "D-3", irSource, domain.ReleaseCoverage, at.Add(2*time.Minute), domain.LoadingDeparture), application.InternalRestrictionDecisionRejected)
	expectIROutcome(t, db.decide("SYN-IR-NONE", "D-4", irSource, domain.ReleaseCoverage, at, domain.LoadingDeparture), application.InternalRestrictionNotFound)

	tenant := viewValue(t, domain.NewTenantID, admissionTenant)
	var listed, elsewhere []domain.InternalRestriction
	db.inTransaction(func(ctx context.Context) error {
		var err error
		listed, err = db.store.ListInternalByScope(ctx, tenant, viewValue(t, domain.NewDecisionScopeReference, admissionParcel))
		if err != nil {
			return err
		}
		elsewhere, err = db.store.ListInternalByScope(ctx, tenant, viewValue(t, domain.NewDecisionScopeReference, "SYN-PCL-OTHER"))
		return err
	})
	if len(listed) != 1 || len(elsewhere) != 0 {
		t.Fatalf("按范围盘出 = %d / 别的范围 = %d", len(listed), len(elsewhere))
	}
	decisions := listed[0].Decisions()
	if len(decisions) != 2 || decisions[0].ID().String() != "D-1" || decisions[1].ID().String() != "D-2" {
		t.Fatalf("决定要按接受次序读回：%+v", decisions)
	}
	if len(listed[0].Coverage()) != 2 {
		t.Fatalf("覆盖项逐项读回：%+v", listed[0].Coverage())
	}
}

// Covers: 写口在无事务上下文一律被 RequireExecutor 拒绝（ErrTransactionRequired），不退回连接池旁路写入；对限制头取行锁的读回
// 也一样——锁只在事务里成立，无事务时不能悄悄读一份不带锁的。
func TestInternalRestrictionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	db := newInternalRestrictionDB(t)
	tenant := viewValue(t, domain.NewTenantID, admissionTenant)
	id := viewValue(t, domain.NewInternalRestrictionID, "SYN-IR-NOTX")
	scope := viewValue(t, domain.NewDecisionScopeReference, admissionParcel)
	restriction, err := domain.EstablishInternalRestriction(domain.InternalRestrictionSpec{
		ID:        id,
		Source:    viewValue(t, domain.NewRestrictionSource, irSource),
		Basis:     viewValue(t, domain.NewRestrictionBasis, "SYN-RULE-DUAL-USE@v3"),
		Mode:      domain.ManualDecision,
		DecidedBy: viewValue(t, domain.NewRestrictionDecider, "SYN-ROLE-REVIEWER"),
		Condition: viewValue(t, domain.NewRestrictionReleaseCondition, "SYN-COND-LICENCE"),
		StartedAt: admissionDBAt.Add(-time.Hour),
		Coverage:  []domain.CoverageSpec{{Scope: scope, Action: domain.LoadingDeparture, Standing: domain.CoverageInForce}},
	})
	if err != nil {
		t.Fatalf("构造限制：%v", err)
	}
	decision, err := domain.NewRestrictionDecision(domain.RestrictionDecisionSpec{
		ID:        viewValue(t, domain.NewRestrictionDecisionID, "D-NOTX"),
		Kind:      domain.ReleaseCoverage,
		Source:    viewValue(t, domain.NewRestrictionSource, irSource),
		Evidence:  viewValue(t, domain.NewRestrictionEvidence, "SYN-EVIDENCE-1"),
		DecidedBy: viewValue(t, domain.NewRestrictionDecider, "SYN-ROLE-REVIEWER"),
		Authority: viewValue(t, domain.NewRestrictionAuthority, "SYN-DELEGATION-1"),
		At:        admissionDBAt,
		Covers:    []domain.CoverageKey{{Scope: scope, Action: domain.LoadingDeparture}},
	})
	if err != nil {
		t.Fatalf("构造决定：%v", err)
	}

	if _, err := db.store.Save(t.Context(), tenant, restriction); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务建立内部合规限制应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, _, err := db.store.FindForDecision(t.Context(), tenant, id); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务对限制取锁读回应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := db.store.AppendDecision(t.Context(), tenant, id, decision); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务追加决定应返回 ErrTransactionRequired，实得：%v", err)
	}
}
