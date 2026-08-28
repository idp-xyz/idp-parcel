// Package postgres 是代收与清分的真库适配器：写口把登记与记账放进 0001 建的六张表，
// 点读口按键把它们读回领域对象。
//
// 写入代数一律照 ADR-0031：不 UPSERT，撞键由无 conflict target 的 DO NOTHING 折成
// `已登记`交回，内容是否同一份由编排读回自己比。**本包没有任何 UPDATE 记账或删除
// 记账的路径**——唯一的 UPDATE 是回汇批次那一格单向状态推进，且带 `WHERE state =
// 'COLLECTED'` 守着方向。
package postgres

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

// ledgerColumns 把分户账四维键摊成落库参数。四维总是一起出现——键在 SQL 上是五列
// （含租户），拆开传会让某一处漏掉一维而 SQL 照样跑得过。
func ledgerColumns(tenant domain.TenantID, key domain.SubledgerKey) []any {
	return []any{
		tenant.String(),
		key.Customer().String(),
		key.LegalEntity().String(),
		key.Currency().String(),
		key.Channel().String(),
	}
}

// rebuildLedgerKey 把库里的四维还原成领域键，逐维走同一道构造门。坏行在读口拦下
// 上抛，不进答案（判据同关务读口的逐行重建）。
func rebuildLedgerKey(customerRaw, entityRaw, currencyRaw, channelRaw string) (domain.SubledgerKey, error) {
	customer, err := domain.NewCustomerReference(customerRaw)
	if err != nil {
		return domain.SubledgerKey{}, fmt.Errorf("rebuild subledger key: %w", err)
	}
	entity, err := domain.NewLegalEntityReference(entityRaw)
	if err != nil {
		return domain.SubledgerKey{}, fmt.Errorf("rebuild subledger key: %w", err)
	}
	currency, err := domain.NewCurrencyCode(currencyRaw)
	if err != nil {
		return domain.SubledgerKey{}, fmt.Errorf("rebuild subledger key: %w", err)
	}
	channel, err := domain.NewCollectionChannelReference(channelRaw)
	if err != nil {
		return domain.SubledgerKey{}, fmt.Errorf("rebuild subledger key: %w", err)
	}
	key, err := domain.NewSubledgerKey(customer, entity, currency, channel)
	if err != nil {
		return domain.SubledgerKey{}, fmt.Errorf("rebuild subledger key: %w", err)
	}
	return key, nil
}

// rebuildMoney 把币种与最小单位金额还原成领域金额。零或负金额在库上撞 CHECK，读到
// 这一格说明行被绕过写口改过——上抛而不是折成零额。
func rebuildMoney(currencyRaw string, amountMinor int64) (domain.Money, error) {
	currency, err := domain.NewCurrencyCode(currencyRaw)
	if err != nil {
		return domain.Money{}, fmt.Errorf("rebuild money: %w", err)
	}
	amount, err := domain.NewMoney(currency, amountMinor)
	if err != nil {
		return domain.Money{}, fmt.Errorf("rebuild money: %w", err)
	}
	return amount, nil
}
