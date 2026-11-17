package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// ContractSellPriceArrangements 实现 ports.ContractSellPriceArrangementView：按已唯一选出的客户合同版本
// 点读销售价格约定（ADR-0380 决定二的点读半边）。与 Save 同表、分口：写在 PublicationRegistry，读给结算
// 顺着闭包取——写与读分开的理由同 CommercialPublicationView（F-2）。
type ContractSellPriceArrangements struct {
	db *bentopg.DB
}

func NewContractSellPriceArrangements(db *bentopg.DB) (*ContractSellPriceArrangements, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &ContractSellPriceArrangements{db: db}, nil
}

var _ ports.ContractSellPriceArrangementView = (*ContractSellPriceArrangements)(nil)

// LoadContractSellPriceArrangement 取回一份客户合同版本的销售价格约定。
//
// found=false = 这一版合同没有销售价格约定（无父行）。读取失败与坏数据走 error，不折成 found=false：
// 来源与不计销售价依据不是恰一在场，是写口被绕开之后的行，折成「没有约定」会让结算按旧合同去读两本采用册。
// 显式租户必须与合同版本同一身份（ADR-0003/0040）。重建走同一扇构造门。来源名照库里的原串重建，不去空白：
// 这个名字要逐字对上计价一侧的范围标识。
func (repository *ContractSellPriceArrangements) LoadContractSellPriceArrangement(
	ctx context.Context,
	tenant domain.TenantID,
	contract domain.CommercialVersion,
) (domain.ContractSellPriceArrangement, bool, error) {
	none := domain.ContractSellPriceArrangement{}
	if tenant.String() == "" ||
		contract.ObjectID().String() == "" || contract.Version().String() == "" {
		return none, false, fmt.Errorf("load contract sell price arrangement: tenant and contract identity are required")
	}
	if tenant != contract.Tenant() {
		return none, false, fmt.Errorf("load contract sell price arrangement: tenant does not own this contract")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load contract sell price arrangement: %w", err)
	}

	var existingBasis *string
	err = querier.QueryRow(ctx,
		`SELECT no_sell_price_basis
		   FROM party_commercial.customer_contract_sell_price_arrangement
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant.String(),
		uint8(domain.CustomerContractObject),
		contract.ObjectID().String(),
		contract.Version().String(),
	).Scan(&existingBasis)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load contract sell price arrangement: %w", err)
	}

	rawSources, err := scanSellPriceSources(ctx, querier, tenant.String(),
		uint8(domain.CustomerContractObject), contract.ObjectID().String(), contract.Version().String())
	if err != nil {
		return none, false, fmt.Errorf("load contract sell price arrangement: %w", err)
	}
	if (existingBasis != nil) == (len(rawSources) > 0) {
		return none, false, fmt.Errorf(
			"load contract sell price arrangement: 销售价格来源与不计销售价的依据恰有一个在场,读回的这份约定两者%s",
			bothOrNeither(existingBasis == nil))
	}

	var sources []domain.PricingScopeReference
	var basis domain.NoSellPriceBasis
	if existingBasis != nil {
		basis, err = domain.NewNoSellPriceBasis(*existingBasis)
		if err != nil {
			return none, false, fmt.Errorf("load contract sell price arrangement: %w", err)
		}
	} else {
		sources = make([]domain.PricingScopeReference, 0, len(rawSources))
		for _, raw := range rawSources {
			source, err := domain.NewPricingScopeReference(raw)
			if err != nil {
				return none, false, fmt.Errorf("load contract sell price arrangement: %w", err)
			}
			sources = append(sources, source)
		}
	}

	arrangement, err := domain.DeclareContractSellPriceArrangement(contract, sources, basis)
	if err != nil {
		return none, false, fmt.Errorf("load contract sell price arrangement: %w", err)
	}
	return arrangement, true, nil
}
