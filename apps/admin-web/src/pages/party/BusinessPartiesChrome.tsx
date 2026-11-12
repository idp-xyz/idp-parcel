import { Fragment, useEffect, useRef, type KeyboardEvent, type ReactNode } from 'react';
import { RefreshCw } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { useToast } from '@idpxyz/ui-theme-runtime';
import { formatCount, type PartyTypeChip } from './business-parties-workbench';
import type { BusinessPartyTypeFilter } from './business-party-list';
import { partyTileVisuals } from './party-role-visuals';

// 往来单位页新版式的页头、视图签与工具条上的筛选芯片（票 business-party-redesign/02），以及主从分栏时名录栏的栏头（票 03）。
// 页头只做一件事，一行：页名、一句说明（放不下就截断）、动作。列表上方每多一行就少看一行数据，所以待办不另占一行，
// 跟在视图签的右端；各视图自己的数只在签上出现一次。

/** 刷新收尾时报什么：答出来了报「已刷新」，没答出来报原因。 */
export type RefreshOutcome = { ok: true } | { ok: false; message?: string };

export interface PartyRefresh {
  onRefresh: () => void;
  /** 有没有一问在途，由调用方给。 */
  refreshing: boolean;
  refreshOutcome: RefreshOutcome;
}

/** 刷新钮，页头与名录栏的栏头各摆一枚：按了转起来，收尾报一句。 */
export function PartyRefreshButton({ onRefresh, refreshing, refreshOutcome }: PartyRefresh) {
  const { addToast } = useToast();
  // 只报用户按钮触发的那一次刷新，且要先看到在途再看到结束：首取或别处触发的重取不冒这一句（做法同 WorkbenchPageTemplate）。
  const requested = useRef(false);
  const seen = useRef(false);
  useEffect(() => {
    if (!requested.current) return;
    if (refreshing) {
      seen.current = true;
      return;
    }
    if (!seen.current) return;
    requested.current = false;
    seen.current = false;
    if (refreshOutcome.ok) addToast({ type: 'success', title: '已刷新' });
    else addToast({ type: 'error', title: '刷新未形成答案', message: refreshOutcome.message });
  }, [refreshing, refreshOutcome, addToast]);

  const handleRefresh = () => {
    if (refreshing) return;
    requested.current = true;
    seen.current = false;
    onRefresh();
  };

  return (
    <Button
      className="text-idpxyz-breadcrumb"
      variant="ghost"
      size="icon"
      onClick={handleRefresh}
      disabled={refreshing}
      title={refreshing ? '刷新中…' : '刷新'}
      aria-label={refreshing ? '刷新中…' : '刷新'}
    >
      <RefreshCw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} aria-hidden="true" />
    </Button>
  );
}

export function BusinessPartiesPageHead({
  title,
  description,
  refresh,
  actions,
}: {
  title: string;
  description: string;
  refresh: PartyRefresh;
  /** 写入口；没有登记权限时调用方不给，这里只剩刷新。 */
  actions?: ReactNode;
}) {
  return (
    <div className="flex shrink-0 items-center gap-6 border-b border-idpxyz-border px-6 py-3">
      <div className="flex min-w-0 flex-1 items-baseline gap-3">
        <h1 className="shrink-0 type-title font-semibold leading-tight text-idpxyz-textBright">{title}</h1>
        <p className="min-w-0 truncate type-body text-idpxyz-breadcrumb">{description}</p>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <PartyRefreshButton {...refresh} />
        {actions}
      </div>
    </div>
  );
}

/**
 * 名录栏的栏头（票 business-party-redesign/03）：主从分栏时页头不摆，页名、刷新与写入口收在这里。栏窄，不摆那句说明；
 * 写入口由调用方给短的一组。页名仍是这一页的一级标题。
 */
export function PartyRosterHead({ title, refresh, actions }: { title: string; refresh: PartyRefresh; actions?: ReactNode }) {
  return (
    <div className="flex h-[52px] shrink-0 items-center gap-1.5 border-b border-idpxyz-border bg-idpxyz-editor pl-4 pr-3">
      <h1 className="min-w-0 flex-1 truncate type-title font-semibold leading-tight text-idpxyz-textBright">{title}</h1>
      <PartyRefreshButton {...refresh} />
      {actions}
    </div>
  );
}

export interface ViewTab<Id extends string> {
  id: Id;
  label: string;
  /** 这一视图有几条；null 即还没有答案，不摆数——「0」是一句真话，不拿它占位。 */
  count: number | null;
}

/**
 * 视图签：同一份主数据的几种看法，同级、一键换。待办跟在同一行右端，有才出现。`compact` 是名录栏里的那一形
 * （票 business-party-redesign/03）：矮一些、签与签挨得近，三枚签带着数也放得进最窄的栏；栏里不摆待办。
 */
