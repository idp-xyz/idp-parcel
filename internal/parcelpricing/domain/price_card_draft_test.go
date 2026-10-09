package domain_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证价卡草稿册的一行（ADR-0101 决定三；票 price-card-import/03）：四格状态各自的进入条件、
// 同版再录的三种落点（重放、修订、内容已固定），以及草稿经重建门读回整图重验。

var draftSubmittedAt = time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)

const otherSourceSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func draftPlan(t *testing.T, baseAmount string) domain.PricingPlanVersion {
	t.Helper()
	return syntheticPlan(t, "draft", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, baseAmount, domain.PricingWeightActualOnly, nil)
}

func draftSource(t *testing.T, name, sha string) domain.SourceFileIdentity {
	t.Helper()
	source, err := domain.NewSourceFileIdentity(name, sha)
	if err != nil {
		t.Fatalf("构造源文件身份：%v", err)
	}
	return source
}

func draftProblem(t *testing.T, sheet string, row int, column, code string) domain.PriceCardDraftProblem {
	t.Helper()
	problem, err := domain.NewPriceCardDraftProblem(sheet, row, column, code, "这一格立不住")
	if err != nil {
		t.Fatalf("构造逐格问题：%v", err)
	}
	return problem
}

// validatedSubmission 是一份读通了的录入：方案过了构造门，方向授权引用齐备。
func validatedSubmission(t *testing.T, plan domain.PricingPlanVersion, source domain.SourceFileIdentity) domain.PriceCardDraftSubmission {
	t.Helper()
	return domain.PriceCardDraftSubmission{
		Tenant: mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Plan:   plan.Reference(),
		Source: source,
		Content: &domain.PriceCardDraftContent{
			Plan:                   plan,
			DirectionAuthorization: versionReference(t, domain.ArtifactCommercialAuthorization, "SYN-AUTH-COST-DIR-01", "v1"),
		},
		Submitter:   "https://id.syn.example/dex#SYN-OPERATOR-01",
		SubmittedAt: draftSubmittedAt,
	}
}

// problemSubmission 是一份读出了方案身份、其余某些格有问题的录入。
func problemSubmission(t *testing.T, source domain.SourceFileIdentity, problems ...domain.PriceCardDraftProblem) domain.PriceCardDraftSubmission {
	t.Helper()
	return domain.PriceCardDraftSubmission{
		Tenant:      mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Plan:        versionReference(t, domain.ArtifactPricingPlan, "plan-draft", "v1"),
		Source:      source,
		Problems:    problems,
		Submitter:   "https://id.syn.example/dex#SYN-OPERATOR-01",
		SubmittedAt: draftSubmittedAt,
	}
}

func submitDraft(t *testing.T, submission domain.PriceCardDraftSubmission) domain.PriceCardDraft {
	t.Helper()
	draft, err := domain.SubmitPriceCardDraft(submission)
	if err != nil {
		t.Fatalf("立草稿：%v", err)
	}
	return draft
}

// 没读通的录入进`草稿`：只记逐格问题，不带方案也不带方向授权（spec 自决第 1 格）。
func TestASubmissionWithProblemsEntersDraft(t *testing.T) {
	problems := []domain.PriceCardDraftProblem{
		draftProblem(t, "tables", 2, "currency", "CELL_INVALID"),
		draftProblem(t, "rates_weight_zone", 3, "amount", "CELL_NUMERIC_NOT_EXACT"),
	}
	draft := submitDraft(t, problemSubmission(t, draftSource(t, "card.xlsx", synSourceSHA), problems...))

	if draft.Status() != domain.PriceCardDraftStatusDraft || draft.Status().String() != "DRAFT" {
		t.Fatalf("status = %v", draft.Status())
	}
	if _, has := draft.Content(); has {
		t.Fatal("`草稿`带了内容")
	}
	if got := draft.Problems(); len(got) != 2 || got[0].Sheet() != "tables" || got[0].Row() != 2 || got[0].Column() != "currency" ||
		got[0].Code() != "CELL_INVALID" || got[1].Code() != "CELL_NUMERIC_NOT_EXACT" {
		t.Fatalf("problems = %+v", got)
	}
	if draft.Plan().ID() != "plan-draft" || draft.Plan().Version() != "v1" || draft.SourceFile().SHA256() != synSourceSHA ||
		draft.Submitter() != "https://id.syn.example/dex#SYN-OPERATOR-01" || !draft.SubmittedAt().Equal(draftSubmittedAt) {
		t.Fatalf("draft = %+v", draft)
	}
	if _, approved := draft.Approver(); approved {
		t.Fatal("新录入的草稿带了批准者")
	}
}

