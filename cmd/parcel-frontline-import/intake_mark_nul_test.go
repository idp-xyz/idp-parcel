package main

import (
	"errors"
	"strings"
	"testing"
)

// Covers: 观察号码那一格带 NUL 的行在译装时整批拒（票 no-reception-source/02 评审阻断）的真库面：收寄记录原样保全外部标识
// 观察（ADR-0199 决定一），而 Postgres 的 text 与 jsonb 存不下 NUL——放进编排那一行只会回滚记未决，重跑多少次都一样。
// 三种观察都在译装时拒成坏行，报文点名行号与 mark 列，册上不落。明确接收那一支在来源保全之前就已在身份核对按这个号查 PS 时
// 撞错，同由译装挡住。身份核对接生产的关联册读口。夹具全为合成（S 级）。
func TestFrontlineImportRejectsAMarkCellCarryingNUL(t *testing.T) {
	for name, tail := range map[string]string{
		"明确接收": "RECEIVED,SYN-RECEIPT-0016,,2026-09-08T09:15:00+08:00\n",
		"明确拒收": "REFUSED,,SYN-REASON-PACKAGING-DAMAGED,2026-09-08T09:15:00+08:00\n",
		"仅扫描":  "SCAN_ONLY,,,2026-09-08T09:15:00+08:00\n",
	} {
		t.Run(name, func(t *testing.T) {
			importer, receptions := verticalImporter(t, nil)
			row := "INTAKE-1,SYN-BATCH-1,SYN-SLIP-0016,SYN-CLERK-A,SYN-NODE-SZX,SYN-CUSTOMER-1,SYN-HU-0016,SYN-MARK-\x00-0016," + tail

			batch, err := decodeIntakeTemplate(synTenant(t), []byte(templateHeader+row))
			if err == nil {
				results := importIntake(t.Context(), batch, importer)
				t.Fatalf("译装放过了带 NUL 的号，导入答：第 %d 行 → %s [%s] %s",
					results[0].Line, results[0].Outcome, results[0].Disposition, results[0].Detail)
			}
			if !errors.Is(err, ErrIntakeTemplate) || !strings.Contains(err.Error(), "第 2 行") || !strings.Contains(err.Error(), "mark=") ||
				!strings.Contains(err.Error(), "NUL") {
				t.Fatalf("报文 = %q，要带收寄模板不合格、第 2 行、mark 列与 NUL", err)
			}
			if _, found := findReception(t, receptions, boundedSourcePrefix+"SYN-SLIP-0016"); found {
				t.Fatal("坏行落了册")
			}
		})
	}
}
