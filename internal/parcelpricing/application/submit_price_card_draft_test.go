package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// 本文件证价卡录入编排（ADR-0101 决定三；票 price-card-import/03）：与预览同一段解码，读法决定草稿进哪一格，
// 同版再录按册上那一行答重放、修订或内容已固定，连方案身份都读不出来的文件不落行。

var draftClockAt = time.Date(2026, 10, 9, 4, 0, 0, 0, time.UTC)

const draftSubmitter = "https://id.syn.example/dex#SYN-OPERATOR-01"

// draftRegisterFake 是草稿册的内存替身：一版一行，落点交领域判，同生产适配器的分工。
type draftRegisterFake struct {
	rows  map[string]domain.PriceCardDraft
	calls int
	err   error
}

func newDraftRegisterFake() *draftRegisterFake {
	return &draftRegisterFake{rows: map[string]domain.PriceCardDraft{}}
}

func draftKey(draft domain.PriceCardDraft) string {
	return draft.Tenant().String() + "/" + draft.Plan().ID() + "/" + draft.Plan().Version()
}

func (fake *draftRegisterFake) SubmitDraft(_ context.Context, draft domain.PriceCardDraft) (ports.PriceCardDraftSubmitOutcome, error) {
	fake.calls++
	if fake.err != nil {
		return ports.PriceCardDraftSubmitOutcomeInvalid, fake.err
	}
	existing, found := fake.rows[draftKey(draft)]
	if !found {
		fake.rows[draftKey(draft)] = draft
		return ports.PriceCardDraftSaved, nil
	}
	resubmission, err := existing.ResubmissionOf(draft)
	if err != nil {
		return ports.PriceCardDraftSubmitOutcomeInvalid, err
	}
	switch resubmission {
	case domain.PriceCardDraftResubmissionReplay:
		return ports.PriceCardDraftReplayed, nil
	case domain.PriceCardDraftResubmissionRevision:
		fake.rows[draftKey(draft)] = draft
		return ports.PriceCardDraftRevised, nil
	default:
		return ports.PriceCardDraftContentFixed, nil
	}
}