// 读通了的录入进`已校验`：带过了构造门的方案（内容摘要已算出）与方向授权引用，没有逐格问题。
func TestASubmissionWithContentEntersValidated(t *testing.T) {
	plan := draftPlan(t, "30")
	draft := submitDraft(t, validatedSubmission(t, plan, draftSource(t, "card.xlsx", synSourceSHA)))

	if draft.Status() != domain.PriceCardDraftStatusValidated || draft.Status().String() != "VALIDATED" {
		t.Fatalf("status = %v", draft.Status())
	}
	content, has := draft.Content()
	if !has || content.Plan.ContentDigest() != plan.ContentDigest() || content.DirectionAuthorization.ID() != "SYN-AUTH-COST-DIR-01" {
		t.Fatalf("content = %+v, %v", content, has)
	}
	if len(draft.Problems()) != 0 {
		t.Fatalf("problems = %+v", draft.Problems())
	}
}

// 状态由录入的内容决定、不由调用方声明，所以内容与问题恰有一样；其余格立不住同样不成草稿。
func TestAnIncoherentSubmissionIsNotADraft(t *testing.T) {
	plan := draftPlan(t, "30")
	source := draftSource(t, "card.xlsx", synSourceSHA)
	problem := draftProblem(t, "tables", 2, "currency", "CELL_INVALID")
	cases := map[string]func(*domain.PriceCardDraftSubmission){
		"内容与问题并存":  func(s *domain.PriceCardDraftSubmission) { s.Problems = []domain.PriceCardDraftProblem{problem} },
		"内容与问题都没有": func(s *domain.PriceCardDraftSubmission) { s.Content = nil },
		"内容的方案不是键上那一版": func(s *domain.PriceCardDraftSubmission) {
			s.Plan = versionReference(t, domain.ArtifactPricingPlan, "plan-draft", "v2")
		},
		"方向授权不是授权工件": func(s *domain.PriceCardDraftSubmission) {
			s.Content.DirectionAuthorization = versionReference(t, domain.ArtifactRateTable, "SYN-AUTH-COST-DIR-01", "v1")
		},
		"方案没立住": func(s *domain.PriceCardDraftSubmission) { s.Content.Plan = domain.PricingPlanVersion{} },
		"缺租户":   func(s *domain.PriceCardDraftSubmission) { s.Tenant = domain.TenantID{} },
		"键不是方案引用": func(s *domain.PriceCardDraftSubmission) {
			s.Plan = versionReference(t, domain.ArtifactRateTable, "plan-draft", "v1")
		},
		"缺源文件身份":  func(s *domain.PriceCardDraftSubmission) { s.Source = domain.SourceFileIdentity{} },
		"录入者为空":   func(s *domain.PriceCardDraftSubmission) { s.Submitter = "  " },
		"录入者带边空白": func(s *domain.PriceCardDraftSubmission) { s.Submitter = " SYN-OPERATOR-01" },
		"缺录入时刻":   func(s *domain.PriceCardDraftSubmission) { s.SubmittedAt = time.Time{} },
		"问题是零值": func(s *domain.PriceCardDraftSubmission) {
			s.Content, s.Problems = nil, []domain.PriceCardDraftProblem{{}}
		},
	}
	for name, mutate := range cases {
		submission := validatedSubmission(t, plan, source)
		mutate(&submission)
		if _, err := domain.SubmitPriceCardDraft(submission); !errors.Is(err, domain.ErrInvalidPriceCardDraft) {
			t.Errorf("%s: err = %v, want ErrInvalidPriceCardDraft", name, err)
		}
	}
}

