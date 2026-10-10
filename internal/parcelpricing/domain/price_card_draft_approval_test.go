package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证价卡草稿的批准与发布推进（ADR-0101 决定五、六；票 price-card-import/04）：批准门按租户的审批职责规则逐格裁，
// 只接`已校验`；发布交出的登记就是草稿上记下的那几样加批准者，摘要原样带过去；发布时刻只跟在批准之后。

const draftSubmitter = "https://id.syn.example/dex#SYN-OPERATOR-01"

var draftApprovedAt = draftSubmittedAt.Add(time.Hour)

func grant(t *testing.T, name string) domain.OperatorGrant {
	t.Helper()
	return mustValue(t, domain.NewOperatorGrant, name)
}

func operatorSubject(t *testing.T, reference string, grants ...string) domain.OperatorSubject {
	t.Helper()
	held := make([]domain.OperatorGrant, 0, len(grants))
	for _, name := range grants {
		held = append(held, grant(t, name))
	}
	subject, err := domain.NewOperatorSubject(reference, held)
	if err != nil {
		t.Fatalf("构造操作者主体：%v", err)
	}
	return subject
}

// approvalDutyRule 立 SYN-TENANT-01 的一条规则；requiredGrant 为空即不要求授予。
func approvalDutyRule(t *testing.T, distinct bool, requiredGrant string) domain.PriceCardApprovalDutyRule {
	t.Helper()
	var required domain.OperatorGrant
	if requiredGrant != "" {
		required = grant(t, requiredGrant)
	}
	rule, err := domain.NewPriceCardApprovalDutyRule(mustValue(t, domain.NewTenantID, "SYN-TENANT-01"), distinct, required)
	if err != nil {
		t.Fatalf("构造审批职责规则：%v", err)
	}
	return rule
}

func validatedDraft(t *testing.T) domain.PriceCardDraft {
	t.Helper()
	return submitDraft(t, validatedSubmission(t, draftPlan(t, "30"), draftSource(t, "card.xlsx", synSourceSHA)))
}

// Covers: ADR-0101 决定六 / spec 自决第 2 格——审批职责规则逐格裁：要求不同主体时录入者自批被拒；要求持某一格授予时
// 授予集里没有它被拒；两格都过即转`已批准`并记下批准者与批准时刻；两格都不要求是租户说出的「单人可批」，录入者自批也放行。
func TestApprovalIsGatedByTheApprovalDutyRule(t *testing.T) {
	validated := validatedDraft(t)

	if _, err := validated.Approve(operatorSubject(t, draftSubmitter, "SYN-GRANT-PRICING-SUPERVISOR"),
		approvalDutyRule(t, true, ""), draftApprovedAt); !errors.Is(err, domain.ErrDraftApproverIsSubmitter) {
		t.Fatalf("录入者自批：err = %v，want ErrDraftApproverIsSubmitter", err)
	}
	if _, err := validated.Approve(operatorSubject(t, "https://id.syn.example/dex#SYN-OPERATOR-02", "REGISTRY_CONFIGURATION_WRITE"),
		approvalDutyRule(t, false, "SYN-GRANT-PRICING-SUPERVISOR"), draftApprovedAt); !errors.Is(err, domain.ErrDraftApproverLacksRequiredGrant) {
		t.Fatalf("授予不足：err = %v，want ErrDraftApproverLacksRequiredGrant", err)
	}

	approved, err := validated.Approve(operatorSubject(t, "https://id.syn.example/dex#SYN-OPERATOR-02", "SYN-GRANT-PRICING-SUPERVISOR"),
		approvalDutyRule(t, true, "SYN-GRANT-PRICING-SUPERVISOR"), draftApprovedAt)
	if err != nil {
		t.Fatalf("两格都过：%v", err)
	}
	if approved.Status() != domain.PriceCardDraftStatusApproved {
		t.Fatalf("status = %v，want APPROVED", approved.Status())
	}
	if approver, has := approved.Approver(); !has || approver != "https://id.syn.example/dex#SYN-OPERATOR-02" {
		t.Fatalf("approver = %q, %v", approver, has)
	}
	if at, has := approved.ApprovedAt(); !has || !at.Equal(draftApprovedAt) {
		t.Fatalf("approvedAt = %v, %v", at, has)
	}
	if validated.Status() != domain.PriceCardDraftStatusValidated {
		t.Fatal("批准改了接收者：草稿是值类型，推进只交回新值")
	}

	selfApproved, err := validated.Approve(operatorSubject(t, draftSubmitter), approvalDutyRule(t, false, ""), draftApprovedAt)
	if err != nil {
		t.Fatalf("两格都不要求时录入者自批：%v", err)
	}
	if approver, _ := selfApproved.Approver(); approver != draftSubmitter {
		t.Fatalf("approver = %q，want 录入者本人", approver)
	}
}

