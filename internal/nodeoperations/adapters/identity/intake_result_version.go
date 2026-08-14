// Package identity 为 node-operations 签发标识。它不碰数据库，也不碰领域规则：
// 签发要回答的只有一件事——交出一个此前从未交出过的标识。
package identity

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"io"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// intakeResultVersionPrefix 让一个收寄结果版本在日志与库行里一眼认得出来源。它不参与
// 唯一性，唯一性全在随机那一段；横线加在拼接处而不写进常量，取值由本上下文自定但形状
// 全仓统一（MCP-1 的签发面裁定第四条）。
const intakeResultVersionPrefix = "INTAKEV"

// intakeResultVersionBytes 是随机段的字节数。128 位使重复在实际签发量下不可达，而
// 收寄结果版本要跨上下文当幂等键用（parcel-shipment 的采用判断按它幂等）——重号一次
// 就会让两次收寄在下游被当成同一次。
const intakeResultVersionBytes = 16

// versionEncoding 是不带填充的大写 base32。选它不选十六进制，是因为 RFC 4648 的字母表
// 是 A-Z 与 2-7：不含 0、1、8、9，于是 0 与 O、1 与 I 不可能混——照着工单念一个标识时
// 怕的正是这个，而十六进制的 0/O 恰好撞上。26 字符也比十六进制的 32 短。
var versionEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

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
// 读者——熵源读不出来时签发必须报错，绝不能退化成一个可预测的值，而那一支拿真熵源
// 逼不出来。
func NewIntakeResultVersionsFrom(entropy io.Reader) (*IntakeResultVersions, error) {
	if entropy == nil {
		return nil, fmt.Errorf("node operations identity: entropy reader is nil")
	}
	return &IntakeResultVersions{entropy: entropy}, nil
}

var _ ports.IntakeIdentityFactory = (*IntakeResultVersions)(nil)

// NextIntakeResultVersion 签发一个新的收寄结果版本。
//
// 不看 ctx：本实现只读本机熵源，没有一处可取消的等待，查一次 ctx 只会让读的人以为
// 这里会阻塞。端口签名上留着 ctx 是给库序列那一类实现的（MCP-1 的签发面裁定第二条）。
//
// 熵源读不满报错而不是用读到的半截凑——半截随机是可预测的。
func (factory *IntakeResultVersions) NextIntakeResultVersion(
	_ context.Context,
) (domain.IntakeResultVersion, error) {
	raw := make([]byte, intakeResultVersionBytes)
	if _, err := io.ReadFull(factory.entropy, raw); err != nil {
		return domain.IntakeResultVersion{}, fmt.Errorf("next intake result version: %w", err)
	}
	return domain.NewIntakeResultVersion(
		intakeResultVersionPrefix + "-" + versionEncoding.EncodeToString(raw))
}