// 逐格问题照读口交出的原样记；领域只守形状：码与说明不能空，行号不能为负。表名与列键可空——整表层面的问题没有列键。
func TestADraftProblemKeepsItsCoordinates(t *testing.T) {
	if _, err := domain.NewPriceCardDraftProblem("tables", 0, "", "SHEET_MISSING", "缺表 tables"); err != nil {
		t.Fatalf("整表层面的问题被拒：%v", err)
	}
	for name, build := range map[string]func() error{
		"码为空": func() error { _, err := domain.NewPriceCardDraftProblem("tables", 2, "currency", "", "x"); return err },
		"码带边空白": func() error {
			_, err := domain.NewPriceCardDraftProblem("tables", 2, "currency", " CELL_INVALID", "x")
			return err
		},
		"说明为空": func() error {
			_, err := domain.NewPriceCardDraftProblem("tables", 2, "currency", "CELL_INVALID", " ")
			return err
		},
		"行号为负": func() error {
			_, err := domain.NewPriceCardDraftProblem("tables", -1, "currency", "CELL_INVALID", "x")
			return err
		},
	} {
		if err := build(); !errors.Is(err, domain.ErrInvalidPriceCardDraft) {
			t.Errorf("%s: err = %v, want ErrInvalidPriceCardDraft", name, err)
		}
	}
}

// 同版再录的三种落点（ADR-0126 决定三，价卡照搬）：同内容是重放，不论此刻在哪一格；`已批准`之前换内容是修订；
// `已批准`之后换内容答内容已固定。录入者不是内容：换个人录同一份文件仍是重放。
func TestResubmittingTheSameVersionReplaysRevisesOrMeetsFixedContent(t *testing.T) {
	plan, otherPlan := draftPlan(t, "30"), draftPlan(t, "31")
	source, renamed := draftSource(t, "card.xlsx", synSourceSHA), draftSource(t, "card-final.xlsx", synSourceSHA)
	resaved := draftSource(t, "card.xlsx", otherSourceSHA)
	problem := draftProblem(t, "tables", 2, "currency", "CELL_INVALID")

	validated := submitDraft(t, validatedSubmission(t, plan, source))
	withProblems := submitDraft(t, problemSubmission(t, source, problem))
	approved := withTraces(t, validated, domain.PriceCardDraftStatusApproved)
	published := withTraces(t, validated, domain.PriceCardDraftStatusPublished)

	anotherSubmitter := validatedSubmission(t, plan, source)
	anotherSubmitter.Submitter = "https://id.syn.example/dex#SYN-OPERATOR-02"
	anotherSubmitter.SubmittedAt = draftSubmittedAt.Add(time.Hour)

	cases := map[string]struct {
		existing domain.PriceCardDraft
		incoming domain.PriceCardDraft
		want     domain.PriceCardDraftResubmission
	}{
		"草稿再录同一份":         {withProblems, submitDraft(t, problemSubmission(t, source, problem)), domain.PriceCardDraftResubmissionReplay},
		"已校验再录同一份":        {validated, submitDraft(t, validatedSubmission(t, plan, source)), domain.PriceCardDraftResubmissionReplay},
		"换个人录同一份":         {validated, submitDraft(t, anotherSubmitter), domain.PriceCardDraftResubmissionReplay},
		"草稿改好了再录":         {withProblems, submitDraft(t, validatedSubmission(t, plan, resaved)), domain.PriceCardDraftResubmissionRevision},
		"已校验换了金额":         {validated, submitDraft(t, validatedSubmission(t, otherPlan, resaved)), domain.PriceCardDraftResubmissionRevision},
		"已校验换了文件名":        {validated, submitDraft(t, validatedSubmission(t, plan, renamed)), domain.PriceCardDraftResubmissionRevision},
		"已校验改坏了再录":        {validated, submitDraft(t, problemSubmission(t, resaved, problem)), domain.PriceCardDraftResubmissionRevision},
		"草稿换了一组问题":        {withProblems, submitDraft(t, problemSubmission(t, source, draftProblem(t, "tables", 3, "currency", "CELL_INVALID"))), domain.PriceCardDraftResubmissionRevision},
		"已批准再录同一份":        {approved, submitDraft(t, validatedSubmission(t, plan, source)), domain.PriceCardDraftResubmissionReplay},
		"已批准换了金额":         {approved, submitDraft(t, validatedSubmission(t, otherPlan, resaved)), domain.PriceCardDraftResubmissionContentFixed},
		"已发布换了文件名":        {published, submitDraft(t, validatedSubmission(t, plan, renamed)), domain.PriceCardDraftResubmissionContentFixed},
		"已发布改坏了再录":        {published, submitDraft(t, problemSubmission(t, resaved, problem)), domain.PriceCardDraftResubmissionContentFixed},
		"已发布再录同一份":        {published, submitDraft(t, validatedSubmission(t, plan, source)), domain.PriceCardDraftResubmissionReplay},
		"同一份换个人录进已批准的那一版": {approved, submitDraft(t, anotherSubmitter), domain.PriceCardDraftResubmissionReplay},
	}
	for name, testCase := range cases {
		got, err := testCase.existing.ResubmissionOf(testCase.incoming)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got != testCase.want || got.String() == "" {
			t.Errorf("%s: resubmission = %v, want %v", name, got, testCase.want)
		}
	}
}

