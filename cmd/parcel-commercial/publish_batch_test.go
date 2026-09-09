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
// 结算政策项的 contentDigest 是 CanonicalizePublicationContent 对这份正文算出的：本册接进 PCC-1 后对账门对带正文的项
// 开门，随手写的占位串答`未受理`；改正文任一格都要重算这个串。
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
      "contentDigest": "PCC-1:cab5c83b74127b94abb6f8dcfa27611995ffbe4cf9dfa1ee263f9f9faad0bebe",
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

// customerServiceRuleBatchBody 先发一份服务产品壳，再发一份指名它的客户服务规则版本，正文挂在同一
// 产品上。两项同批：后项装载的整册看得见前项，指名引用因此在一批内前后相依（AT-PC-011）。
func customerServiceRuleBatchBody(appliesTo string) string {
	return `{"items": [
    {
      "tenantId": "tenant-1",
      "kind": "SERVICE_PRODUCT",
      "objectId": "product-1",
      "version": "v1",
      "scope": "scope-1",
      "contentDigest": "sha256:product-1",
      "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "approval": {"reference": "approval-product-1", "source": "source-product-1", "approvedAt": "2026-01-02T00:00:00Z"},
      "approvalRoleStanding": "CONFIRMED"
    },
    {
      "tenantId": "tenant-1",
      "kind": "CUSTOMER_SERVICE_RULE",
      "objectId": "csr-1",
      "version": "v1",
      "scope": "scope-1",
      "contentDigest": "sha256:csr-1",
      "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "references": {"SERVICE_PRODUCT": "product-1"},
      "approval": {"reference": "approval-csr-1", "source": "source-csr-1", "approvedAt": "2026-01-02T00:00:00Z"},
      "approvalRoleStanding": "CONFIRMED",
      "declarations": {
        "customerServiceRuleBody": {
          "serviceProduct": "` + appliesTo + `",
          "responsible": "operator-1",
          "scope": "scope-1",
          "claimDeadlines": [
            {"kind": "FIRST_CLAIM", "startEvent": "event-delivered", "days": 30, "calendar": "calendar-cn"}
          ],
          "minimumMaterials": [
            {"claimKind": "claim-loss", "materials": ["material-photo", "material-invoice"]}
          ]
        }
      }
    }
  ]}`
}

// Covers: 票 party-commercial-context-gaps/05 端到端于真库——经进程口发出去的客户服务规则正文，
// 消费方按闭包选中的版本壳经 CustomerServiceRuleContentView 点读得回来，两项俱在。
//
// 它补的是翻译用例与应用用例都拿不到的东西：那两条各自证「批文没变形」与「正文交到了持久化面」，
// 证不了「写进库的三张表读回来还是那一版规则」。第二段证 ADR-0104 Decision 四写入半边在进程口
// 也拦得住：壳指名 product-1 而正文说挂在别的产品上，那一项以技术失败停批、正文一行不落。
func TestAPublishedCustomerServiceRuleIsReadBackByTheContentView(t *testing.T) {
	dsn := freshMigratedDSN(t)
	if code := runCLI(t, dsn, "publish", "-input", batchFile(t, customerServiceRuleBatchBody("product-1"))); code != exitLanded {
		t.Fatalf("客户服务规则批 exit = %d, want %d", code, exitLanded)
	}

	registry := loadScope(t, dsn, "tenant-1", "scope-1")
	tenant, err := pcdomain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	objectID, err := pcdomain.NewCommercialObjectID("csr-1")
	if err != nil {
		t.Fatalf("对象：%v", err)
	}
	label, err := pcdomain.NewCommercialVersionLabel("v1")
	if err != nil {
		t.Fatalf("版本号：%v", err)
	}
	version, present := registry.Lookup(tenant, pcdomain.CustomerServiceRuleObject, objectID, label)
	if !present {
		t.Fatal("发出去的客户服务规则版本壳不在整册里")
	}

	rule, found, err := loadCustomerServiceRule(t, dsn, tenant, version)
	if err != nil || !found {
		t.Fatalf("点读：found=%v err=%v", found, err)
	}
	if deadline, ok := rule.ClaimDeadline(pcdomain.FirstClaimDeadline); !ok || deadline.DurationDays() != 30 {
		t.Fatalf("首次索赔期限 = (%#v, %v)", deadline, ok)
	}
	claimKind, err := pcdomain.NewClaimKindReference("claim-loss")
	if err != nil {
		t.Fatalf("索赔类型：%v", err)
	}
	if materials, ok := rule.MinimumMaterialsFor(claimKind); !ok || len(materials.Materials()) != 2 {
		t.Fatalf("最低材料 = (%#v, %v)", materials, ok)
	}

	t.Run("an applicability disagreeing with the shell stops the batch before any row lands", func(t *testing.T) {
		other := freshMigratedDSN(t)
		if code := runCLI(t, other, "publish", "-input", batchFile(t, customerServiceRuleBatchBody("product-OTHER"))); code != exitTechnical {
			t.Fatalf("分歧批 exit = %d, want %d", code, exitTechnical)
		}
		if got := countVersions(t, other, "csr-1"); got != 0 {
			t.Fatalf("分歧的规则版本壳落了 %d 行, want 0", got)
		}
		if got := countVersions(t, other, "product-1"); got != 1 {
			t.Fatalf("前一项服务产品落了 %d 行, want 1——批不是聚合，前项不因后项失败被撤出", got)
		}
	})
}

