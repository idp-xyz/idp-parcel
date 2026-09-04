package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件是证据项与证据披露版本的端口（`UC-VE-007` 步 2「保全……材料，形成证据项并记录
// 版本、冲突、有效性及可披露范围」）。证据项是被引的本体：异常案件、客户索赔项与追偿
// 事项各在自己身上引用它（CONTEXT「同一证据项可以被……分别引用」），这里不带任何案件
// 或索赔键——带了就把「谁引用它」写进了本体，受控复用（`AT-VE-147`）就成了复制。
//
// 它与材料归集面（MaterialReceiptRegistry / ClaimEvidenceView）是两回事：那边登的是
// 「某项索赔的某件要求材料收讫了」，键是资格目录签发的材料要求引用，供最低材料维作差；
// 这边登的是材料本体的证据评价与对外披露版本。合并会让「收讫」被读成「采信」。

type EvidenceSaveOutcome uint8

const (
	EvidenceSaveOutcomeInvalid EvidenceSaveOutcome = iota
	EvidenceSaved
	EvidenceAlreadyRecorded
)

// EvidenceStore 保存证据项与它的披露版本。租户是最高数据隔离边界（ADR-0003），跨越它
// 必须在签名上看得见。
//
// 证据项的幂等界线是（提供方 + 内容指纹）：同一提供方再次提交同一份材料是同一证据项
// （FindByProviderDigest 据此短路），另一提供方提交同样的字节是另一项——证据项保存来源
// 与提供方（CONTEXT），来源不同就不是同一份证据。Save 撞该唯一约束交回 AlreadyRecorded
// （ADR-0031 写入代数，事务保持可用）。
//
// 披露版本按（证据项 + 脱敏指纹 + 披露范围）成行，只增不改：一个版本是「明确披露范围
// + 脱敏版本」这一对，同一份脱敏内容对两个相对方是两个版本；FindDisclosure 按这三维答
// 已有与否。读回经 PrepareDisclosure 重过构造门（脱敏指纹不得与原件相同），一次坏写入
// 不得变成一份看起来合法的披露版本。
type EvidenceStore interface {
	FindByID(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.EvidenceItemID,
	) (domain.EvidenceItem, bool, error)
	FindByProviderDigest(
		ctx context.Context,
		tenant domain.TenantID,
		provider domain.EvidenceProviderReference,
		digest domain.EvidenceContentDigest,
	) (domain.EvidenceItem, bool, error)
	Save(ctx context.Context, tenant domain.TenantID, item domain.EvidenceItem) (EvidenceSaveOutcome, error)
	FindDisclosure(
		ctx context.Context,
		tenant domain.TenantID,
		item domain.EvidenceItemID,
		redacted domain.EvidenceContentDigest,
		scope string,
	) (domain.EvidenceDisclosureVersion, bool, error)
	SaveDisclosure(
		ctx context.Context,
		tenant domain.TenantID,
		version domain.EvidenceDisclosureVersion,
	) (EvidenceSaveOutcome, error)
}

// EvidenceIdentityFactory 签发证据项标识。与其余身份工厂分开，理由相同：证据到达由
// 它自己的入口触发，合并会让一个编排依赖它根本不签发的身份。
type EvidenceIdentityFactory interface {
	NextEvidenceItemID(ctx context.Context) (domain.EvidenceItemID, error)
}
