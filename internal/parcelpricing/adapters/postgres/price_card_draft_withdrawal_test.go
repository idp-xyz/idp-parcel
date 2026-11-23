package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 已批准的草稿撤下之后仍带着批准时的内容与批准痕迹，可按已撤下筛出来。同一版本号再录入，内容相同或不同都答已撤下，
// 行不被替换。迁移之前的列面（不写撤下三列）读回不变：新列为空，状态与批准痕迹照旧。

func TestAnApprovedDraftWithdrawsAndItsVersionStaysWithdrawn(t *testing.T) {
	store := newDraftStore(t)
	plan := catalogPlan(t, "SYN-PLAN-DRAFT-01", "v1", domain.PricingDirectionBuy, "30")
	source := draftSourceOf(t, "card.xlsx", catalogSourceSHA)
	submitted := validatedDraftOf(t, "SYN-TENANT-01", plan, source, draftAt)
	if outcome := store.submit(t, submitted); outcome != ports.PriceCardDraftSaved {
		t.Fatalf("首录 = %v", outcome)
	}
	approvedAt := draftAt.Add(time.Hour)
	approved := approvedBy(t, store.loadDraft(t, submitted), draftProgressApprover, approvedAt)
	if outcome := store.advanceDraft(t, approved); outcome != ports.PriceCardDraftAdvanced {
		t.Fatalf("批准 = %v", outcome)
	}

	withdrawnAt := approvedAt.Add(time.Minute)
	withdrawer, err := domain.NewOperatorSubject("https://id.syn.example/dex#SYN-OPERATOR-03", nil)
	if err != nil {
		t.Fatal(err)
	}
	withdrawn, err := store.loadDraft(t, submitted).Withdraw(withdrawer, "  客户拒绝这套价  ", withdrawnAt)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := store.advanceDraft(t, withdrawn); outcome != ports.PriceCardDraftAdvanced {
		t.Fatalf("撤下 = %v", outcome)
	}

	read := store.loadDraft(t, submitted)
	content, has := read.Content()
	who, whoOK := read.Withdrawer()
	when, whenOK := read.WithdrawnAt()
	reason, reasonOK := read.WithdrawalReason()
	approver, approverOK := read.Approver()
	if read.Status() != domain.PriceCardDraftStatusWithdrawn || !has || content.Plan.ContentDigest() != plan.ContentDigest() ||
		!whoOK || who != withdrawer.Reference() || !whenOK || !when.Equal(withdrawnAt) || !reasonOK || reason != "客户拒绝这套价" ||
		!approverOK || approver != draftProgressApprover || !read.SubmittedAt().Equal(draftAt) {
		t.Fatalf("读回 = status %v content %v who %q %v when %v %v reason %q %v approver %q %v",
			read.Status(), has, who, whoOK, when, whenOK, reason, reasonOK, approver, approverOK)
	}
	if _, published := read.PublishedAt(); published {
		t.Fatal("已撤下带着发布时刻")
	}
	if listed := store.list(t, "SYN-TENANT-01", domain.PriceCardDraftStatusWithdrawn); len(listed) != 1 || listed[0].Plan().ID() != plan.Reference().ID() {
		t.Fatalf("按已撤下筛选 = %+v", planIDsOf(listed))
	}
	if listed := store.list(t, "SYN-TENANT-01", domain.PriceCardDraftStatusApproved); len(listed) != 0 {
		t.Fatalf("已撤下仍出现在已批准里：%+v", planIDsOf(listed))
	}

	if outcome := store.submit(t, validatedDraftOf(t, "SYN-TENANT-01", plan, source, withdrawnAt.Add(time.Hour))); outcome != ports.PriceCardDraftWithdrawn {
		t.Fatalf("同一份再录 = %v，want WITHDRAWN", outcome)
	}
	changed := catalogPlan(t, "SYN-PLAN-DRAFT-01", "v1", domain.PricingDirectionBuy, "31")
	if outcome := store.submit(t, validatedDraftOf(t, "SYN-TENANT-01", changed, draftSourceOf(t, "other.xlsx", draftOtherSourceSHA), withdrawnAt.Add(2*time.Hour))); outcome != ports.PriceCardDraftWithdrawn {
		t.Fatalf("换内容再录 = %v，want WITHDRAWN", outcome)
	}
	kept := store.loadDraft(t, submitted)
	keptContent, _ := kept.Content()
	keptReason, _ := kept.WithdrawalReason()
	if kept.Status() != domain.PriceCardDraftStatusWithdrawn || keptContent.Plan.ContentDigest() != plan.ContentDigest() ||
		keptReason != "客户拒绝这套价" || !kept.SubmittedAt().Equal(draftAt) {
		t.Fatal("再录替换或改写了已撤下的那一行")
	}
}

