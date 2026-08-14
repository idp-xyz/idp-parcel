// Package identity 为 node-operations 签发标识。它不碰数据库，也不碰领域规则：
// 签发要回答的只有一件事——交出一个此前从未交出过的标识。
package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// intakeResultVersionPrefix 让一个收寄结果版本在日志与库行里一眼认得出来源，形状与
// customs-compliance 的签发面对齐（`CC-CASE-` / `CC-SUBV-`）。它不参与唯一性，唯一性
// 全在随机那一段。
const intakeResultVersionPrefix = "NO-INTAKEV-"

// intakeResultVersionBytes 是随机段的字节数。128 位使重复在实际签发量下不可达，而
// 收寄结果版本要跨上下文当幂等键用（parcel-shipment 的采用判断按它幂等）——重号一次
// 就会让两次收寄在下游被当成同一次。
const intakeResultVersionBytes = 16

// IntakeResultVersions 实现 ports.IntakeIdentityFactory。
//
// 取随机而不取数据库序列，有两条理由。其一，编排把签发失败当`收寄待确认`处置，而
// 序列会把签发绑到数据库可用性上——库一忙，一次本可以完成的收寄就停在未决。其二，
// 收寄结果版本不需要顺序：更正形成新版本靠回指连成链，不靠号大小；给它一个递增号
// 反而会让读的人以为号次序就是业务次序。
type IntakeResultVersions struct {
	entropy io.Reader
}

func NewIntakeResultVersions() *IntakeResultVersions {
	return &IntakeResultVersions{entropy: rand.Reader}
}

// NewIntakeResultVersionsFrom 用指定熵源构造。它存在只为让门禁用例喂一个必然失败的
// 读者——熵源读不出来时签发必须报错，绝不能退化成一个可预测的值。
func NewIntakeResultVersionsFrom(entropy io.Reader) (*IntakeResultVersions, error) {
	if entropy == nil {
		return nil, fmt.Errorf("node operations identity: entropy reader is nil")
	}
	return &IntakeResultVersions{entropy: entropy}, nil
}

var _ ports.IntakeIdentityFactory = (*IntakeResultVersions)(nil)

// NextIntakeResultVersion 签发一个新的收寄结果版本。
//
// 先看 ctx：调用方已经放弃时不该再签发一个没人会用的标识——它会出现在日志里，看起来
// 像一次发生过的收寄。熵源读不满同样报错而不是用读到的半截凑——半截随机是可预测的。
func (factory *IntakeResultVersions) NextIntakeResultVersion(
	ctx context.Context,
) (domain.IntakeResultVersion, error) {
	if err := ctx.Err(); err != nil {
		return domain.IntakeResultVersion{}, fmt.Errorf("next intake result version: %w", err)
	}

	raw := make([]byte, intakeResultVersionBytes)
	if _, err := io.ReadFull(factory.entropy, raw); err != nil {
		return domain.IntakeResultVersion{}, fmt.Errorf("next intake result version: %w", err)
	}
	return domain.NewIntakeResultVersion(intakeResultVersionPrefix + hex.EncodeToString(raw))
}