// Covers: 票 04 第 3 条「只接`已校验`」——`草稿`没有可批的内容，`已批准`与`已发布`已过了这一格，三者都不重复批。
func TestOnlyAValidatedDraftCanBeApproved(t *testing.T) {
	validated := validatedDraft(t)
	drafts := map[string]domain.PriceCardDraft{
		"DRAFT":     submitDraft(t, problemSubmission(t, draftSource(t, "card.xlsx", synSourceSHA), draftProblem(t, "价表", 3, "base", "AMOUNT_INVALID"))),
		"APPROVED":  withTraces(t, validated, domain.PriceCardDraftStatusApproved),
		"PUBLISHED": withTraces(t, validated, domain.PriceCardDraftStatusPublished),
	}
	for name, draft := range drafts {
		t.Run(name, func(t *testing.T) {
			_, err := draft.Approve(operatorSubject(t, "https://id.syn.example/dex#SYN-OPERATOR-02"),
				approvalDutyRule(t, false, ""), draftApprovedAt.Add(time.Hour))
			if !errors.Is(err, domain.ErrPriceCardDraftNotValidated) {
				t.Fatalf("err = %v，want ErrPriceCardDraftNotValidated", err)
			}
		})
	}
}

// 规则不是本租户的、规则或批准者是零值、批准时刻缺或早于录入：都是装配或调用方的错，不是业务答案，各给哨兵。
func TestApprovalRefusesInputsThatAreNotABusinessAnswer(t *testing.T) {
	validated := validatedDraft(t)
	approver := operatorSubject(t, "https://id.syn.example/dex#SYN-OPERATOR-02")

	otherTenant, err := domain.NewPriceCardApprovalDutyRule(mustValue(t, domain.NewTenantID, "SYN-TENANT-02"), false, domain.OperatorGrant{})
	if err != nil {
		t.Fatalf("构造他租户规则：%v", err)
	}
	if _, err := validated.Approve(approver, otherTenant, draftApprovedAt); !errors.Is(err, domain.ErrPriceCardApprovalDutyRuleTenantMismatch) {
		t.Fatalf("他租户规则：err = %v", err)
	}
	if _, err := validated.Approve(approver, domain.PriceCardApprovalDutyRule{}, draftApprovedAt); !errors.Is(err, domain.ErrInvalidPriceCardApprovalDutyRule) {
		t.Fatalf("零值规则：err = %v", err)
	}
	if _, err := validated.Approve(domain.OperatorSubject{}, approvalDutyRule(t, false, ""), draftApprovedAt); !errors.Is(err, domain.ErrInvalidOperatorSubject) {
		t.Fatalf("零值批准者：err = %v", err)
	}
	for name, at := range map[string]time.Time{"缺时刻": {}, "早于录入": draftSubmittedAt.Add(-time.Second)} {
		if _, err := validated.Approve(approver, approvalDutyRule(t, false, ""), at); !errors.Is(err, domain.ErrInvalidPriceCardDraft) {
			t.Fatalf("%s：err = %v", name, err)
		}
	}
}

// Covers: ADR-0101 决定四、五——发布交给登记的就是草稿上那一份：方案是同一个值，规范化版本与内容摘要逐字节相等；源文件
// 身份与方向授权引用照抄；发布批准责任方是批准者主体，不是录入者。没批准的草稿没有登记可交。
func TestTheRegistrationIsTheApprovedDraftItself(t *testing.T) {
	validated := validatedDraft(t)
	if _, err := validated.Registration(); !errors.Is(err, domain.ErrPriceCardDraftNotApproved) {
		t.Fatalf("已校验未批准：err = %v，want ErrPriceCardDraftNotApproved", err)
	}

	approved, err := validated.Approve(operatorSubject(t, "https://id.syn.example/dex#SYN-OPERATOR-02"), approvalDutyRule(t, true, ""), draftApprovedAt)
	if err != nil {
		t.Fatalf("批准：%v", err)
	}
	registration, err := approved.Registration()
	if err != nil {
		t.Fatalf("交登记：%v", err)
	}
	content, _ := approved.Content()
	if registration.Plan().ContentDigest() != content.Plan.ContentDigest() ||
		registration.Plan().CanonicalizationVersion() != content.Plan.CanonicalizationVersion() {
		t.Fatalf("摘要漂移：登记 %s/%s，草稿 %s/%s",
			registration.Plan().CanonicalizationVersion(), registration.Plan().ContentDigest(),
			content.Plan.CanonicalizationVersion(), content.Plan.ContentDigest())
	}
	if registration.Tenant() != approved.Tenant() || registration.SourceFile() != approved.SourceFile() ||
		!registration.DirectionAuthorization().SameIdentity(content.DirectionAuthorization) {
		t.Fatalf("登记没照抄草稿：%+v", registration)
	}
	if registration.PublicationApprover() != "https://id.syn.example/dex#SYN-OPERATOR-02" {
		t.Fatalf("publicationApprover = %q，want 批准者主体", registration.PublicationApprover())
	}
	published, err := approved.MarkPublished(draftApprovedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("记发布：%v", err)
	}
	if again, err := published.Registration(); err != nil || again.Plan().ContentDigest() != registration.Plan().ContentDigest() {
		t.Fatalf("已发布草稿交回的登记变了：%v", err)
	}
}