func TestARowWrittenBeforeWithdrawalColumnsReadsBackUnchanged(t *testing.T) {
	store := newDraftStore(t)
	validatedPlan := catalogPlan(t, "SYN-PLAN-OLD-VALIDATED", "v1", domain.PricingDirectionBuy, "30")
	approvedPlan := catalogPlan(t, "SYN-PLAN-OLD-APPROVED", "v1", domain.PricingDirectionSell, "45")
	source := draftSourceOf(t, "card.xlsx", catalogSourceSHA)
	validated := validatedDraftOf(t, "SYN-TENANT-01", validatedPlan, source, draftAt)
	approved := approvedBy(t, validatedDraftOf(t, "SYN-TENANT-01", approvedPlan, source, draftAt), draftProgressApprover, draftAt.Add(time.Hour))

	insertPreWithdrawalRow(t, store, validated)
	insertPreWithdrawalRow(t, store, approved)

	readValidated := store.loadDraft(t, validated)
	if readValidated.Status() != domain.PriceCardDraftStatusValidated || readValidated.Submitter() != validated.Submitter() ||
		!readValidated.SubmittedAt().Equal(draftAt) {
		t.Fatalf("已校验旧行 = status %v submitter %q at %v", readValidated.Status(), readValidated.Submitter(), readValidated.SubmittedAt())
	}
	if _, ok := readValidated.Approver(); ok {
		t.Fatal("已校验旧行读出了批准者")
	}
	if _, ok := readValidated.Withdrawer(); ok {
		t.Fatal("已校验旧行读出了撤下人")
	}
	got, _ := readValidated.Content()
	want, _ := validated.Content()
	if got.Plan.ContentDigest() != want.Plan.ContentDigest() {
		t.Fatal("已校验旧行的内容摘要变了")
	}

	readApproved := store.loadDraft(t, approved)
	approver, ok := readApproved.Approver()
	approvedAt, atOK := readApproved.ApprovedAt()
	if readApproved.Status() != domain.PriceCardDraftStatusApproved || !ok || approver != draftProgressApprover || !atOK || !approvedAt.Equal(draftAt.Add(time.Hour)) {
		t.Fatalf("已批准旧行 = status %v approver %q %v at %v %v", readApproved.Status(), approver, ok, approvedAt, atOK)
	}
	if _, published := readApproved.PublishedAt(); published {
		t.Fatal("已批准旧行读出了发布时刻")
	}
	if _, withdrawn := readApproved.Withdrawer(); withdrawn {
		t.Fatal("已批准旧行读出了撤下人")
	}
	if _, when := readApproved.WithdrawnAt(); when {
		t.Fatal("已批准旧行读出了撤下时刻")
	}
	if _, reason := readApproved.WithdrawalReason(); reason {
		t.Fatal("已批准旧行读出了撤下原因")
	}
	got, _ = readApproved.Content()
	want, _ = approved.Content()
	if got.Plan.ContentDigest() != want.Plan.ContentDigest() || !readApproved.SubmittedAt().Equal(draftAt) {
		t.Fatal("已批准旧行的内容或录入时刻变了")
	}

	var by *string
	var at *time.Time
	var reason *string
	if err := store.pool.QueryRow(t.Context(),
		`SELECT withdrawn_by, withdrawn_at, withdrawal_reason
		   FROM parcel_pricing.price_card_draft
		  WHERE plan_id = 'SYN-PLAN-OLD-APPROVED'`).Scan(&by, &at, &reason); err != nil || by != nil || at != nil || reason != nil {
		t.Fatalf("旧行的撤下列被填上了：%v %v %v %v", by, at, reason, err)
	}
}

func (store draftStore) loadDraft(t *testing.T, draft domain.PriceCardDraft) domain.PriceCardDraft {
	t.Helper()
	loaded, found, err := store.drafts.LoadPriceCardDraft(t.Context(), draft.Tenant(), draft.Plan())
	if err != nil || !found {
		t.Fatalf("读草稿：found=%v err=%v", found, err)
	}
	return loaded
}

func (store draftStore) advanceDraft(t *testing.T, draft domain.PriceCardDraft) ports.PriceCardDraftAdvanceOutcome {
	t.Helper()
	var outcome ports.PriceCardDraftAdvanceOutcome
	err := store.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		advanced, advanceErr := store.drafts.AdvancePriceCardDraft(ctx, draft)
		outcome = advanced
		return advanceErr
	})
	if err != nil {
		t.Fatalf("推进：%v", err)
	}
	return outcome
}

// insertPreWithdrawalRow 按 0011 的列面落一行，不写撤下三列。迁移把这三列加成可空之后，这样的行就是迁移前已在册的形状。
func insertPreWithdrawalRow(t *testing.T, store draftStore, draft domain.PriceCardDraft) {
	t.Helper()
	document, err := domain.MarshalPriceCardDraftContent(draft)
	if err != nil {
		t.Fatal(err)
	}
	var canonicalization, digest, authorizationID, authorizationVersion *string
	if content, has := draft.Content(); has {
		version, sum := content.Plan.CanonicalizationVersion(), content.Plan.ContentDigest()
		id, authVersion := content.DirectionAuthorization.ID(), content.DirectionAuthorization.Version()
		canonicalization, digest, authorizationID, authorizationVersion = &version, &sum, &id, &authVersion
	}
	var approver *string
	var approvedAt *time.Time
	if who, ok := draft.Approver(); ok {
		at, _ := draft.ApprovedAt()
		approver, approvedAt = &who, &at
	}
	tag, err := store.pool.Exec(t.Context(),
		`INSERT INTO parcel_pricing.price_card_draft
			(tenant_id, plan_id, plan_version, status, source_file_name, source_file_sha256,
			 canonicalization, content_digest, authorization_id, authorization_version,
			 content_document, submitter, submitted_at, approver, approved_at, published_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NULL)`,
		draft.Tenant().String(), draft.Plan().ID(), draft.Plan().Version(), draft.Status().String(),
		draft.SourceFile().Name(), draft.SourceFile().SHA256(),
		canonicalization, digest, authorizationID, authorizationVersion,
		document, draft.Submitter(), draft.SubmittedAt().UTC(), approver, approvedAt,
	)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("按迁移前的列面落行：%v（%d 行）", err, tag.RowsAffected())
	}
}
