import { useState } from 'react';
import { AlertTriangle, Ban, Copy, ExternalLink, MoreHorizontal, PencilLine } from 'lucide-react';
import { Button, DropdownMenu, MenuItem, MenuSeparator } from '@idpxyz/ui-primitives';
import { statusToneBanner, statusToneText, type StatusTone } from '../../components/status-tone';
import type { BusinessPartyRecord, PartyRelationshipRecord, BusinessPartyListResponseBody } from './api';
import { businessPartyObjectHash } from './business-parties-address';
import { labelOf, partyNameUnknownNote, partyRoleLabels, partyTypeFallbackLabel, relationshipStatusLabels } from './presentation';
import { ourCompanyLabel } from './business-party-list';
import { partyRelationshipNeedsAttention } from './party-relationship-list';
import { Instant, useCopyToClipboard } from './detail-primitives';
import { useMayWriteRegistry } from '../use-operator-grants';
import type { ApiResult } from '../catalogue-api';
import { BusinessPartyDeactivationDialog } from './BusinessPartyDeactivationDialog';
import { identityMayBeDeactivated } from './identity-deactivation-form';

// 业务参与方页两册头区的件：类型标签、横幅与「更多」菜单。身份册的详情栏与对象页各摆一份同样的头区（照 LegalEntityHeaderParts.tsx，
// 票 group-legal-entities-uiux/09：同一对象两处结构一致），件只在这里写一遍，两处各自排版。不摆生命周期步进：状态药丸
// 已经说了在哪一格，停用另有时点，步进是第二遍（用户 2026-11-03 裁定，与公司主体两页同）。
// 横幅正文用正文色、语气只给描边与图标（理由在 components/status-tone.ts）。

/**
 * 类型标签，列表类型格与右栏头部同一份（票 business-party-ux/04），说法同 business-party-list.ts 的 businessPartyTypeLabel：
 * 我方公司一枚、排在前面，用提示语气的淡底与描边好与角色分开——强调色作字在画布上不足 4.5:1，所以字仍是正文色；各角色各一枚，
 * 中性灰底；两样都没有时是弱色字「暂无合作关系」，不做成标签——它说的是「没有」，不该和有的东西一样醒目。
 * `ours` 与 `roles` 取自读口在这一行上答的类别（business-party-list.ts 的 partyIsOurCompany、partyRolesOf）。
 */
export function BusinessPartyTypeTags({ ours, roles }: { ours: boolean; roles: readonly string[] }) {
  if (!ours && roles.length === 0) {
    return <span className="whitespace-nowrap type-body text-idpxyz-breadcrumb">{partyTypeFallbackLabel}</span>;
  }
  return (
    <span className="flex flex-wrap items-center gap-1">
      {ours ? (
        <span className={`whitespace-nowrap rounded border px-1.5 py-0.5 type-body font-medium ${statusToneBanner.info}`}>{ourCompanyLabel}</span>
      ) : null}
      {roles.map((role) => (
        <span key={role} className="whitespace-nowrap rounded bg-idpxyz-hover px-1.5 py-0.5 type-body font-medium text-idpxyz-text">
          {labelOf(partyRoleLabels, role)}
        </span>
      ))}
    </span>
  );
}

/** 这一行要不要摆横幅：停用了。参与方册是名称的来源，没有「待补」可报。 */
export function businessPartyHasBanners(row: BusinessPartyRecord): boolean {
  return row.deactivatedAt !== undefined;
}

/** 停用那一句：从哪一刻起、依据哪张单据。对象页的横幅与工作台右栏概要上方的提示条各套各的框，话只在这里写一遍。 */
export function BusinessPartyDeactivationNote({ deactivatedAt, deactivationBasis }: { deactivatedAt: string; deactivationBasis?: string }) {
  return (
    <span>
      自 <Instant value={deactivatedAt} /> 起停用，新的业务不再找它
      {deactivationBasis ? ` — 来源单据 ${deactivationBasis}` : ''}
    </span>
  );
}