// controlPolicyBatchBody 发一份接受前财务控制策略版本，正文是两项组合控制。策略不指名任何对象——合同 → 策略
// 那层关系由客户合同正文的绑定拥有（ADR-0115 Decision 四），策略自己不必先等谁发布。
// 壳上的摘要是 CanonicalizePublicationContent 对这份正文算出的那一个：本册已接进服务端规范化（票 admin-write-faces/13），
// 对账门会拿声明的串与算出的比，随手写的串会被拒；改正文任一格都要重算这一串。
func controlPolicyBatchBody() string {
	return `{"items": [
    {
      "tenantId": "tenant-1",
      "kind": "PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY",
      "objectId": "fcp-1",
      "version": "v1",
      "scope": "scope-1",
      "contentDigest": "PCC-1:07cd1a96103b19f524b8472a906f16afba5f96d1c7fecfdcf09c9bfa9e9a809c",
      "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "approval": {"reference": "approval-fcp-1", "source": "source-fcp-1", "approvedAt": "2026-01-02T00:00:00Z"},
      "approvalRoleStanding": "CONFIRMED",
      "declarations": {
        "preAcceptanceFinancialControlPolicyBody": {
          "jointPassCondition": "ALL_CONTROLS_PASS",
          "controls": [
            {"control": "CREDIT_CHECK", "chargeScope": "charge-scope-a", "order": 2, "onFailure": "AUTHORIZED_DISPOSITION", "responsibility": "operator-legal-1"},
            {"control": "PREPAID_FREEZE", "chargeScope": "charge-scope-a", "order": 1, "onFailure": "REJECT", "responsibility": "customer-1"}
          ]
        }
      }
    }
  ]}`
}

