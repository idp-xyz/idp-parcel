package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// RecheckOnDeclarationRetiredAdapter 把 PS 的声明撤除门铃译成按号重核（ADR-0211 决定三）。与关联已形成那一只同一个重核入口：
// 按号找的重核对象只有一组定义（PendingIdentityLookup.FindRecheckableByMark），两种门铃都叫它再问一次 PS；声明撤除只会让只因歧义
// 信号的冲突行答出唯一命中，同号的零候选待识别多问一次，答案照旧不追加。门铃不是身份答案，身份照旧由重核经身份核对读口问 PS。
type RecheckOnDeclarationRetiredAdapter struct {
	recheck RecheckByMark
}

var _ noinbox.RetiredCustomerParcelNumberDeclarationHandler = (*RecheckOnDeclarationRetiredAdapter)(nil)

func NewRecheckOnDeclarationRetiredAdapter(recheck RecheckByMark) (*RecheckOnDeclarationRetiredAdapter, error) {
	if recheck == nil {
		return nil, errors.New("node operations parcelshipment adapter: recheck on declaration retired needs the recheck orchestration")
	}
	return &RecheckOnDeclarationRetiredAdapter{recheck: recheck}, nil
}

// HandleRetiredCustomerParcelNumberDeclaration 原样转交租户与号码，错误原样交回，理由同 RecheckOnRelationFormedAdapter 的同形方法。
func (adapter *RecheckOnDeclarationRetiredAdapter) HandleRetiredCustomerParcelNumberDeclaration(
	ctx context.Context,
	retired noinbox.RetiredCustomerParcelNumberDeclaration,
) error {
	tenant, err := nodomain.NewTenantID(retired.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	if _, err := adapter.recheck.RecheckByMark(ctx, tenant, retired.Number); err != nil {
		return err
	}
	return nil
}
