import { AlertTriangle, Ban, Copy, ExternalLink, MoreHorizontal } from 'lucide-react';
import { Button, DropdownMenu, MenuItem, MenuSeparator } from '@idpxyz/ui-primitives';
import { statusToneBanner, statusToneText, type StatusTone } from '../../components/status-tone';
import type { BusinessPartyRecord, PartyRelationshipRecord } from './api';
import { businessPartyObjectHash, deactivationLinkHash } from './business-parties-address';
import { labelOf, partyNameUnknownNote, relationshipStatusLabels } from './presentation';
import { partyRelationshipNeedsAttention } from './party-relationship-list';
import { Instant, useCopyToClipboard } from './detail-primitives';
import { useMayWriteRegistry } from '../use-operator-grants';

// 业务参与方页两册头区的件：横幅与「更多」菜单。身份册的详情栏与对象页各摆一份同样的头区（照 LegalEntityHeaderParts.tsx，
// 票 group-legal-entities-uiux/09：同一对象两处结构一致），件只在这里写一遍，两处各自排版。生命周期条借法人那一份
// （LegalEntityLifecycle）：两册身份同一套状态代数、同三格，不另画一条。
// 横幅正文用正文色、语气只给描边与图标（理由在 components/status-tone.ts）。

/** 这一行要不要摆横幅：停用了。参与方册是名称的来源，没有「待补」可报。 */
export function businessPartyHasBanners(row: BusinessPartyRecord): boolean {
  return row.deactivatedAt !== undefined;
}

export function BusinessPartyBanners({ row }: { row: BusinessPartyRecord }) {
  if (!row.deactivatedAt) return null;
  return (
    <div className={`flex items-center gap-2 rounded border px-2.5 py-1.5 text-[12px] ${statusToneBanner.danger}`}>
      <Ban className={`h-3.5 w-3.5 shrink-0 ${statusToneText.danger}`} aria-hidden="true" />
      <span>
        自 <Instant value={row.deactivatedAt} /> 起停用，新的业务不再找它
        {row.deactivationBasis ? ` — 来源单据 ${row.deactivationBasis}` : ''}
      </span>
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
  onOpenObjectPage,
  onDeactivate,
}: {
  row: BusinessPartyRecord;
  onOpenObjectPage?: () => void;
  onDeactivate?: () => void;
}) {
  const copy = useCopyToClipboard();
  // 没有登记册配置写授予的操作者不摆登记与发布入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const deactivate = () => {
    if (onDeactivate) onDeactivate();
    else window.location.hash = deactivationLinkHash({ kind: 'BUSINESS_PARTY', id: row.partyId });
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
      {mayWrite ? <MenuSeparator /> : null}
      {mayWrite ? (
        <MenuItem onSelect={deactivate}>
          <Ban className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
          停用这家单位…
        </MenuItem>
      ) : null}
    </DropdownMenu>
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
        <div className={`flex items-center gap-2 rounded border px-2.5 py-1.5 text-[12px] ${statusToneBanner[tone]}`}>
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
    <div className={`flex items-center gap-2 rounded border px-2.5 py-1.5 text-[12px] ${statusToneBanner.warning}`}>
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
