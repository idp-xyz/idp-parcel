package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 实际承运商判断登记册与承运主体身份读口（tf-segment-lifecycle-closure/02，ADR-0103）。
//
// 另开一个文件而不是并进 ports.go 或 fulfillment_segment.go：判断不是段上的一格（ADR-0103 决定二），
// 端口也不挂在段登记册上——挂上去，「按段批量改承运商」那种口迟早会顺着同一个接口长出来。

// ActualCarrierJudgmentKey 是判断的键：（租户，实际履约段）。一段一份判断历史，没有对象维也没有版本维
// ——版本在聚合内部按序号追加，键上带版本会让「当前版是哪一版」变成调用方要自己算的事。
type ActualCarrierJudgmentKey struct {
	TenantID domain.TenantID
	Segment  domain.FulfillmentSegmentReference
}

// ActualCarrierJudgmentRecord 是一份判断连同它全部版本越过提交边界留下的东西。RecordedAt 是最近一次
// 落库的时刻，与版本上的业务时间、判断形成时间都不合并——三者三种归属。
type ActualCarrierJudgmentRecord struct {
	Key        ActualCarrierJudgmentKey
	Judgment   domain.ActualCarrierJudgment
	RecordedAt time.Time
}

// JudgmentOpenOutcome 是段成立时首登判断的结果。撞键是业务答案不是错误（ADR-0031）：两个对象并发进
// 同一个段时各自都会试着开一份，后到的读回赢家即可。
type JudgmentOpenOutcome uint8

const (
	JudgmentOpenOutcomeInvalid JudgmentOpenOutcome = iota
	JudgmentOpened
	JudgmentAlreadyOpened
)

func (outcome JudgmentOpenOutcome) String() string {
	switch outcome {
	case JudgmentOpened:
		return "OPENED"
	case JudgmentAlreadyOpened:
		return "ALREADY_OPENED"
	default:
		return ""
	}
}

// JudgmentVersionAppendOutcome 是追加一版的结果。`版本已在册`说的是同一序号已被另一次写入占去——两条
// 编排各自基于同一个当前版算出了同一个下一序号，后写的那一方要读回再来，不是错误也不是重放。
type JudgmentVersionAppendOutcome uint8

const (
	JudgmentVersionAppendOutcomeInvalid JudgmentVersionAppendOutcome = iota
	JudgmentVersionAppended
	JudgmentVersionAlreadyRecorded
)

func (outcome JudgmentVersionAppendOutcome) String() string {
	switch outcome {
	case JudgmentVersionAppended:
		return "APPENDED"
	case JudgmentVersionAlreadyRecorded:
		return "ALREADY_RECORDED"
	default:
		return ""
	}
}

// ActualCarrierJudgmentRegistry 按段找回判断的全部版本、首登、追加版本。**只插不改**：三个口没有一个
// 能改写既有版本，「不追溯覆盖原来的未知期间和判断历史」（CONTEXT）因此在接口上就表达不出反面。
//
// FindByKey 交回整份历史而不是只交当前版：当前版由聚合派生（序号最大者），转换门又要拿当前版的依据
// 去判重与派生——两条读路只会分叉。读回过重建门，坏行在适配器暴露而不是流到判断里。
//
// Open 首登：头行连同首版同笔落，撞键答`已开`。AppendVersion 只插指名的那一版，撞序号答`版本已在册`。
// 写口按框架合同要求环境事务：判断是段成立那一笔的一部分，半个判断（有头无首版）是领域产不出的东西。
type ActualCarrierJudgmentRegistry interface {
	FindByKey(ctx context.Context, key ActualCarrierJudgmentKey) (ActualCarrierJudgmentRecord, bool, error)
	Open(ctx context.Context, record ActualCarrierJudgmentRecord) (JudgmentOpenOutcome, error)
	AppendVersion(
		ctx context.Context,
		key ActualCarrierJudgmentKey,
		version domain.ActualCarrierJudgmentVersion,
		recordedAt time.Time,
	) (JudgmentVersionAppendOutcome, error)
}

// CarrierIdentityDirectory 答「这个承运主体身份在 party-commercial 上登了没有」——判断值只引用已登记的
// 参与方或运营法人身份，本上下文不铸（CONTEXT Boundaries；ADR-0103 决定六）。消费侧适配器落在
// internal/transportfulfillment/adapters/partycommercial/（ADR-0025）。
//
// 它只答存在性，不答生效与否也不答名称：身份的生命周期归 PC，本口多答一格就是在这里复制一份会过期的
// 推导。读取失败走 error，**不得折成 false**——把「读不到」读成「未登记」，一次故障就会变成一版
// 待确认（承运主体身份未登记），而两者的恢复动作不同（ADR-0029）。
type CarrierIdentityDirectory interface {
	IdentityRegistered(ctx context.Context, tenant domain.TenantID, subject domain.CarrierSubject) (bool, error)
}
