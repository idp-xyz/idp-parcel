package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

// 本文件是代收分户账查阅的伴生列表读端口(ADR-0077,票 admin-remainder-mechanism-batch/04):
// 管理台 cod-ledger 页的供数面。读口不与记账写入方共接口——扩写侧接口会拆全部写侧
// 测试替身,伴生读端口另立(判据同 partycommercial ServiceProductCatalogueRead 那句)。
//
// 上列的是**检索列面**的照实转写:分户账开立面照登记字段转写,六位置余额按记账派生
// ——迁移 0001 自注点名「各位置余额由 subledger_posting 派生、按键取记账是本表唯一的
// 读法」,派生是求和不是判断;回汇批次照册转写,批次成员不展开(成员就是引用批次的
// 汇付记账,批次不另持第二份成员清单)。目录上列不重建领域对象、不形成判断。
//
// 租户在方法签名上(ADR-0077 Decision 五);Limit 必须为正,每页多大由接入面按渠道
// 契约裁决,读口只拒绝无意义的取值;空登记册如实交回空列表(Decision 四:空册本身
// 就是内容,续办是登记责任方去登记口开立,不折成未配置)。

// CodSubledgerPositionBalances 是一本分户账六个资金位置的派生余额,币种最小单位。
// 余额=去向侧入账之和−来源侧出账之和;写口守余额非负,读口照实转写——旁路写入
// 造出的负余额原样透出,不钳位:钳掉的那截正是要人去查的证据。
type CodSubledgerPositionBalances struct {
	InTransitAtChannelMinor int64
	AwaitingAllocationMinor int64
	PayableToCustomerMinor  int64
	RemittedMinor           int64
	ShortfallMinor          int64
	SurplusMinor            int64
}

// CodSubledgerRemittanceBatch 是引用某本分户账的一个回汇批次:标识、单向状态
// (COLLECTED / HANDED_FOR_PAYMENT)、归集截点与形成时刻,照册转写。
type CodSubledgerRemittanceBatch struct {
	Batch            string
	State            string
	CollectedThrough time.Time
	FormedAt         time.Time
}

// CodSubledgerCatalogueRow 是代收分户账目录上列的一行:四维键(货主客户、责任法人、
// 币种、代收渠道)、保管依据引用、开立时间、六位置派生余额、记账笔数与引用本账的
// 回汇批次。
//
// Batches 空切片即「尚无批次」——回汇周期与汇付通道属实例半边,未配置时如实为空,
// 不代填;「已开立但当期无记账」的账以全零余额在场,与「未开立」(整行不在列)是
// 两个相反的答案(迁移 0001 开立面自注)。PostingCount 把「从未记账」与「记账相抵
// 为零」分开——两态在六个零余额上长着同一张脸,笔数是唯一分得开它们的照实转写
// (语义同 domain.SubledgerBalance 的记账笔数)。
type CodSubledgerCatalogueRow struct {
	Customer     string
	LegalEntity  string
	Currency     string
	Channel      string
	CustodyBasis string
	OpenedAt     time.Time
	Balances     CodSubledgerPositionBalances
	PostingCount int64
	Batches      []CodSubledgerRemittanceBatch
}

// CodSubledgerCatalogueRead 是代收分户账的伴生列表读端口。
type CodSubledgerCatalogueRead interface {
	ListCodSubledgers(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]CodSubledgerCatalogueRow, error)
}
