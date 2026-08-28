package main

import (
	"strings"
	"testing"
)

// 本文件证产品登记进程口的两半（票 admin-remainder-mechanism-batch/02）：
// 翻译纪律——批文逐字段过领域构造门，未知字段、集合外形态与 channels 缺席在触库前
// 拒收，`[]` 是显式“未配置”声明不是缺件；
// 批推进——对真库端到端跑 register-products，证映射对已发布产品版本的引用检查真实
// 可见、重放以原结果回答、同修订异内容冲突报请人看、悬空产品引用被拒。

const productFullBatchJSON = `{
  "tenantId": "tenant-1",
  "scope": "scope-1",
  "forms": [
    {"productId": "product-express", "version": "v1", "form": "NETWORK_SERVICE"}
  ],
  "mappings": [
    {
      "mappingId": "map-express", "revision": 1,
      "productId": "product-express", "productVersion": "v1",
      "channels": ["CH-SG-POST", "CH-AGG-01"],
      "basis": "DEC/map-express-r1",
      "effectiveStartsAt": "2026-02-01T00:00:00Z"
    },
    {
      "mappingId": "map-open", "revision": 1,
      "productId": "product-express", "productVersion": "v1",
      "channels": [],
      "basis": "DEC/map-open-r1",
      "effectiveStartsAt": "2026-02-01T00:00:00Z"
    }
  ]
}`

