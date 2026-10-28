import { X } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import type { PartyRelationshipRecord } from './api';
import { labelOf, partyRoleLabels, relationshipStatusLabels, updateOrdinalLabel } from './presentation';
import { TechnicalDetails } from './TechnicalDetails';
import { PartyRelationshipBanners, PartyRelationshipMoreMenu } from './BusinessPartyHeaderParts';
import { PartyRelationshipEndActions } from './PartyRelationshipEndActions';
import { DetailRow, Fact, FactStrip, Instant, UnknownPartyName, statusBadge } from './detail-primitives';
import { useMayWriteRegistry } from '../use-operator-grants';

/**
 * 业务参与方工作台关系册的详情栏（票 business-parties-uiux/01）：头部（标识 + 角色 / 状态徽章 + 双方一行 + 引用签）·
 * 「更多」菜单 + 关闭 · 横幅 · 撤销 / 到期动作 · 事实条 · 各格。没有生命周期条：关系状态是登记进来的事实（候选、生效、
 * 到期、撤销、替代），不是按时点导出的一条线，画成主线会把三种终局读成先后。
 *
 * 一个事实只出现一处（票 group-legal-entities-uiux/06）：更新次数、有效区间两端与登记时间归事实条，角色与状态归徽章，终止时点
 * 归横幅；标题用一句人话说这段关系（「甲 是 乙 的客户」），标识、适用范围、依据、后继与租户收进底部的「技术详情」。
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
  // 没有登记册配置写授予的操作者不摆登记与发布入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`参与方关系 ${row.relationshipId}`}>
      <div className="space-y-3 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate text-[16px] font-semibold text-idpxyz-textBright">
                {row.holderNameKnown ? row.holderName : <UnknownPartyName />}
              </h2>
              <span className="rounded bg-idpxyz-hover px-1.5 py-0.5 text-[12px] font-medium text-idpxyz-breadcrumb">
                {labelOf(partyRoleLabels, row.role)}
              </span>
              {statusBadge(relationshipStatusLabels, row.status)}
            </div>
            {/* 方向由「持有方对相对方持有该角色」表达（CONTEXT）；给人读就是一句「甲是乙的客户」，不另起方向格。 */}
            <p className="mt-0.5 text-[13px] text-idpxyz-text">
              是 {row.counterpartyNameKnown ? row.counterpartyName : <UnknownPartyName />} 的{labelOf(partyRoleLabels, row.role)}
            </p>
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
        <Fact label="变更">{updateOrdinalLabel(row.revision)}</Fact>
        <Fact label="开始">
          <Instant value={row.effectiveStartsAt} />
        </Fact>
        {/* 无终点说「长期有效」：有效期间未声明终点即持续有效（CONTEXT 口径），不是「未知」。 */}
        <Fact label="结束">{row.effectiveEndsAt ? <Instant value={row.effectiveEndsAt} /> : '长期有效'}</Fact>
        <Fact label="登记时间">
          <Instant value={row.registeredAt} />
        </Fact>
      </FactStrip>

      <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
        <dl>
          <DetailRow label={labelOf(partyRoleLabels, row.role)}>{row.holderNameKnown ? row.holderName : <UnknownPartyName />}</DetailRow>
          <DetailRow label="对方">{row.counterpartyNameKnown ? row.counterpartyName : <UnknownPartyName />}</DetailRow>
          {row.endBasis ? (
            <DetailRow label="终止的来源单据" mono>
              {row.endBasis}
            </DetailRow>
          ) : null}
        </dl>
        <TechnicalDetails
          className="mt-4"
          rows={[
            { label: '关系标识', value: row.relationshipId },
            { label: '当前修订号', value: String(row.revision) },
            { label: '持有方标识', value: row.holderId },
            { label: '相对方标识', value: row.counterpartyId },
            { label: '适用范围', value: row.scope },
            { label: '登记依据', value: row.basis },
            { label: '后继关系', value: row.successorId },
            { label: '租户', value: row.tenantId },
          ]}
        />
      </div>
    </div>
  );
}
