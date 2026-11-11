import { useEffect, useRef, type ReactNode } from 'react';
import { ChevronLeft, ChevronRight, Inbox } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { statusToneSurface } from '../../components/status-tone';
import { revealClass, useDelayedReveal } from '../../components/use-delayed-reveal';
import { cursorPagerControls } from '../../templates/cursor-pagination';
import { StateSlot, type StateSlotProps, type TemplateViewState } from '../../templates/state-slot';
import { labelFor, toneFor, type StatusTone } from '../../vocabulary/status';
import { partyFooterPageText } from './business-parties-workbench';
import { partyRosterPageText } from './party-roster';
import { PartySearchBox, type PartyWorkbenchFooter } from './PartyWorkbench';
import { usePartyListKeys } from './use-party-list-keys';

// 往来单位页的主从分栏（票 business-party-redesign/03）：选中一家之后，左边是定宽的名录栏，右边整块是这一家的详情。
// 表（PartyWorkbench.tsx）在右栏开着时只剩一半宽，五六列挤成两三列，既不像表也不像名单；这里把左边换成一份只为「找到下一家」
// 而排的名单——一行两句（名称、类型）加一个状态，别的都在右栏里——右栏拿到剩下的全部宽度。
// - 栏宽固定、不可拖：宽屏 420，视口不足 1440 时 360。名单一行只有两句话，拖宽了也不多显什么。
// - 栏自己带栏头（页名与写入口）与视图签：分栏时页头与整行的视图签不摆，右栏才能从页面顶到底。
// - 选中的行不切换：再点一次不收右栏（收栏是 Esc 与右栏的关闭钮），免得在名单里连点两下把详情点没了。
// - 行上没有悬停底色，与表同（用户 2026-11-03 裁定）。
// 键盘与表是同一套（use-party-list-keys.ts）。

export interface PartyRosterProps<Row> {
  /** 栏的名字，读屏念它。 */
  label: string;
  /** 栏头：页名与写入口。 */
  head: ReactNode;
  /** 视图签。 */
  tabs: ReactNode;
  search: { value: string; onChange: (value: string) => void; placeholder: string };
  /** 检索框下面的筛选件，摆在两列的格里、一件一格；要占整行的自己带 `col-span-2`。 */
  tools?: ReactNode;
  rows: Row[];
  rowKey: (row: Row) => string;
  selectedKey: string | null;
  /** 换到某一行，或收起右栏（null）。 */
  onSelect: (key: string | null) => void;
  /** 一行里摆什么：图标块、名称与类型两句、状态。 */
  renderRow: (row: Row) => ReactNode;
  /** ready 态下筛没了时的那句话。 */
  noMatchText: string;
  onClearFilters?: () => void;
  footer: PartyWorkbenchFooter;
  viewState: TemplateViewState;
  stateOverride?: StateSlotProps['override'];
  /** 右栏。 */
  detail: ReactNode;
}

const SKELETON_ROWS = 12;

/** 名录栏里的下拉：占满它那一格；筛着的时候描边换成强调色，栏里没有「清除筛选」，哪一项在筛得一眼看得出。 */
export function partyRosterSelectClass(filtering: boolean): string {
  return `w-full min-w-0 rounded border bg-idpxyz-inputBg px-2 py-1 type-body text-idpxyz-text focus:border-idpxyz-accent focus:outline-none ${
    filtering ? 'border-idpxyz-accent' : 'border-idpxyz-border'
  }`;
}

// 状态点的颜色照用语词典给这个状态的语气（vocabulary/status.ts），与状态胶囊同源；中性一格词典没有对应的语气色，用弱字色。
const rosterStatusDot: Record<StatusTone, string> = {
  good: statusToneSurface.success,
  info: statusToneSurface.info,
  warn: statusToneSurface.warning,
  bad: statusToneSurface.danger,
  muted: 'bg-idpxyz-textMuted',
};

/** 名录栏一行行尾的状态：一个色点加词。不用胶囊——一栏几十行各带一枚描边的胶囊，比名称还抢眼。 */
export function PartyRosterStatus({ code }: { code: string }) {
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap type-body text-idpxyz-breadcrumb">
      <span className={`h-[7px] w-[7px] shrink-0 rounded-full ${rosterStatusDot[toneFor(code)]}`} aria-hidden="true" />
      {labelFor(code)}
    </span>
  );
}

