package main

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	psparty "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	shipmentports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件是票 label-channel/30 完成判据 4、5 的真库半边：生产装配在授权映射未配置时如实停下、什么都不写；
// 缝配上合成映射与合成授权后，关闭 → 重开走通、决定与判断意图同事务落库；入队失败整步回滚；不在事务里调编排
// 写口即拒。测试输入全是隔离合成，只记 `S`，不进生产装配。

const (
	synContinuedAttemptTenant      = "SYN-TENANT-CA"
	synContinuedAttemptScope       = "SYN-SCOPE-CA"
	synContinuedAttemptLegalEntity = "SYN-LEGAL-CA"
	synContinuedAttemptLevel       = "SYN-LEVEL-OPS-CA"
	synContinuedAttemptOperator    = "SYN-OPERATOR-ROLE-CA"
)

var continuedAttemptSeedAt = time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)

// synContinuedAttemptRequestSource 是实例半边映射的合成替身：不管问什么，都交回与 seedSYNContinuedAttemptGrant
// 同键的坐标。生产为 nil，这里配上只为走到落册之后。
type synContinuedAttemptRequestSource struct{ t *testing.T }

func (source synContinuedAttemptRequestSource) FormRequestCoordinates(
	context.Context,
	shipmentports.ContinuedAttemptDecisionAuthorizationQuery,
) (psparty.ContinuedAttemptDecisionRequestCoordinates, bool, error) {
	source.t.Helper()
	return psparty.ContinuedAttemptDecisionRequestCoordinates{
		OperatorRole: mustValue(source.t, pcdomain.NewOperatorRoleReference, synContinuedAttemptOperator),
		LegalEntity:  mustValue(source.t, pcdomain.NewLegalEntityReference, synContinuedAttemptLegalEntity),
		Level:        mustValue(source.t, pcdomain.NewAuthorityLevel, synContinuedAttemptLevel),
		Scope:        mustValue(source.t, pcdomain.NewCommercialScopeReference, synContinuedAttemptScope),
		Evidence:     mustValue(source.t, pcdomain.NewEvidenceReference, "SYN-EVIDENCE-CA-1"),
	}, true, nil
}

type failingContinuedAttemptHandoff struct{}

func (failingContinuedAttemptHandoff) HandOffContinuedAttemptDecision(context.Context, shipmentports.ContinuedAttemptDecisionHandoffIntent) error {
	return errors.New("outbox unavailable (synthetic)")
}

// Covers: 判据 4 前半与红线「无授权规则 → 未决不是拒绝」在生产装配上的形——映射未配置（实例半边）时编排停在
// `未决 · 授权口不可用`，不代拟坐标、不冒充`授权规则未配置`、不默认任何角色有关闭权；册上无行、outbox 无封。
func TestTheWiredContinuedAttemptDecisionStopsHonestlyWhenTheAuthorizationMappingIsNotConfigured(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	decisions, err := buildContinuedAttemptDecisionOrchestration(db)
	if err != nil {
		t.Fatalf("装配关闭 / 重开编排：%v", err)
	}
	identity, parcel := continuedAttemptIdentity(t, "SYN-PARCEL-CA-UNCONFIGURED")

	result, err := decisions.FormControlledClosure(t.Context(), continuedAttemptClosureCommand(t, identity, parcel))
	if err != nil {
		t.Fatalf("形成关闭：%v", err)
	}
	if result.Outcome() != shipmentapp.ContinuedAttemptDecisionUndecided ||
		result.PendingReason() != shipmentapp.ContinuedAttemptAuthorityUnavailable {
		t.Fatalf("outcome / pending = %v / %v, want UNDECIDED / AUTHORITY_UNAVAILABLE——映射未配置时只能停下", result.Outcome(), result.PendingReason())
	}
	if _, found := continuedAttemptRegisterOf(t, db, identity.TenantID(), parcel); found {
		t.Fatal("未决却开了册")
	}
	if got := envelopeCountOfType(t, db, pspostgres.ContinuedAttemptDecisionEventType); got != 0 {
		t.Fatalf("outbox 里有 %d 封判断意图, want 0", got)
	}
}

