package main

import (
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
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