export function PartyRoster<Row>({
  label,
  head,
  tabs,
  search,
  tools,
  rows,
  rowKey,
  selectedKey,
  onSelect,
  renderRow,
  noMatchText,
  onClearFilters,
  footer,
  viewState,
  stateOverride,
  detail,
}: PartyRosterProps<Row>) {
  const searchRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const ready = viewState.kind === 'ready';
  usePartyListKeys({
    listRef,
    searchRef,
    keys: rows.map(rowKey),
    selectedKey,
    detailOpen: true,
    ready,
    onSelect,
    centerFirstReveal: true,
    revealWhenReady: true,
  });

  // 栏是点了表里的一行才挂上来的，那一行随表卸掉，焦点没了着落：落到栏里选中的那一行上，↑/↓ 接着走。只在挂上来时看一次，
  // 也只在焦点确实没有着落时落，不从别处抢。
  useEffect(() => {
    if (selectedKey === null || document.activeElement !== document.body) return;
    listRef.current?.querySelector<HTMLElement>(`[data-row-key="${CSS.escape(selectedKey)}"]`)?.focus({ preventScroll: true });
  }, []);

  const pagination = footer.pagination;
  const pager = pagination ? cursorPagerControls(pagination.page, pagination.next) : null;

  const list =
    rows.length === 0 ? (
      <div className="flex flex-col items-center px-4 py-12 text-center text-idpxyz-breadcrumb">
        <Inbox className="mb-3 h-7 w-7 opacity-50" aria-hidden="true" />
        <p className="type-value">{noMatchText}</p>
        {onClearFilters && (
          <Button variant="ghost" size="sm" className="mt-2 type-body text-idpxyz-breadcrumb" onClick={onClearFilters}>
            清除筛选
          </Button>
        )}
      </div>
    ) : (
      <ul>
        {rows.map((row) => {
          const key = rowKey(row);
          const selected = selectedKey === key;
          return (
            <li key={key}>
              <button
                type="button"
                data-row-key={key}
                aria-current={selected ? 'true' : undefined}
                onClick={() => onSelect(key)}
                // 焦点框只画在未选中的行上：焦点与选中落在同一行时，底色加左竖线已经说明了「在这」。
                className={`flex h-[60px] w-full items-center gap-3 border-b border-idpxyz-border px-4 text-left ${
                  selected
                    ? 'bg-idpxyz-activeItem shadow-[inset_3px_0_0_0_var(--idpxyz-accent)] focus-visible:outline-none'
                    : 'focus-visible:outline focus-visible:outline-1 focus-visible:-outline-offset-1 focus-visible:outline-idpxyz-accent'
                }`}
              >
                {renderRow(row)}
              </button>
            </li>
          );
        })}
      </ul>
    );

  return (
    <div className="flex min-h-0 flex-1 overflow-hidden">
      <aside
        aria-label={label}
        className="flex w-[360px] shrink-0 flex-col overflow-hidden border-r border-idpxyz-border bg-idpxyz-panelBg min-[1440px]:w-[420px]"
      >
        {head}
        {tabs}
        <div className="grid shrink-0 grid-cols-2 gap-2 border-b border-idpxyz-border px-4 py-2.5">
          <PartySearchBox inputRef={searchRef} {...search} className="col-span-2" />
          {tools}
        </div>
        <div ref={listRef} data-pane="list" className={`min-h-0 flex-1 ${viewState.kind === 'loading' ? 'overflow-hidden' : 'overflow-y-auto'}`}>
          {ready ? (
            list
          ) : viewState.kind === 'loading' ? (
            <RosterSkeleton />
          ) : (
            <div className="px-4 pb-8 pt-10">
              <StateSlot state={viewState} override={stateOverride} />
            </div>
          )}
        </div>
        <div className="flex h-9 shrink-0 items-center justify-between gap-2 border-t border-idpxyz-border pl-4 pr-2 type-caption text-idpxyz-breadcrumb">
          <span className="min-w-0 truncate tabular-nums" title={ready ? footer.countText ?? undefined : undefined}>
            {viewState.kind === 'loading' ? '正在读取…' : ready ? footer.countText : null}
          </span>
          {ready && pagination && pager ? (
            <nav aria-label="翻页" className="flex shrink-0 items-center gap-0.5" data-cursor-pagination>
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6"
                disabled={!pager.canPrevious}
                onClick={pagination.onPrevious}
                aria-label="上一页"
                title="上一页"
              >
                <ChevronLeft className="h-3.5 w-3.5" aria-hidden="true" />
              </Button>
              <span className="px-1 tabular-nums" aria-label={partyFooterPageText(pagination.page, pagination.size, pagination.total)}>
                {partyRosterPageText(pagination.page, pagination.size, pagination.total)}
              </span>
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6"
                disabled={!pager.canNext}
                onClick={() => pagination.next !== null && pagination.onNext(pagination.next)}
                aria-label="下一页"
                title="下一页"
              >
                <ChevronRight className="h-3.5 w-3.5" aria-hidden="true" />
              </Button>
            </nav>
          ) : null}
        </div>
      </aside>
      <div data-pane="detail" className="h-full min-h-0 min-w-0 flex-1 overflow-hidden">
        {detail}
      </div>
    </div>
  );
}

/** 加载中的骨架：与真的一行同高、同样的图标块加两句。答得快时不露（use-delayed-reveal.ts），免得闪一下。 */
function RosterSkeleton() {
  const revealCls = revealClass(useDelayedReveal());
  return (
    <div className={`animate-pulse ${revealCls}`} role="status" aria-label="加载中">
      {Array.from({ length: SKELETON_ROWS }, (_, row) => (
        <div key={row} className="flex h-[60px] items-center gap-3 border-b border-idpxyz-border px-4">
          <span className="h-8 w-8 shrink-0 rounded-lg bg-idpxyz-hover" />
          <span className="flex min-w-0 flex-1 flex-col gap-2">
            <span className="block h-2.5 rounded bg-idpxyz-hover" style={{ width: `${[62, 48, 70, 55][row % 4]}%` }} />
            <span className="block h-2 rounded bg-idpxyz-hover" style={{ width: `${[38, 30, 44][row % 3]}%` }} />
          </span>
        </div>
      ))}
    </div>
  );
}
