import { AlertTriangle, Ban, Copy, ExternalLink, MoreHorizontal, Users } from 'lucide-react';
import { Button, DropdownMenu, MenuItem, MenuSeparator } from '@idpxyz/ui-primitives';
import { statusToneBanner, statusToneText } from '../../components/status-tone';
import { ADDRESS_KEYWORD_PARAM, hashWithQueryValue } from '../../templates/address-query';
import type { GroupLegalEntityRecord } from './api';
import { identityLayerAbsentNote, identityStatusLabels, labelOf, partyNameUnknownNote } from './presentation';
import {
  legalEntityLifecycle,
  lifecycleConnectorReached,
  type LifecycleStage,
  type LifecycleStageState,
} from './legal-entity-lifecycle';
import { Instant, useCopyToClipboard } from './detail-primitives';

// 责任法人头区的三件：生命周期条、停用与待补横幅、「更多」菜单。工作台详情栏与对象页各摆一份同样的头区
// （票 group-legal-entities-uiux/09：同一法人两处结构一致），件只在这里写一遍，两处各自排版。

const stageDotClass: Record<LifecycleStageState, string> = {
  done: statusToneBanner.success,
  current: 'border-idpxyz-accent bg-idpxyz-activeItem text-idpxyz-accent',
  skipped: 'border-dashed border-idpxyz-border bg-transparent text-idpxyz-breadcrumb',
  pending: 'border-idpxyz-border bg-idpxyz-inputBg text-idpxyz-breadcrumb',
};

// 旁支终态（已停用）只有两态：没停用时一圈虚线不点亮，停用了才亮成警示色。
const terminalDotClass = (reached: boolean) =>
  reached ? statusToneBanner.danger : 'border-dashed border-idpxyz-border bg-transparent text-idpxyz-breadcrumb';

/** 一段的圆点与名字。圆点不编号：编号会把三段读成一条必经的流程。 */
function StageMark({ stage }: { stage: LifecycleStage }) {
  const dotClass = stage.terminal ? terminalDotClass(stage.state === 'current') : stageDotClass[stage.state];
  const reached = stage.state === 'current' || stage.state === 'done';
  return (
    <div className="flex flex-col items-center gap-1" aria-current={stage.state === 'current' ? 'step' : undefined}>
      <span
        className={`flex h-5 w-5 items-center justify-center rounded-full border text-[12px] font-semibold ${dotClass}`}
        aria-hidden="true"
      >
        {stage.state === 'done' ? '✓' : stage.state === 'skipped' ? '–' : reached ? '●' : ''}
      </span>
      <span
        className={`text-[12px] font-normal leading-none ${
          stage.state === 'current' ? 'font-medium text-idpxyz-textBright' : 'text-idpxyz-breadcrumb'
        }`}
      >
        {labelOf(identityStatusLabels, stage.code)}
        {stage.state === 'skipped' ? '（未经过）' : ''}
      </span>
    </div>
  );
}

/**
 * 主线（已登记 → 已生效）一条，已停用隔一道虚线摆在右侧作旁支终态（票 group-legal-entities-uiux/06）：停用是例外分支，
 * 不是每个法人迟早走到的下一步。分段判读在 legal-entity-lifecycle.ts。
 */
export function LegalEntityLifecycle({ row }: { row: GroupLegalEntityRecord }) {
  const stages = legalEntityLifecycle(row);
  if (stages === null) return null;
  const main = stages.filter((stage) => !stage.terminal);
  const terminal = stages.find((stage) => stage.terminal);
  return (
    <div className="flex items-center gap-3" role="group" aria-label="身份生命周期">
      <ol className="flex flex-1 items-center">
        {main.map((stage, index) => (
          <li key={stage.code} className="flex flex-1 items-center last:flex-none">
            <StageMark stage={stage} />
            {index < main.length - 1 && (
              <span
                className={`mx-1.5 mb-3.5 h-px flex-1 ${
                  lifecycleConnectorReached(stage, main[index + 1]) ? 'bg-emerald-500/40' : 'bg-idpxyz-border'
                }`}
                aria-hidden="true"
              />
            )}
          </li>
        ))}
      </ol>
      {terminal ? (
        <div className="border-l border-dashed border-idpxyz-border pl-3">
          <StageMark stage={terminal} />
        </div>
      ) : null}
    </div>
  );
}

