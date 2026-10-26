package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	pricingdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

const catalogueReviewsTarget = "/pricing-reference-catalogue-reviews"

// catalogueReviewStage 在一只真库上登演示种子那一版分区目录（SYN-TENANT-01，登记者 SYN-PRICING-OPS-01），装生产复核编排与
// 经操作者渠道的目录复核端点：生产铸造器、PP 认证适配器与操作者 Intake，只有令牌校验与操作者册是替身——那位合成操作者绑
// SYN-TENANT-01、持登记册配置写一格授予。
type catalogueReviewStage struct {
	db       *bentopg.DB
	pool     *pgxpool.Pool
	endpoint http.Handler
	rules    *pppostgres.ReferenceReviewDutyRules
}

func newCatalogueReviewStage(t *testing.T) catalogueReviewStage {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing", "reference-catalogue-zone-cn-sg.json"))
	if err != nil {
		t.Fatalf("读演示目录种子：%v", err)
	}
	catalogue, err := pricingdomain.RehydrateReferenceCatalogueRegistration(raw)
	if err != nil {
		t.Fatalf("重建演示目录：%v", err)
	}
	register, err := buildReferenceCatalogueRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配目录登记编排：%v", err)
	}
	if outcome, err := register.Handle(t.Context(), pricingapp.RegisterReferenceCatalogueCommand{Registration: catalogue}); err != nil || outcome != pricingapp.ReferenceCatalogueRecorded {
		t.Fatalf("登演示目录：outcome = %s err = %v", outcome, err)
	}
	review, err := buildReferenceCatalogueReviewOrchestration(db)
	if err != nil {
		t.Fatalf("装配目录复核编排：%v", err)
	}
	rules, err := pppostgres.NewReferenceReviewDutyRules(db)
	if err != nil {
		t.Fatal(err)
	}
	return catalogueReviewStage{
		db:       db,
		pool:     pool,
		endpoint: pricinghttp.NewReviewReferenceCatalogueEndpoint(operatorRegistriesForRegistryWriter(t).pricing, review),
		rules:    rules,
	}
}

func (stage catalogueReviewStage) registerRule(t *testing.T, distinct bool, grant string) {
	t.Helper()
	var required pricingdomain.OperatorGrant
	if grant != "" {
		required = mustValue(t, pricingdomain.NewOperatorGrant, grant)
	}
	rule, err := pricingdomain.NewReferenceReviewDutyRule(mustValue(t, pricingdomain.NewTenantID, "SYN-TENANT-01"), distinct, required)
	if err != nil {
		t.Fatal(err)
	}
	if err := stage.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		outcome, err := stage.rules.SaveReferenceReviewDutyRule(ctx, rule)
		if err == nil && outcome != ports.ReferenceReviewDutyRuleSaved {
			t.Errorf("登规则 = %s", outcome)
		}
		return err
	}); err != nil {
		t.Fatalf("登规则：%v", err)
	}
}

func (stage catalogueReviewStage) review(t *testing.T, body string) (int, string) {
	t.Helper()
	response := postAsOperator(stage.endpoint, catalogueReviewsTarget, presentedOperatorToken, body)
	var answer struct {
		Outcome string `json:"outcome"`
	}
	if response.Code < 300 {
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatalf("解答复 %s：%v", response.Body.String(), err)
		}
	}
	return response.Code, answer.Outcome
}

func (stage catalogueReviewStage) reviewRows(t *testing.T) int {
	t.Helper()
	var count int
	if err := stage.pool.QueryRow(t.Context(), `SELECT count(*) FROM parcel_pricing.reference_catalogue_review WHERE tenant_id = 'SYN-TENANT-01'`).Scan(&count); err != nil {
		t.Fatalf("数复核行：%v", err)
	}
	return count
}

const demoCatalogueReview = `{"catalogueId":"SYN-CAT-ZONE-CN-SG","catalogueVersion":"v1","decision":"APPROVED",` +
	`"basis":"SYN-REVIEW/合成分区目录逐条对照公布表（S 级）","reviewedAt":"2026-01-02T00:00:00Z"}`