// Covers: 判据 1、2、4、5 的真库半边——缝配上合成映射、真授权册登了关闭与重开两条 grant 后：关闭落册（四件取自 PC：
// 决定方是运营角色、授权角色是 grant 的等级、授权依据快照是 grant 版本引用；截断边界 = 决定标识）并同事务入队一封
// 事件 ID 含决定标识的判断意图；重开追加成第二条、第二封；裸编排不在事务里调用即拒；入队失败整步回滚、册上无行。
func TestTheWiredContinuedAttemptDecisionWalksCloseAndReopenAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seedSYNContinuedAttemptGrant(t, db, pcdomain.ControlledClosureAction, "SYN-AUTH-CLOSE-CA")
	seedSYNContinuedAttemptGrant(t, db, pcdomain.ReopeningAction, "SYN-AUTH-REOPEN-CA")

	orchestration, err := buildContinuedAttemptDecisionOrchestrationWith(db, continuedAttemptDecisionSeams{
		Requests: synContinuedAttemptRequestSource{t: t},
	})
	if err != nil {
		t.Fatalf("装配关闭 / 重开编排：%v", err)
	}
	identity, parcel := continuedAttemptIdentity(t, "SYN-PARCEL-CA-1")

	closed, err := orchestration.Shell.FormControlledClosure(t.Context(), continuedAttemptClosureCommand(t, identity, parcel))
	if err != nil {
		t.Fatalf("形成关闭：%v", err)
	}
	if closed.Outcome() != shipmentapp.ContinuedAttemptDecisionFormed {
		t.Fatalf("outcome = %v（pending %v / refusal %v）, want FORMED", closed.Outcome(), closed.PendingReason(), closed.Refusal())
	}
	closure, _ := closed.Decision()
	if closure.Decider().String() != synContinuedAttemptOperator ||
		closure.AuthorityRole().String() != synContinuedAttemptLevel ||
		closure.AuthoritySnapshot().String() != "SYN-AUTH-CLOSE-CA/v1" {
		t.Fatalf("四件 = 决定方 %q / 授权角色 %q / 授权依据 %q，不是 PC 交回的", closure.Decider(), closure.AuthorityRole(), closure.AuthoritySnapshot())
	}
	if closure.CutoffBoundary().String() != closure.ID().String() {
		t.Fatalf("cutoff = %q, id = %q", closure.CutoffBoundary(), closure.ID())
	}
	register, found := continuedAttemptRegisterOf(t, db, identity.TenantID(), parcel)
	if !found || len(register.Decisions()) != 1 || register.Judge(false) != domain.ContinuedAttemptControlledClosed {
		t.Fatalf("册 found=%v decisions=%d judge=%v", found, len(register.Decisions()), register.Judge(false))
	}
	if got := envelopeCountForDecision(t, db, closure.ID()); got != 1 {
		t.Fatalf("关闭的判断意图 %d 封, want 1", got)
	}

	reopened, err := orchestration.Shell.FormReopening(t.Context(), continuedAttemptReopeningCommand(t, identity, parcel, closure.ID()))
	if err != nil {
		t.Fatalf("形成重开：%v", err)
	}
	if reopened.Outcome() != shipmentapp.ContinuedAttemptDecisionFormed {
		t.Fatalf("outcome = %v（pending %v / refusal %v）, want FORMED", reopened.Outcome(), reopened.PendingReason(), reopened.Refusal())
	}
	reopening, _ := reopened.Decision()
	if reopening.AuthoritySnapshot().String() != "SYN-AUTH-REOPEN-CA/v1" || reopening.RelatedPriorClosure() != closure.ID() {
		t.Fatalf("重开 = 授权依据 %q / 所解关闭 %q", reopening.AuthoritySnapshot(), reopening.RelatedPriorClosure())
	}
	register, _ = continuedAttemptRegisterOf(t, db, identity.TenantID(), parcel)
	if len(register.Decisions()) != 2 || register.Judge(false) != domain.ContinuedAttemptOpen {
		t.Fatalf("册 decisions=%d judge=%v, want 2 / OPEN", len(register.Decisions()), register.Judge(false))
	}
	if got := envelopeCountForDecision(t, db, reopening.ID()); got != 1 {
		t.Fatalf("重开的判断意图 %d 封, want 1", got)
	}

	// 判据 5：裸编排不在事务里调用——写口按框架合同无事务即拒，错误上抛而不是静默落库。
	_, bareParcel := continuedAttemptIdentity(t, "SYN-PARCEL-CA-BARE")
	if _, err := orchestration.Inner.FormControlledClosure(t.Context(), continuedAttemptClosureCommand(t, identity, bareParcel)); err == nil {
		t.Fatal("不在事务里调用编排却没有在写口处报错")
	}
	if _, found := continuedAttemptRegisterOf(t, db, identity.TenantID(), bareParcel); found {
		t.Fatal("无事务的调用居然落了册")
	}

	// 判据 4：入队失败整步回滚——换一个会失败的判断意图口，关闭返回 error，册上一行都没有。
	failing, err := buildContinuedAttemptDecisionOrchestrationWith(db, continuedAttemptDecisionSeams{
		Requests: synContinuedAttemptRequestSource{t: t},
		Handoff:  failingContinuedAttemptHandoff{},
	})
	if err != nil {
		t.Fatalf("装配失败交接的编排：%v", err)
	}
	_, rolledBackParcel := continuedAttemptIdentity(t, "SYN-PARCEL-CA-ROLLBACK")
	if _, err := failing.Shell.FormControlledClosure(t.Context(), continuedAttemptClosureCommand(t, identity, rolledBackParcel)); err == nil {
		t.Fatal("入队失败没有以 error 交回")
	}
	if _, found := continuedAttemptRegisterOf(t, db, identity.TenantID(), rolledBackParcel); found {
		t.Fatal("入队失败后册上仍有行——决定落了、判断意图丢了")
	}
}

