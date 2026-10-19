package main

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 生产装配的实例半边形状（ADR-0132 越权风险点 2）——处置授权在 PC 授权动作词汇加格之前如实是
// `授权规则未配置`：编排对一份`已提交`委托上的处置答 AUTHORITY_RULES_NOT_CONFIGURED，不落库、不签发决定标识、
// 不释放，也不把它说成「你无权处置」或「没停在等处置」（授权先于一切判断）。谁把装配点上的未配置授权器换成
// 一份「开发用」放行，这条会先红。
func TestTheProductionDispositionAssemblyAnswersRulesNotConfigured(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	version := submittedRequestOnRealAssembly(t, db)
	disposition, err := buildDispositionOrchestration(db)
	if err != nil {
		t.Fatalf("装配授权处置编排：%v", err)
	}

	command := submissionCommand(t)
	result, err := disposition.Handle(t.Context(), shipmentapp.DisposeShipmentRequestCommand{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: version,
		Disposer:          mustValue(t, domain.NewDisposerReference, "syn-disposer-1"),
		Choice:            domain.DisposeByRejection,
		Reason:            mustValue(t, domain.NewDispositionReasonReference, "SYN-DISPOSITION-REASON-1"),
		Evidence:          mustValue(t, domain.NewDispositionEvidenceReference, "syn-disposition-evidence-1"),
	})
	if err != nil {
		t.Fatalf("处置：%v", err)
	}
	if got := result.Outcome(); got != shipmentapp.AuthorizedDispositionAuthorityRulesNotConfigured {
		t.Fatalf("outcome = %q, want AUTHORITY_RULES_NOT_CONFIGURED", got)
	}
	if result.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q, want SUBMITTED——未配置的授权不得改写任何东西", result.State())
	}

	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	request, found, err := requests.FindBySourceIdentity(t.Context(), command.Identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	if request.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("库里 state = %q, want SUBMITTED", request.State())
	}
	if _, recorded := request.AcceptanceDecisionTask().AuthorizedDisposition(); recorded {
		t.Fatal("没拿到授权的处置落了库")
	}
}

// synRDispositionAuthorizer 是处置授权口替身：一律已授权并签发固定的合成授权引用。PC 的授权动作词汇今天没有「授权
// 处置」一格，真授权册登记不出处置授权，已授权之后那一段只能经它取证。
type synRDispositionAuthorizer struct{ t *testing.T }

func (double synRDispositionAuthorizer) AuthorizeDisposition(
	context.Context,
	psports.AuthorizedDispositionAuthorizationQuery,
) (psports.AuthorizedDispositionAuthorization, error) {
	double.t.Helper()
	return psports.AuthorizedDispositionAuthorization{
		Outcome:   psports.AuthorizationGranted,
		Authority: mustValue(double.t, domain.NewDispositionAuthorityReference, "SYN-DISPOSE-RULE-1/v1"),
	}, nil
}