// 判落点只对同一版有意义：键不同的两份根本不会撞在一起，交进来就是调用方的错；拿一份已批准的当「再录」同样是。
func TestResubmissionIsOnlyAskedOfTheSameVersion(t *testing.T) {
	plan := draftPlan(t, "30")
	source := draftSource(t, "card.xlsx", synSourceSHA)
	existing := submitDraft(t, validatedSubmission(t, plan, source))

	otherTenant := validatedSubmission(t, plan, source)
	otherTenant.Tenant = mustValue(t, domain.NewTenantID, "SYN-TENANT-02")
	otherVersion := problemSubmission(t, source, draftProblem(t, "tables", 2, "currency", "CELL_INVALID"))
	otherVersion.Plan = versionReference(t, domain.ArtifactPricingPlan, "plan-draft", "v2")

	for name, incoming := range map[string]domain.PriceCardDraft{
		"另一个租户":    submitDraft(t, otherTenant),
		"另一版":      submitDraft(t, otherVersion),
		"已批准的不是录入": withTraces(t, existing, domain.PriceCardDraftStatusApproved),
		"零值":       {},
	} {
		if _, err := existing.ResubmissionOf(incoming); !errors.Is(err, domain.ErrInvalidPriceCardDraft) {
			t.Errorf("%s: err = %v, want ErrInvalidPriceCardDraft", name, err)
		}
	}
}

// 持久化形状留在领域：内容文档经重建门读回，状态、内容与问题逐项同答，再折一次逐字节相同。
func TestADraftRoundTripsThroughTheRehydrationDoor(t *testing.T) {
	plan := draftPlan(t, "30")
	source := draftSource(t, "card.xlsx", synSourceSHA)
	for name, draft := range map[string]domain.PriceCardDraft{
		"草稿":  submitDraft(t, problemSubmission(t, source, draftProblem(t, "tables", 2, "currency", "CELL_INVALID"), draftProblem(t, "tables", 0, "", "SHEET_MISSING"))),
		"已校验": submitDraft(t, validatedSubmission(t, plan, source)),
	} {
		document, err := domain.MarshalPriceCardDraftContent(draft)
		if err != nil {
			t.Fatalf("%s: 折内容文档：%v", name, err)
		}
		rebuilt, err := domain.RehydratePriceCardDraft(specOf(draft, document))
		if err != nil {
			t.Fatalf("%s: 重建：%v", name, err)
		}
		again, err := domain.MarshalPriceCardDraftContent(rebuilt)
		if err != nil {
			t.Fatalf("%s: 重建后再折：%v", name, err)
		}
		if !bytes.Equal(document, again) {
			t.Fatalf("%s: 内容文档往返不同答\n首次=%s\n再次=%s", name, document, again)
		}
		if replay, err := draft.ResubmissionOf(rebuilt); err != nil || replay != domain.PriceCardDraftResubmissionReplay {
			t.Fatalf("%s: 读回的不是同一份内容：%v %v", name, replay, err)
		}
		if rebuilt.Status() != draft.Status() || rebuilt.Submitter() != draft.Submitter() || !rebuilt.SubmittedAt().Equal(draft.SubmittedAt()) {
			t.Fatalf("%s: rebuilt = %+v", name, rebuilt)
		}
	}
	if !strings.Contains(string(mustContentDocument(t, submitDraft(t, validatedSubmission(t, plan, source)))), `"contentDigest":"`+plan.ContentDigest()+`"`) {
		t.Fatal("已校验的内容文档里没有方案快照（复用登记文档那组折装函数）")
	}
}

