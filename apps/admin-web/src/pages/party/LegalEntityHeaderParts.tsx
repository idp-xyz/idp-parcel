import { useState, type ReactNode } from 'react';
import { AlertTriangle, Ban, Copy, ExternalLink, MoreHorizontal } from 'lucide-react';
import { Button, DropdownMenu, MenuItem, MenuSeparator } from '@idpxyz/ui-primitives';
import { statusToneBanner, statusToneSurface, statusToneText } from '../../components/status-tone';
import type { GroupLegalEntityRecord } from './api';
import { LegalEntityDeactivationDialog } from './LegalEntityDeactivationDialog';
import { identityLayerAbsentNote, identityStatusLabels, labelOf, partyNameUnknownNote, profileRevisionLabel } from './presentation';
import {
  legalEntityLifecycle,
  lifecycleConnectorReached,
  type IdentityLifecycleInput,
  type LifecycleStage,
  type LifecycleStageState,
} from './legal-entity-lifecycle';
import { Instant, useCopyToClipboard } from './detail-primitives';
import { useMayWriteRegistry } from '../use-operator-grants';
import { StatusPill } from '../../components/StatusPill';
import { referencedCountNote, referencedCountText, type ProfileStatusCell } from './legal-entity-references';

// 责任法人头区的几件：生命周期小步进、停用与待补横幅、「更多」菜单，以及头区下那一行关键事实。工作台详情栏
// 与对象页各摆一份同样的头区（票 group-legal-entities-uiux/09：同一法人两处结构一致），件只在这里写一遍，两处各自排版。

/** 首屏「资料状态」格：答到了用状态胶囊，还没读到显占位，读不到显「—」——判读在 profileStatusCellOf。 */
export function ProfileStatusValue({ cell }: { cell: ProfileStatusCell }) {
  if (cell.kind === 'pending') return <span className="text-idpxyz-breadcrumb">…</span>;
  if (cell.kind === 'unavailable') return <span className="text-idpxyz-breadcrumb">—</span>;
  return <StatusPill label={cell.status.label} tone={cell.status.tone} className="font-sans" />;
}

/** 首屏「资料版本」格：这一刻生效的那一版的短写「第 N 版」（轴名由标签承担）；没有生效版本显「—」。 */
function ProfileVersionValue({ cell }: { cell: ProfileStatusCell }) {
  if (cell.kind === 'pending') return <span className="text-idpxyz-breadcrumb">…</span>;
  if (cell.kind === 'unavailable' || cell.effectiveRevision === undefined) return <span className="text-idpxyz-breadcrumb">—</span>;
  return <span className="tabular-nums">{profileRevisionLabel(cell.effectiveRevision)}</span>;
}

export interface LegalEntityHeaderFact {
  label: string;
  value: ReactNode;
  /** 悬停说明（口径）。 */
  title?: string;
}

/**
 * 头区下那一行关键事实（票 company-subject-ux/08 第 2 条），详情栏与对象页同一份：生效（已停用时多一格停用）、资料状态、
 * 资料版本、被引用。一个事实首屏只出现一处（票 group-legal-entities-uiux/06）：编码、注册地、注册号在 meta 行，名称是标题，
 * 身份版本只在「登记记录」签看。详情栏把它摆成 FactStrip inline，对象页接在模板 meta 行后面——模板在头区与签之间只留了
 * 指标卡那一格，一行小字只能走 meta。
 */
export function legalEntityHeaderFacts(
  row: GroupLegalEntityRecord,
  profileCell: ProfileStatusCell,
  referenced: number | null,
): LegalEntityHeaderFact[] {
  return [
    { label: '生效', value: <Instant value={row.effectiveFrom} precision="day" /> },
    ...(row.deactivatedAt ? [{ label: '停用', value: <Instant value={row.deactivatedAt} precision="day" /> }] : []),
    { label: '资料状态', value: <ProfileStatusValue cell={profileCell} /> },
    { label: '资料版本', value: <ProfileVersionValue cell={profileCell} /> },
    { label: '被引用', value: referencedCountText(referenced), title: referencedCountNote },
  ];
}

// 圆点小面积实色取语气前景色（status-tone.ts 的 statusToneSurface 那一档）；没走到的段只画一圈描边。
const stageDotClass: Record<LifecycleStageState, string> = {
  done: statusToneSurface.success,
  current: 'bg-idpxyz-accent',
  skipped: 'border border-dashed border-idpxyz-border bg-transparent',
  pending: 'border border-idpxyz-border bg-transparent',
};

// 旁支终态（已停用）只有两态：没停用时一圈虚线不点亮，停用了才亮成警示色。
const terminalDotClass = (reached: boolean) =>
  reached ? statusToneSurface.danger : 'border border-dashed border-idpxyz-border bg-transparent';

