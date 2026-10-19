package main

import (
	"path/filepath"
	"strings"
	"testing"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证受控批量口认授权交付方这一种角色（ADR-0212 决定一；PC CONTEXT「参与方关系」）：角色名过翻译，登记走既有
// 参与方关系口的代数；演示种子那份授权交付关系批在真库上落得下、重放答原结果。

const authorizedDeliveryBatchJSON = `{
  "tenantId": "tenant-1",
  "businessParties": [
    {"partyId": "party-deliverer", "name": "合成交付方", "revision": 1, "basis": "REG/party-deliverer", "effectiveFrom": "2026-01-01T00:00:00Z"},
    {"partyId": "party-shipper", "name": "合成货主", "revision": 1, "basis": "REG/party-shipper", "effectiveFrom": "2026-01-01T00:00:00Z"}
  ],
  "customerAccounts": [
    {"accountId": "account-shipper", "customerPartyId": "party-shipper", "revision": 1, "basis": "REG/account-shipper", "effectiveFrom": "2026-01-02T00:00:00Z"}
  ],
  "relationships": [
    {
      "relationshipId": "rel-delivery-1", "revision": 1,
      "holder": "party-deliverer", "counterparty": "party-shipper",
      "role": "AUTHORIZED_DELIVERING_PARTY", "scope": "customerAccount=account-shipper&node=SYN-NODE-SHA-HUB",
      "basis": "MANDATE/rel-delivery-1",
      "effectiveStartsAt": "2026-01-03T00:00:00Z",
      "approval": {"reference": "approval-delivery-1", "approvedAt": "2026-01-02T12:00:00Z"}
    }
  ]
}`

// Covers: 角色名 AUTHORIZED_DELIVERING_PARTY 过翻译成领域里的授权交付方角色，不当作集合外取值拒收。
func TestPartyBatchTranslationAcceptsTheAuthorizedDeliveringPartyRole(t *testing.T) {
	commands, err := partyBatchFromJSON([]byte(authorizedDeliveryBatchJSON))
	if err != nil {
		t.Fatalf("翻译授权交付关系批：%v", err)
	}
	if got := commands[len(commands)-1].label; got != "参与方关系 rel-delivery-1 r1" {
		t.Fatalf("末项标签 = %q", got)
	}
	role, err := partyRoleFromName("AUTHORIZED_DELIVERING_PARTY")
	if err != nil || role != pcdomain.AuthorizedDeliveringPartyRole {
		t.Fatalf("角色名镜像 = (%v, %v), want AuthorizedDeliveringPartyRole", role, err)
	}
}

// Covers: 完成判据「登记写面与受控批量口能登记新角色，重放与冲突照既有参与方关系口的代数」的进程口半边，端到端于真库：
// 首批落册、重放整批以原结果回答、同修订异正文报冲突请人看；范围里的客户账户不属相对方时未受理。
func TestRegisterPartiesBatchLandsAnAuthorizedDeliveryRelationship(t *testing.T) {
	dsn := freshMigratedDSN(t)
	batch := batchFile(t, authorizedDeliveryBatchJSON)
	if code := runCLI(t, dsn, "register-parties", "-input", batch); code != exitLanded {
		t.Fatalf("首批 exit = %d, want %d", code, exitLanded)
	}
	if code := runCLI(t, dsn, "register-parties", "-input", batch); code != exitLanded {
		t.Fatalf("重放批 exit = %d, want %d", code, exitLanded)
	}
	conflicted := batchFile(t, strings.Replace(authorizedDeliveryBatchJSON, "MANDATE/rel-delivery-1", "MANDATE/rel-delivery-1-amended", 1))
	if code := runCLI(t, dsn, "register-parties", "-input", conflicted); code != exitAttention {
		t.Fatalf("冲突批 exit = %d, want %d", code, exitAttention)
	}
	foreign := batchFile(t, `{
	  "tenantId": "tenant-1",
	  "businessParties": [
	    {"partyId": "party-other", "name": "合成另一货主", "revision": 1, "basis": "REG/party-other", "effectiveFrom": "2026-01-01T00:00:00Z"}
	  ],
	  "relationships": [
	    {
	      "relationshipId": "rel-delivery-2", "revision": 1,
	      "holder": "party-deliverer", "counterparty": "party-other",
	      "role": "AUTHORIZED_DELIVERING_PARTY", "scope": "customerAccount=account-shipper",
	      "basis": "MANDATE/rel-delivery-2", "effectiveStartsAt": "2026-01-03T00:00:00Z"
	    }
	  ]
	}`)
	if code := runCLI(t, dsn, "register-parties", "-input", foreign); code != exitAttention {
		t.Fatalf("相对方不是账户所属参与方 exit = %d, want %d", code, exitAttention)
	}
}

// Covers: 完成判据「演示」——演示种子按 seed.sh 的次序（注册号类型目录 → 参与方身份四册 → 授权交付关系）在真库上
// 落得下，重放答原结果。演示租户的合成交付方替 SYN-ACCOUNT-01 送站，证据记 S。
func TestDemoSeedAuthorizedDeliveryBatchLands(t *testing.T) {
	dsn := freshMigratedDSN(t)
	seeds := filepath.Join(moduleRoot(t), "scripts", "demo-seeds", "data", "commercial")
	for _, step := range []struct{ command, file string }{
		{command: "register-registration-number-types", file: "register-registration-number-types.json"},
		{command: "register-parties", file: "register-parties.json"},
		{command: "register-parties", file: "register-authorized-delivery.json"},
		{command: "register-parties", file: "register-authorized-delivery.json"},
	} {
		if code := runCLI(t, dsn, step.command, "-input", filepath.Join(seeds, step.file)); code != exitLanded {
			t.Fatalf("%s %s exit = %d, want %d", step.command, step.file, code, exitLanded)
		}
	}
}