// 后两格只能从前一格推进而来（推进动作归票 04），所以读回门按格查各自的痕迹：已批准带批准者与不早于录入的批准时刻，
// 已发布再带不早于批准的发布时刻。
func TestApprovedAndPublishedDraftsCarryTheirTraces(t *testing.T) {
	validated := submitDraft(t, validatedSubmission(t, draftPlan(t, "30"), draftSource(t, "card.xlsx", synSourceSHA)))

	approved := withTraces(t, validated, domain.PriceCardDraftStatusApproved)
	if approver, has := approved.Approver(); !has || approver != "SYN-APPROVER" {
		t.Fatalf("approver = %q, %v", approver, has)
	}
	if _, has := approved.PublishedAt(); has {
		t.Fatal("已批准未发布的草稿带了发布时刻")
	}
	published := withTraces(t, validated, domain.PriceCardDraftStatusPublished)
	if at, has := published.PublishedAt(); !has || !at.Equal(draftSubmittedAt.Add(2*time.Hour)) {
		t.Fatalf("publishedAt = %v, %v", at, has)
	}
	if content, has := published.Content(); !has || content.Plan.ContentDigest() == "" {
		t.Fatal("已发布的草稿没有内容")
	}
}

// 读回门拒的是这几条路径都写不出来的行：痕迹与状态对不上、内容与状态对不上、文档里的方案不是键上那一版、摘要被改过。
func TestTheRehydrationDoorRefusesRowsNoPathCouldHaveWritten(t *testing.T) {
	plan := draftPlan(t, "30")
	source := draftSource(t, "card.xlsx", synSourceSHA)
	validated := submitDraft(t, validatedSubmission(t, plan, source))
	withProblems := submitDraft(t, problemSubmission(t, source, draftProblem(t, "tables", 2, "currency", "CELL_INVALID")))
	validatedDocument, problemDocument := mustContentDocument(t, validated), mustContentDocument(t, withProblems)
	approvedAt, publishedAt := draftSubmittedAt.Add(time.Hour), draftSubmittedAt.Add(2*time.Hour)

	cases := map[string]domain.RehydratePriceCardDraftSpec{}
	add := func(name string, base domain.PriceCardDraft, document []byte, mutate func(*domain.RehydratePriceCardDraftSpec)) {
		spec := specOf(base, document)
		mutate(&spec)
		cases[name] = spec
	}
	add("草稿带着方案", withProblems, validatedDocument, func(s *domain.RehydratePriceCardDraftSpec) {})
	add("已校验没有方案", validated, problemDocument, func(s *domain.RehydratePriceCardDraftSpec) {})
	add("草稿一条问题也没有", withProblems, []byte(`{}`), func(s *domain.RehydratePriceCardDraftSpec) {})
	add("已校验带着批准痕迹", validated, validatedDocument, func(s *domain.RehydratePriceCardDraftSpec) {
		s.Approver, s.ApprovedAt = "SYN-APPROVER", approvedAt
	})
	add("已批准缺批准者", validated, validatedDocument, func(s *domain.RehydratePriceCardDraftSpec) {
		s.Status, s.ApprovedAt = domain.PriceCardDraftStatusApproved, approvedAt
	})
	add("已批准早于录入", validated, validatedDocument, func(s *domain.RehydratePriceCardDraftSpec) {
		s.Status, s.Approver, s.ApprovedAt = domain.PriceCardDraftStatusApproved, "SYN-APPROVER", draftSubmittedAt.Add(-time.Minute)
	})
	add("已批准带着发布时刻", validated, validatedDocument, func(s *domain.RehydratePriceCardDraftSpec) {
		s.Status, s.Approver, s.ApprovedAt, s.PublishedAt = domain.PriceCardDraftStatusApproved, "SYN-APPROVER", approvedAt, publishedAt
	})
	add("已发布缺发布时刻", validated, validatedDocument, func(s *domain.RehydratePriceCardDraftSpec) {
		s.Status, s.Approver, s.ApprovedAt = domain.PriceCardDraftStatusPublished, "SYN-APPROVER", approvedAt
	})
	add("已发布早于批准", validated, validatedDocument, func(s *domain.RehydratePriceCardDraftSpec) {
		s.Status, s.Approver, s.ApprovedAt, s.PublishedAt = domain.PriceCardDraftStatusPublished, "SYN-APPROVER", approvedAt, approvedAt.Add(-time.Second)
	})
	add("草稿带着批准痕迹", withProblems, problemDocument, func(s *domain.RehydratePriceCardDraftSpec) {
		s.Status, s.Approver, s.ApprovedAt = domain.PriceCardDraftStatusApproved, "SYN-APPROVER", approvedAt
	})
	add("文档里的方案不是键上那一版", validated, validatedDocument, func(s *domain.RehydratePriceCardDraftSpec) {
		s.Plan = versionReference(t, domain.ArtifactPricingPlan, "plan-draft", "v2")
	})
	add("状态不在四格里", validated, validatedDocument, func(s *domain.RehydratePriceCardDraftSpec) { s.Status = domain.PriceCardDraftStatus(9) })
	add("录入者为空", validated, validatedDocument, func(s *domain.RehydratePriceCardDraftSpec) { s.Submitter = "" })
	add("文档不是 JSON", validated, []byte(`not json`), func(s *domain.RehydratePriceCardDraftSpec) {})
	add("摘要被改过", validated, tamperedDocument(t, validatedDocument, func(document map[string]any) {
		document["plan"].(map[string]any)["contentDigest"] = "sha256:not-the-digest"
	}), func(s *domain.RehydratePriceCardDraftSpec) {})
	add("方向授权不是授权工件", validated, tamperedDocument(t, validatedDocument, func(document map[string]any) {
		document["directionAuthorization"].(map[string]any)["kind"] = string(domain.ArtifactRateTable)
	}), func(s *domain.RehydratePriceCardDraftSpec) {})

	for name, spec := range cases {
		if _, err := domain.RehydratePriceCardDraft(spec); !errors.Is(err, domain.ErrInvalidRehydratedPriceCardDraft) {
			t.Errorf("%s: err = %v, want ErrInvalidRehydratedPriceCardDraft", name, err)
		}
	}

	foreign := tamperedDocument(t, validatedDocument, func(document map[string]any) {
		document["plan"].(map[string]any)["canonicalization"] = "PPC-2"
	})
	if _, err := domain.RehydratePriceCardDraft(specOf(validated, foreign)); !errors.Is(err, domain.ErrCanonicalizationVersionUnsupported) {
		t.Fatalf("别的规范化版本：err = %v, want ErrCanonicalizationVersionUnsupported", err)
	}
}