/** 一段的圆点与名字，点在前词在后排成一行。圆点不编号：编号会把三段读成一条必经的流程。 */
function StageMark({ stage }: { stage: LifecycleStage }) {
  const dotClass = stage.terminal ? terminalDotClass(stage.state === 'current') : stageDotClass[stage.state];
  return (
    <span className="inline-flex items-center gap-1.5" aria-current={stage.state === 'current' ? 'step' : undefined}>
      <span className={`h-2 w-2 shrink-0 rounded-full ${dotClass}`} aria-hidden="true" />
      <span
        className={`whitespace-nowrap text-[12px] leading-5 ${
          stage.state === 'current' ? 'font-medium text-idpxyz-textBright' : 'text-idpxyz-breadcrumb'
        }`}
      >
        {labelOf(identityStatusLabels, stage.code)}
        {stage.state === 'skipped' ? '（未经过）' : ''}
      </span>
    </span>
  );
}

/**
 * 主线（已登记 → 已生效）一条，已停用隔一道虚线摆在右侧作旁支终态（票 group-legal-entities-uiux/06）：停用是例外分支，
 * 不是每个法人迟早走到的下一步。分段判读在 legal-entity-lifecycle.ts。
 *
 * 紧凑的一行小步进（票 company-subject-ux/08）：段与段之间只隔一小截连线，整条按内容定宽、不撑满容器——撑满时宽屏上
 * 两点各占一端、中间一条横贯的长线，读不出这是三段。放在头区 meta 行末尾或指标格里都是同一形。
 */
export function LegalEntityLifecycle({ row }: { row: IdentityLifecycleInput }) {
  const stages = legalEntityLifecycle(row);
  if (stages === null) return null;
  const main = stages.filter((stage) => !stage.terminal);
  const terminal = stages.find((stage) => stage.terminal);
  return (
    <div className="inline-flex items-center gap-2" role="group" aria-label="身份生命周期">
      <ol className="inline-flex items-center gap-2">
        {main.map((stage, index) => (
          <li key={stage.code} className="inline-flex items-center gap-2">
            <StageMark stage={stage} />
            {index < main.length - 1 && (
              <span
                className={`h-px w-4 ${
                  lifecycleConnectorReached(stage, main[index + 1]) ? 'bg-emerald-500/40' : 'bg-idpxyz-border'
                }`}
                aria-hidden="true"
              />
            )}
          </li>
        ))}
      </ol>
      {terminal ? (
        <div className="border-l border-dashed border-idpxyz-border pl-2">
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
            自 <Instant value={row.deactivatedAt} /> 起停用，新的签约与开票不再用它
            {row.deactivationBasis ? ` — 来源单据 ${row.deactivationBasis}` : ''}
          </span>
        </div>
      )}
      {!row.identityLayerRegistered && (
        <div className={`flex items-center gap-2 rounded border px-2.5 py-1.5 text-[12px] ${statusToneBanner.warning}`}>
          <AlertTriangle className={`h-3.5 w-3.5 shrink-0 ${statusToneText.warning}`} aria-hidden="true" />
          <span>待补：注册地与注册号{identityLayerAbsentNote}。用页头的「新增公司主体」填同一个编码再保存一次即可补齐。</span>
        </div>
      )}
      {!row.partyNameKnown && (
        <div className={`flex items-center gap-2 rounded border px-2.5 py-1.5 text-[12px] ${statusToneBanner.warning}`}>
          <AlertTriangle className={`h-3.5 w-3.5 shrink-0 ${statusToneText.warning}`} aria-hidden="true" />
          <span>
            待查：{partyNameUnknownNote}（{row.partyId}）。多半是当初登记时出了问题；请到「往来单位」里核对这家单位。
          </span>
        </div>
      )}
    </>
  );
}

/**
 * 「更多」菜单。动作只摆本产品真有的：停用开本页的对话框（LegalEntityDeactivationDialog，票 company-subject-ux/03），
 * 不再把人送去往来单位页——那一页的停用表单照旧是业务参与方与客户账户的停用入口。「在新标签打开对象页」只在详情栏里有
 * ——对象页自己就是那张标签。
 *
 * 对话框的开关由菜单自己记：对象页与详情栏各摆一份菜单，两处都要同一个对话框，记在这里就不用两边各写一遍。
 */
export function LegalEntityMoreMenu({
  row,
  onOpenObjectPage,
  onChanged,
}: {
  row: GroupLegalEntityRecord;
  onOpenObjectPage?: () => void;
  /** 停用落册后回调；页面借它重取法人册让「已停用」立刻可见（对象页是 retry，详情栏经列表页传进来）。 */
  onChanged?: () => void;
}) {
  const copy = useCopyToClipboard();
  // 没有登记册配置写授予的操作者不摆登记与发布入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const [deactivating, setDeactivating] = useState(false);
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
        <MenuItem onSelect={() => copy('编码', row.legalEntityId)}>
          <Copy className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
          复制编码
        </MenuItem>
        {mayWrite ? <MenuSeparator /> : null}
        {/* 菜单上点名的是这家公司本身，不是它所钉的业务参与方——后者是另一个身份，停错了收不回；对话框把种类与对象锁死。 */}
        {mayWrite ? (
          <MenuItem onSelect={() => setDeactivating(true)}>
            <Ban className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
            停用这个公司主体…
          </MenuItem>
        ) : null}
      </DropdownMenu>
      {mayWrite ? (
        <LegalEntityDeactivationDialog
          row={row}
          open={deactivating}
          onOpenChange={setDeactivating}
          onDeactivated={() => onChanged?.()}
        />
      ) : null}
    </>
  );
}
