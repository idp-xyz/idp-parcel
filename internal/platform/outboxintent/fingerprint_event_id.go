package outboxintent

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"go.idp.xyz/idp-bento-go/eventing"
)

// FingerprintEventID 铸定长的信封 ID：口名前缀 + "/" + 各维以 \x00 拼接后的 sha256 十六进制。
//
// 信封 ID 的上限是 eventing.MaxEventIDLength，而租户、范围、案件这类引用多长归实例半边、本仓给不出上界；把它们
// 原样拼进 ID 就是把上限押在别人的长度上——旧串接形（钉 `a0cb6fef`，票 sa-cc/19 作者 tip）下，`tenant-a` /
// `SYN-UNIT-RD` / `SYN-DUTY-RD/v1` / `bank-fact-2` 四个短合成引用加一段六十四位指纹就拼出 138 字节，被
// Envelope.Validate 确定性拒收。哈希把长度钉死在「前缀 + 六十四」，与任何一维多长无关；口名前缀让同一个来源名
// 下的几只口 ID 空间互不相交（EnqueueOnce 按（来源, 事件 ID）查重）；\x00 分隔让维度边界移位算不出同一个 ID。
// 代价是 ID 不再可读，运维从 ID 反查走载荷——载荷照旧全量带引用，分区键与 Subject 保留可读形。
//
// 先在 customs-compliance 的三只核对口落成同形（票 sa-cc/29），settlement-accounting 随后要用同一公式，按本包
// 头注的 rule-of-three 提炼至此（票 sa-cc/34 裁决 2）。
func FingerprintEventID(portName string, dimensions ...string) eventing.EventID {
	digest := sha256.Sum256([]byte(strings.Join(dimensions, "\x00")))
	return eventing.EventID(portName + "/" + hex.EncodeToString(digest[:]))
}
