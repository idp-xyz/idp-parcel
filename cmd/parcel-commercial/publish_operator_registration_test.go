package main

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 本文件对真实 PostgreSQL 16 证进程口那一半的 ADR-0094 决定四：受控批量口发布带时点策略声明的
// 规则包，「参数已登记」信封随那一项同事务进 Outbox（票 first-tenant-runway/07 D4）。
//
// application 层用例拿替身证「编排交了意图」，adapters/postgres 用例证「意图进得了 Outbox」；
// 这里补的是两者之间那一格——进程口真把 Outbox 交接接到了发布处理器上。装配漏接在编译期就红
// （构造器收交接口为必需依赖），本条守的是接上的是**会写库的那一只**而不是别的什么。
//
// 事件类型与 ID 字面在这里再写一遍：它们是跨上下文契约，共用常量只能证明它等于自己。
const operatorRegistrationCompletedEventType = "party-commercial.commercial-authority.operator-registration-completed"

// rulePackageItemWithAsOf 在最小发布项上加一条可达性判断的时点策略声明。
func rulePackageItemWithAsOf(objectID, digest string) string {
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
      "approvalRoleStanding": "CONFIRMED",
      "declarations": {
        "asOfPolicies": [
          {"judgment": "NETWORK_REACHABILITY", "semantics": "AT_ACCEPTANCE", "policyVersion": "asof-policy/v1"}
        ]
      }
    }`
}

func countOperatorRegistrationEnvelopes(t *testing.T, dsn, eventID string) int {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("开池核对：%v", err)
	}
	defer pool.Close()

	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, operatorRegistrationCompletedEventType,
	).Scan(&count); err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

// Covers: ADR-0094 决定四 —— 受控批量口登记时点策略，同一项落库即有一封「参数已登记」在 Outbox；
// 重跑同一批答重复，信封仍只有一封（ADR-0043 重放重发同一份，由认领 ID 去重）。
func TestPublishingAsOfPoliciesThroughTheCLIEnqueuesTheOperatorRegistrationEnvelope(t *testing.T) {
	dsn := freshMigratedDSN(t)
	batch := batchFile(t, `{"items":[`+rulePackageItemWithAsOf("rules-asof", "digest-asof")+`]}`)
	eventID := "tenant-1/AS_OF_POLICY/rules-asof/v1/operator-registration-completed"

	if code := runCLI(t, dsn, "publish", "-input", batch); code != exitLanded {
		t.Fatalf("首次发布退出码 = %d, want %d", code, exitLanded)
	}
	if count := countOperatorRegistrationEnvelopes(t, dsn, eventID); count != 1 {
		t.Fatalf("首次发布后 outbox 行数 = %d，want 1", count)
	}

	if code := runCLI(t, dsn, "publish", "-input", batch); code != exitLanded {
		t.Fatalf("重放退出码 = %d, want %d", code, exitLanded)
	}
	if count := countOperatorRegistrationEnvelopes(t, dsn, eventID); count != 1 {
		t.Fatalf("重放后 outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

// Covers: 没有时点声明的发布不发信封——受控口与在线口共用的判据在编排里，这里证进程口没有另加
// 一条「凡发布皆发」的旁路。
func TestPublishingWithoutAsOfPoliciesThroughTheCLIEnqueuesNothing(t *testing.T) {
	dsn := freshMigratedDSN(t)
	batch := batchFile(t, `{"items":[`+rulePackageItem("rules-plain", "digest-plain")+`]}`)

	if code := runCLI(t, dsn, "publish", "-input", batch); code != exitLanded {
		t.Fatalf("发布退出码 = %d, want %d", code, exitLanded)
	}
	if count := countOperatorRegistrationEnvelopes(t, dsn,
		"tenant-1/AS_OF_POLICY/rules-plain/v1/operator-registration-completed"); count != 0 {
		t.Fatalf("没有时点声明却入了 %d 封", count)
	}
}