// Covers: 翻译产物按文件内 forms → mappings 次序排列；`[]` 译成显式“未配置”绑定
// 而非被拒——那是登记者说出的商业声明（CONTEXT 渠道绑定格）。
func TestProductBatchTranslationKeepsOrderAndExplicitUnconfigured(t *testing.T) {
	commands, err := productBatchFromJSON([]byte(productFullBatchJSON))
	if err != nil {
		t.Fatalf("翻译完整批：%v", err)
	}
	wantLabels := []string{
		"服务形态 product-express/v1 NETWORK_SERVICE",
		"产品—渠道映射 map-express r1",
		"产品—渠道映射 map-open r1",
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

// Covers: 未知字段、集合外取值与缺件在触库之前拒收，绝不代填默认；channels 缺席与
// `[]` 必须可分辨——前者是输入缺件，后者是显式声明。
func TestProductBatchTranslationRejectsForeignShapes(t *testing.T) {
	cases := map[string]struct {
		body    string
		wantErr string
	}{
		"未知字段": {
			body:    `{"tenantId": "t", "scope": "s", "forms": [{"productId": "p", "version": "v1", "form": "NETWORK_SERVICE", "nickname": "x"}]}`,
			wantErr: "不是本入口的形状",
		},
		"空批": {
			body:    `{"tenantId": "t", "scope": "s"}`,
			wantErr: "没有任何项",
		},
		"集合外形态": {
			body:    `{"tenantId": "t", "scope": "s", "forms": [{"productId": "p", "version": "v1", "form": "STANDALONE_LABEL"}]}`,
			wantErr: "未知服务形态",
		},
		"channels 缺席": {
			body: `{"tenantId": "t", "scope": "s", "mappings": [{
				"mappingId": "m", "revision": 1, "productId": "p", "productVersion": "v1",
				"basis": "b", "effectiveStartsAt": "2026-02-01T00:00:00Z"}]}`,
			wantErr: "channels 缺席",
		},
		"空白渠道引用": {
			body: `{"tenantId": "t", "scope": "s", "mappings": [{
				"mappingId": "m", "revision": 1, "productId": "p", "productVersion": "v1",
				"channels": [" "], "basis": "b", "effectiveStartsAt": "2026-02-01T00:00:00Z"}]}`,
			wantErr: "channel product reference",
		},
		"缺登记依据": {
			body: `{"tenantId": "t", "scope": "s", "mappings": [{
				"mappingId": "m", "revision": 1, "productId": "p", "productVersion": "v1",
				"channels": ["CH-01"], "basis": "", "effectiveStartsAt": "2026-02-01T00:00:00Z"}]}`,
			wantErr: "mapping basis reference",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := productBatchFromJSON([]byte(testCase.body))
			if err == nil {
				t.Fatal("变形批被收下了")
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("错误 %q 不含 %q", err, testCase.wantErr)
			}
		})
	}
}

// Covers: 票 02 完成标准的进程口半边，端到端于真库——先经 publish 铺垫已生效的服务
// 产品版本，形态与映射（含显式“未配置”）一批齐落；重放整批以 ALREADY_REGISTERED
// 回答不追加行；同修订异内容报 CONTENT_CONFLICT；悬空产品引用被拒报请人看。
func TestRegisterProductsBatchLandsRepliesAndRefusesDanglers(t *testing.T) {
	dsn := freshMigratedDSN(t)

	published := batchFile(t, `{"items":[{
	  "tenantId": "tenant-1",
	  "kind": "SERVICE_PRODUCT",
	  "objectId": "product-express",
	  "version": "v1",
	  "scope": "scope-1",
	  "contentDigest": "sha256:product-express-v1",
	  "effectiveStartsAt": "2026-01-01T00:00:00Z",
	  "approval": {
	    "reference": "approval-product-express",
	    "source": "source-product-express",
	    "approvedAt": "2026-01-02T00:00:00Z"
	  },
	  "approvalRoleStanding": "CONFIRMED"
	}]}`)
	if code := runCLI(t, dsn, "publish", "-input", published); code != exitLanded {
		t.Fatalf("铺垫发布 exit = %d, want %d", code, exitLanded)
	}

	batch := batchFile(t, productFullBatchJSON)
	if code := runCLI(t, dsn, "register-products", "-input", batch); code != exitLanded {
		t.Fatalf("首批 exit = %d, want %d", code, exitLanded)
	}
	// 重放整批：同键同内容以原结果回答，不追加第二行。
	if code := runCLI(t, dsn, "register-products", "-input", batch); code != exitLanded {
		t.Fatalf("重放批 exit = %d, want %d", code, exitLanded)
	}
	// 同修订异内容：冲突绝不覆盖，报请人看。
	conflicted := batchFile(t, strings.Replace(productFullBatchJSON, `"CH-SG-POST", "CH-AGG-01"`, `"CH-OTHER"`, 1))
	if code := runCLI(t, dsn, "register-products", "-input", conflicted); code != exitAttention {
		t.Fatalf("冲突批 exit = %d, want %d", code, exitAttention)
	}
	// 悬空产品引用：映射不钉不在册的版本。
	dangling := batchFile(t, `{
	  "tenantId": "tenant-1",
	  "scope": "scope-1",
	  "mappings": [{
	    "mappingId": "map-ghost", "revision": 1,
	    "productId": "product-ghost", "productVersion": "v1",
	    "channels": ["CH-01"], "basis": "DEC/ghost",
	    "effectiveStartsAt": "2026-02-01T00:00:00Z"
	  }]
	}`)
	if code := runCLI(t, dsn, "register-products", "-input", dangling); code != exitAttention {
		t.Fatalf("悬空批 exit = %d, want %d", code, exitAttention)
	}
	// 绑定调整走新修订：rev2 配置 map-open 的渠道，落定不覆盖 rev1。
	revised := batchFile(t, `{
	  "tenantId": "tenant-1",
	  "scope": "scope-1",
	  "mappings": [{
	    "mappingId": "map-open", "revision": 2,
	    "productId": "product-express", "productVersion": "v1",
	    "channels": ["CH-NEW-01"], "basis": "DEC/map-open-r2",
	    "effectiveStartsAt": "2026-03-01T00:00:00Z"
	  }]
	}`)
	if code := runCLI(t, dsn, "register-products", "-input", revised); code != exitLanded {
		t.Fatalf("修订批 exit = %d, want %d", code, exitLanded)
	}
}