// 发布时刻只跟在批准之后：`已校验`记不了发布；早于批准立不住；记下之后整份过重建门读得回来。
func TestPublicationIsRecordedOnlyAfterApproval(t *testing.T) {
	validated := validatedDraft(t)
	if _, err := validated.MarkPublished(draftApprovedAt); !errors.Is(err, domain.ErrPriceCardDraftNotApproved) {
		t.Fatalf("已校验记发布：err = %v", err)
	}
	approved, err := validated.Approve(operatorSubject(t, "https://id.syn.example/dex#SYN-OPERATOR-02"), approvalDutyRule(t, false, ""), draftApprovedAt)
	if err != nil {
		t.Fatalf("批准：%v", err)
	}
	if _, err := approved.MarkPublished(draftApprovedAt.Add(-time.Second)); !errors.Is(err, domain.ErrInvalidPriceCardDraft) {
		t.Fatalf("发布早于批准：err = %v", err)
	}
	publishedAt := draftApprovedAt.Add(time.Minute)
	published, err := approved.MarkPublished(publishedAt)
	if err != nil {
		t.Fatalf("记发布：%v", err)
	}
	if _, err := published.MarkPublished(publishedAt); !errors.Is(err, domain.ErrPriceCardDraftNotApproved) {
		t.Fatalf("重复记发布：err = %v", err)
	}

	for _, draft := range []domain.PriceCardDraft{approved, published} {
		spec := specOf(draft, mustContentDocument(t, draft))
		spec.Approver, _ = draft.Approver()
		spec.ApprovedAt, _ = draft.ApprovedAt()
		spec.PublishedAt, _ = draft.PublishedAt()
		if _, err := domain.RehydratePriceCardDraft(spec); err != nil {
			t.Fatalf("%v 草稿过不了重建门：%v", draft.Status(), err)
		}
	}
}

// 操作者主体与规则的构造门：主体引用必带、授予集里不许有空格；规则的租户必带，要求的授予可缺（零值 = 不要求）。
func TestOperatorSubjectAndApprovalDutyRuleConstruction(t *testing.T) {
	if _, err := domain.NewOperatorSubject(" ", nil); !errors.Is(err, domain.ErrInvalidOperatorSubject) {
		t.Fatalf("空主体引用：err = %v", err)
	}
	if _, err := domain.NewOperatorSubject("op", []domain.OperatorGrant{{}}); !errors.Is(err, domain.ErrInvalidOperatorSubject) {
		t.Fatalf("授予集里的零值：err = %v", err)
	}
	if _, err := domain.NewOperatorGrant(" "); err == nil {
		t.Fatal("空授予名应被拒")
	}
	subject := operatorSubject(t, "op", "A", "B")
	if !subject.Holds(grant(t, "B")) || subject.Holds(grant(t, "C")) {
		t.Fatalf("Holds 答错：%v", subject.Grants())
	}
	if _, err := domain.NewPriceCardApprovalDutyRule(domain.TenantID{}, true, domain.OperatorGrant{}); !errors.Is(err, domain.ErrInvalidPriceCardApprovalDutyRule) {
		t.Fatalf("缺租户：err = %v", err)
	}
	loose := approvalDutyRule(t, false, "")
	if loose.RequiresDistinctSubjects() {
		t.Fatal("不要求不同主体的规则答成了要求")
	}
	if _, required := loose.RequiredGrant(); required {
		t.Fatal("不要求授予的规则答成了要求")
	}
}
