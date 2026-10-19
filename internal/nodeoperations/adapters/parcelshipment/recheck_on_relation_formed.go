package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// RecheckByMark 是按号重核那一个入口，application.RecheckPendingIdentityHandler 满足它。
type RecheckByMark interface {
	RecheckByMark(ctx context.Context, tenant nodomain.TenantID, mark string) ([]application.RecheckPendingIdentityResult, error)
}

// RecheckOnRelationFormedAdapter 把 PS 的门铃译成按号重核（ADR-0199 决定三）：门铃是叫 NO 再问一次的铃，不是身份答案，这里只
// 把 PS 的「号码」交给重核当作外部标识观察去比，身份照旧由重核经身份核对读口问 PS。
type RecheckOnRelationFormedAdapter struct {
	recheck RecheckByMark
}

var _ noinbox.FormedExternalIdentifierRelationHandler = (*RecheckOnRelationFormedAdapter)(nil)

func NewRecheckOnRelationFormedAdapter(recheck RecheckByMark) (*RecheckOnRelationFormedAdapter, error) {
	if recheck == nil {
		return nil, errors.New("node operations parcelshipment adapter: recheck on relation formed needs the recheck orchestration")
	}
	return &RecheckOnRelationFormedAdapter{recheck: recheck}, nil
}

// HandleFormedExternalIdentifierRelation 原样转交租户与号码。重核的错误原样交回，消费门据此回滚、信封重投；租户译不成本上下文
// 的值也交回错误——消费门已把缺租户的门铃当毒丸拒了，走到这里还译不成是一格编程错误，不能悄悄丢掉。
func (adapter *RecheckOnRelationFormedAdapter) HandleFormedExternalIdentifierRelation(
	ctx context.Context,
	formed noinbox.FormedExternalIdentifierRelation,
) error {
	tenant, err := nodomain.NewTenantID(formed.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	if _, err := adapter.recheck.RecheckByMark(ctx, tenant, formed.Number); err != nil {
		return err
	}
	return nil
}
