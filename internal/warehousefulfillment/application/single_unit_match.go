package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// SingleUnitMatch 是给一件货配上的那张出库请求。Waiting 是这位货主的这种货此刻还有几张单等着复核，含配上的这一张。
type SingleUnitMatch struct {
	Request        domain.OutboundRequestID
	DocumentNumber string
	Owner          domain.OwnerAccountID
	Goods          domain.GoodsCode
	Waiting        int
}

// SingleUnitMatcher 在复核打包台扫到一件货时，替它配一张单品单件的出库请求：单品单件波次的货是合在一起拿回来的，
// 哪一件发给哪一张单在拣货时没有定，到这里才定——同一种货的单是等价的，配给等得最久的那一张。
type SingleUnitMatcher struct {
	waves interface {
		SingleUnitAwaitingCheck(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode) ([]ports.SingleUnitCandidate, error)
	}
	goods interface {
		FindGoodsByCode(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, code domain.IdentifierCode, at time.Time) (ports.GoodsRow, bool, error)
	}
	clock ports.Clock
}

func NewSingleUnitMatcher(
	waves interface {
		SingleUnitAwaitingCheck(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode) ([]ports.SingleUnitCandidate, error)
	},
	goods interface {
		FindGoodsByCode(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, code domain.IdentifierCode, at time.Time) (ports.GoodsRow, bool, error)
	},
	clock ports.Clock,
) *SingleUnitMatcher {
	return &SingleUnitMatcher{waves: waves, goods: goods, clock: clock}
}

// Match 认扫到的码：它是某位货主的商品编码，或是那位货主登记过的条码、平台商品码。商品编码只在一位货主之内唯一，
// 所以按等复核的单里出现的每一位货主各认一次，认出来的商品要正好是那张单拣的那种货。几位货主都认得出这个码时，
// 取等得最久的那一张。没有等复核的单要这种货，found 为假。
func (matcher *SingleUnitMatcher) Match(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.IdentifierCode) (SingleUnitMatch, bool, error) {
	candidates, err := matcher.waves.SingleUnitAwaitingCheck(ctx, tenant, warehouse)
	if err != nil {
		return SingleUnitMatch{}, false, fmt.Errorf("match single unit order: %w", err)
	}
	at := matcher.clock.Now()
	// 每位货主只认一次：这个码在他那里指向哪种货，认不出记成空。
	resolved := map[string]string{}
	resolve := func(owner domain.OwnerAccountID) (string, error) {
		if goods, known := resolved[owner.String()]; known {
			return goods, nil
		}
		row, found, err := matcher.goods.FindGoodsByCode(ctx, tenant, owner, code, at)
		if err != nil {
			return "", err
		}
		goods := ""
		if found {
			goods = row.GoodsCode
		}
		resolved[owner.String()] = goods
		return goods, nil
	}
	var match SingleUnitMatch
	found := false
	for _, candidate := range candidates {
		if found {
			if candidate.Owner == match.Owner && candidate.Goods == match.Goods {
				match.Waiting++
			}
			continue
		}
		goods, err := resolve(candidate.Owner)
		if err != nil {
			return SingleUnitMatch{}, false, fmt.Errorf("match single unit order: %w", err)
		}
		if goods == "" || goods != candidate.Goods.String() {
			continue
		}
		match = SingleUnitMatch{
			Request: candidate.Request, DocumentNumber: candidate.DocumentNumber, Owner: candidate.Owner, Goods: candidate.Goods, Waiting: 1,
		}
		found = true
	}
	return match, found, nil
}
