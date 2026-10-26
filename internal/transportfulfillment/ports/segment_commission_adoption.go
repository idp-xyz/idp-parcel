package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 履约段采用运输委托这本册的存取口（ADR-0204）。另立一个文件、不并进 ports.go：ports.go 里的接口有别的包写的替身实现，
// 往那些接口上加方法，替身就编不过（tf-chain-plan「跨票通则」X4）；新口放新文件，谁也不必跟着改。

// SegmentCommissionAdoptionKey 是采用连线的幂等键：一段此刻至多一份当前采用，所以键只到段。解除之后的再采用是这一段的下一代
// （ADR-0232 决定一），代次由册在落行时排，不在键上：登记方说的是「这一段采用哪一份」，不是「第几次采用」。
type SegmentCommissionAdoptionKey struct {
	TenantID domain.TenantID
	Segment  domain.FulfillmentSegmentReference
}

// SegmentCommissionAdoptionRecord 是一次采用越过提交边界留下的东西。ContentDigest 收登记方声明的那一对（委托、服务方），
// 同段再来时据它分已有与冲突。
type SegmentCommissionAdoptionRecord struct {
	Key           SegmentCommissionAdoptionKey
	ContentDigest string
	Adoption      domain.SegmentCommissionAdoption
	RecordedAt    time.Time
	// Generation 是读回时这一段的第几代采用，由册在 Save 时排定；Save 不读它。解除据它指名解的是哪一代。
	Generation int
	// AdoptedBy 是经运营决定口作出这一代采用的操作者（ADR-0303 复核记录），只进审计：重放与冲突只比 ContentDigest，采用的各道门
	// 读不到它。每一代各记各的；解除另有解除人。可缺。
	AdoptedBy domain.RegistrantReference
}

type AdoptionSaveOutcome uint8

const (
	AdoptionSaveOutcomeInvalid AdoptionSaveOutcome = iota
	AdoptionSaved
	AdoptionAlreadyRegistered
)

// SegmentCommissionAdoptionStore 按段找回当前采用并保存新一代采用（写入代数同 ADR-0031）。没有改、没有删：解除另成一份事实、记在
// 解除册上（SegmentCommissionAdoptionWithdrawalStore，ADR-0232 决定一），采用那一行原样留着。
//
// FindBySegment 也供运输收费发生项登记按段回读采购上下文：段已采用委托时，服务方与协议从那份委托的快照取（登记只读，
// 经 SegmentCommissionAdoptionView；ADR-0207 文末补记）。
type SegmentCommissionAdoptionStore interface {
	// FindBySegment 交这一段此刻的当前采用：最后一代、且没有解除。已解除的各代不是当前采用，读口上不出现。
	FindBySegment(ctx context.Context, key SegmentCommissionAdoptionKey) (SegmentCommissionAdoptionRecord, bool, error)
	// Save 落这一段的下一代采用。这一段还有当前采用（或另一笔并发首交先落了同一代）答 AdoptionAlreadyRegistered，不覆盖先到者，
	// 由编排读回再按摘要分已有与冲突。
	Save(ctx context.Context, record SegmentCommissionAdoptionRecord) (AdoptionSaveOutcome, error)
}