export function BusinessPartyBanners({ row }: { row: BusinessPartyRecord }) {
  if (!row.deactivatedAt) return null;
  return (
    <div className={`flex items-center gap-2 rounded border px-2.5 py-1.5 type-body ${statusToneBanner.danger}`}>
      <Ban className={`h-3.5 w-3.5 shrink-0 ${statusToneText.danger}`} aria-hidden="true" />
      <BusinessPartyDeactivationNote deactivatedAt={row.deactivatedAt} deactivationBasis={row.deactivationBasis} />
    </div>
  );
}

/**
 * 身份行的「更多」菜单。动作只摆本产品真有的：停用走本页的停用表单（一个命令带种类，预填种类与这个参与方的标识），
 * 「在新标签打开对象页」只在详情栏里有——对象页自己就是那张标签。详情栏把 onDeactivate 交进来，是为了让页面记下
 * 「这次是从列表压进登记视图的」（BusinessPartiesPage 的 pushedRegister）；对象页在另一张标签上，没有这一笔要记，直接
 * 改地址，壳层会切回列表那张标签。
 */
export function BusinessPartyMoreMenu({
  row,
  parties,
  liveRelationships = null,
  onOpenObjectPage,
  onCorrectIdentity,
  onChanged,
}: {
  row: BusinessPartyRecord;
  /** 页面持有的参与方列表答案，给停用对话框（候选与版本号建议）；首取回来之前为 null。 */
  parties: ApiResult<BusinessPartyListResponseBody> | null;
  /** 它名下在用的合作关系有几段，给停用对话框的说明；不知道就是 null。 */
  liveRelationships?: number | null;
  onOpenObjectPage?: () => void;
  /**
   * 去「更正登记信息」的编辑态。只有详情栏给：对象页的页头自己摆着这一枚按钮，菜单里不再重复。摆不摆照对象页那一枚的规则——
   * 有写授予就摆，我方公司也摆（改的是名称，注册号那几格表单自己不摆）。
   */
  onCorrectIdentity?: () => void;
  /** 停用落册后回调；页面借它重取参与方册让「已停用」立刻可见。 */
  onChanged?: () => void;
}) {
  const copy = useCopyToClipboard();
  // 没有登记册配置写授予的操作者不摆登记与发布入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  // 停用开在本页的对话框里（票 business-party-edit-mode/01），不再送去列表页登记视图的停用表单。
  const [deactivating, setDeactivating] = useState(false);
  // 已经登过停用的不摆这一项（identityMayBeDeactivated）；分隔线只为它画，一并不摆。
  const canDeactivate = mayWrite && identityMayBeDeactivated(row);
  return (
    <>
    <DropdownMenu
      align="right"
      trigger={
        <Button variant="outline" size="sm" className="px-2 text-idpxyz-breadcrumb" aria-label="更多动作">
          <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
        </Button>
      }
    >
      {onOpenObjectPage ? (
        <MenuItem onSelect={onOpenObjectPage}>
          <ExternalLink className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
          在新标签打开对象页
        </MenuItem>
      ) : null}
      <MenuItem onSelect={() => copy('编码', row.partyId)}>
        <Copy className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
        复制编码
      </MenuItem>
      {mayWrite && onCorrectIdentity ? (
        <MenuItem onSelect={onCorrectIdentity}>
          <PencilLine className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
          更正登记信息…
        </MenuItem>
      ) : null}
      {canDeactivate ? <MenuSeparator /> : null}
      {canDeactivate ? (
        <MenuItem onSelect={() => setDeactivating(true)}>
          <Ban className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
          停用这家单位…
        </MenuItem>
      ) : null}
    </DropdownMenu>
    {canDeactivate ? (
      <BusinessPartyDeactivationDialog
        row={row}
        parties={parties}
        liveRelationships={liveRelationships}
        open={deactivating}
        onOpenChange={setDeactivating}
        onDeactivated={() => onChanged?.()}
      />
    ) : null}
    </>
  );
}

