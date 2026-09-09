package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
)

// CommercialResolutionKeyStore 是解析键登记面的持久化半边（syn-wall-door-audit 票 03
// 件 2），实现 pspartycommercial.ResolutionKeyStore。
//
// 它只搬运字符串行，不认识 party-commercial 的任何领域类型：词汇翻译在
// adapters/partycommercial 那半边。两半分开是两条架构门禁的交集——翻译不许待在通用
// postgres 包里，驱动不许出现在持久化适配器之外。
type CommercialResolutionKeyStore struct {
	db *bentopg.DB
}

func NewCommercialResolutionKeyStore(db *bentopg.DB) (*CommercialResolutionKeyStore, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &CommercialResolutionKeyStore{db: db}, nil
}

var _ pspartycommercial.ResolutionKeyStore = (*CommercialResolutionKeyStore)(nil)

// RegisterResolutionKey 落一行登记。先读回既有再决定写不写，冲突路径一行不动——判读
// 纪律与声明写入面相同（见 partycommercial/adapters/postgres 的声明写入注释）。
func (repository *CommercialResolutionKeyStore) RegisterResolutionKey(
	ctx context.Context,
	row pspartycommercial.ResolutionKeyRow,
) (pspartycommercial.ResolutionKeySaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return pspartycommercial.ResolutionKeySaveOutcomeInvalid, fmt.Errorf("register resolution key: %w", err)
	}

	var existingScope, existingLegal, existingPolicy string
	var existingAnchorAt time.Time
	var existingBases []string
	var existingCounterparty, existingChargeScope, existingCurrency *string
	var existingCreditLevel, existingCreditChargeType *string
	err = executor.QueryRow(ctx,
		`SELECT scope_ref, legal_entity_ref, anchor_policy_version, anchor_at, required_bases,
		        settlement_counterparty_ref, settlement_charge_scope_ref, settlement_currency_code,
		        credit_level, credit_charge_type
		   FROM parcel_shipment.commercial_resolution_key_registration
		  WHERE tenant_id = $1 AND customer_account_id = $2`,
		row.TenantID, row.CustomerAccountID,
	).Scan(&existingScope, &existingLegal, &existingPolicy, &existingAnchorAt, &existingBases,
		&existingCounterparty, &existingChargeScope, &existingCurrency,
		&existingCreditLevel, &existingCreditChargeType)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if _, err := executor.Exec(ctx,
			`INSERT INTO parcel_shipment.commercial_resolution_key_registration
				(tenant_id, customer_account_id, scope_ref, legal_entity_ref,
				 anchor_policy_version, anchor_at, required_bases,
				 settlement_counterparty_ref, settlement_charge_scope_ref, settlement_currency_code,
				 credit_level, credit_charge_type)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			row.TenantID,
			row.CustomerAccountID,
			row.Scope,
			row.LegalEntity,
			row.AnchorPolicy,
			row.AnchorAt.UTC(),
			row.RequiredBases,
			// 缺席写 NULL 不写空串：库内 `..._settlement_paired` / `..._credit_paired` 按 IS NULL
			// 判在场与否，空串会被它们当成在场，于是一行不该带选择维度的登记从缝里过去。
			nullableText(row.SettlementCounterparty),
			nullableText(row.SettlementChargeScope),
			nullableText(row.SettlementCurrency),
			nullableText(row.CreditLevel),
			nullableText(row.CreditChargeType),
		); err != nil {
			return pspartycommercial.ResolutionKeySaveOutcomeInvalid, fmt.Errorf("register resolution key: %w", err)
		}
		return pspartycommercial.ResolutionKeySaved, nil
	case err != nil:
		return pspartycommercial.ResolutionKeySaveOutcomeInvalid, fmt.Errorf("register resolution key: %w", err)
	}

	if existingScope != row.Scope ||
		existingLegal != row.LegalEntity ||
		existingPolicy != row.AnchorPolicy ||
		!existingAnchorAt.Equal(row.AnchorAt.UTC()) ||
		!sameResolutionBases(existingBases, row.RequiredBases) ||
		// 结算三维与信用二维一并比：换了费用范围、币种或授信等级就是换了一套解析口径，与换
		// 范围、换锚点同级，不静默覆盖。漏比它，一次改维会被答成`已登记`，而库里留着旧维。
		textOf(existingCounterparty) != row.SettlementCounterparty ||
		textOf(existingChargeScope) != row.SettlementChargeScope ||
		textOf(existingCurrency) != row.SettlementCurrency ||
		textOf(existingCreditLevel) != row.CreditLevel ||
		textOf(existingCreditChargeType) != row.CreditChargeType {
		return pspartycommercial.ResolutionKeyContentConflict, nil
	}
	return pspartycommercial.ResolutionKeyAlreadyRegistered, nil
}

// textOf 是 nullableText 的反向：库里的 NULL 读回成空串，登记面据此判「这一维缺席」。
func textOf(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// FindResolutionKey 读回一行登记。查无行是 (zero, false, nil)：显式未配置等租户来登记，
// 读取失败等依赖恢复，压成一格调用方就不知道该催人还是该重试。
func (repository *CommercialResolutionKeyStore) FindResolutionKey(
	ctx context.Context,
	tenant, customer string,
) (pspartycommercial.ResolutionKeyRow, bool, error) {
	none := pspartycommercial.ResolutionKeyRow{}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("find resolution key: %w", err)
	}

	row := pspartycommercial.ResolutionKeyRow{TenantID: tenant, CustomerAccountID: customer}
	var counterparty, chargeScope, currency, creditLevel, creditChargeType *string
	err = querier.QueryRow(ctx,
		`SELECT scope_ref, legal_entity_ref, anchor_policy_version, anchor_at, required_bases,
		        settlement_counterparty_ref, settlement_charge_scope_ref, settlement_currency_code,
		        credit_level, credit_charge_type
		   FROM parcel_shipment.commercial_resolution_key_registration
		  WHERE tenant_id = $1 AND customer_account_id = $2`,
		tenant, customer,
	).Scan(&row.Scope, &row.LegalEntity, &row.AnchorPolicy, &row.AnchorAt, &row.RequiredBases,
		&counterparty, &chargeScope, &currency, &creditLevel, &creditChargeType)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("find resolution key: %w", err)
	}
	row.SettlementCounterparty = textOf(counterparty)
	row.SettlementChargeScope = textOf(chargeScope)
	row.SettlementCurrency = textOf(currency)
	row.CreditLevel = textOf(creditLevel)
	row.CreditChargeType = textOf(creditChargeType)
	return row, true, nil
}

func sameResolutionBases(existing, incoming []string) bool {
	if len(existing) != len(incoming) {
		return false
	}
	set := make(map[string]bool, len(existing))
	for _, base := range existing {
		set[base] = true
	}
	for _, base := range incoming {
		if !set[base] {
			return false
		}
	}
	return true
}
