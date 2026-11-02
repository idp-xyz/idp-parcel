package pricecardtemplate

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证条件树与计算树的层数上限（maxTreeDepth）：超限在进领域构造之前就拦，问题落在超限的节点上，两种载体同话同坐标；
// 恰在上限上的树照常立得住，且登记快照读得回来。

// conditionChainCard 把 fullCard 里偏远附加费的触发条件换成一条 levels 层的单链：根与中间各层都是只有一个子节点的
// ALL_OF，最底一层是判定。leafRow 是叶子在 conditions 表里的行号。
func conditionChainCard(levels int) (card filled, leafRow int) {
	card = fullCard().clone()
	var kept rows
	for _, row := range card.data[sheetConditions] {
		if row["nodeId"] != "C-REMOTE" {
			kept = append(kept, row)
		}
	}
	kept = append(kept, map[string]string{"nodeId": "C-REMOTE", "kind": "ALL_OF"})
	parent := "C-REMOTE"
	for level := 2; level < levels; level++ {
		id := fmt.Sprintf("C-REMOTE-L%06d", level)
		kept = append(kept, map[string]string{"nodeId": id, "parentId": parent, "kind": "ALL_OF"})
		parent = id
	}
	kept = append(kept, map[string]string{"nodeId": "C-REMOTE-LEAF", "parentId": parent, "kind": "PREDICATE", "source": "ZONE", "category": "Z1"})
	card.data[sheetConditions] = kept
	return card, len(kept) + 1
}

// Covers: `AT-PP-050`——条件树恰在层数上限上照常读通（两种载体同一个摘要），登记快照读得回来；多一层即在超限的节点上报
// TREE_INVALID，不再交领域构造，所以没有连带的构造门问题，两种载体的问题清单逐字相同。
func TestAConditionTreeAtTheDepthCapIsReadAndOneLevelMoreIsRefusedAtTheNode(t *testing.T) {
	atCap, _ := conditionChainCard(maxTreeDepth)
	fromFile := read(t, atCap.bytes(t))
	plan := validated(t, fromFile)
	if key := readingKey(t, readWorkbookJSON(t, atCap.workbookJSON(t))); key != readingKey(t, fromFile) {
		t.Fatalf("恰在上限上：两种载体读法不同\n%s", key)
	}
	// 层数过了约五千，登记快照的 JSON 嵌套超过 encoding/json 的上限、读不回来；上限以内的树要能来回。
	tenant := value(t, domain.NewTenantID, "SYN-TENANT-01")
	source := try(domain.NewSourceFileIdentity("SYN-DEPTH.xlsx", strings.Repeat("a", 64)))
	snapshot := try(domain.MarshalPriceCardRegistration(try(domain.NewPriceCardRegistration(tenant, plan, source, fromFile.Content.DirectionAuthorization, "SYN-PRICING-GOVERNOR-01"))))
	if _, err := domain.RehydratePriceCardRegistration(snapshot); err != nil {
		t.Fatalf("恰在上限上的树登记快照读不回来：%v", err)
	}

	tooDeep, leafRow := conditionChainCard(maxTreeDepth + 1)
	fromFile = read(t, tooDeep.bytes(t))
	if !fromFile.Accepted || fromFile.Content != nil || len(fromFile.Problems) != 1 ||
		!hasProblem(fromFile.Problems, at{sheetConditions, leafRow, "parentId", ports.TemplateTreeInvalid}) {
		t.Fatalf("多一层：想要叶子 parentId 格上恰一条 TREE_INVALID：%s", describe(fromFile.Problems))
	}
	message := fromFile.Problems[0].Message
	for _, want := range []string{"条件树最多 32 层", "C-REMOTE-LEAF", "第 33 层"} {
		if !strings.Contains(message, want) {
			t.Errorf("话里该有「%s」：%s", want, message)
		}
	}
	if key := readingKey(t, readWorkbookJSON(t, tooDeep.workbookJSON(t))); key != readingKey(t, fromFile) {
		t.Fatalf("多一层：两种载体读法不同\n%s", key)
	}
}

// Covers: `AT-PP-050`——计算树同一道门：取较大值一层套一层到第 33 层，超限的两个节点各报一条，话里说的是计算树；领域那道
// 「取较大值不套取较大值」的构造门不再跑，因为构造走到超限的节点就停了。
func TestACalculationTreeDeeperThanTheCapIsRefusedAtTheNodesPastIt(t *testing.T) {
	card := fullCard().clone()
	var kept rows
	for _, row := range card.data[sheetCalculations] {
		if !strings.HasPrefix(row["nodeId"], "K-OVERSIZE") {
			kept = append(kept, row)
		}
	}
	kept = append(kept, map[string]string{"nodeId": "K-OVERSIZE", "method": "GREATER_OF"})
	parent := "K-OVERSIZE"
	for level := 2; level <= maxTreeDepth; level++ {
		id := fmt.Sprintf("K-G%02d", level)
		kept = append(kept,
			map[string]string{"nodeId": id, "parentId": parent, "method": "GREATER_OF"},
			map[string]string{"nodeId": fmt.Sprintf("K-F%02d", level), "parentId": parent, "method": "FIXED_AMOUNT", "amount": "1"})
		parent = id
	}
	kept = append(kept,
		map[string]string{"nodeId": "K-PAST-A", "parentId": parent, "method": "FIXED_AMOUNT", "amount": "1"},
		map[string]string{"nodeId": "K-PAST-B", "parentId": parent, "method": "FIXED_AMOUNT", "amount": "2"})
	card.data[sheetCalculations] = kept

	reading := read(t, card.bytes(t))
	if reading.Content != nil || len(reading.Problems) != 2 {
		t.Fatalf("想要恰两条问题：%s", describe(reading.Problems))
	}
	for index, id := range []string{"K-PAST-A", "K-PAST-B"} {
		problem := reading.Problems[index]
		if problem.Sheet != sheetCalculations || problem.Column != "parentId" || problem.Code != ports.TemplateTreeInvalid ||
			!strings.Contains(problem.Message, "计算树最多 32 层") || !strings.Contains(problem.Message, id) {
			t.Errorf("第 %d 条：%+v", index, problem)
		}
	}
	if key := readingKey(t, readWorkbookJSON(t, card.workbookJSON(t))); key != readingKey(t, reading) {
		t.Fatalf("两种载体读法不同\n%s", key)
	}
}

// Covers: `AT-PP-050`——层数上限把领域那段平方压住：领域每层构造都把整棵子树校验一遍，一条近十万节点的条件树单链若逐层
// 构造是节点数的平方；在第 33 层即拦，读取的代价跟着节点数走，不随链长平方增长。
func TestALongConditionChainIsStoppedAtTheCapNotBuiltLevelByLevel(t *testing.T) {
	card, _ := conditionChainCard(maxRowsPerSheet - len(fullCard().data[sheetConditions]))
	body := compactJSON(t, card.workbookJSON(t))
	start := time.Now()
	reading := readWorkbookJSON(t, body)
	elapsed := time.Since(start)
	if reading.Content != nil || len(reading.Problems) != 1 || reading.Problems[0].Code != ports.TemplateTreeInvalid {
		t.Fatalf("想要恰一条 TREE_INVALID：%s", describe(reading.Problems))
	}
	t.Logf("体 %d 字节，读取 %v", len(body), elapsed)
	if elapsed > 10*time.Second {
		t.Fatalf("长链读了 %v；该在层数上限处停", elapsed)
	}
}