// awaitingDispositionOnRealAssembly 把真提交装出的那份`已提交`委托停到`等待授权处置`并落库：经领域 Decide，适用组只有
// 接受前财务控制，那一项是`无法判定`+ 续办路径「授权处置」。走完整接受链停到这一格由 cmd/parcel-dispatch 的处置闭环
// 取证；这里要证的是处置口在真库上的答复，直接落等待态是取证捷径，不是生产路径。交回读回的当前提交版本。
func awaitingDispositionOnRealAssembly(t *testing.T, db *bentopg.DB) domain.SubmissionVersionID {
	t.Helper()
	version := submittedRequestOnRealAssembly(t, db)
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	command := submissionCommand(t)
	request, found, err := requests.FindBySourceIdentity(t.Context(), command.Identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	applicable, err := domain.NewApplicableCheckGroups(domain.PreAcceptanceFinancialControlCheck)
	if err != nil {
		t.Fatalf("new applicable check groups: %v", err)
	}
	basis, err := domain.NewCommercialBasisSnapshot(domain.CommercialBasisSnapshotSpec{
		ResolutionID: mustValue(t, domain.NewCommercialResolutionID, "SYN-RES-1"),
		RulePackage:  mustValue(t, domain.NewRulePackageReference, "SYN-RULES-1/v1"),
		ViewRevision: mustValue(t, domain.NewCommercialViewRevision, "SYN-VIEW-1"),
		Applicable:   applicable,
		ManualReview: domain.ManualReviewNotRequiredByRules,
	})
	if err != nil {
		t.Fatalf("new commercial basis snapshot: %v", err)
	}
	control, err := domain.NewUndeterminedAcceptanceCheck(
		domain.PreAcceptanceFinancialControlCheck,
		domain.DeclaredParcelID{},
		mustValue(t, domain.NewCheckReason, "SYN-CREDIT-CHECK-INSUFFICIENT"),
		domain.ResumeByAuthorizedDisposition,
	)
	if err != nil {
		t.Fatalf("形成等处置的校验：%v", err)
	}
	waiting, err := request.Decide(domain.AcceptanceDecisionSpec{
		DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "SYN-DECISION-1"),
		Checks:     []domain.AcceptanceCheck{control},
		Basis:      basis,
		DecidedAt:  time.Date(2026, 8, 21, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if path, present := waiting.AcceptanceDecisionTask().WaitingOn(); !present || path != domain.ResumeByAuthorizedDisposition {
		t.Fatalf("夹具 waitingOn = %q（present=%v），want AUTHORIZED_DISPOSITION", path, present)
	}
	err = db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		saved, err := requests.Save(txCtx, command.Identity, waiting)
		if err != nil {
			return err
		}
		if saved != psports.ShipmentRequestSaved {
			return fmt.Errorf("save outcome = %v", saved)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("落等待授权处置：%v", err)
	}
	return version
}

// Covers: 票 operator-channel/36 与 ADR-0132 决定二——`拒绝`去向经生产的操作者 Intake 与真库原样重放，答 200
// `TASK_ALREADY_CLOSED` 并回显决定种类与首份的实际决定方、结构化原因（处置`拒绝`形成的决定带主动拒绝留痕）。处置授权
// 只有授权器是合成替身，理由见 synRDispositionAuthorizer。测试输入是合成数据，只记 `S`。
func TestARejectionDispositionReplayEchoesTheFirstDeciderThroughTheOperatorIntake(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	version := awaitingDispositionOnRealAssembly(t, db)
	disposition, err := dispositionOrchestrationWith(db, synRDispositionAuthorizer{t: t})
	if err != nil {
		t.Fatalf("装配授权处置编排：%v", err)
	}
	views, err := pspostgres.NewShipmentRequestViews(db)
	if err != nil {
		t.Fatalf("构造委托查阅读口：%v", err)
	}
	command := submissionCommand(t)
	intake, err := shipmenthttp.NewOperatorDecisionIntake(
		channelOperatorAuthenticator{tenant: command.Identity.TenantID(), decision: shipmenthttp.OperatorDecisionAuthorizedDisposition},
		views, unwiredDecisionTargets{})
	if err != nil {
		t.Fatalf("构造操作者 Intake：%v", err)
	}
	endpoint := shipmenthttp.NewDisposeShipmentRequestEndpoint(intake, disposition)
	const path = "/shipment-requests/authorized-dispositions"
	body := `{"shipmentRequestId": "` + command.ShipmentRequestID.String() + `", "submissionVersionId": "` + version.String() +
		`", "choice": "REJECT", "reason": "SYN-DISPOSITION-REASON-1"}`

	if status, first := postDecision(t, endpoint, path, "SYN-OPERATOR-01", body); status != http.StatusCreated || first.Outcome != "RECORDED" {
		t.Fatalf("首次处置答 %d %+v，want 201 RECORDED", status, first)
	}
	status, replay := postDecision(t, endpoint, path, "SYN-OPERATOR-01", body)
	if status != http.StatusOK || replay.Outcome != "TASK_ALREADY_CLOSED" || replay.DecisionKind != "REJECTED" {
		t.Fatalf("重放答 %d %+v，want 200 TASK_ALREADY_CLOSED REJECTED", status, replay)
	}
	if replay.Decider != synFirstDecider || replay.RejectionReason != "SYN-DISPOSITION-REASON-1" {
		t.Fatalf("重放回显决定方 %q 原因 %q，want %q / SYN-DISPOSITION-REASON-1", replay.Decider, replay.RejectionReason, synFirstDecider)
	}
}
