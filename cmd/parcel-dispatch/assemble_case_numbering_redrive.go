package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// caseNumberingRedrivePageSize 是每一拍从案件撞号册取多少件到期的命中再试。它不是业务阈值：撞号要等人换格式，一拍处理不完
// 的下一拍接着来；页小一点，一拍不会因为撞号册积了一大堆而拖住信封派发。
const caseNumberingRedrivePageSize = 20

// caseNumberingRedriveBeat 在每一拍派发之后把案件撞号册上到期的命中原样再交一次分诊（CONTEXT「异常案件」：撞号的那次命中
// 留待规则册换格式后再分诊；UC-VE-004 交接）。
//
// 恢复放在这里而不是信封重投上：派发框架每认领一次就记一次尝试，到 MaxAttempts 即落 ABANDONED，没有让一条信封不耗预算地
// 原样重投的口，也没有把 ABANDONED 的信封重新入队的口。撞号要等人换格式，重投多少次都一样——让信封回滚重投，只会占着分区
// 烧完预算、连同命中一起丢掉。所以撞号那一拍信封入账、命中记进撞号册，这里按到期时刻定时再试，与按截止日截单
// （cycleCutBeat）同一种做法。
//
// 同一页可能被并存的另一个派发进程同时取到：Redrive 先抢册上那一行（SKIP LOCKED），抢不到答跳过，只有一边真去分诊。
//
// 仍撞的那几件作为本拍的错误交给循环记日志，单列一类（veapplication.ErrCaseNumberingRuleCollision）：看日志的人要去
// 「设置 → 单据编号」换格式，不是去查依赖或传输。每件按 veapplication.CaseNumberingRedriveInterval 推后，日志不会每拍刷屏。
type caseNumberingRedriveBeat struct {
	inner      Beat
	derive     *tenantBoundProjectionDerive
	redrive    *veapplication.CaseNumberingRedrive
	transactor bentoapp.Transactor
}

func (beat caseNumberingRedriveBeat) DispatchOnce(ctx context.Context) (int, error) {
	published, err := beat.inner.DispatchOnce(ctx)
	return published, errors.Join(err, beat.redriveDue(ctx))
}

func (beat caseNumberingRedriveBeat) redriveDue(ctx context.Context) error {
	due, err := beat.redrive.Due(ctx, caseNumberingRedrivePageSize)
	if err != nil {
		return fmt.Errorf("case numbering redrive: %w", err)
	}
	var failures []error
	for _, collision := range due {
		hit := collision.Hit
		triage, err := vepostgres.NewTriageRules(beat.derive.db, collision.Tenant)
		if err != nil {
			failures = append(failures, fmt.Errorf("case numbering redrive %s/%s: %w", collision.Tenant, hit.Parcel, err))
			continue
		}
		var outcome veapplication.CaseNumberingRedriveOutcome
		if err := beat.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var redriveErr error
			outcome, redriveErr = beat.redrive.Redrive(txCtx, beat.derive.signalsFor(triage), collision)
			return redriveErr
		}); err != nil {
			failures = append(failures, fmt.Errorf("case numbering redrive %s/%s: %w", collision.Tenant, hit.Parcel, err))
			// 那一笔已回滚、到期时刻没动：另开一笔推后，免得读不动的几件每拍占满页首（CaseNumberingRedriveInterval 的注释）。
			if deferErr := beat.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
				return beat.redrive.Defer(txCtx, collision.Key())
			}); deferErr != nil {
				failures = append(failures, fmt.Errorf("case numbering redrive %s/%s: defer: %w", collision.Tenant, hit.Parcel, deferErr))
			}
			continue
		}
		if outcome == veapplication.CaseNumberingStillColliding {
			failures = append(failures, fmt.Errorf(
				"%w: tenant %s, parcel %s, signal %s: format %q still renders exception case numbers already in use; "+
					"register another format under 设置 → 单据编号 (retried every %s)",
				veapplication.ErrCaseNumberingRuleCollision, collision.Tenant, hit.Parcel, hit.Kind, collision.Template,
				veapplication.CaseNumberingRedriveInterval))
		}
	}
	return errors.Join(failures...)
}