// Covers: AT-PP-014 与 AT-PP-015 的目录操作者面——`/pricing-reference-catalogue-reviews` 经操作者渠道走到真库：租户没登规则答
// 200 NOT_CONFIGURED、不落复核行；载荷自报复核人或带系统代写那一格答 400；规则要求一格操作者没持有的授予答 200
// REVIEWER_NOT_QUALIFIED、仍不落行。测试输入是隔离合成，只记 `S`。
func TestTheOperatorChannelReviewsACatalogueOnlyUnderTheTenantRuleAgainstARealDatabase(t *testing.T) {
	stage := newCatalogueReviewStage(t)

	if status, outcome := stage.review(t, demoCatalogueReview); status != http.StatusOK || outcome != "NOT_CONFIGURED" {
		t.Fatalf("没登规则：%d %s", status, outcome)
	}
	for _, smuggled := range []string{`"reviewer":"SYN-PRICING-REVIEW-01",`, `"systemDelegatedUnderExemption":true,`} {
		response := postAsOperator(stage.endpoint, catalogueReviewsTarget, presentedOperatorToken, `{`+smuggled+demoCatalogueReview[1:])
		if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Fatalf("载荷带 %s 答 %d %s", smuggled, response.Code, response.Body.String())
		}
	}
	stage.registerRule(t, true, "SYN-GRANT-REFERENCE-REVIEW")
	if status, outcome := stage.review(t, demoCatalogueReview); status != http.StatusOK || outcome != "REVIEWER_NOT_QUALIFIED" {
		t.Fatalf("要求一格没持有的授予：%d %s", status, outcome)
	}
	if n := stage.reviewRows(t); n != 0 {
		t.Fatalf("拒下的复核落了 %d 行", n)
	}
}

// Covers: AT-PP-015 的目录操作者面（放行那一支）——规则要求的那一格授予正是操作者信封里持有的，另一个人复核答 201 RECORDED，
// 那一版自复核时刻起在用：要求授予的租户经这一口复核得了目录，受控登记口那一路做不到（ADR-0262 越权风险点 4）。
func TestTheOperatorChannelReviewsACatalogueWithTheRequiredGrantAgainstARealDatabase(t *testing.T) {
	stage := newCatalogueReviewStage(t)
	stage.registerRule(t, true, "REGISTRY_CONFIGURATION_WRITE")
	if status, outcome := stage.review(t, demoCatalogueReview); status != http.StatusCreated || outcome != "RECORDED" {
		t.Fatalf("持有要求的授予：%d %s", status, outcome)
	}
	reviews, err := pppostgres.NewReferenceCatalogueReviews(stage.db)
	if err != nil {
		t.Fatal(err)
	}
	inForce, outcome, err := reviews.ResolveInForce(t.Context(), mustValue(t, pricingdomain.NewTenantID, "SYN-TENANT-01"),
		pricingdomain.CatalogueKindZone, "SYN-CAT-ZONE-CN-SG", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil || outcome != ports.CatalogueVersionInForce || inForce.Version() != "v1" {
		t.Fatalf("复核通过的目录没进在用：%v outcome=%d err=%v", inForce, outcome, err)
	}
}

// registerReferenceReviewDutyRule 给一个租户登一条要求不同主体、不要求授予的参考复核职责规则——演示租户登的就是这一形。
func registerReferenceReviewDutyRule(t *testing.T, db *bentopg.DB, tenant string) {
	t.Helper()
	rules, err := pppostgres.NewReferenceReviewDutyRules(db)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := pricingdomain.NewReferenceReviewDutyRule(mustValue(t, pricingdomain.NewTenantID, tenant), true, pricingdomain.OperatorGrant{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := rules.SaveReferenceReviewDutyRule(ctx, rule)
		return err
	}); err != nil {
		t.Fatalf("登参考复核职责规则：%v", err)
	}
}
