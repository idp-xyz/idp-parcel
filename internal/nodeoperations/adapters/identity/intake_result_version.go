// Package identity 为 node-operations 签发标识。签发内核共用 internal/platform/identity；
// 本包只决定两件属本上下文的事：前缀取什么值，以及签出的字符串交给哪个领域构造函数。
package identity

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
)

// intakeResultVersionPrefix 让一个收寄结果版本在日志与工单里一眼认得出是什么。它不参与
// 唯一性，也不带上下文名——领域侧各标识已是互不相通的 Go 类型，张冠李戴编译期就拦得住。
const intakeResultVersionPrefix = "INTAKEV"

// IntakeResultVersions 实现 ports.IntakeIdentityFactory。
//
// 它是本上下文唯一的签发器，但仍旧一个端口一个类型、不与将来别的签发口合并：合并会让
// 一个编排依赖它根本不签发的身份。内核共用不触及这条约束。
type IntakeResultVersions struct {
	minter platformidentity.Minter
}

func NewIntakeResultVersions(options ...platformidentity.Option) (*IntakeResultVersions, error) {
	minter, err := platformidentity.NewMinter(intakeResultVersionPrefix, options...)
	if err != nil {
		return nil, fmt.Errorf("node operations identity: %w", err)
	}
	return &IntakeResultVersions{minter: minter}, nil
}

var _ ports.IntakeIdentityFactory = (*IntakeResultVersions)(nil)

// NextIntakeResultVersion 签发一个新的收寄结果版本。
//
// 不看 ctx：内核只读本机熵源，没有一处可取消的等待。端口签名上留着 ctx 是给库序列那一类
// 实现的，本实现如实不用。
//
// 签不出号时如实报错——编排为此留了`收寄待确认`那一格，拿一个可预测的值顶上会让两次收寄
// 在 parcel-shipment 的采用判断里被当成同一次（那一步正按本版本幂等）。
func (factory *IntakeResultVersions) NextIntakeResultVersion(
	_ context.Context,
) (domain.IntakeResultVersion, error) {
	minted, err := factory.minter.Next()
	if err != nil {
		return domain.IntakeResultVersion{}, fmt.Errorf("next intake result version: %w", err)
	}
	return domain.NewIntakeResultVersion(minted)
}