// approve 把册上那一行经重建门推成`已批准`：批准动作不归本票，这里只造出那一格。
func (fake *draftRegisterFake) approve(t *testing.T, key string) {
	t.Helper()
	draft := fake.rows[key]
	document, err := domain.MarshalPriceCardDraftContent(draft)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := domain.RehydratePriceCardDraft(domain.RehydratePriceCardDraftSpec{
		Tenant: draft.Tenant(), Plan: draft.Plan(), Status: domain.PriceCardDraftStatusApproved, Source: draft.SourceFile(),
		Document: document, Submitter: draft.Submitter(), SubmittedAt: draft.SubmittedAt(),
		Approver: "https://id.syn.example/dex#SYN-APPROVER", ApprovedAt: draft.SubmittedAt().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	fake.rows[key] = approved
}

func submitHandler(t *testing.T, reader ports.PriceCardTemplateReader, register ports.PriceCardDraftRegister) *application.SubmitPriceCardDraftHandler {
	t.Helper()
	handler, err := application.NewSubmitPriceCardDraftHandler(reader, register, fixedClock{at: draftClockAt})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func submitCommand(t *testing.T, raw string) application.SubmitPriceCardDraftCommand {
	t.Helper()
	return application.SubmitPriceCardDraftCommand{Tenant: previewTenant(t), Submitter: draftSubmitter, FileName: "card.xlsx", Raw: []byte(raw)}
}

func validatedReading(t *testing.T, amount string) ports.PriceCardTemplateReading {
	t.Helper()
	plan := planWithAmount(t, amount)
	return ports.PriceCardTemplateReading{
		TemplateVersion: "PPT-1", Accepted: true, Plan: plan.Reference(),
		Content: &ports.PriceCardTemplateContent{
			Plan:                   plan,
			DirectionAuthorization: pptest.IdentityReference(t, domain.ArtifactCommercialAuthorization, "SYN-AUTH-COST-DIR-01", "v1"),
		},
	}
}

func problemReading(t *testing.T, problems ...ports.PriceCardTemplateProblem) ports.PriceCardTemplateReading {
	t.Helper()
	return ports.PriceCardTemplateReading{
		TemplateVersion: "PPT-1", Accepted: true, Plan: pricingPlanForPreview(t).Reference(), Problems: problems,
	}
}

// 读通了的文件进`已校验`：录入者取自命令（信封）、录入时刻取时钟；答复里的读法与同一份字节过预览逐项相同。
func TestAValidatedReadingIsSavedAsAValidatedDraft(t *testing.T) {
	reader, register := &templateReaderDouble{reading: validatedReading(t, "30")}, newDraftRegisterFake()
	result, err := submitHandler(t, reader, register).Handle(context.Background(), submitCommand(t, "card bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != application.PriceCardDraftSubmitted || result.Outcome.String() != "DRAFT_SUBMITTED" || !result.HasDraft {
		t.Fatalf("result = %+v", result)
	}
	if result.Draft.Status() != domain.PriceCardDraftStatusValidated || result.Draft.Submitter() != draftSubmitter ||
		!result.Draft.SubmittedAt().Equal(draftClockAt) || len(register.rows) != 1 {
		t.Fatalf("draft = %+v rows = %d", result.Draft, len(register.rows))
	}

	preview, err := previewHandler(t, reader).Handle(context.Background(), application.PreviewPriceCardImportCommand{
		Tenant: previewTenant(t), FileName: "card.xlsx", Raw: []byte("card bytes"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Reading.Outcome != preview.Outcome || result.Reading.SourceFile != preview.SourceFile ||
		result.Reading.Content.Plan.ContentDigest() != preview.Content.Plan.ContentDigest() {
		t.Fatalf("录入的读法与预览不同：%+v vs %+v", result.Reading, preview)
	}
	content, _ := result.Draft.Content()
	if content.Plan.ContentDigest() != preview.Content.Plan.ContentDigest() || result.Draft.SourceFile() != preview.SourceFile {
		t.Fatal("草稿上的摘要或源文件身份与预览不同")
	}
}

// 读出了方案身份、其余格有问题的文件进`草稿`：逐格问题照读口交出的原样记，不带内容。
func TestAReadingWithProblemsIsSavedAsADraftWithItsProblems(t *testing.T) {
	problems := []ports.PriceCardTemplateProblem{
		{Sheet: "tables", Row: 2, Column: "currency", Code: ports.TemplateCellInvalid, Message: "币种立不住"},
		{Sheet: "rates_weight_zone", Row: 0, Code: ports.TemplateSheetMissing, Message: "缺表"},
	}
	register := newDraftRegisterFake()
	result, err := submitHandler(t, &templateReaderDouble{reading: problemReading(t, problems...)}, register).
		Handle(context.Background(), submitCommand(t, "broken bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != application.PriceCardDraftSubmitted || result.Draft.Status() != domain.PriceCardDraftStatusDraft {
		t.Fatalf("result = %+v", result)
	}
	recorded := result.Draft.Problems()
	if len(recorded) != 2 || recorded[0].Sheet() != "tables" || recorded[0].Row() != 2 || recorded[0].Column() != "currency" ||
		recorded[0].Code() != "CELL_INVALID" || recorded[0].Message() != "币种立不住" || recorded[1].Code() != "SHEET_MISSING" {
		t.Fatalf("problems = %+v", recorded)
	}
	if _, has := result.Draft.Content(); has || len(register.rows) != 1 {
		t.Fatal("`草稿`带了内容，或没落行")
	}
}

// 同一版再录：同一份字节是重放；`已批准`之前换一份是修订、那一行被替换；批准之后换一份答内容已固定，同一份仍是重放。
func TestResubmissionsAnswerReplayRevisionOrFixedContent(t *testing.T) {
	reader, register := &templateReaderDouble{reading: validatedReading(t, "30")}, newDraftRegisterFake()
	handler := submitHandler(t, reader, register)
	submit := func(raw string) application.SubmitPriceCardDraftResult {
		t.Helper()
		result, err := handler.Handle(context.Background(), submitCommand(t, raw))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	if outcome := submit("v1 bytes").Outcome; outcome != application.PriceCardDraftSubmitted {
		t.Fatalf("首录 = %v", outcome)
	}
	if outcome := submit("v1 bytes").Outcome; outcome != application.PriceCardDraftReplayed || outcome.String() != "DRAFT_REPLAYED" {
		t.Fatalf("同一份再录 = %v", outcome)
	}
	reader.reading = problemReading(t, ports.PriceCardTemplateProblem{Sheet: "tables", Row: 2, Column: "currency", Code: ports.TemplateCellInvalid, Message: "x"})
	if outcome := submit("broken bytes").Outcome; outcome != application.PriceCardDraftRevised || outcome.String() != "DRAFT_REVISED" {
		t.Fatalf("已校验改坏了再录 = %v", outcome)
	}
	key := previewTenant(t).String() + "/SYN-PLAN-PREVIEW/v1"
	if register.rows[key].Status() != domain.PriceCardDraftStatusDraft {
		t.Fatalf("修订没替换那一行：%v", register.rows[key].Status())
	}
	reader.reading = validatedReading(t, "31")
	if outcome := submit("v2 bytes").Outcome; outcome != application.PriceCardDraftRevised {
		t.Fatalf("草稿改好了再录 = %v", outcome)
	}

	register.approve(t, key)
	reader.reading = validatedReading(t, "32")
	result := submit("v3 bytes")
	if result.Outcome != application.PriceCardDraftContentFixed || result.Outcome.String() != "CONTENT_FIXED" {
		t.Fatalf("已批准换内容 = %v", result.Outcome)
	}
	if content, _ := result.Draft.Content(); content.Plan.ContentDigest() != planWithAmount(t, "32").ContentDigest() {
		t.Fatal("内容已固定时交回的不是本次拟录的那份")
	}
	if register.rows[key].Status() != domain.PriceCardDraftStatusApproved {
		t.Fatal("内容已固定却动了册上那一行")
	}
	reader.reading = validatedReading(t, "31")
	if outcome := submit("v2 bytes").Outcome; outcome != application.PriceCardDraftReplayed {
		t.Fatalf("已批准再录同一份 = %v", outcome)
	}
}

// 连方案身份都读不出来的文件不落行：答未受理，带读口交出的那一条问题，草稿册一次也没被调到。
func TestAFileWithoutAPlanIdentityIsNotAcceptedAndLeavesNoRow(t *testing.T) {
	register := newDraftRegisterFake()
	reader := &templateReaderDouble{reading: ports.PriceCardTemplateReading{
		TemplateVersion: "PPT-1",
		Problems:        []ports.PriceCardTemplateProblem{{Code: ports.TemplatePlanIdentityUnreadable, Message: "planId 与 planVersion 读不出来"}},
	}}
	result, err := submitHandler(t, reader, register).Handle(context.Background(), submitCommand(t, "blank template"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != application.PriceCardDraftNotAccepted || result.Outcome.String() != "NOT_ACCEPTED" || result.HasDraft {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Reading.Problems) != 1 || result.Reading.Problems[0].Code != ports.TemplatePlanIdentityUnreadable {
		t.Fatalf("problems = %+v", result.Reading.Problems)
	}
	if register.calls != 0 {
		t.Fatalf("未受理却调了草稿册 %d 次", register.calls)
	}
}

// 命令立不住时不读文件、不碰草稿册：缺租户、缺录入者、文件名不合格都答未受理。
func TestACommandWithoutTenantSubmitterOrFileNameIsNotAccepted(t *testing.T) {
	for name, mutate := range map[string]func(*application.SubmitPriceCardDraftCommand){
		"缺租户":   func(c *application.SubmitPriceCardDraftCommand) { c.Tenant = domain.TenantID{} },
		"缺录入者":  func(c *application.SubmitPriceCardDraftCommand) { c.Submitter = "" },
		"录入者空白": func(c *application.SubmitPriceCardDraftCommand) { c.Submitter = "  " },
		"文件名为空": func(c *application.SubmitPriceCardDraftCommand) { c.FileName = "" },
	} {
		reader, register := &templateReaderDouble{reading: validatedReading(t, "30")}, newDraftRegisterFake()
		command := submitCommand(t, "card bytes")
		mutate(&command)
		result, err := submitHandler(t, reader, register).Handle(context.Background(), command)
		if err != nil || result.Outcome != application.PriceCardDraftNotAccepted || reader.read != nil || register.calls != 0 {
			t.Errorf("%s: outcome=%v err=%v read=%v calls=%d", name, result.Outcome, err, reader.read != nil, register.calls)
		}
	}
}

// 草稿册读写失败不是答案：编排如实上抛，端点据此答「没形成答案」，不折成某一格落点。
func TestARegisterFailureIsNotAnAnswer(t *testing.T) {
	register := newDraftRegisterFake()
	register.err = errors.New("register unreachable")
	if _, err := submitHandler(t, &templateReaderDouble{reading: validatedReading(t, "30")}, register).
		Handle(context.Background(), submitCommand(t, "card bytes")); err == nil {
		t.Fatal("草稿册失败被吞了")
	}
}

func TestSubmitHandlerNeedsItsDependencies(t *testing.T) {
	reader, register, clock := &templateReaderDouble{}, newDraftRegisterFake(), fixedClock{at: draftClockAt}
	for name, build := range map[string]func() error{
		"缺读口":  func() error { _, err := application.NewSubmitPriceCardDraftHandler(nil, register, clock); return err },
		"缺草稿册": func() error { _, err := application.NewSubmitPriceCardDraftHandler(reader, nil, clock); return err },
		"缺时钟":  func() error { _, err := application.NewSubmitPriceCardDraftHandler(reader, register, nil); return err },
	} {
		if build() == nil {
			t.Errorf("%s: 依赖缺席也立起来了", name)
		}
	}
}

func planWithAmount(t *testing.T, amount string) domain.PricingPlanVersion {
	t.Helper()
	return pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-PLAN-PREVIEW", "v1"),
		TableReference:        pptest.IdentityReference(t, domain.ArtifactRateTable, "SYN-TABLE-PREVIEW", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, domain.ArtifactWeightPolicy, "SYN-WEIGHT-PREVIEW", "v1"),
		Scope:                 "SYN-SCOPE-01",
		Direction:             domain.PricingDirectionBuy,
		Purpose:               domain.PricingPurposeSupplierCost,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "CNY",
		Period:                pptest.Period{StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		RateEntryID:           "SYN-RATE-PREVIEW",
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "30",
		RateAmount:            amount,
		WeightRounding:        domain.RoundingCeiling,
		WeightStepKilograms:   "0.5",
	})
}