// Covers: 票 party-commercial-context-gaps/07 端到端于真库——经进程口发出去的接受前财务控制策略正文，消费方
// 按闭包选中的版本壳经 PreAcceptanceFinancialControlPolicyContentView 点读得回来，两项按判断顺序俱在。
//
// 它补的是翻译用例与应用用例都拿不到的东西：那两条各自证「批文没变形」与「正文交到了持久化面」，证不了
// 「写进库的两张表读回来还是那一版策略」。
// contractDelegationBatchBody 发一份客户合同版本，随行两条合同委派：客户账户委派资料修订给商务等级、
// 责任法人委派给文员等级。合同不指名规则包也能发——委派是这份合同说的话，不依赖合同其它正文。
func contractDelegationBatchBody() string {
	return `{"items": [
    {
      "tenantId": "tenant-1",
      "kind": "CUSTOMER_CONTRACT",
      "objectId": "contract-1",
      "version": "v1",
      "scope": "scope-1",
      "contentDigest": "sha256:contract-1",
      "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "approval": {"reference": "approval-contract-1", "source": "source-1", "approvedAt": "2025-12-15T00:00:00Z"},
      "approvalRoleStanding": "CONFIRMED",
      "declarations": {
        "contractDelegations": [
          {"delegatorKind": "CUSTOMER_ACCOUNT", "delegator": "account-1", "action": "SOURCE_DATA_AMENDMENT",
           "scope": "scope-1", "level": "level-commercial", "effectiveStartsAt": "2026-01-01T00:00:00Z"},
          {"delegatorKind": "LEGAL_ENTITY", "delegator": "legal-1", "action": "SOURCE_DATA_AMENDMENT",
           "scope": "scope-1", "level": "level-clerk", "effectiveStartsAt": "2026-01-01T00:00:00Z", "effectiveEndsAt": "2026-03-01T00:00:00Z"}
        ]
      }
    }
  ]}`
}

// Covers: 票 party-commercial-context-gaps/08 端到端于真库——经进程口随合同版本发出去的合同委派，按合同版本
// 点读得回两条，按（范围 + 时点）装载只得当时有效的那些（ADR-0116 Decision 二 / 三）。
//
// 它补的是翻译用例与应用用例都拿不到的东西：那两条各自证「批文没变形」与「声明交到了持久化面」，证不了
// 「写进库的两张表读回来还是那一版合同的委派」。
func TestPublishedContractDelegationsAreReadBackByBothViews(t *testing.T) {
	dsn := freshMigratedDSN(t)
	if code := runCLI(t, dsn, "publish", "-input", batchFile(t, contractDelegationBatchBody())); code != exitLanded {
		t.Fatalf("合同批 exit = %d, want %d", code, exitLanded)
	}

	registry := loadScope(t, dsn, "tenant-1", "scope-1")
	tenant, err := pcdomain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	objectID, err := pcdomain.NewCommercialObjectID("contract-1")
	if err != nil {
		t.Fatalf("对象：%v", err)
	}
	label, err := pcdomain.NewCommercialVersionLabel("v1")
	if err != nil {
		t.Fatalf("版本号：%v", err)
	}
	contract, present := registry.Lookup(tenant, pcdomain.CustomerContractObject, objectID, label)
	if !present {
		t.Fatal("发出去的合同版本壳不在整册里")
	}

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("开池点读：%v", err)
	}
	defer pool.Close()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("框架 DB：%v", err)
	}
	delegations, err := pcpostgres.NewContractDelegations(db)
	if err != nil {
		t.Fatalf("构造合同委派读口：%v", err)
	}

	content, found, err := delegations.LoadContractDelegations(t.Context(), tenant, contract)
	if err != nil || !found {
		t.Fatalf("点读：found=%v err=%v", found, err)
	}
	rows := content.Delegations()
	if len(rows) != 2 || rows[0].Level().String() != "level-clerk" || rows[0].Delegator().Kind() != pcdomain.LegalEntityDelegator ||
		rows[1].Level().String() != "level-commercial" || rows[1].Delegator().Reference() != "account-1" {
		t.Fatalf("委派 = %#v", rows)
	}

	scope, err := pcdomain.NewCommercialScopeReference("scope-1")
	if err != nil {
		t.Fatalf("范围：%v", err)
	}
	effective, err := delegations.LoadEffectiveDelegations(t.Context(), tenant, scope, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("按范围时点装载：%v", err)
	}
	if len(effective) != 1 || effective[0].Level().String() != "level-commercial" || !effective[0].Contract().SameVersionAs(contract) {
		t.Fatalf("2026-06 仍有效的委派 = %#v, want 只剩客户账户那条（法人那条 03-01 到期）", effective)
	}
}