// 关系终止三格的横幅语气：撤销是人为中止，用警示色；到期与替代是按登记走到的边界，用提示色。
const endedTone: Record<string, StatusTone> = { REVOKED: 'danger', EXPIRED: 'info', SUPERSEDED: 'info' };

/** 这一行要不要摆横幅：已终止（撤销 / 到期 / 替代），或双方之一待查。 */
export function partyRelationshipHasBanners(row: PartyRelationshipRecord): boolean {
  return row.endedAt !== undefined || partyRelationshipNeedsAttention(row);
}

/**
 * 关系行的横幅。终止横幅只说「自何时起成了什么」：终止依据与后继是事实，归详情各格，不在横幅里重复一遍
 * （票 group-legal-entities-uiux/06 的去重纪律）。待查横幅与法人册「参与方悬空」同一句：写入门失败留下的悬空，登新修订补不上。
 */
export function PartyRelationshipBanners({ row }: { row: PartyRelationshipRecord }) {
  const tone = endedTone[row.status] ?? 'info';
  return (
    <>
      {row.endedAt && (
        <div className={`flex items-center gap-2 rounded border px-2.5 py-1.5 type-body ${statusToneBanner[tone]}`}>
          <Ban className={`h-3.5 w-3.5 shrink-0 ${statusToneText[tone]}`} aria-hidden="true" />
          <span>
            自 <Instant value={row.endedAt} /> 起{labelOf(relationshipStatusLabels, row.status)}，之后的新业务不再按这段关系办
          </span>
        </div>
      )}
      {!row.holderNameKnown && <UnknownPartyBanner side="持有方" partyId={row.holderId} />}
      {!row.counterpartyNameKnown && <UnknownPartyBanner side="相对方" partyId={row.counterpartyId} />}
    </>
  );
}

function UnknownPartyBanner({ side, partyId }: { side: string; partyId: string }) {
  return (
    <div className={`flex items-center gap-2 rounded border px-2.5 py-1.5 type-body ${statusToneBanner.warning}`}>
      <AlertTriangle className={`h-3.5 w-3.5 shrink-0 ${statusToneText.warning}`} aria-hidden="true" />
      <span>
        待查：{side}
        {partyNameUnknownNote}（{partyId}）。多半是当初登记时出了问题；请到「往来单位」里核对这家单位。
      </span>
    </div>
  );
}

/**
 * 关系行的「更多」菜单：复制关系标识，以及打开双方各自的参与方对象页——关系的两端都是身份册上的对象，从关系这边
 * 跳过去看它「对我们是什么」比回身份册再找一遍近。撤销与到期不在菜单里：那两个动作要填依据与时刻，摆在栏内
 * （PartyRelationshipEndActions）。
 */
export function PartyRelationshipMoreMenu({ row }: { row: PartyRelationshipRecord }) {
  const copy = useCopyToClipboard();
  const openParty = (partyId: string) => {
    window.location.hash = businessPartyObjectHash(partyId);
  };
  return (
    <DropdownMenu
      align="right"
      trigger={
        <Button variant="outline" size="sm" className="px-2 text-idpxyz-breadcrumb" aria-label="更多动作">
          <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
        </Button>
      }
    >
      <MenuItem onSelect={() => copy('关系标识', row.relationshipId)}>
        <Copy className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
        复制关系标识
      </MenuItem>
      <MenuSeparator />
      <MenuItem onSelect={() => openParty(row.holderId)}>
        <ExternalLink className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
        打开{row.holderNameKnown ? row.holderName : '这家单位'}
      </MenuItem>
      <MenuItem onSelect={() => openParty(row.counterpartyId)}>
        <ExternalLink className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
        打开{row.counterpartyNameKnown ? row.counterpartyName : '对方'}
      </MenuItem>
    </DropdownMenu>
  );
}