export function BusinessPartiesViewTabs<Id extends string>({
  tabs,
  value,
  onChange,
  attention,
  compact = false,
}: {
  tabs: readonly ViewTab<Id>[];
  value: Id;
  onChange: (next: Id) => void;
  attention?: ReactNode;
  compact?: boolean;
}) {
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
    const index = tabs.findIndex((tab) => tab.id === value);
    const next = tabs[(index + (event.key === 'ArrowRight' ? 1 : tabs.length - 1)) % tabs.length];
    event.preventDefault();
    onChange(next.id);
    event.currentTarget.querySelector<HTMLElement>(`[data-view-tab="${next.id}"]`)?.focus();
  };
  return (
    <div
      className={`flex shrink-0 items-stretch border-b border-idpxyz-border bg-idpxyz-panelBg ${compact ? 'h-[38px] px-2' : 'h-[42px] px-3'}`}
    >
      <div role="tablist" aria-label="视图" className="flex items-stretch gap-0.5" onKeyDown={onKeyDown}>
        {tabs.map((tab) => {
          const active = tab.id === value;
          return (
            <button
              key={tab.id}
              type="button"
              role="tab"
              data-view-tab={tab.id}
              aria-selected={active}
              tabIndex={active ? 0 : -1}
              onClick={() => onChange(tab.id)}
              className={`-mb-px inline-flex items-center whitespace-nowrap border-b-2 type-section focus-visible:outline focus-visible:outline-1 focus-visible:-outline-offset-2 focus-visible:outline-idpxyz-accent ${
                compact ? 'gap-1.5 px-2' : 'gap-2 px-3'
              } ${active ? 'border-idpxyz-accent text-idpxyz-textBright' : 'border-transparent text-idpxyz-breadcrumb hover:text-idpxyz-text'}`}
            >
              {tab.label}
              {tab.count !== null ? (
                <span
                  className={`rounded-full px-1.5 type-caption leading-[18px] tabular-nums ${
                    active ? 'bg-idpxyz-activeItem text-idpxyz-textBright' : 'bg-idpxyz-hover text-idpxyz-breadcrumb'
                  }`}
                >
                  {formatCount(tab.count)}
                </span>
              ) : null}
            </button>
          );
        })}
      </div>
      {attention && !compact ? <div className="ml-auto flex min-w-0 items-center gap-5 pr-3">{attention}</div> : null}
    </div>
  );
}

/** 视图签右端的一项待办：几段待确认、几家未生效。点了带着对应的筛选跳过去。 */
export function AttentionItem({ children, onClick }: { children: ReactNode; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className="group inline-flex items-center gap-1.5 whitespace-nowrap type-caption text-idpxyz-text">
      <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-amber-500" aria-hidden="true" />
      <span className="group-hover:underline">{children}</span>
      <span className="text-[color:var(--ds-text-link-hover)]">查看</span>
    </button>
  );
}

/**
 * 工具条上的一枚筛选芯片：词、可缺的数、可缺的色点。选中用强调色描边加选中底色，不用实心强调色——一排芯片里实心的一枚
 * 会比页头的主按钮还抢眼。数为 null 即此刻数不出，只写词。
 */
export function PartyFilterChip({
  label,
  count = null,
  active,
  disabled = false,
  dot,
  title,
  onClick,
}: {
  label: string;
  count?: number | null;
  active: boolean;
  /** 当前条件下一条都没有：留在原位、压淡、点不动，免得一排芯片边筛边跳。 */
  disabled?: boolean;
  /** 色点的底色类；只是帮人认，词照写。 */
  dot?: string;
  title?: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      disabled={disabled}
      title={title}
      onClick={onClick}
      className={`inline-flex h-7 shrink-0 items-center gap-1.5 whitespace-nowrap rounded-full border px-2.5 type-body focus-visible:outline focus-visible:outline-1 focus-visible:outline-offset-1 focus-visible:outline-idpxyz-accent ${
        active
          ? 'border-idpxyz-accent bg-idpxyz-activeItem text-idpxyz-textBright'
          : disabled
            ? 'cursor-default border-idpxyz-border text-idpxyz-breadcrumb opacity-50'
            : 'border-idpxyz-border text-idpxyz-text hover:bg-idpxyz-hover'
      }`}
    >
      {dot ? <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${dot}`} aria-hidden="true" /> : null}
      {label}
      {count !== null ? (
        <span className={`tabular-nums ${active ? 'text-idpxyz-textBright' : 'text-idpxyz-breadcrumb'}`}>{formatCount(count)}</span>
      ) : null}
    </button>
  );
}

/**
 * 类型芯片一排：全部、各角色（带各自的色点，与行首图标块同一套颜色）、一道分隔、我方公司与「未担任角色」。
 * 点选中的那一枚等于点「全部」，由调用方在 onPick 里定。
 */
export function PartyTypeChipRow({
  chips,
  onPick,
}: {
  chips: readonly PartyTypeChip[];
  onPick: (type: BusinessPartyTypeFilter) => void;
}) {
  return (
    <div role="group" aria-label="类型" className="flex shrink-0 items-center gap-1.5">
      <span className="shrink-0 pr-0.5 type-caption text-idpxyz-breadcrumb" aria-hidden="true">
        类型
      </span>
      {chips.map((chip, index) => (
        <Fragment key={chip.type}>
          {chip.group === 'other' && chips[index - 1]?.group !== 'other' ? (
            <span className="mx-0.5 h-4 w-px shrink-0 bg-idpxyz-border" aria-hidden="true" />
          ) : null}
          <PartyFilterChip
            label={chip.label}
            count={chip.count}
            active={chip.active}
            disabled={chip.disabled}
            dot={chip.group === 'role' && chip.type !== 'ALL' ? partyTileVisuals[chip.type].dot : undefined}
            onClick={() => onPick(chip.type)}
          />
        </Fragment>
      ))}
    </div>
  );
}