func continuedAttemptIdentity(t *testing.T, parcel string) (domain.SourceIdentity, domain.DeclaredParcelID) {
	t.Helper()
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, synContinuedAttemptTenant),
		mustValue(t, domain.NewCustomerAccountID, "SYN-CUSTOMER-CA"),
		mustValue(t, domain.NewSource, "SYN-SOURCE-CA"),
		mustValue(t, domain.NewSourceRequestKey, "SYN-SUBMIT-KEY-CA-"+parcel),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	return identity, mustValue(t, domain.NewDeclaredParcelID, parcel)
}

func continuedAttemptClosureCommand(t *testing.T, identity domain.SourceIdentity, parcel domain.DeclaredParcelID) shipmentapp.FormControlledClosureCommand {
	t.Helper()
	return shipmentapp.FormControlledClosureCommand{
		Identity:                    identity,
		Parcel:                      parcel,
		Requester:                   mustValue(t, domain.NewRequesterReference, "SYN-SHIPPER-ACCOUNT-CA"),
		Reason:                      mustValue(t, domain.NewContinuedAttemptReasonReference, "SYN-REASON-SHIPPER-STOP"),
		EffectiveAt:                 continuedAttemptSeedAt.Add(time.Hour),
		ResponsibilitySourceKind:    shipmentapp.ShipperInstructionResponsibilitySource,
		ResponsibilitySourceSubject: "SYN-SHIPPER-INSTRUCTION-CA",
	}
}

func continuedAttemptReopeningCommand(t *testing.T, identity domain.SourceIdentity, parcel domain.DeclaredParcelID, closure domain.ContinuedAttemptDecisionID) shipmentapp.FormReopeningCommand {
	t.Helper()
	return shipmentapp.FormReopeningCommand{
		Identity:                     identity,
		Parcel:                       parcel,
		Requester:                    mustValue(t, domain.NewRequesterReference, "SYN-SHIPPER-ACCOUNT-CA"),
		Reason:                       mustValue(t, domain.NewContinuedAttemptReasonReference, "SYN-REASON-RESUME"),
		EffectiveAt:                  continuedAttemptSeedAt.Add(2 * time.Hour),
		RelatedPriorClosure:          closure,
		ShipperAccount:               mustValue(t, domain.NewRequesterReference, "SYN-SHIPPER-ACCOUNT-CA"),
		ShipperAuthorizationEvidence: "SYN-SHIPPER-REAUTH-CA",
	}
}

