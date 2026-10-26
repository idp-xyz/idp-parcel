package main

import (
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 本文件证受控批量口的关系结束子命令（票 ps-external-mark-relations/17）：三节各译成对应命令；真库上撤销、到期、替代逐项
// 落下终止修订，重放整批答原结果；撤销之后授权交付判断即时答否。

// endingSetupBatchJSON 立起要结束的那几段关系：一段授权交付关系（撤销它）、一段已过终点的客户关系（登记到期）、一段
// 客户关系（由另一参与方接替）。时刻都早于今天——结束口拒晚于登记时刻的结束。
const endingSetupBatchJSON = `{
  "tenantId": "tenant-1",
  "businessParties": [
    {"partyId": "party-deliverer", "name": "合成交付方", "revision": 1, "basis": "REG/party-deliverer", "effectiveFrom": "2026-01-01T00:00:00Z"},
    {"partyId": "party-shipper", "name": "合成货主", "revision": 1, "basis": "REG/party-shipper", "effectiveFrom": "2026-01-01T00:00:00Z"},
    {"partyId": "party-agent", "name": "合成代理", "revision": 1, "basis": "REG/party-agent", "effectiveFrom": "2026-01-01T00:00:00Z"},
    {"partyId": "party-agent-next", "name": "合成接替代理", "revision": 1, "basis": "REG/party-agent-next", "effectiveFrom": "2026-01-01T00:00:00Z"}
  ],
  "customerAccounts": [
    {"accountId": "account-shipper", "customerPartyId": "party-shipper", "revision": 1, "basis": "REG/account-shipper", "effectiveFrom": "2026-01-02T00:00:00Z"}
  ],
  "relationships": [
    {"relationshipId": "rel-delivery", "revision": 1, "holder": "party-deliverer", "counterparty": "party-shipper",
     "role": "AUTHORIZED_DELIVERING_PARTY", "scope": "customerAccount=account-shipper", "basis": "MANDATE/rel-delivery",
     "effectiveStartsAt": "2026-01-03T00:00:00Z", "approval": {"reference": "approval-delivery", "approvedAt": "2026-01-02T12:00:00Z"}},
    {"relationshipId": "rel-term", "revision": 1, "holder": "party-agent", "counterparty": "party-shipper",
     "role": "CARRIER_AGENT", "scope": "scope-1", "basis": "CONTRACT/rel-term",
     "effectiveStartsAt": "2026-01-03T00:00:00Z", "effectiveEndsAt": "2026-04-01T00:00:00Z",
     "approval": {"reference": "approval-term", "approvedAt": "2026-01-02T12:00:00Z"}},
    {"relationshipId": "rel-agency", "revision": 1, "holder": "party-agent", "counterparty": "party-shipper",
     "role": "CARRIER_AGENT", "scope": "scope-1", "basis": "CONTRACT/rel-agency",
     "effectiveStartsAt": "2026-01-03T00:00:00Z", "approval": {"reference": "approval-agency", "approvedAt": "2026-01-02T12:00:00Z"}}
  ]
}`

const endingBatchJSON = `{
  "tenantId": "tenant-1",
  "revocations": [
    {"relationshipId": "rel-delivery", "revision": 2, "basis": "REVOCATION/rel-delivery", "at": "2026-05-01T00:00:00Z"}
  ],
  "expirations": [
    {"relationshipId": "rel-term", "revision": 2, "basis": "TERM-ENDED/rel-term", "at": "2026-04-01T00:00:00Z"}
  ],
  "supersessions": [
    {"relationshipId": "rel-agency", "revision": 2, "successor": "party-agent-next", "at": "2026-05-01T00:00:00Z",
     "successorRelationshipId": "rel-agency-next", "basis": "HANDOVER/rel-agency",
     "approval": {"reference": "approval-handover", "approvedAt": "2026-04-30T00:00:00Z"}}
  ]
}`

// Covers: 票 17 裁定「受控批量口同形加三种」的翻译半边——三节各译成一项、回显标签点名动作与落点修订；未知键、空批、替代
// 缺批准都在连库之前拒。
func TestRelationshipEndingBatchTranslatesTheThreeLines(t *testing.T) {
	commands, err := relationshipEndingBatchFromJSON([]byte(endingBatchJSON))
	if err != nil {
		t.Fatalf("翻译关系结束批：%v", err)
	}
	labels := make([]string, 0, len(commands))
	for _, command := range commands {
		labels = append(labels, command.label)
	}
	want := []string{
		"撤销参与方关系 rel-delivery r2",
		"到期参与方关系 rel-term r2",
		"替代参与方关系 rel-agency r2，由 party-agent-next 接替（rel-agency-next）",
	}
	if strings.Join(labels, "|") != strings.Join(want, "|") {
		t.Fatalf("标签 = %q, want %q", labels, want)
	}
	for name, body := range map[string]string{
		"未知键": strings.Replace(endingBatchJSON, `"basis": "REVOCATION/rel-delivery"`, `"reason": "x", "basis": "REVOCATION/rel-delivery"`, 1),
		"空批":  `{"tenantId": "tenant-1"}`,
		"替代缺批准": strings.Replace(endingBatchJSON, `,
     "approval": {"reference": "approval-handover", "approvedAt": "2026-04-30T00:00:00Z"}`, "", 1),
	} {
		if _, err := relationshipEndingBatchFromJSON([]byte(body)); err == nil {
			t.Fatalf("%s：翻译没拒", name)
		}
	}
}

// Covers: 票 17 完成判据「……CLI 各层用例；撤销后授权交付判断即时答否（真库）」的进程口半边——经 register-parties 立起关系、
// 授权交付判断答是；经 end-party-relationships 一批撤销、到期、替代各落一笔终止修订，退 0；同一刻再判即答否，接替关系落册；
// 重放整批退 0；同修订异依据的撤销答冲突、退 2。
func TestEndPartyRelationshipsBatchLandsOnARealDatabase(t *testing.T) {
	dsn := freshMigratedDSN(t)
	if code := runCLI(t, dsn, "register-parties", "-input", batchFile(t, endingSetupBatchJSON)); code != exitLanded {
		t.Fatalf("立起关系 exit = %d, want %d", code, exitLanded)
	}

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("连接测试库：%v", err)
	}
	t.Cleanup(pool.Close)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	judgments, err := pcpostgres.NewAuthorizedDeliveryJudgments(db)
	if err != nil {
		t.Fatalf("构造授权交付判断读口：%v", err)
	}
	registry, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatalf("构造身份登记册：%v", err)
	}
	tenant := mustPC(t, pcdomain.NewTenantID, "tenant-1")
	asked := time.Date(2026, 5, 2, 8, 0, 0, 0, time.UTC)
	judge := func() pcdomain.AuthorizedDeliveryAnswer {
		t.Helper()
		answer, err := judgments.JudgeAuthorizedDelivery(t.Context(), pcdomain.AuthorizedDeliveryQuery{
			Tenant:          tenant,
			DeliveringParty: mustPC(t, pcdomain.NewPartyID, "party-deliverer"),
			CustomerAccount: mustPC(t, pcdomain.NewCustomerAccountID, "account-shipper"),
			Node:            mustPC(t, pcdomain.NewLogisticsNodeReference, "SYN-NODE-SHA-HUB"),
			At:              asked,
		})
		if err != nil {
			t.Fatalf("授权交付判断：%v", err)
		}
		return answer
	}
	if got := judge(); got != pcdomain.DeliveryAuthorized {
		t.Fatalf("撤销之前 = %s, want AUTHORIZED", got)
	}

	batch := batchFile(t, endingBatchJSON)
	if code := runCLI(t, dsn, "end-party-relationships", "-input", batch); code != exitLanded {
		t.Fatalf("结束批 exit = %d, want %d", code, exitLanded)
	}
	if got := judge(); got != pcdomain.DeliveryNotAuthorized {
		t.Fatalf("撤销之后 = %s, want NOT_AUTHORIZED", got)
	}
	for id, want := range map[string]pcdomain.RelationshipStatus{
		"rel-delivery":    pcdomain.RelationshipRevoked,
		"rel-term":        pcdomain.RelationshipExpired,
		"rel-agency":      pcdomain.RelationshipSuperseded,
		"rel-agency-next": pcdomain.RelationshipEffective,
	} {
		latest, found, err := registry.LoadLatestRelationship(t.Context(), tenant, mustPC(t, pcdomain.NewRelationshipID, id))
		if err != nil || !found || latest.Relationship().Status() != want {
			t.Fatalf("%s 读回 = (%v, %v, %v), want %s", id, latest.Relationship().Status(), found, err, want)
		}
	}

	if code := runCLI(t, dsn, "end-party-relationships", "-input", batch); code != exitLanded {
		t.Fatalf("重放批 exit = %d, want %d", code, exitLanded)
	}
	conflicted := batchFile(t, `{"tenantId": "tenant-1", "revocations": [
	  {"relationshipId": "rel-delivery", "revision": 2, "basis": "REVOCATION/other", "at": "2026-05-01T00:00:00Z"}]}`)
	if code := runCLI(t, dsn, "end-party-relationships", "-input", conflicted); code != exitAttention {
		t.Fatalf("冲突批 exit = %d, want %d", code, exitAttention)
	}
}

func mustPC[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}
