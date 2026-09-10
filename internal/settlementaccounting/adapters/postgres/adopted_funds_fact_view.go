package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// AdoptedFundsFactView 实现 ports.AdoptedFundsFactView：按（租户、事实引用、采用版本）取一条已采用
// 事实的本体，给下游按信封引用回查用（票 sa-cc/03）。与 ExternalFundsFacts 分型：读方拿不到 Save。
// 版本进 WHERE 而不是读回再比——库里将来一事实多行（更正版本各成一行）时，这一句照样只答被问的那一版。
type AdoptedFundsFactView struct {
	db *bentopg.DB
}

func NewAdoptedFundsFactView(db *bentopg.DB) (*AdoptedFundsFactView, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &AdoptedFundsFactView{db: db}, nil
}

var _ ports.AdoptedFundsFactView = (*AdoptedFundsFactView)(nil)

// LoadAdoptedFundsFact 取信封所指的那一版。没有这一版（还没落、或存的是另一版）只回 false，
// 不拿当前版顶替：信封先后与版本先后不同源。
func (view *AdoptedFundsFactView) LoadAdoptedFundsFact(
	ctx context.Context,
	tenant domain.TenantID,
	fact domain.FundsFactReference,
	version domain.FundsFactVersion,
) (domain.ExternalFundsFact, bool, error) {
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ExternalFundsFact{}, false, fmt.Errorf("load adopted funds fact: %w", err)
	}
	key := ports.FundsFactKey{TenantID: tenant, Fact: fact}
	record, found, err := scanExternalFundsFact(querier.QueryRow(ctx,
		externalFundsFactSelect+`
		  WHERE tenant_id = $1
		    AND fact_id = $2
		    AND version = $3`,
		tenant.String(),
		fact.String(),
		version.String(),
	), key)
	if err != nil {
		return domain.ExternalFundsFact{}, false, fmt.Errorf("load adopted funds fact: %w", err)
	}
	if !found {
		return domain.ExternalFundsFact{}, false, nil
	}
	return record.Fact, true, nil
}
