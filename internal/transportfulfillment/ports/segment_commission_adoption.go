package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 履约段采用运输委托这本册的存取口（ADR-0204）。另立一个文件、不并进 ports.go：ports.go 里的接口有别的包写的替身实现，
// 往那些接口上加方法，替身就编不过（tf-chain-plan「跨票通则」X4）；新口放新文件，谁也不必跟着改。

// SegmentCommissionAdoptionKey 是采用连线的幂等键：一段至多采用一份委托，所以键只到段。
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
}

type AdoptionSaveOutcome uint8

const (
	AdoptionSaveOutcomeInvalid AdoptionSaveOutcome = iota
	AdoptionSaved
	AdoptionAlreadyRegistered
)

// SegmentCommissionAdoptionStore 按段找回并保存采用连线（写入代数同 ADR-0031）。没有改、没有删：采用之后不改、不解除
// （ADR-0204 越权风险点「没有反向解除采用的面」），这个口上就表达不出改与删。
//
// FindBySegment 也供运输收费发生项登记按段回读采购上下文：段已采用委托时，服务方与协议从那份委托的快照取（登记只读，
// 经 SegmentCommissionAdoptionView；ADR-0207 文末补记）。
type SegmentCommissionAdoptionStore interface {
	FindBySegment(ctx context.Context, key SegmentCommissionAdoptionKey) (SegmentCommissionAdoptionRecord, bool, error)
	// Save 撞上同一段已有的采用答 AdoptionAlreadyRegistered，不覆盖先到者，由编排读回再按摘要分已有与冲突。
	Save(ctx context.Context, record SegmentCommissionAdoptionRecord) (AdoptionSaveOutcome, error)
}