// 库上的列与查阅读口的筛选都写状态名：四格逐一认得回来，别的名字不认。
func TestEveryDraftStatusIsNamedAndParsedBack(t *testing.T) {
	for _, status := range []domain.PriceCardDraftStatus{
		domain.PriceCardDraftStatusDraft, domain.PriceCardDraftStatusValidated,
		domain.PriceCardDraftStatusApproved, domain.PriceCardDraftStatusPublished,
	} {
		parsed, ok := domain.ParsePriceCardDraftStatus(status.String())
		if !ok || parsed != status {
			t.Errorf("%v: parsed = %v, %v", status, parsed, ok)
		}
	}
	for _, name := range []string{"", "draft", "PENDING_APPROVAL"} {
		if _, ok := domain.ParsePriceCardDraftStatus(name); ok {
			t.Errorf("%q 被认成了一格状态", name)
		}
	}
}

func mustContentDocument(t *testing.T, draft domain.PriceCardDraft) []byte {
	t.Helper()
	document, err := domain.MarshalPriceCardDraftContent(draft)
	if err != nil {
		t.Fatalf("折内容文档：%v", err)
	}
	return document
}

func specOf(draft domain.PriceCardDraft, document []byte) domain.RehydratePriceCardDraftSpec {
	return domain.RehydratePriceCardDraftSpec{
		Tenant:      draft.Tenant(),
		Plan:        draft.Plan(),
		Status:      draft.Status(),
		Source:      draft.SourceFile(),
		Document:    document,
		Submitter:   draft.Submitter(),
		SubmittedAt: draft.SubmittedAt(),
	}
}

// withTraces 经重建门造一份已批准或已发布的草稿：推进动作归票 04，本票只立这两格的形与读回门。
func withTraces(t *testing.T, validated domain.PriceCardDraft, status domain.PriceCardDraftStatus) domain.PriceCardDraft {
	t.Helper()
	spec := specOf(validated, mustContentDocument(t, validated))
	spec.Status, spec.Approver, spec.ApprovedAt = status, "SYN-APPROVER", draftSubmittedAt.Add(time.Hour)
	if status == domain.PriceCardDraftStatusPublished {
		spec.PublishedAt = draftSubmittedAt.Add(2 * time.Hour)
	}
	draft, err := domain.RehydratePriceCardDraft(spec)
	if err != nil {
		t.Fatalf("重建 %v 草稿：%v", status, err)
	}
	return draft
}

func tamperedDocument(t *testing.T, document []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatalf("解内容文档：%v", err)
	}
	mutate(decoded)
	tampered, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("重封内容文档：%v", err)
	}
	return tampered
}