// sourceDataAmendmentBatchBody 两份接单规则包：rules-open 未封闭带两格，rules-closed 封闭零格。
func sourceDataAmendmentBatchBody() string {
	return `{"items": [
    {
      "tenantId": "tenant-1", "kind": "ACCEPTANCE_RULE_PACKAGE", "objectId": "rules-open", "version": "v1",
      "scope": "scope-1", "contentDigest": "sha256:rules-open", "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "approval": {"reference": "approval-open", "source": "source-open", "approvedAt": "2026-01-02T00:00:00Z"},
      "approvalRoleStanding": "CONFIRMED",
      "declarations": {"sourceDataAmendment": {"closed": false, "rules": [
        {"dataGroup": "consignee.address", "stage": "ACCEPTED_NOT_YET_RECEIVED", "intent": "CORRECTION", "allowance": "ALLOWED"},
        {"dataGroup": "consignee.address", "stage": "CUSTOMS_SUBMITTED", "intent": "CORRECTION", "allowance": "DISALLOWED"}
      ]}}
    },
    {
      "tenantId": "tenant-1", "kind": "ACCEPTANCE_RULE_PACKAGE", "objectId": "rules-closed", "version": "v1",
      "scope": "scope-1", "contentDigest": "sha256:rules-closed", "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "approval": {"reference": "approval-closed", "source": "source-closed", "approvedAt": "2026-01-02T00:00:00Z"},
      "approvalRoleStanding": "CONFIRMED",
      "declarations": {"sourceDataAmendment": {"closed": true}}
    }
  ]}`
}

// Covers: 票 pc-gaps/10 真库端到端——受控批文 `sourceDataAmendment` 一节经进程口发布，写进 0027 两表，再经 PS 消费
// 适配器将来读的那个口（StageContentDeclarations.LoadSourceDataAmendmentAllowance）读回：未封闭的逐格与缺格读法、
// 封闭零格的「一律不允许」都从生产读口算出来（ADR-0120 Decision 三、四、六）。
func TestAPublishedSourceDataAmendmentAllowanceIsReadBackByTheContentView(t *testing.T) {
	dsn := freshMigratedDSN(t)
	if code := runCLI(t, dsn, "publish", "-input", batchFile(t, sourceDataAmendmentBatchBody())); code != exitLanded {
		t.Fatalf("规则包批 exit = %d, want %d", code, exitLanded)
	}

	registry := loadScope(t, dsn, "tenant-1", "scope-1")
	tenant, err := pcdomain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	lookup := func(objectID string) pcdomain.CommercialVersion {
		t.Helper()
		id, err := pcdomain.NewCommercialObjectID(objectID)
		if err != nil {
			t.Fatalf("对象：%v", err)
		}
		label, err := pcdomain.NewCommercialVersionLabel("v1")
		if err != nil {
			t.Fatalf("版本号：%v", err)
		}
		version, present := registry.Lookup(tenant, pcdomain.AcceptanceRulePackageObject, id, label)
		if !present {
			t.Fatalf("发出去的规则包 %s 不在整册里", objectID)
		}
		return version
	}

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("开池点读：%v", err)
	}
	defer pool.Close()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("框架 DB：%v", err)
	}
	reader, err := pcpostgres.NewStageContentDeclarations(db)
	if err != nil {
		t.Fatalf("构造阶段内容读口：%v", err)
	}
	address, err := pcdomain.NewSourceDataGroupReference("consignee.address")
	if err != nil {
		t.Fatalf("资料组：%v", err)
	}

	open, found, err := reader.LoadSourceDataAmendmentAllowance(t.Context(), tenant, lookup("rules-open"))
	if err != nil || !found {
		t.Fatalf("点读 rules-open：found=%v err=%v", found, err)
	}
	if open.Closed() || len(open.Rules()) != 2 ||
		open.AllowanceFor(address, pcdomain.DeclaredAcceptedNotYetReceived, pcdomain.DeclaredCorrectionIntent) != pcdomain.AmendmentAllowed ||
		open.AllowanceFor(address, pcdomain.DeclaredCustomsSubmitted, pcdomain.DeclaredCorrectionIntent) != pcdomain.AmendmentDisallowed ||
		open.AllowanceFor(address, pcdomain.DeclaredReceivedOrMeasured, pcdomain.DeclaredCorrectionIntent) != pcdomain.AmendmentAllowanceNotDeclared {
		t.Fatalf("rules-open 读回变形：closed=%v rules=%#v", open.Closed(), open.Rules())
	}

	closed, found, err := reader.LoadSourceDataAmendmentAllowance(t.Context(), tenant, lookup("rules-closed"))
	if err != nil || !found {
		t.Fatalf("点读 rules-closed：found=%v err=%v", found, err)
	}
	if !closed.Closed() || len(closed.Rules()) != 0 ||
		closed.AllowanceFor(address, pcdomain.DeclaredAcceptedNotYetReceived, pcdomain.DeclaredSupplementIntent) != pcdomain.AmendmentDisallowed {
		t.Fatalf("rules-closed 读回变形：closed=%v rules=%d", closed.Closed(), len(closed.Rules()))
	}
}

