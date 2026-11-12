import { X } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import type { PartyRelationshipRecord } from './api';
import { labelOf, partyRoleLabels, updateOrdinalLabel } from './presentation';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { StatusPill } from '../../components/StatusPill';
import { PartyRelationshipBanners, PartyRelationshipMoreMenu } from './BusinessPartyHeaderParts';
import { PartyRelationshipEndActions } from './PartyRelationshipEndActions';
import { DetailRow, Fact, Instant, UnknownPartyName } from './detail-primitives';
import { PartyRoleTag, PartyTile, partyRoleTileKind } from './party-role-visuals';
import { useMayWriteRegistry } from '../use-operator-grants';

/**
 * 业务参与方工作台关系册的详情栏（票 business-parties-uiux/01；外框自票 business-party-redesign/04）：头区第一排是图标块 ·
 * 一句人话的标题（持有方 + 角色标签 + 状态徽章，下一行「是 乙 的客户」）· 「更多」菜单 + 关闭；下面一行事实，再下面是横幅与
 * 撤销 / 到期动作；正文是各格与技术详情。没有生命周期条：关系状态是登记进来的事实（候选、生效、到期、撤销、替代），不是按
 * 时点导出的一条线，画成主线会把三种终局读成先后。
 *
 * 头区与往来单位右栏、「我方关联」右栏同一种排法（BusinessPartyDetailPanel.tsx）：三个视图的右栏一眼看得出是同一页的东西。
 * 图标块按这段关系的角色取——一段关系说的就是「谁担任什么」，而持有方自己是什么类型要别的册才推得出，那几本册没读全时会推错。
 * 换的只是外框，栏里有哪几项与原来一样。
 *
 * 一个事实只出现一处（票 group-legal-entities-uiux/06）：更新次数、有效区间两端与登记时间归那一行事实，角色与状态归徽章，终止
 * 时点归横幅；标题用一句人话说这段关系（「甲 是 乙 的客户」），标识、适用范围、依据、后继与租户收进底部的「技术详情」。那一行
 * 事实里的时刻照旧显到秒：撤销与到期只影响边界之后的新决定，边界在哪一刻要看得见。
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
      <div className="space-y-3 border-b border-idpxyz-border px-6 pb-3.5 pt-4">
        {/* 名称那一块要 280 才排得开；这一排放不下它与动作时动作整组折到下一行、靠右，不去挤名称。 */}
        <div className="flex flex-wrap items-start gap-x-3.5 gap-y-2.5">
          <PartyTile large kind={partyRoleTileKind(row.role)} />
          <div className="min-w-0 flex-[1_1_280px]">
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
              <h2
                className="line-clamp-2 min-w-0 break-words type-display font-semibold leading-7 text-idpxyz-textBright"
                title={row.holderNameKnown ? row.holderName : undefined}
              >
                {row.holderNameKnown ? row.holderName : <UnknownPartyName />}
              </h2>
              <PartyRoleTag role={row.role} />
              <StatusPill code={row.status} />
            </div>
            {/* 方向由「持有方对相对方持有该角色」表达（CONTEXT）；给人读就是一句「甲是乙的客户」，不另起方向格。 */}
            <p className="mt-0.5 type-value text-idpxyz-text">
              是 {row.counterpartyNameKnown ? row.counterpartyName : <UnknownPartyName />} 的{labelOf(partyRoleLabels, row.role)}
            </p>
          </div>
          <div className="ml-auto flex shrink-0 items-center gap-1.5">
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

        {/* 一行事实，放不下才折行。无终点说「长期有效」：有效期间未声明终点即持续有效（CONTEXT 口径），不是「未知」。 */}
        <dl className="flex min-w-0 flex-wrap items-center gap-x-5 gap-y-1">
          <Fact inline label="变更">
            {updateOrdinalLabel(row.revision)}
          </Fact>
          <Fact inline label="开始">
            <Instant value={row.effectiveStartsAt} />
          </Fact>
          <Fact inline label="结束">
            {row.effectiveEndsAt ? <Instant value={row.effectiveEndsAt} /> : '长期有效'}
          </Fact>
          <Fact inline label="登记时间">
            <Instant value={row.registeredAt} />
          </Fact>
        </dl>

        <PartyRelationshipBanners row={row} />

        {mayWrite ? <PartyRelationshipEndActions key={row.relationshipId} row={row} onEnded={onEnded} /> : null}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-6 py-4">
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
            { label: '后继关系', value: row.successorId ?? '' },
            { label: '租户', value: row.tenantId },
          ]}
        />
      </div>
    </div>
  );
}
