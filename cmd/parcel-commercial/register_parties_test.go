package main

import (
	"strings"
	"testing"
)

// 本文件证参与方登记进程口的两半（票 admin-remainder-mechanism-batch/01）：
// 翻译纪律——批文逐字段过领域构造门，未知字段与集合外取值在触库前拒收；
// 批推进——对真库端到端跑 register-parties / deactivate-party-identity，
// 证批内前项已提交的行对后项的引用检查真实可见（应用层替身测试证不了提交边界）。

const partyFullBatchJSON = `{
  "tenantId": "tenant-1",
  "businessParties": [
    {"partyId": "party-acme", "name": "Acme 物流", "revision": 1, "basis": "REG/party-acme", "effectiveFrom": "2026-01-01T00:00:00Z"},
    {"partyId": "party-beta", "name": "Beta 贸易", "revision": 1, "basis": "REG/party-beta", "effectiveFrom": "2026-01-01T00:00:00Z"}
  ],
  "legalEntities": [
    {"legalEntityId": "legal-acme", "partyId": "party-acme", "revision": 1, "basis": "REG/legal-acme", "effectiveFrom": "2026-01-02T00:00:00Z"}
  ],
  "customerAccounts": [
    {"accountId": "account-beta", "customerPartyId": "party-beta", "revision": 1, "basis": "REG/account-beta", "effectiveFrom": "2026-01-02T00:00:00Z"}
  ],
  "relationships": [
    {
      "relationshipId": "rel-acme-beta", "revision": 1,
      "holder": "party-acme", "counterparty": "party-beta",
      "role": "CUSTOMER", "scope": "scope-1", "basis": "CONTRACT/rel-1",
      "effectiveStartsAt": "2026-01-03T00:00:00Z",
      "approval": {"reference": "approval-rel-1", "approvedAt": "2026-01-02T12:00:00Z"}
    }
  ]
}`

// Covers: 翻译产物按文件内 parties → entities → accounts → relationships 的次序排列——
// 这个次序是批内前项先落库、后项引用检查看得见前项的前提，翻译层乱序会让合法批
// 被自己的引用检查拒绝。
func TestPartyBatchTranslationKeepsReferenceOrder(t *testing.T) {
	commands, err := partyBatchFromJSON([]byte(partyFullBatchJSON))
	if err != nil {
		t.Fatalf("翻译完整批：%v", err)
	}
	wantLabels := []string{
		"参与方 party-acme r1",
		"参与方 party-beta r1",
		"责任法人 legal-acme r1",
		"客户账户 account-beta r1",
		"参与方关系 rel-acme-beta r1",
	}
	if len(commands) != len(wantLabels) {
		t.Fatalf("命令数 = %d, want %d", len(commands), len(wantLabels))
	}
	for index, want := range wantLabels {
		if commands[index].label != want {
			t.Fatalf("第 %d 项标签 = %q, want %q", index+1, commands[index].label, want)
		}
	}
}

// Covers: 未知字段、集合外取值与缺件在触库之前拒收，绝不代填默认（与 publish 翻译
// 同款纪律）。
func TestPartyBatchTranslationRejectsForeignShapes(t *testing.T) {
	cases := map[string]struct {
		body    string
		wantErr string
	}{
		"未知字段": {
			body:    `{"tenantId": "t", "businessParties": [{"partyId": "p", "name": "n", "revision": 1, "basis": "b", "effectiveFrom": "2026-01-01T00:00:00Z", "nickname": "x"}]}`,
			wantErr: "不是本入口的形状",
		},
		"空批": {
			body:    `{"tenantId": "t"}`,
			wantErr: "没有任何项",
		},
		"集合外角色": {
			body: `{"tenantId": "t", "relationships": [{
				"relationshipId": "r", "revision": 1, "holder": "a", "counterparty": "b",
				"role": "FRIEND", "scope": "s", "basis": "c", "effectiveStartsAt": "2026-01-01T00:00:00Z"}]}`,
			wantErr: "未知参与方角色",
		},
		"缺登记依据": {
			body:    `{"tenantId": "t", "businessParties": [{"partyId": "p", "name": "n", "revision": 1, "basis": "", "effectiveFrom": "2026-01-01T00:00:00Z"}]}`,
			wantErr: "identity basis reference",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := partyBatchFromJSON([]byte(testCase.body))
			if err == nil {
				t.Fatal("变形批被收下了")
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("错误 %q 不含 %q", err, testCase.wantErr)
			}
		})
	}
}

