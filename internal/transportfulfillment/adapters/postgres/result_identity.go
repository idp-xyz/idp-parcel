package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 版本号的前缀只为可读，不承载语义：版本不得可解析出租户、对象或尝试——那些维度已经
// 在登记键上，写进号里就成了第二处定义。
const (
	pickupResultVersionPrefix   = "PRV-"
	deliveryResultVersionPrefix = "DRV-"
)

// ResultVersions 实现 ports.PickupIdentityFactory 与 ports.DeliveryIdentityFactory：
// 用 transport_fulfillment 自己的两条序列签发揽收与交付的结果版本。
//
// 一个类型担两个端口是因为两者是同一件事的两个域，装配处仍按端口各自注入——需要把
// 其中一个换成别的签发方式时，替换的是装配那一行，不必先把类型拆开。
type ResultVersions struct {
	db *bentopg.DB
}

func NewResultVersions(db *bentopg.DB) (*ResultVersions, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &ResultVersions{db: db}, nil
}

var (
	_ ports.PickupIdentityFactory   = (*ResultVersions)(nil)
	_ ports.DeliveryIdentityFactory = (*ResultVersions)(nil)
)

// NextPickupResultVersion 逐成功对象签发揽收结果版本。来源更正形成新版本而不覆盖本版
// （parcel-shipment 的采用判断按版本幂等），所以这里绝不能复用上一次的号。
func (factory *ResultVersions) NextPickupResultVersion(
	ctx context.Context,
) (domain.PickupResultVersion, error) {
	sequence, err := factory.next(ctx, "transport_fulfillment.pickup_result_version_seq")
	if err != nil {
		return domain.PickupResultVersion{}, fmt.Errorf("next pickup result version: %w", err)
	}
	return domain.NewPickupResultVersion(fmt.Sprintf("%s%012d", pickupResultVersionPrefix, sequence))
}

// NextDeliveryResultVersion 为首登与 POD 更正各签一个新版本。更正版回指前版、原版本
// 不删，版本链因此要求两代号不同——同号会被 CorrectProof 在构造期直接拒掉。
func (factory *ResultVersions) NextDeliveryResultVersion(
	ctx context.Context,
) (domain.DeliveryResultVersion, error) {
	sequence, err := factory.next(ctx, "transport_fulfillment.delivery_result_version_seq")
	if err != nil {
		return domain.DeliveryResultVersion{}, fmt.Errorf("next delivery result version: %w", err)
	}
	return domain.NewDeliveryResultVersion(fmt.Sprintf("%s%012d", deliveryResultVersionPrefix, sequence))
}

// next 推进一条序列。序列名是包内常量拼进 SQL 而不是参数：nextval 的参数是 regclass，
// 用占位符传字符串会让它变成一次按名查找，而序列名在这里本来就不该来自外部。
//
// 走 RequireExecutor：nextval 推进序列状态，是写不是读——发到读副本上推进的是另一份
// 状态，两边各自从 1 开始，版本号就会撞。
func (factory *ResultVersions) next(ctx context.Context, sequence string) (int64, error) {
	executor, err := factory.db.RequireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var value int64
	if err := executor.QueryRow(ctx, `SELECT nextval('`+sequence+`')`).Scan(&value); err != nil {
		return 0, err
	}
	return value, nil
}