// continuedAttemptRegisterOf 在自己的事务里读回册：仓储读口按框架合同也要事务，测试不绕它。
func continuedAttemptRegisterOf(t *testing.T, db *bentopg.DB, tenant domain.TenantID, parcel domain.DeclaredParcelID) (domain.ContinuedAttemptRegister, bool) {
	t.Helper()
	registers, err := pspostgres.NewContinuedAttemptRegisters(db)
	if err != nil {
		t.Fatalf("构造登记册仓储：%v", err)
	}
	var register domain.ContinuedAttemptRegister
	var found bool
	// 事务闭包里不 Fatalf：FailNow 走 Goexit，事务连接不归还，测试清理的 pool.Close 会永远等它。
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		loaded, present, loadErr := registers.FindByParcel(txCtx, tenant, parcel)
		if loadErr != nil {
			return loadErr
		}
		register, found = loaded, present
		return nil
	}); err != nil {
		t.Fatalf("读回登记册：%v", err)
	}
	return register, found
}

func envelopeCountForDecision(t *testing.T, db *bentopg.DB, decision domain.ContinuedAttemptDecisionID) int {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	err = querier.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1 AND event_id LIKE '%' || $2 || '%'`,
		pspostgres.ContinuedAttemptDecisionEventType, decision.String(),
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计信封行数：%v", err)
	}
	return count
}

// seedSYNContinuedAttemptGrant 往真授权册里登记一条覆盖合成坐标的关闭或重开授权（照 seedSYNReviewGrant）：
// 经 PC 领域的重建门与 SaveGrant 走生产写口，租户取命令身份上的那一个——跨上下文只靠这个字面对上。
func seedSYNContinuedAttemptGrant(t *testing.T, db *bentopg.DB, action pcdomain.AuthorizedAction, objectID string) {
	t.Helper()
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustValue(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-"+objectID),
		mustValue(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-"+objectID),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      mustValue(t, pcdomain.NewTenantID, synContinuedAttemptTenant),
		Kind:          pcdomain.AuthorizationRuleObject,
		ObjectID:      mustValue(t, pcdomain.NewCommercialObjectID, objectID),
		Version:       mustValue(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         mustValue(t, pcdomain.NewCommercialScopeReference, synContinuedAttemptScope),
		ContentDigest: mustValue(t, pcdomain.NewCommercialContentDigest, "sha256:"+objectID),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   approvedAt,
		EffectiveAt:   approvedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("重建已生效授权规则版本：%v", err)
	}
	grant, err := pcdomain.NewAuthorityGrant(
		version,
		action,
		mustValue(t, pcdomain.NewLegalEntityReference, synContinuedAttemptLegalEntity),
		mustValue(t, pcdomain.NewAuthorityLevel, synContinuedAttemptLevel),
		mustValue(t, pcdomain.NewCommercialScopeReference, synContinuedAttemptScope),
		interval,
	)
	if err != nil {
		t.Fatalf("授权授予：%v", err)
	}
	grants, err := pcpostgres.NewAuthorityGrants(db)
	if err != nil {
		t.Fatalf("构造授权册：%v", err)
	}
	var saved pcports.GrantSaveOutcome
	err = db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, saveErr := grants.SaveGrant(txCtx, grant)
		if saveErr != nil {
			return saveErr
		}
		saved = outcome
		return nil
	})
	if err != nil {
		t.Fatalf("登记合成授权：%v", err)
	}
	if saved != pcports.GrantSaved {
		t.Fatalf("登记授权 = %v, want SAVED", saved)
	}
}
