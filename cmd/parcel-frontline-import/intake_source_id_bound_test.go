package main

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-bento-go/eventing"
)

// 本文件在真库上钉前线导入的来源标识上限（票 operator-channel/54）：恰在收寄意图信封的 Subject 上限内的一行照常形成收寄、
// 交出意图；越过一个字节或带控制字符的那一行在译装时整批拒，册上不落、意图零封。编排照 importerWithSwitchableHandoff 装，
// 意图交付开着——要证的是生产的意图交付收得下恰在上限的那一封。夹具全为合成（S 级）。

// Covers: 票 operator-channel/54 完成判据「恰在上限内照常形成收寄并交出意图（真库）」。
func TestFrontlineImportHandsOffTheIntentForASourceIDRightAtTheEnvelopeLimit(t *testing.T) {
	pool, importer, handoff, receptions := importerWithSwitchableHandoff(t)
	handoff.down = false

	factRef := factRefFilling(t, eventing.MaxSubjectLength)
	results := importTemplate(t, importer, []byte(templateHeader+receivedRowIn("SYN-BATCH-1", factRef)))
	assertDisposition(t, results, 0, rowLanded, "INTAKE_FORMED")
	if strings.Contains(results[0].Detail, "意图未交出") {
		t.Fatalf("恰在上限内的一行没交出意图：%s", results[0].Detail)
	}
	if _, found := findReception(t, receptions, boundedSourcePrefix+factRef); !found {
		t.Fatalf("恰在上限内的一行没落册")
	}
	if got := importedNodeIntakeIntents(t, pool); got != 1 {
		t.Fatalf("意图 %d 封，要 1", got)
	}
}

// Covers: 票 operator-channel/54 完成判据「越过一个字节的那一行答坏行、册上不落、报告点名；带控制字符同」。修前这一行照常
// 落册、答 INTAKE_FORMED，明细写「重跑同一文件会重发同一份」，而重发的那一封每次都被信封拒收。
func TestFrontlineImportRejectsASourceIDTheIntentEnvelopeCannotCarry(t *testing.T) {
	cases := []struct {
		name    string
		factRef func(t *testing.T) string
		wants   string
	}{
		{"越过一个字节", func(t *testing.T) string { return factRefFilling(t, eventing.MaxSubjectLength+1) }, "请缩短批次号或行事实号"},
		{"带控制字符", func(*testing.T) string { return "SYN-SLIP-\a0001" }, "请删掉批次号或行事实号里的不可见字符"},
	}
	for _, spec := range cases {
		t.Run(spec.name, func(t *testing.T) {
			pool, importer, handoff, receptions := importerWithSwitchableHandoff(t)
			handoff.down = false

			factRef := spec.factRef(t)
			batch, err := decodeIntakeTemplate(synTenant(t), []byte(templateHeader+receivedRowIn("SYN-BATCH-1", factRef)))
			if err == nil {
				results := importIntake(t.Context(), batch, importer)
				t.Fatalf("译装放过了信封装不下的来源标识（%d 字节），导入答：第 %d 行 → %s [%s] %s",
					len(batch.Rows[0].Command.SourceID), results[0].Line, results[0].Outcome, results[0].Disposition, results[0].Detail)
			}
			if !errors.Is(err, ErrIntakeTemplate) || !strings.Contains(err.Error(), "第 2 行") || !strings.Contains(err.Error(), spec.wants) {
				t.Fatalf("报文 = %q，要带收寄模板不合格、第 2 行与「%s」", err, spec.wants)
			}
			if _, found := findReception(t, receptions, boundedSourcePrefix+factRef); found {
				t.Fatal("坏行落了册")
			}
			if got := importedNodeIntakeIntents(t, pool); got != 0 {
				t.Fatalf("意图 %d 封，要 0", got)
			}
		})
	}
}
