package main

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// withdrawOnRealDatabase 经领域转移与生产委托仓储把一份已落库的`已提交`委托撤回并保存。撤回授权是实例半边、
// 生产装配里还没有租户取值，所以撤回这一笔不经编排；被测的是撤回之后再读这份委托的几条路。
func withdrawOnRealDatabase(t *testing.T, db *bentopg.DB, identity domain.SourceIdentity) domain.Withdrawal {
	t.Helper()
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	var withdrawal domain.Withdrawal
	err = db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		request, found, err := requests.FindBySourceIdentity(txCtx, identity)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("委托读不回来")
		}
		withdrawn, err := request.WithdrawByCustomer(domain.WithdrawalSpec{
			DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "SYN-DECISION-WITHDRAWN-1"),
			Authority:  mustValue(t, domain.NewWithdrawalAuthorityReference, "SYN-WITHDRAW-ROLE-1"),
			Requester:  mustValue(t, domain.NewWithdrawalRequesterReference, "SYN-CUSTOMER-CONTACT-1"),
			Reason:     mustValue(t, domain.NewWithdrawalReasonReference, "SYN-ORDER-CANCELLED"),
			DecidedAt:  envelopeProofAnchor.Add(time.Hour),
		})
		if err != nil {
			return err
		}
		withdrawal, _ = withdrawn.Withdrawal()
		_, err = requests.Save(txCtx, identity, withdrawn)
		return err
	})
	if err != nil {
		t.Fatalf("撤回并保存：%v", err)
	}
	return withdrawal
}

// Covers: 票 docs-consistency/code/36、ADR-0255 决定一、ADR-0286——提交口按来源身份读回已撤回的原委托的两条路：客户原样
// 重放那份提交答`既有结果`；以撤回重提方向关联它重新提交则关联成立（`AT-PS-076`「原委托不恢复，建立关联新委托」）。
// 已撤回行读不回时，前者答 500、后者报读不到原委托。方向与原委托终态互证仍在：指着已撤回委托却声称「已拒绝修正」
// 答关联不适用。
func TestASubmissionReadsBackAWithdrawnRequest(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	submittedRequestOnRealAssembly(t, db)
	original := submissionCommand(t)
	withdrawOnRealDatabase(t, db, original.Identity)
	submission, _ := envelopeMintingSubmission(t, db)

	replay, err := submission.Handle(t.Context(), submissionCommand(t))
	if err != nil {
		t.Fatalf("原样重放已被撤回的提交：%v", err)
	}
	if got := replay.Outcome(); got != shipmentapp.OutcomeExistingResult {
		t.Fatalf("重放 outcome = %v, want EXISTING_RESULT", got)
	}

	linked := func(key string, kind domain.RequestLinkKind) shipmentapp.SubmitShipmentRequestCommand {
		command := submissionCommand(t)
		identity, err := domain.NewSourceIdentity(original.Identity.TenantID(), original.Identity.CustomerAccountID(),
			original.Identity.Source(), mustValue(t, domain.NewSourceRequestKey, key))
		if err != nil {
			t.Fatalf("关联新委托的来源身份：%v", err)
		}
		command.Identity = identity
		command.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "syn-digest-"+key)
		command.ShipmentRequestID = mustValue(t, domain.NewShipmentRequestID, "syn-request-"+key)
		command.BatchID = mustValue(t, domain.NewSubmissionBatchID, "syn-batch-"+key)
		command.Link = shipmentapp.PriorRequestClaim{
			PriorIdentity:  original.Identity,
			PriorRequestID: original.ShipmentRequestID,
			Kind:           kind,
		}
		return command
	}

	wrongKind, err := submission.Handle(t.Context(), linked("SYN-KEY-WRONG-KIND", domain.LinkRejectedCorrection))
	if err != nil {
		t.Fatalf("以已拒绝修正关联已撤回委托：%v", err)
	}
	if got := wrongKind.Outcome(); got != shipmentapp.OutcomeLinkIneligible {
		t.Fatalf("方向不符 outcome = %v, want LINK_INELIGIBLE", got)
	}

	resubmission := linked("SYN-KEY-RESUBMISSION", domain.LinkWithdrawnResubmission)
	result, err := submission.Handle(t.Context(), resubmission)
	if err != nil {
		t.Fatalf("以撤回重提关联重新提交：%v", err)
	}
	if got := result.Outcome(); got != shipmentapp.OutcomeSubmitted {
		t.Fatalf("撤回重提 outcome = %v, want SUBMITTED", got)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	created, found, err := requests.FindBySourceIdentity(t.Context(), resubmission.Identity)
	if err != nil || !found {
		t.Fatalf("读回关联新委托：err=%v found=%v", err, found)
	}
	link, established := created.PriorRequestLink()
	if !established || link.Kind() != domain.LinkWithdrawnResubmission || link.PriorRequestID() != original.ShipmentRequestID {
		t.Fatalf("关联出处 = %+v（established=%v）, want 撤回重提指向 syn-request-1", link, established)
	}
	prior, found, err := requests.FindBySourceIdentity(t.Context(), original.Identity)
	if err != nil || !found || prior.State() != domain.ShipmentRequestWithdrawn {
		t.Fatalf("原委托 = %q（found=%v err=%v）；关联新委托不得动原委托，它仍是已撤回", prior.State(), found, err)
	}
}

// Covers: 票 docs-consistency/code/36、`AT-PS-068`、`AT-PS-076`——生产装配的撤回编排（真来源仓储、真委托仓储）读回一份
// 已撤回的委托：授权映射未配置时这一轮停在`撤回授权不可用`，未决交回的是读到的真实状态`已撤回`而不是读不回；客户重发
// 同一撤回走已保全路径，答`决定已形成`并带原撤回，不再形成第二次撤回。已撤回行读不回时，前一轮答`委托读不回`、
// 重放上抛读错。
func TestTheWiredWithdrawalReadsBackAnAlreadyWithdrawnRequest(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	identity := withdrawalSeedIdentity(t)
	seedSubmittedShipmentRequest(t, db, identity)
	original := withdrawOnRealDatabase(t, db, identity)

	withdrawal, err := buildWithdrawalOrchestration(db)
	if err != nil {
		t.Fatalf("装配撤回编排：%v", err)
	}
	command := withdrawalCommand(t, identity)

	first, err := withdrawal.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("撤回一份已撤回的委托：%v", err)
	}
	if first.Outcome() != shipmentapp.WithdrawalUndecided || first.PendingReason() != shipmentapp.WithdrawalAuthorityUnavailable {
		t.Fatalf("outcome = %v reason = %v, want UNDECIDED / WITHDRAWAL_AUTHORITY_UNAVAILABLE——委托读得回，停在授权",
			first.Outcome(), first.PendingReason())
	}
	if first.State() != domain.ShipmentRequestWithdrawn {
		t.Fatalf("未决交回的状态 = %v, want WITHDRAWN——这一轮读到的事实", first.State())
	}

	replay, err := withdrawal.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重发同一撤回：%v", err)
	}
	if got := replay.Outcome(); got != shipmentapp.WithdrawalDecisionAlreadyFormed {
		t.Fatalf("重发 outcome = %v, want DECISION_ALREADY_FORMED", got)
	}
	record, present := replay.Withdrawal()
	if !present || record.DecisionID() != original.DecisionID() || !record.DecidedAt().Equal(original.DecidedAt()) ||
		record.Requester() != original.Requester() || record.Reason() != original.Reason() || record.Authority() != original.Authority() {
		t.Fatalf("重发答的撤回 = %+v（present=%v）, want 原撤回 %+v", record, present, original)
	}
}