// Covers: 停用批翻译——身份种类是封闭三值，集合外拒收；合法批逐项译出。
func TestDeactivationBatchTranslation(t *testing.T) {
	commands, err := deactivationCommandsFromJSON([]byte(`{
	  "tenantId": "tenant-1",
	  "deactivations": [
	    {"kind": "BUSINESS_PARTY", "id": "party-acme", "revision": 2, "basis": "DEREG/party-acme", "at": "2026-06-01T00:00:00Z"}
	  ]
	}`))
	if err != nil {
		t.Fatalf("翻译停用批：%v", err)
	}
	if len(commands) != 1 || commands[0].ID != "party-acme" || commands[0].Revision != 2 {
		t.Fatalf("停用命令译错：%+v", commands)
	}

	if _, err := deactivationCommandsFromJSON([]byte(`{
	  "tenantId": "tenant-1",
	  "deactivations": [
	    {"kind": "RELATIONSHIP", "id": "rel-1", "revision": 2, "basis": "b", "at": "2026-06-01T00:00:00Z"}
	  ]
	}`)); err == nil || !strings.Contains(err.Error(), "未知身份种类") {
		t.Fatalf("关系不该走停用口（终止走撤销/到期/替代），err = %v", err)
	}
}

// Covers: 票 01 完成标准的进程口半边，端到端于真库——一批四册齐落（批内后项的引用
// 检查真实看见前项已提交的行）、重放整批以 ALREADY_REGISTERED 回答不追加行、同修订
// 异正文报 CONTENT_CONFLICT 请人看、停用落新修订且重放停用不重复落笔。
func TestRegisterPartiesBatchLandsRepliesAndDeactivates(t *testing.T) {
	dsn := freshMigratedDSN(t)

	batch := batchFile(t, partyFullBatchJSON)
	if code := runCLI(t, dsn, "register-parties", "-input", batch); code != exitLanded {
		t.Fatalf("首批 exit = %d, want %d", code, exitLanded)
	}
	// 重放整批：同键同内容以原结果回答，不追加第二行。
	if code := runCLI(t, dsn, "register-parties", "-input", batch); code != exitLanded {
		t.Fatalf("重放批 exit = %d, want %d", code, exitLanded)
	}
	// 同修订异正文：冲突绝不覆盖，报请人看。
	conflicted := batchFile(t, strings.Replace(partyFullBatchJSON, "Acme 物流", "Acme 物流改名", 1))
	if code := runCLI(t, dsn, "register-parties", "-input", conflicted); code != exitAttention {
		t.Fatalf("冲突批 exit = %d, want %d", code, exitAttention)
	}

	deactivation := batchFile(t, `{
	  "tenantId": "tenant-1",
	  "deactivations": [
	    {"kind": "CUSTOMER_ACCOUNT", "id": "account-beta", "revision": 2, "basis": "DEREG/account-beta", "at": "2026-06-01T00:00:00Z"}
	  ]
	}`)
	if code := runCLI(t, dsn, "deactivate-party-identity", "-input", deactivation); code != exitLanded {
		t.Fatalf("停用批 exit = %d, want %d", code, exitLanded)
	}
	// 重放停用：已停用册面同修订同内容以重放回答，不重复落笔。
	if code := runCLI(t, dsn, "deactivate-party-identity", "-input", deactivation); code != exitLanded {
		t.Fatalf("停用重放 exit = %d, want %d", code, exitLanded)
	}
	// 从未登记的身份：停用答 NOT_FOUND，报请人看。
	missing := batchFile(t, `{
	  "tenantId": "tenant-1",
	  "deactivations": [
	    {"kind": "BUSINESS_PARTY", "id": "party-ghost", "revision": 1, "basis": "DEREG/ghost", "at": "2026-06-01T00:00:00Z"}
	  ]
	}`)
	if code := runCLI(t, dsn, "deactivate-party-identity", "-input", missing); code != exitAttention {
		t.Fatalf("悬空停用 exit = %d, want %d", code, exitAttention)
	}
}
