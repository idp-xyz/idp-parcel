package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证进程口的批推进（syn-wall-door-audit 票 03 件 3）。
//
// 它补的是 application 层 TestAConflictingItemDoesNotRetractAnEarlierSavedItem 拿不到
// 的东西：那条走登记册替身，证不了写进去的行真的在库里。本条端到端跑 runPublish——
// 前一项的行必须真已提交，否则后一项那次同对象异正文根本撞不出 CONTENT_CONFLICT，
// 断言因此不可能空过。
//
// **它守不住事务边界，别指望**：AT-PC-011 的「逐项独立成败」在本进程口靠的是循环里
// 每项一个事务，而**冲突不是错误**——把循环整个包进一个事务，第二项照样判冲突、
// 事务照样提交，本条依旧绿。真要钉住那个结构，得让后一项以技术失败收场再看前一项
// 还在不在，而技术失败今天只来自基础设施故障，从批文里造不出来。这一格没有守门人，
// 照实记在此处与票 03。

// freshMigratedDSN 建一个空库、施加真实迁移计划，交回连接串。
//
// 不能用 pgtest.Pool：它交回的是池，而进程口按连接串自己开池（openDatabase），
// 测试要能把同一个库喂给它。
func freshMigratedDSN(t *testing.T) string {
	t.Helper()
	dsn := pgtest.FreshDatabase(t)

	ctx := t.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("连接测试库：%v", err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := migrate.Run(ctx, conn, quiet); err != nil {
		t.Fatalf("施加迁移计划：%v", err)
	}
	if err := conn.Close(ctx); err != nil {
		t.Fatalf("关闭迁移连接：%v", err)
	}
	return dsn
}

func batchFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("写批文：%v", err)
	}
	return path
}

// rulePackageItem 造一份最小发布项：不带声明、生效边界已开，落库即取效。
func rulePackageItem(objectID, digest string) string {
	return `{
      "tenantId": "tenant-1",
      "kind": "ACCEPTANCE_RULE_PACKAGE",
      "objectId": "` + objectID + `",
      "version": "v1",
      "scope": "scope-1",
      "contentDigest": "` + digest + `",
      "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "approval": {
        "reference": "approval-` + objectID + `",
        "source": "source-` + objectID + `",
        "approvedAt": "2026-01-02T00:00:00Z"
      },
      "approvalRoleStanding": "CONFIRMED"
    }`
}

