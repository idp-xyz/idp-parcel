package postgres_test

import (
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// PBC-08 行为面负向证据（bento-gate-reeval 票 02）：本包漏网写口在无事务上下文必须被
// RequireExecutor 拒绝。拒绝先于任何入参解读，所以传零值/nil 就够；留痕口的完整性门
// 在守卫之前，那一格传完整轨迹。按对象族分三个测试函数，与本包既有测试的分文件
// 粒度对齐。
func TestFactAndProjectionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()

	facts, err := adapter.NewAcceptedFacts(db)
	if err != nil {
		t.Fatalf("构造事实库：%v", err)
	}
	if _, err := facts.Save(ctx, ports.FactRecord{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存已接受事实应返回 ErrTransactionRequired，实得：%v", err)
	}

	projections, err := adapter.NewProjections(db)
	if err != nil {
		t.Fatalf("构造投影库：%v", err)
	}
	if err := projections.Save(ctx, domain.TenantID{}, domain.TrackingProjection{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存投影应返回 ErrTransactionRequired，实得：%v", err)
	}

	episodes, err := adapter.NewSignalEpisodes(db)
	if err != nil {
		t.Fatalf("构造信号发作期库：%v", err)
	}
	if err := episodes.SaveRaised(ctx, ports.RaisedSignalRecord{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存已升起信号应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := episodes.SaveHit(ctx, domain.TenantID{}, &domain.SignalEpisode{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存信号命中应返回 ErrTransactionRequired，实得：%v", err)
	}

	etas, err := adapter.NewETAs(db)
	if err != nil {
		t.Fatalf("构造 ETA 库：%v", err)
	}
	if err := etas.Save(ctx, domain.TenantID{}, domain.ETAPrediction{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存 ETA 应返回 ErrTransactionRequired，实得：%v", err)
	}

	gaps, err := adapter.NewVisibilityGaps(db)
	if err != nil {
		t.Fatalf("构造可见性缺口库：%v", err)
	}
	if err := gaps.Save(ctx, domain.TenantID{}, domain.VisibilityGap{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存可见性缺口应返回 ErrTransactionRequired，实得：%v", err)
	}

	notifications, err := adapter.NewCustomerNotifications(db)
	if err != nil {
		t.Fatalf("构造客户通知库：%v", err)
	}
	if _, err := notifications.Save(ctx, domain.TenantID{}, &domain.CustomerNotification{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存客户通知应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestClaimAndDispositionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()

	claims, err := adapter.NewClaims(db)
	if err != nil {
		t.Fatalf("构造索赔库：%v", err)
	}
	if _, err := claims.Save(ctx, domain.TenantID{}, &domain.ClaimItem{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存索赔项应返回 ErrTransactionRequired，实得：%v", err)
	}

	recoveries, err := adapter.NewRecoveries(db)
	if err != nil {
		t.Fatalf("构造追偿库：%v", err)
	}
	if _, err := recoveries.Save(ctx, domain.TenantID{}, domain.RecoveryMatter{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存追偿事项应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := recoveries.AppendAction(ctx, domain.TenantID{}, domain.RecoveryAction{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务追加追偿动作应返回 ErrTransactionRequired，实得：%v", err)
	}

	dispositions, err := adapter.NewDispositionRequests(db)
	if err != nil {
		t.Fatalf("构造处置请求库：%v", err)
	}
	if _, err := dispositions.Save(ctx, domain.TenantID{}, &domain.DispositionRequest{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存处置请求应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := dispositions.SaveSupersession(ctx, domain.TenantID{}, &domain.DispositionRequest{}, &domain.DispositionRequest{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存处置替代关系应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestCatalogAndTraceWritesRefuseToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()

	registrar, err := adapter.NewCatalogRegistrar(db)
	if err != nil {
		t.Fatalf("构造目录登记器：%v", err)
	}
	if _, err := registrar.RegisterMilestoneMapping(ctx, domain.TenantID{}, ports.MilestoneMappingRegistration{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记里程碑映射应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := registrar.RegisterTriageRules(ctx, domain.TenantID{}, ports.TriageRuleRegistration{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记分诊规则应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := registrar.RegisterDisclosurePolicy(ctx, domain.TenantID{}, ports.DisclosurePolicyRegistration{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记披露规则应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := registrar.RegisterNotificationPolicy(ctx, domain.TenantID{}, ports.NotificationPolicyRegistration{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记通知策略应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := registrar.RegisterClaimEligibility(ctx, domain.TenantID{}, ports.ClaimEligibilityRegistration{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记索赔资格目录应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := registrar.RegisterClaimAuthorization(ctx, domain.TenantID{}, ports.ClaimAuthorizationRegistration{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记索赔授权目录应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := registrar.RegisterExceptionDisclosureRules(ctx, domain.TenantID{}, ports.ExceptionDisclosureRuleRegistration{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记异常披露规则应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := registrar.RegisterConflictSignalRule(ctx, domain.TenantID{}, ports.ConflictSignalRuleRegistration{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记冲突信号规则应返回 ErrTransactionRequired，实得：%v", err)
	}

	decisions, err := adapter.NewDisclosureDecisions(db)
	if err != nil {
		t.Fatalf("构造披露决定登记册：%v", err)
	}
	if _, err := decisions.Save(ctx, domain.TenantID{}, domain.DisclosureDecision{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存披露决定应返回 ErrTransactionRequired，实得：%v", err)
	}

	evidence, err := adapter.NewEvidenceItems(db)
	if err != nil {
		t.Fatalf("构造证据登记册：%v", err)
	}
	if _, err := evidence.Save(ctx, domain.TenantID{}, domain.EvidenceItem{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存证据项应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := evidence.SaveDisclosure(ctx, domain.TenantID{}, domain.EvidenceDisclosureVersion{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存证据披露版本应返回 ErrTransactionRequired，实得：%v", err)
	}

	receipts, err := adapter.NewMaterialReceiptRegistrar(db)
	if err != nil {
		t.Fatalf("构造归集面写入方：%v", err)
	}
	if _, err := receipts.RegisterReceipt(ctx, domain.TenantID{}, ports.MaterialReceipt{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记材料收讫应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := receipts.RevokeReceipt(ctx, domain.TenantID{}, ports.MaterialReceiptRevocation{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务撤销材料收讫应返回 ErrTransactionRequired，实得：%v", err)
	}

	executions, err := adapter.NewChannelExecutions(db)
	if err != nil {
		t.Fatalf("构造留痕库：%v", err)
	}
	trace := adapter.ChannelExecution{
		Command:         "register-milestone-mapping",
		RecordReference: "mapping-ntx",
		OSUser:          "operator",
		Hostname:        "workstation",
		Outcome:         "REGISTERED",
		ExecutedAt:      time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC),
	}
	if err := executions.Append(ctx, trace); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务追加留痕应返回 ErrTransactionRequired，实得：%v", err)
	}
}
