import { X } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import type { PartyRelationshipRecord } from './api';
import { labelOf, partyRoleLabels, relationshipStatusLabels } from './presentation';
import { PartyRelationshipBanners, PartyRelationshipMoreMenu } from './BusinessPartyHeaderParts';
import { PartyRelationshipEndActions } from './PartyRelationshipEndActions';
import {
  DetailRow,
  Fact,
  FactStrip,
  Instant,
  RefChip,
  UnknownPartyName,
  statusBadge,
  useCopyToClipboard,
} from './detail-primitives';
import { useMayWriteRegistry } from '../use-operator-grants';

/**
 * 业务参与方工作台关系册的详情栏（票 business-parties-uiux/01）：头部（标识 + 角色 / 状态徽章 + 双方一行 + 引用签）·
 * 「更多」菜单 + 关闭 · 横幅 · 撤销 / 到期动作 · 事实条 · 各格。没有生命周期条：关系状态是登记进来的事实（候选、生效、
 * 到期、撤销、替代），不是按时点导出的一条线，画成主线会把三种终局读成先后。
 *
 * 一个事实只出现一处（票 group-legal-entities-uiux/06）：修订号、有效区间两端与登记时间归事实条，适用范围与依据归引用签，
 * 角色与状态归徽章，终止时点归横幅；各格只留双方（标识带复制、名称单列）、终止依据、后继与租户。
 *
 * 撤销 / 到期动作（PartyRelationshipEndActions）在栏内、按写授予收起（ADR-0315）；答 `ENDED` 时页面重取关系册——重取保留旧答案
 * （register-list.ts），栏不关，这一行就地换成终止后的那一笔。
 */
export function PartyRelationshipDetailPanel({
  row,
  onClose,
  onEnded,
}: {
  row: PartyRelationshipRecord;
  onClose: () => void;
  onEnded: () => void;
}) {
  const copy = useCopyToClipboard();
  // 没有登记册配置写授予的操作者不摆登记与发布入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`参与方关系 ${row.relationshipId}`}>
      <div className="space-y-3 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate font-mono text-[16px] font-semibold text-idpxyz-textBright">{row.relationshipId}</h2>
              <span className="rounded bg-idpxyz-hover px-1.5 py-0.5 text-[12px] font-medium text-idpxyz-breadcrumb">
                {labelOf(partyRoleLabels, row.role)}
              </span>
              {statusBadge(relationshipStatusLabels, row.status)}
            </div>
            {/* 方向由持有方 → 相对方的次序表达（CONTEXT：方向由「哪一方对哪一方持有该角色」表达），不另起一格。 */}
            <p className="mt-0.5 text-[13px] text-idpxyz-text">
              <span className="text-idpxyz-breadcrumb">持有方 </span>
              {row.holderNameKnown ? row.holderName : <UnknownPartyName />}
              <span className="text-idpxyz-breadcrumb"> → 相对方 </span>
              {row.counterpartyNameKnown ? row.counterpartyName : <UnknownPartyName />}
            </p>
            <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
              <RefChip label="范围" value={row.scope} title="适用范围" onCopy={copy} />
              <RefChip label="依据" value={row.basis} title="登记依据" onCopy={copy} />
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            <PartyRelationshipMoreMenu row={row} />
            <Button
              className="text-idpxyz-breadcrumb"
              variant="ghost"
              size="icon"
              onClick={onClose}
              aria-label="关闭详情"
              title="关闭（Esc）"
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </Button>
          </div>
        </div>

        <PartyRelationshipBanners row={row} />

        {mayWrite ? <PartyRelationshipEndActions key={row.relationshipId} row={row} onEnded={onEnded} /> : null}
      </div>

      <FactStrip>
        <Fact label="当前修订">r{row.revision}</Fact>
        <Fact label="生效起">
          <Instant value={row.effectiveStartsAt} />
        </Fact>
        {/* 无终点说「持续有效」：有效期间未声明终点即持续有效（CONTEXT 口径），不是「未知」。 */}
        <Fact label="生效止">{row.effectiveEndsAt ? <Instant value={row.effectiveEndsAt} /> : '持续有效'}</Fact>
        <Fact label="登记时间">
          <Instant value={row.registeredAt} />
        </Fact>
      </FactStrip>

      <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
        <dl>
          <DetailRow label="持有方" mono onCopy={() => copy('持有方', row.holderId)}>
            {row.holderId}
          </DetailRow>
          <DetailRow label="持有方名称">{row.holderNameKnown ? row.holderName : <UnknownPartyName />}</DetailRow>
          <DetailRow label="相对方" mono onCopy={() => copy('相对方', row.counterpartyId)}>
            {row.counterpartyId}
          </DetailRow>
          <DetailRow label="相对方名称">{row.counterpartyNameKnown ? row.counterpartyName : <UnknownPartyName />}</DetailRow>
          <DetailRow label="终止依据" mono>
            {row.endBasis ?? <span className="text-idpxyz-breadcrumb">—</span>}
          </DetailRow>
          <DetailRow label="后继" mono>
            {row.successorId ?? <span className="text-idpxyz-breadcrumb">—</span>}
          </DetailRow>
          <DetailRow label="租户" mono>
            {row.tenantId}
          </DetailRow>
        </dl>
      </div>
    </div>
  );
}
