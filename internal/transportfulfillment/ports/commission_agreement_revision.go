package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 委托协议换版的存取口与它的门铃（ADR-0231 决定三、四）。另立文件、不进 ports.go，理由同 SegmentCommissionAdoptionStore 的文件头。
//
// 换版读回不另开口：委托按键读回时连同各次换版一起装回（TransportCommissionStore.FindByKey），事实按哪一版判只问委托
// （domain.TransportCommission.AgreementTermsAt）。

// CommissionAgreementRevisionRecord 是一次换版越过提交边界留下的东西：哪一份委托，换的是哪一版。
type CommissionAgreementRevisionRecord struct {
	Key      TransportCommissionKey
	Revision domain.CommissionAgreementRevision
}

type CommissionAgreementRevisionSaveOutcome uint8

const (
	CommissionAgreementRevisionSaveOutcomeInvalid CommissionAgreementRevisionSaveOutcome = iota
	CommissionAgreementRevisionSaved
	// CommissionAgreementRevisionAlreadyRegistered：同一份委托上已有这个号或这份依据。编排先取了换版锁、再读委托、再按「已有几版 + 1」
	// 铸号，正常走不到这一格；走到了是锁没守住，编排读回再判。
	CommissionAgreementRevisionAlreadyRegistered
)

// CommissionAgreementRevisionStore 是换版的写口，都要环境事务。
//
//   - HoldRevisions：在这一笔事务里占住这一份委托的换版，到事务结束才放。换版是「读委托、按已有几版铸号、查依据有没有换过、写」，
//     两笔并发的换版各自读到同一个版数就会撞号，同一份依据并发重放也会各自判「没换过」；占住之后后一笔在前一笔提交之后才读委托。
//     不写任何行，不锁委托行：委托行的取消、开始与前移各有自己的写口与并发闸，这把锁只给换版用。
//   - SaveRevision：落一行换版，只插不改。
type CommissionAgreementRevisionStore interface {
	HoldRevisions(ctx context.Context, key TransportCommissionKey) error
	SaveRevision(ctx context.Context, record CommissionAgreementRevisionRecord) (CommissionAgreementRevisionSaveOutcome, error)
}

// CommissionAgreementRevisedIntent 是「委托换了版」的门铃：信封只带（租户、委托、号），消费方按键回读。门铃的消费方是实际履约补形成
// （ADR-0231 决定五），它对采用这份委托的各段上已结束的参与再交一次登记用例。
type CommissionAgreementRevisedIntent struct {
	Record CommissionAgreementRevisionRecord
}

// CommissionAgreementRevisionHandoff 由 OutboxCommissionAgreementRevisionHandoff 实现：门铃与换版同一事务入队。两者分两笔，就会有一次
// 换版落了册而没有任何东西回头补形成。
type CommissionAgreementRevisionHandoff interface {
	HandOffCommissionAgreementRevision(ctx context.Context, intent CommissionAgreementRevisedIntent) error
}