func countVersions(t *testing.T, dsn, objectID string) int {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("开池核对：%v", err)
	}
	defer pool.Close()

	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM party_commercial.commercial_version
		  WHERE tenant_id = $1 AND object_id = $2`,
		"tenant-1", objectID,
	).Scan(&count); err != nil {
		t.Fatalf("统计版本行：%v", err)
	}
	return count
}

func runCLI(t *testing.T, dsn string, args ...string) int {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, func(key string) string {
		if key == envDatabaseDSN {
			return dsn
		}
		return ""
	}, &out, &errOut)
	t.Logf("stdout:\n%s", out.String())
	if errOut.Len() > 0 {
		t.Logf("stderr:\n%s", errOut.String())
	}
	return code
}

// Covers: AT-PC-011「发布批逐项独立成败」的进程口半边——同一批里后一项撞内容冲突，
// 不得把前一项已落库的发布带走。
func TestPublishBatchKeepsEarlierItemWhenALaterItemConflicts(t *testing.T) {
	dsn := freshMigratedDSN(t)

	// 先把 rules-conflict 按一份正文发出去，好让第二批里的同对象异正文撞上冲突。
	seeded := batchFile(t, `{"items":[`+rulePackageItem("rules-conflict", "sha256:original")+`]}`)
	if code := runCLI(t, dsn, "publish", "-input", seeded); code != exitLanded {
		t.Fatalf("铺垫批 exit = %d, want %d", code, exitLanded)
	}

	// 第二批两项：全新对象在前，异正文的同对象在后。
	batch := batchFile(t, `{"items":[`+
		rulePackageItem("rules-fresh", "sha256:fresh")+`,`+
		rulePackageItem("rules-conflict", "sha256:CHANGED")+
		`]}`)
	if code := runCLI(t, dsn, "publish", "-input", batch); code != exitAttention {
		t.Fatalf("混合批 exit = %d, want %d（有冲突要报请人看）", code, exitAttention)
	}

	if n := countVersions(t, dsn, "rules-fresh"); n != 1 {
		t.Fatalf("rules-fresh 版本行 = %d, want 1——后一项冲突把前一项已落库的发布带走了", n)
	}
	if n := countVersions(t, dsn, "rules-conflict"); n != 1 {
		t.Fatalf("rules-conflict 版本行 = %d, want 1——冲突不得顶替也不得追加第二版", n)
	}
}

// settlementBatchBody 造一份两项批：先发客户合同，再发结算政策连同它的六维正文。
// 合同在前是必需的——结算政策的版本壳指名它，指名引用未发布时发布停在未决。
const settlementBatchBody = `{"items": [
    {
      "tenantId": "tenant-1",
      "kind": "CUSTOMER_CONTRACT",
      "objectId": "contract-1",
      "version": "v1",
      "scope": "scope-1",
      "contentDigest": "sha256:contract-1",
      "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "approval": {"reference": "approval-contract-1", "source": "source-1", "approvedAt": "2025-12-15T00:00:00Z"},
      "approvalRoleStanding": "CONFIRMED"
    },
    {
      "tenantId": "tenant-1",
      "kind": "SETTLEMENT_POLICY",
      "objectId": "settlement-1",
      "version": "v1",
      "scope": "scope-1",
      "contentDigest": "sha256:settlement-1",
      "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "references": {"CUSTOMER_CONTRACT": "contract-1"},
      "approval": {"reference": "approval-settlement-1", "source": "source-1", "approvedAt": "2025-12-15T00:00:00Z"},
      "approvalRoleStanding": "CONFIRMED",
      "declarations": {
        "settlementPolicyBody": {
          "method": "PREPAID",
          "legalEntity": "legal-1",
          "counterparty": "customer-1",
          "contract": {"objectId": "contract-1", "version": "v1"},
          "chargeScope": "charge-prepaid",
          "currency": "CNY",
          "effectiveStartsAt": "2026-01-01T00:00:00Z"
        }
      }
    }
  ]}`

func loadScope(t *testing.T, dsn, tenant, scope string) *pcdomain.CommercialRegistry {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("开池装载整册：%v", err)
	}
	defer pool.Close()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("框架 DB：%v", err)
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造发布登记册：%v", err)
	}
	tenantID, err := pcdomain.NewTenantID(tenant)
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	scopeRef, err := pcdomain.NewCommercialScopeReference(scope)
	if err != nil {
		t.Fatalf("范围：%v", err)
	}
	registry, err := publications.LoadForScope(t.Context(), tenantID, scopeRef)
	if err != nil {
		t.Fatalf("装载整册：%v", err)
	}
	return registry
}

// settlementKey 造一把请求结算依据的解析键。合同维走 NewQualifiedVersionLabel，与发布
// 通道那侧同出一处（ADR-0080）。
func settlementKey(t *testing.T, contractVersion, chargeScope, currency string) pcdomain.ResolutionKey {
	t.Helper()
	object, err := pcdomain.NewCommercialObjectID("contract-1")
	if err != nil {
		t.Fatalf("合同对象：%v", err)
	}
	label, err := pcdomain.NewCommercialVersionLabel(contractVersion)
	if err != nil {
		t.Fatalf("合同版本号：%v", err)
	}
	contract, err := pcdomain.NewQualifiedVersionLabel(object, label)
	if err != nil {
		t.Fatalf("两段式合同指称：%v", err)
	}
	anchorPolicy, err := pcdomain.NewAnchorPolicyVersion("anchor-policy/v1")
	if err != nil {
		t.Fatalf("锚点策略：%v", err)
	}
	anchor, err := pcdomain.NewSelectionAnchor(
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), anchorPolicy)
	if err != nil {
		t.Fatalf("选用锚点：%v", err)
	}
	key := pcdomain.ResolutionKey{
		RequiredBasis: pcdomain.SettlementPolicyObject,
		Purpose:       pcdomain.AcceptanceControlPurpose,
		Anchor:        anchor,
		Settlement:    pcdomain.SettlementSelector{Contract: contract},
	}
	if key.TenantID, err = pcdomain.NewTenantID("tenant-1"); err != nil {
		t.Fatalf("租户：%v", err)
	}
	if key.CustomerAccountID, err = pcdomain.NewCustomerAccountID("customer-1"); err != nil {
		t.Fatalf("客户账户：%v", err)
	}
	if key.LegalEntityCandidate, err = pcdomain.NewLegalEntityReference("legal-1"); err != nil {
		t.Fatalf("责任法人：%v", err)
	}
	if key.Scope, err = pcdomain.NewCommercialScopeReference("scope-1"); err != nil {
		t.Fatalf("范围：%v", err)
	}
	if key.Settlement.Counterparty, err = pcdomain.NewCounterpartyReference("customer-1"); err != nil {
		t.Fatalf("相对方：%v", err)
	}
	if key.Settlement.ChargeScope, err = pcdomain.NewChargeScopeReference(chargeScope); err != nil {
		t.Fatalf("费用范围：%v", err)
	}
	if key.Settlement.Currency, err = pcdomain.NewCurrencyCode(currency); err != nil {
		t.Fatalf("币种：%v", err)
	}
	return key
}

// Covers: 票 commercial-closure-settlement-key/02 的完成标准前两条，端到端于真库——
// 经进程口发出去的结算政策正文真能被解析选中，且六维差一维就命不中。
//
// 它补的是翻译用例与应用用例都拿不到的东西：那两条各自证「批文没变形」与「正文交到了
// 持久化面」，证不了「写进库的行读回来还是那六维」。本条走完发布→落库→装载整册→解析
// 一整条路，任何一维在库里被截断或归并，第一段断言就红。
//
// 差一维那三格是这条路上唯一的守门人：本上下文禁止借宽泛关系跨维归集，而适用范围一旦
// 少一维写进库，它读得回来、看着也像一份正常政策，只是命中的范围比商业责任方约定的宽。
func TestAPublishedSettlementPolicyIsAdoptedOnlyOnTheExactSixDimensions(t *testing.T) {
	dsn := freshMigratedDSN(t)
	if code := runCLI(t, dsn, "publish", "-input", batchFile(t, settlementBatchBody)); code != exitLanded {
		t.Fatalf("结算政策批 exit = %d, want %d", code, exitLanded)
	}

	registry := loadScope(t, dsn, "tenant-1", "scope-1")
	result := pcdomain.ResolveCommercialBasis(registry, settlementKey(t, "v1", "charge-prepaid", "CNY"), nil)
	if result.Outcome() != pcdomain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED——发出去的结算政策没被解析选中", result.Outcome())
	}
	policy, present := result.AdoptedSettlementPolicy()
	if !present {
		t.Fatal("采用了版本却带不出方式与六维范围")
	}
	if policy.Method() != pcdomain.PrepaidMethod {
		t.Fatalf("method = %q, want PREPAID", policy.Method())
	}
	if policy.Applicability().Contract().String() != "contract-1/v1" {
		t.Fatalf("合同维 = %q, want contract-1/v1", policy.Applicability().Contract())
	}

	misses := map[string]pcdomain.ResolutionKey{
		"换币种":   settlementKey(t, "v1", "charge-prepaid", "USD"),
		"换费用范围": settlementKey(t, "v1", "charge-cod", "CNY"),
		"换合同版本": settlementKey(t, "v2", "charge-prepaid", "CNY"),
	}
	for name, key := range misses {
		t.Run(name, func(t *testing.T) {
			outcome := pcdomain.ResolveCommercialBasis(registry, key, nil).Outcome()
			if outcome != pcdomain.NoApplicableBasis {
				t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS——差一维仍被采用了", outcome)
			}
		})
	}
}
