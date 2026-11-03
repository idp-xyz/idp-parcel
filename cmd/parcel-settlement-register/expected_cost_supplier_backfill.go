package main

import (
	"fmt"
	"strings"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	sapartycommercial "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/partycommercial"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// buildExpectedCostSupplierBackfill 装补记口（ADR-0358 决定三）：预期成本册接真册，协议的供应商经结算的消费侧适配器问
// party-commercial 的真读口，与派发进程形成成本时问的是同一只。
func buildExpectedCostSupplierBackfill(db *bentopg.DB) (*application.BackfillExpectedCostSupplierHandler, error) {
	costs, err := sapostgres.NewSupplierExpectedCosts(db)
	if err != nil {
		return nil, fmt.Errorf("构造预期成本册：%w", err)
	}
	agreements, err := pcpostgres.NewSupplierAgreementSuppliers(db)
	if err != nil {
		return nil, fmt.Errorf("构造协议供应商读口：%w", err)
	}
	suppliers, err := sapartycommercial.NewSupplierAgreementSupplier(agreements)
	if err != nil {
		return nil, fmt.Errorf("构造协议供应商：%w", err)
	}
	handler, err := application.NewBackfillExpectedCostSupplierHandler(application.BackfillExpectedCostSupplierDeps{Costs: costs, Suppliers: suppliers})
	if err != nil {
		return nil, fmt.Errorf("构造预期成本供应商补记：%w", err)
	}
	return handler, nil
}

// backfillAnswer 把一次补记译成答复与退出码，逐份列出补了谁、哪几份补不上。归格看恢复动作：全补上（含一份都不用补）→ 0；
// 有几份 PC 不认或答出译不过来的值 → 2，其余照写、整笔提交，补不上的要人去看两侧为什么对不上，重跑同一份还是这几份；
// 命令立不住 → 1；读不动、写不进 → 3，整笔回滚，原样重跑。
func backfillAnswer(command string, result application.BackfillExpectedCostSupplierResult) (string, int) {
	switch result.Outcome {
	case application.ExpectedCostSupplierBackfilled:
		lines := make([]string, 0, len(result.Recorded)+len(result.Unresolved)+1)
		for _, recorded := range result.Recorded {
			lines = append(lines, "补记 "+recorded.Version.String()+" → "+recorded.Supplier.String())
		}
		for _, gap := range result.Unresolved {
			lines = append(lines, "补不上 "+gap.Version.String()+"（协议 "+gap.Agreement.String()+"）："+gap.Gap.String())
		}
		head := fmt.Sprintf("%s: 补记 %d 份", command, len(result.Recorded))
		if len(result.Recorded) == 0 && len(result.Unresolved) == 0 {
			head = command + ": 没有待补记的预期成本"
		}
		if len(result.Unresolved) > 0 {
			head += fmt.Sprintf("，%d 份补不上（PC 不认它们的协议或答出译不过来的值，仍读作未记）", len(result.Unresolved))
			return strings.Join(append([]string{head}, lines...), "\n"), exitConflict
		}
		return strings.Join(append([]string{head}, lines...), "\n"), exitRegistered
	case application.ExpectedCostSupplierBackfillNotAccepted:
		return command + ": 输入立不住（没有租户）", exitUsage
	case application.ExpectedCostSupplierBackfillUndecided:
		return fmt.Sprintf("%s: 未决，整笔回滚：%v", command, result.Cause), exitUndecided
	default:
		return fmt.Sprintf("%s: 补记编排交回了封闭集合以外的结果 %d", command, result.Outcome), exitUndecided
	}
}
