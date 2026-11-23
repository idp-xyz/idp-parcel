package main

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 真库上：已校验撤不了，答未批准；已发布撤不了，答已发布；已批准撤下之后发布答已撤下且不入册；
// 同一版本号再录入，内容相同或不同都答已撤下。报价册不在这条路上。

type fixedPriceCardReading struct {
	reading ports.PriceCardTemplateReading
}

func (reader fixedPriceCardReading) ReadPriceCardTemplate([]byte) ports.PriceCardTemplateReading {
	return reader.reading
}

func TestWithdrawingAnApprovedPriceCardDraftOnTheRealRegister(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	approval, err := buildPriceCardDraftApproval(db)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := buildPriceCardDraftPublication(db, false)
	if err != nil {
		t.Fatal(err)
	}
	withdrawal, err := buildPriceCardDraftWithdrawal(db)
	if err != nil {
		t.Fatal(err)
	}
	drafts, err := pppostgres.NewPriceCardDrafts(db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := pppostgres.NewPriceCardApprovalDutyRules(db)
	if err != nil {
		t.Fatal(err)
	}

	kept := syntheticPriceCardRegistrationVersion(t, "v-withdraw")
	publishedCard := syntheticPriceCardRegistrationVersion(t, "v-publish")
	rule, err := domain.NewPriceCardApprovalDutyRule(kept.Tenant(), true, domain.OperatorGrant{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, saveErr := rules.SavePriceCardApprovalDutyRule(ctx, rule)
		return saveErr
	}); err != nil {
		t.Fatalf("登规则：%v", err)
	}
	storeDraft(t, db, drafts, kept)
	storeDraft(t, db, drafts, publishedCard)

	approver, err := domain.NewOperatorSubject("https://id.syn.example/dex#SYN-OPERATOR-02", nil)
	if err != nil {
		t.Fatal(err)
	}
	withdrawer, err := domain.NewOperatorSubject("https://id.syn.example/dex#SYN-OPERATOR-03", nil)
	if err != nil {
		t.Fatal(err)
	}
	refuseValidated := pricingapp.WithdrawPriceCardDraftCommand{
		Tenant: kept.Tenant(), Plan: kept.Plan().Reference(), Withdrawer: withdrawer, Reason: "客户拒绝这套价",
	}
	if result, err := withdrawal.Handle(t.Context(), refuseValidated); err != nil || result.Outcome.String() != "DRAFT_NOT_APPROVED" {
		t.Fatalf("已校验撤下 = %s, err = %v，想要 DRAFT_NOT_APPROVED", result.Outcome, err)
	}
	if still, _, err := drafts.LoadPriceCardDraft(t.Context(), kept.Tenant(), kept.Plan().Reference()); err != nil || still.Status() != domain.PriceCardDraftStatusValidated {
		t.Fatalf("已校验被撤下改了：%v %v", still.Status(), err)
	}

	if result, err := approval.Handle(t.Context(), pricingapp.ApprovePriceCardDraftCommand{
		Tenant: kept.Tenant(), Plan: kept.Plan().Reference(), Approver: approver,
	}); err != nil || result.Outcome != pricingapp.PriceCardDraftApproved {
		t.Fatalf("批准 = %s, err = %v", result.Outcome, err)
	}
	withdrawn, err := withdrawal.Handle(t.Context(), refuseValidated)
	if err != nil || withdrawn.Outcome.String() != "DRAFT_WITHDRAWN" || !withdrawn.HasDraft {
		t.Fatalf("撤下 = %s has=%v err=%v，想要 DRAFT_WITHDRAWN", withdrawn.Outcome, withdrawn.HasDraft, err)
	}
	if who, _ := withdrawn.Draft.Withdrawer(); who != withdrawer.Reference() {
		t.Fatalf("撤下人 = %q，想要信封里的主体", who)
	}
	if published, err := publication.Handle(t.Context(), pricingapp.PublishPriceCardDraftCommand{
		Tenant: kept.Tenant(), Plan: kept.Plan().Reference(),
	}); err != nil || published.Outcome.String() != "DRAFT_WITHDRAWN" || published.HasRegistration {
		t.Fatalf("发布已撤下 = %s registration=%v err=%v，想要 DRAFT_WITHDRAWN 且不入册", published.Outcome, published.HasRegistration, err)
	}
	var registered int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM parcel_pricing.price_card_version WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3`,
		kept.Tenant().String(), kept.Plan().Reference().ID(), kept.Plan().Reference().Version(),
	).Scan(&registered); err != nil || registered != 0 {
		t.Fatalf("价卡册行数 = %d, err = %v，想要 0", registered, err)
	}

	replayed, err := domain.SubmitPriceCardDraft(domain.PriceCardDraftSubmission{
		Tenant: kept.Tenant(), Plan: kept.Plan().Reference(), Source: kept.SourceFile(),
		Content:     &domain.PriceCardDraftContent{Plan: kept.Plan(), DirectionAuthorization: kept.DirectionAuthorization()},
		Submitter:   "https://id.syn.example/dex#SYN-OPERATOR-09",
		SubmittedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var same ports.PriceCardDraftSubmitOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		var submitErr error
		same, submitErr = drafts.SubmitDraft(ctx, replayed)
		return submitErr
	}); err != nil || same != ports.PriceCardDraftWithdrawn {
		t.Fatalf("同一份再录 = %s, err = %v，想要 WITHDRAWN", same, err)
	}
	content, _ := withdrawn.Draft.Content()
	resubmit, err := pricingapp.NewSubmitPriceCardDraftHandler(fixedPriceCardReading{reading: ports.PriceCardTemplateReading{
		TemplateVersion: "PPT-1", Accepted: true, Plan: kept.Plan().Reference(),
		Content: &ports.PriceCardTemplateContent{Plan: content.Plan, DirectionAuthorization: content.DirectionAuthorization},
	}}, drafts, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	var changed pricingapp.SubmitPriceCardDraftResult
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		var handleErr error
		changed, handleErr = resubmit.Handle(ctx, pricingapp.SubmitPriceCardDraftCommand{
			Tenant: kept.Tenant(), Submitter: "https://id.syn.example/dex#SYN-OPERATOR-09",
			FileName: "other.xlsx", Raw: []byte("different-bytes"),
		})
		return handleErr
	}); err != nil || changed.Outcome.String() != "DRAFT_WITHDRAWN" {
		t.Fatalf("换了文件再录 = %s, err = %v，想要 DRAFT_WITHDRAWN", changed.Outcome, err)
	}
	still, _, err := drafts.LoadPriceCardDraft(t.Context(), kept.Tenant(), kept.Plan().Reference())
	if err != nil || still.Status() != domain.PriceCardDraftStatusWithdrawn {
		t.Fatalf("再录之后 = %v %v", still.Status(), err)
	}
	if reason, _ := still.WithdrawalReason(); reason != "客户拒绝这套价" {
		t.Fatalf("再录改了原因：%q", reason)
	}

	publishDirectionAuthorizationForCard(t, db, publishedCard)
	if result, err := approval.Handle(t.Context(), pricingapp.ApprovePriceCardDraftCommand{
		Tenant: publishedCard.Tenant(), Plan: publishedCard.Plan().Reference(), Approver: approver,
	}); err != nil || result.Outcome != pricingapp.PriceCardDraftApproved {
		t.Fatalf("批准待发布的一版 = %s, err = %v", result.Outcome, err)
	}
	if result, err := publication.Handle(t.Context(), pricingapp.PublishPriceCardDraftCommand{
		Tenant: publishedCard.Tenant(), Plan: publishedCard.Plan().Reference(),
	}); err != nil || result.Outcome != pricingapp.PriceCardDraftPublished || result.Registration != pricingapp.PriceCardRecorded {
		t.Fatalf("发布 = %s / %s, err = %v", result.Outcome, result.Registration, err)
	}
	if result, err := withdrawal.Handle(t.Context(), pricingapp.WithdrawPriceCardDraftCommand{
		Tenant: publishedCard.Tenant(), Plan: publishedCard.Plan().Reference(), Withdrawer: withdrawer, Reason: "想撤下已发布的",
	}); err != nil || result.Outcome.String() != "DRAFT_PUBLISHED" {
		t.Fatalf("已发布撤下 = %s, err = %v，想要 DRAFT_PUBLISHED", result.Outcome, err)
	}
	if still, _, err := drafts.LoadPriceCardDraft(t.Context(), publishedCard.Tenant(), publishedCard.Plan().Reference()); err != nil ||
		still.Status() != domain.PriceCardDraftStatusPublished {
		t.Fatalf("已发布被撤下改了：%v %v", still.Status(), err)
	}
}

func storeDraft(t *testing.T, db *bentopg.DB, drafts *pppostgres.PriceCardDrafts, registration domain.PriceCardRegistration) {
	t.Helper()
	draft, err := domain.SubmitPriceCardDraft(domain.PriceCardDraftSubmission{
		Tenant: registration.Tenant(), Plan: registration.Plan().Reference(), Source: registration.SourceFile(),
		Content:   &domain.PriceCardDraftContent{Plan: registration.Plan(), DirectionAuthorization: registration.DirectionAuthorization()},
		Submitter: "https://id.syn.example/dex#SYN-OPERATOR-01", SubmittedAt: time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond),
	})
	if err != nil {
		t.Fatalf("立草稿：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, submitErr := drafts.SubmitDraft(ctx, draft)
		return submitErr
	}); err != nil {
		t.Fatalf("录入草稿：%v", err)
	}
}
