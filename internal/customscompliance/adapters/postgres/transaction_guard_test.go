package postgres_test

import (
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// PBC-08 行为面负向证据（bento-gate-reeval 票 02）：本包漏网写口在无事务上下文必须被
// RequireExecutor 拒绝。个别写口把「封闭枚举之外即拒」的入参门放在守卫之前（结果层、
// 受控动作、舱单方向），那几格传有效枚举值让请求走到守卫；其余传零值。聚合类写口按
// 夹具族分两个测试函数，与本包既有测试的分文件粒度对齐。
func TestCaseConfigRegistrationsRefuseToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()
	registeredAt := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

	readiness, err := adapter.NewReadinessRegistrations(db)
	if err != nil {
		t.Fatalf("构造就绪登记册：%v", err)
	}
	if _, err := readiness.RegisterReadiness(ctx, domain.TenantID{}, domain.ReadinessJudgment{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记就绪判断应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := readiness.RevokeReadiness(ctx, domain.TenantID{}, domain.ReadinessJudgment{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务撤销就绪应返回 ErrTransactionRequired，实得：%v", err)
	}

	authorities, err := adapter.NewSubmissionAuthorityRegistrations(db)
	if err != nil {
		t.Fatalf("构造申报权威登记册：%v", err)
	}
	if _, err := authorities.GrantSubmissionAuthority(ctx, domain.TenantID{}, domain.SubmissionAuthorization{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务授予申报权威应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := authorities.RevokeSubmissionAuthority(ctx, domain.TenantID{}, domain.SubmissionAuthorization{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务撤销申报权威应返回 ErrTransactionRequired，实得：%v", err)
	}

	rules, err := adapter.NewInterpretationRuleRegistrations(db)
	if err != nil {
		t.Fatalf("构造解释规则登记册：%v", err)
	}
	if _, err := rules.RegisterInterpretationRule(ctx, domain.TenantID{}, domain.RegulatoryReceiptLayer, domain.RegulatoryJurisdictionReference{}, domain.InterpretationRuleReference{}, registeredAt); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记解释规则应返回 ErrTransactionRequired，实得：%v", err)
	}

	obligations, err := adapter.NewObligationInventoryRegistrations(db)
	if err != nil {
		t.Fatalf("构造义务清单登记册：%v", err)
	}
	if _, err := obligations.RegisterObligationCatalog(ctx, domain.TenantID{}, domain.CustomsCaseID{}, registeredAt); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记义务目录应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := obligations.RegisterObligationItem(ctx, domain.TenantID{}, domain.CustomsCaseID{}, ports.ObligationRegistration{AppliesFrom: registeredAt}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记义务项应返回 ErrTransactionRequired，实得：%v", err)
	}

	gates, err := adapter.NewGateConditionRegistrations(db)
	if err != nil {
		t.Fatalf("构造闸门条件登记册：%v", err)
	}
	if _, err := gates.RegisterGateCatalog(ctx, domain.TenantID{}, domain.DecisionScopeReference{}, domain.OutboundRelease, domain.CustomsProcedureReference{}, registeredAt); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记闸门目录应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := gates.RegisterGateFinding(ctx, domain.TenantID{}, domain.DecisionScopeReference{}, domain.OutboundRelease, domain.CustomsProcedureReference{}, domain.PreconditionFinding{State: domain.PreconditionMet}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记闸门核验应返回 ErrTransactionRequired，实得：%v", err)
	}

	requirements, err := adapter.NewCaseRequirementRegistrations(db)
	if err != nil {
		t.Fatalf("构造案件要求登记册：%v", err)
	}
	if _, err := requirements.RegisterCaseRequirementRule(ctx, domain.TenantID{}, domain.RegulatoryJurisdictionReference{}, domain.ImportManifest, domain.CustomsProcedureReference{}, ports.CaseRequirementJudgment{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记案件要求规则应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestCaseAggregateWritesRefuseToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()
	formedAt := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

	cases, err := adapter.NewCustomsCases(db)
	if err != nil {
		t.Fatalf("构造案件库：%v", err)
	}
	if _, err := cases.Save(ctx, ports.CustomsCaseKey{}, domain.CustomsCase{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存案件应返回 ErrTransactionRequired，实得：%v", err)
	}

	restrictions, err := adapter.NewRestrictions(db)
	if err != nil {
		t.Fatalf("构造限制库：%v", err)
	}
	if _, err := restrictions.Save(ctx, domain.TenantID{}, domain.RegulatoryRestriction{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存限制应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := restrictions.Update(ctx, domain.TenantID{}, domain.RegulatoryRestriction{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务更新限制应返回 ErrTransactionRequired，实得：%v", err)
	}

	verifications, err := adapter.NewGateVerifications(db)
	if err != nil {
		t.Fatalf("构造闸门核验库：%v", err)
	}
	if _, err := verifications.Save(ctx, ports.GateVerificationKey{}, domain.ReleaseGateVerification{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存闸门核验应返回 ErrTransactionRequired，实得：%v", err)
	}

	units, err := adapter.NewDeclarationUnits(db)
	if err != nil {
		t.Fatalf("构造申报单元库：%v", err)
	}
	if _, err := units.Save(ctx, domain.TenantID{}, domain.DeclarationUnit{}, formedAt); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存申报单元应返回 ErrTransactionRequired，实得：%v", err)
	}

	submissions, err := adapter.NewDeclarationSubmissions(db)
	if err != nil {
		t.Fatalf("构造申报提交库：%v", err)
	}
	if _, err := submissions.Save(ctx, ports.DeclarationSubmissionRecord{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存申报提交应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := submissions.SaveCorrection(ctx, ports.DeclarationSubmissionRecord{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存原案内更正应返回 ErrTransactionRequired，实得：%v", err)
	}

	dispositions, err := adapter.NewDispositionVerifications(db)
	if err != nil {
		t.Fatalf("构造处置核对库：%v", err)
	}
	if _, err := dispositions.Save(ctx, ports.VerificationKey{}, domain.DispositionVerification{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存处置核对应返回 ErrTransactionRequired，实得：%v", err)
	}

	followUps, err := adapter.NewFollowUps(db)
	if err != nil {
		t.Fatalf("构造后续义务库：%v", err)
	}
	if _, err := followUps.SaveTarget(ctx, ports.FollowUpTargetKey{}, domain.FollowUpTarget{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存后续义务对象应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := followUps.SaveRelation(ctx, ports.FollowUpTargetKey{}, domain.ReplacementRelation{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存替代关系应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := followUps.UpdateRelation(ctx, ports.FollowUpTargetKey{}, domain.ReplacementRelation{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务更新替代关系应返回 ErrTransactionRequired，实得：%v", err)
	}

	manifests, err := adapter.NewManifests(db)
	if err != nil {
		t.Fatalf("构造舱单库：%v", err)
	}
	if _, err := manifests.Save(ctx, domain.TenantID{}, domain.ExternalManifestReference{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存舱单应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := manifests.Update(ctx, domain.TenantID{}, domain.ExternalManifestReference{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务更新舱单应返回 ErrTransactionRequired，实得：%v", err)
	}

	closures, err := adapter.NewCaseClosures(db)
	if err != nil {
		t.Fatalf("构造案件关闭库：%v", err)
	}
	if _, err := closures.Save(ctx, domain.TenantID{}, &domain.CustomsCaseClosure{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存案件关闭应返回 ErrTransactionRequired，实得：%v", err)
	}
}