func TestAPublishedPreAcceptanceFinancialControlPolicyIsReadBackByTheContentView(t *testing.T) {
	dsn := freshMigratedDSN(t)
	if code := runCLI(t, dsn, "publish", "-input", batchFile(t, controlPolicyBatchBody())); code != exitLanded {
		t.Fatalf("策略批 exit = %d, want %d", code, exitLanded)
	}

	registry := loadScope(t, dsn, "tenant-1", "scope-1")
	tenant, err := pcdomain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	objectID, err := pcdomain.NewCommercialObjectID("fcp-1")
	if err != nil {
		t.Fatalf("对象：%v", err)
	}
	label, err := pcdomain.NewCommercialVersionLabel("v1")
	if err != nil {
		t.Fatalf("版本号：%v", err)
	}
	version, present := registry.Lookup(tenant, pcdomain.PreAcceptanceFinancialControlPolicyObject, objectID, label)
	if !present {
		t.Fatal("发出去的策略版本壳不在整册里")
	}

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("开池点读：%v", err)
	}
	defer pool.Close()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("框架 DB：%v", err)
	}
	contents, err := pcpostgres.NewPreAcceptanceFinancialControlPolicyContents(db)
	if err != nil {
		t.Fatalf("构造策略正文读口：%v", err)
	}
	policy, found, err := contents.LoadPreAcceptanceFinancialControlPolicy(t.Context(), tenant, version)
	if err != nil || !found {
		t.Fatalf("点读：found=%v err=%v", found, err)
	}
	if policy.JointPassCondition() != pcdomain.AllControlsPass {
		t.Fatalf("共同通过条件 = %v", policy.JointPassCondition())
	}
	items := policy.Items()
	if len(items) != 2 || items[0].Kind() != pcdomain.PrepaidFreezeControl || items[0].EvaluationOrder() != 1 ||
		items[1].Kind() != pcdomain.CreditCheckControl || items[1].EvaluationOrder() != 2 ||
		items[1].FailureDisposition() != pcdomain.AuthorizedDispositionOnControlFailure ||
		items[1].Responsibility().String() != "operator-legal-1" {
		t.Fatalf("控制项 = %#v", items)
	}
}

func loadCustomerServiceRule(
	t *testing.T,
	dsn string,
	tenant pcdomain.TenantID,
	version pcdomain.CommercialVersion,
) (pcdomain.CustomerServiceRuleVersion, bool, error) {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("开池点读：%v", err)
	}
	defer pool.Close()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("框架 DB：%v", err)
	}
	contents, err := pcpostgres.NewCustomerServiceRuleContents(db)
	if err != nil {
		t.Fatalf("构造客户服务规则读口：%v", err)
	}
	return contents.LoadCustomerServiceRule(t.Context(), tenant, version)
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