/** 这一行要不要摆横幅：停用了，或有待补（判据同 legal-entity-list.ts 的 legalEntityNeedsAttention）。 */
export function legalEntityHasBanners(row: GroupLegalEntityRecord): boolean {
  return row.deactivatedAt !== undefined || !row.identityLayerRegistered || !row.partyNameKnown;
}

/**
 * 停用与待补横幅。两件待补各报各的处置：身份层缺格登下一修订能补；参与方悬空是写入门失败留下的，登新修订补不上。
 * 横幅正文用正文色、语气只给描边与图标（理由在 components/status-tone.ts）。
 */
export function LegalEntityBanners({ row }: { row: GroupLegalEntityRecord }) {
  return (
    <>
      {row.deactivatedAt && (
        <div className={`flex items-center gap-2 rounded border px-2.5 py-1.5 text-[12px] ${statusToneBanner.danger}`}>
          <Ban className={`h-3.5 w-3.5 shrink-0 ${statusToneText.danger}`} aria-hidden="true" />
          <span>
            自 <Instant value={row.deactivatedAt} /> 起停用，不再支持新的商业决定
            {row.deactivationBasis ? ` — 依据 ${row.deactivationBasis}` : ''}
          </span>
        </div>
      )}
      {!row.identityLayerRegistered && (
        <div className={`flex items-center gap-2 rounded border px-2.5 py-1.5 text-[12px] ${statusToneBanner.warning}`}>
          <AlertTriangle className={`h-3.5 w-3.5 shrink-0 ${statusToneText.warning}`} aria-hidden="true" />
          <span>待补：身份层两格{identityLayerAbsentNote}。登记同一法人的下一修订补齐（列表页头「登记责任法人」）。</span>
        </div>
      )}
      {!row.partyNameKnown && (
        <div className={`flex items-center gap-2 rounded border px-2.5 py-1.5 text-[12px] ${statusToneBanner.warning}`}>
          <AlertTriangle className={`h-3.5 w-3.5 shrink-0 ${statusToneText.warning}`} aria-hidden="true" />
          <span>
            待查：{partyNameUnknownNote}（{row.partyId}）。这是写入门失败留下的悬空，登记新修订补不上，需到业务参与方页核对参与方册。
          </span>
        </div>
      )}
    </>
  );
}

/**
 * 「更多」菜单。动作只摆本产品真有的：停用照页头注释的分工留在业务参与方页，去那一页时把参与方标识放进地址的 `?q=`，
 * 身份册挂载时已经筛到这一行。「在新标签打开对象页」只在详情栏里有——对象页自己就是那张标签。
 */
export function LegalEntityMoreMenu({
  row,
  onOpenObjectPage,
}: {
  row: GroupLegalEntityRecord;
  onOpenObjectPage?: () => void;
}) {
  const copy = useCopyToClipboard();
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
      <MenuItem onSelect={() => copy('法人标识', row.legalEntityId)}>
        <Copy className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
        复制法人标识
      </MenuItem>
      <MenuSeparator />
      <MenuItem
        onSelect={() => {
          window.location.hash = hashWithQueryValue('#/business-parties', ADDRESS_KEYWORD_PARAM, row.partyId);
        }}
      >
        <Users className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
        到业务参与方页停用 <span className="ml-1 font-mono text-idpxyz-breadcrumb">{row.partyId}</span>
      </MenuItem>
    </DropdownMenu>
  );
}
