import { useEffect, useState, type ReactNode } from 'react';
import { History, RefreshCw, Star } from 'lucide-react';
import { Button, Tag } from '@idpxyz/ui-primitives';
import { navigationSections, pageTitleById } from '../navigation';
import { statusToneText } from '../components/status-tone';
import { listRecentObjects, recentObjectHash, type RecentObject } from './my-work/recent-objects';
import { listSavedViews, savedViewHash, type SavedView } from './my-work/saved-views';
import { InstantCell, moduleTitleOf } from './my-work/shared';
import { unadmittedObjectsHash } from './customs/cases-address';
import { disputedStatementsHash } from './settlement/reconciliation-address';
import {
  acceptanceReviewCount,
  authorizedDispositionCount,
  controlReleaseCount,
  disputedStatementCount,
  openExceptionCaseCount,
  pendingPublicationCount,
  postIntakeDispositionCount,
  todoNeedsAttention,
  todoNoteText,
  todoStateOf,
  todoValueText,
  unadmittedObjectCount,
  type TodoTileState,
} from './my-work/today-todo';

// 工作台首页 = 今日待办 + 我的工作（最近打开、保存的视图）+ 全部功能入口（设计源「九、怎么落地」第三步）。
//
// 今日待办每格读一个队列读口，数「在等人处理」的件数（判读在 my-work/today-todo.ts）；点一格进那张列表，列表页能按地址筛的
// 就带上筛选落在筛好的那一签。读口按格动态引入：首页在入口块里，静态引各域的 api 会把它们钉回入口块。读不到的格如实说
// 未接通 / 无权查看 / 读不到，不报 0。
//
// 设计源列的是待受理、待处置、卡关、异常、待对账各一格。待处置在今天是几张各有主人的队列（受限运单、收寄后处置、资金释放），
// 合成一格就得替几个读口加总、点进去也落不到一张列表，所以各占一格；合同与价格的审批发布不在设计源那一列里，但它同样是
// 在等人处理的一张队列，留着。卡关与待对账是加出来的，不替换哪一格；各格按设计源的次序排。
//
// 卡关数的是每件运单最近那一刻的放行检查（动作准入判断）有一份没过门的——阻断、未决、未配置；待对账数的是挂着未了结异议的客户对账单——
// 至少一项争议还没有终局结果。两问的定义在各自上下文的读口（关务的 UnadmittedObjects、结算的 DisputedCustomerStatements），
// 首页只拿件数。设计源在待对账旁写的「待确认应收」本产品今天答不出：相对方确认不入账，没有记录不等于客户还没确认，不数。
//
// 原来的「模块就绪度总览」（已接线 / 页面骨架 / 规划占位）是给开发看的，不是给运营看的，首页不再摆；档位仍由
// page-registry 的 readinessOf 派生，要用的工具照常取。我的工作两块读本机浏览器的记录，不发请求。页名一律取导航
// （pageTitleById），不在这里另起一套名字。

interface TodoTile {
  /** 点进去的页 id；格子标题取这页在导航里的名字。 */
  id: string;
  /** 点进去落在筛好的那一签的地址；缺席即只开页。 */
  hash?: string;
  /** 带筛选的格在页名后标出筛的是哪一类（设计源的叫法），与页上那个筛选开关同名。 */
  filterLabel?: string;
  /** 有件数时格子下方那一句：这些件在等什么。 */
  waiting: string;
  load: () => Promise<TodoTileState>;
}

const todoTiles: TodoTile[] = [
  {
    id: 'acceptance-review',
    waiting: '运单等人工复核后才能受理',
    load: async () => {
      const api = await import('./shipment-request/api');
      return todoStateOf(await api.listAcceptanceReviewQueue(), acceptanceReviewCount);
    },
  },
  {
    id: 'authorized-disposition',
    waiting: '运单受限，等有权的同事决定去向',
    load: async () => {
      const api = await import('./shipment-request/api');
      return todoStateOf(await api.listAuthorizedDispositionQueue(), authorizedDispositionCount);
    },
  },
  {
    id: 'post-intake-disposition',
    waiting: '已收寄的货等决定继续、拦截、改址或退运',
    load: async () => {
      const api = await import('./shipment-request/api');
      return todoStateOf(await api.listPostIntakeDispositionRequests(), postIntakeDispositionCount);
    },
  },
  {
    id: 'control-release-refusals',
    waiting: '占用的资金没能自动释放，等人核对',
    load: async () => {
      const api = await import('./shipment-request/api');
      return todoStateOf(await api.listControlReleaseRefusals(), controlReleaseCount);
    },
  },
  {
    id: 'customs-cases',
    hash: unadmittedObjectsHash,
    filterLabel: '卡关',
    waiting: '运单最近一次放行检查没过，卡在关务上',
    load: async () => {
      const api = await import('./customs/api');
      return todoStateOf(await api.listUnadmittedObjects(), unadmittedObjectCount);
    },
  },
  {
    id: 'exception-cases',
    waiting: '异常还没处理完',
    load: async () => {
      const api = await import('./visibility/case-api');
      return todoStateOf(await api.listExceptionCaseRecords(), openExceptionCaseCount);
    },
  },
  {
    id: 'reconciliation',
    hash: disputedStatementsHash,
    filterLabel: '待对账',
    waiting: '客户对账单上有异议，等核实或处置',
    load: async () => {
      const api = await import('./settlement/api');
      return todoStateOf(await api.listDisputedCustomerStatements(), disputedStatementCount);
    },
  },
  {
    id: 'pending-publications',
    waiting: '合同、价格等改动等批准或发布',
    load: async () => {
      const api = await import('./party/publication-draft-api');
      return todoStateOf(await api.listOpenPublicationDrafts(), pendingPublicationCount);
    },
  },
];

/** 首页里每块只列前几条；全量、搜索与按页筛在各自的页面上。 */
const MY_WORK_PREVIEW_LIMIT = 8;

function TodoGrid({ onNavigate }: { onNavigate?: (id: string) => void }) {
  const [states, setStates] = useState<Record<string, TodoTileState>>({});
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setStates({});
    for (const tile of todoTiles) {
      // 一格读不到不拖累别的格：各格各自落定，动态引入失败也只落这一格。
      void tile
        .load()
        .catch((): TodoTileState => ({ kind: 'unavailable' }))
        .then((state) => {
          if (!cancelled) setStates((previous) => ({ ...previous, [tile.id]: state }));
        });
    }
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  return (
    <section className="mb-6">
      <div className="mb-2.5 flex items-center gap-2">
        <h2 className="text-[13px] font-bold text-idpxyz-textBright">今日待办</h2>
        <Button type="button" variant="ghost" size="sm" className="ml-auto" onClick={() => setReloadKey((key) => key + 1)}>
          <RefreshCw className="h-3.5 w-3.5" aria-hidden />
          刷新
        </Button>
      </div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        {todoTiles.map((tile) => {
          const state = states[tile.id] ?? { kind: 'loading' };
          const attention = todoNeedsAttention(state);
          const counted = state.kind === 'count';
          return (
            <button
              key={tile.id}
              type="button"
              onClick={
                tile.hash
                  ? () => {
                      // 带筛选的格直接写 hash：与「我的工作」行点同一条路，外壳经 hashchange 回流开签。
                      window.location.hash = tile.hash as string;
                    }
                  : onNavigate
                    ? () => onNavigate(tile.id)
                    : undefined
              }
              className={`rounded border p-3 text-left hover:bg-idpxyz-hover ${
                attention ? 'border-[color:var(--ds-status-warning-border)]' : 'border-idpxyz-border'
              }`}
            >
              <span className="block text-[12px] text-idpxyz-textMuted">
                {pageTitleById[tile.id] ?? tile.id}
                {tile.filterLabel ? ` · ${tile.filterLabel}` : null}
              </span>
              <span
                className={`mt-1 block font-bold ${counted ? 'text-[22px]' : 'text-[14px]'} ${
                  attention ? statusToneText.warning : counted ? 'text-idpxyz-textBright' : 'text-idpxyz-textMuted'
                }`}
              >
                {todoValueText(state)}
              </span>
              <span className="mt-1 block text-[11px] leading-4 text-idpxyz-textMuted">{todoNoteText(state, tile.waiting)}</span>
            </button>
          );
        })}
      </div>
    </section>
  );
}

// 「我的工作」一块卡的壳：标题 + 条数 + 「查看全部」，空时一句实话而不是样例行。
// 行由调用方给——两块的行形不同（对象行有时刻、视图行有默认标记），壳只管一致的头与空态。
function MyWorkCard({
  icon: Icon,
  title,
  total,
  emptyNote,
  onViewAll,
  children,
}: {
  icon: typeof History;
  title: string;
  total: number;
  emptyNote: string;
  onViewAll?: () => void;
  children: ReactNode;
}) {
  return (
    <section className="rounded border border-idpxyz-border p-4">
      <h2 className="flex items-center gap-2 text-[13px] font-bold text-idpxyz-textBright mb-2.5">
        <Icon className="h-4 w-4 text-idpxyz-textMuted" aria-hidden />
        <span>{title}</span>
        <span className="text-[11px] font-normal text-idpxyz-textMuted">{total} 条</span>
        <Button type="button" variant="ghost" size="sm" className="ml-auto" onClick={onViewAll} disabled={!onViewAll}>
          查看全部
        </Button>
      </h2>
      {total === 0 ? (
        <p className="text-[12px] leading-5 text-idpxyz-textMuted">{emptyNote}</p>
      ) : (
        <ul className="space-y-1">{children}</ul>
      )}
    </section>
  );
}

const myWorkRowClass = 'w-full flex items-center gap-2 rounded px-2 py-1.5 text-left hover:bg-idpxyz-hover';

// 全部功能入口按导航分区原序排，名字取导航；跳过工作台自己这一组（左栏第一组就是它）。
const entrySections = navigationSections.filter((section) => !section.items.some((item) => item.id === 'workbench'));

export function Workbench({ onNavigate }: { onNavigate?: (id: string) => void }) {
  // 一次读进 state：工作台每次从别页回来都重挂载，重读即最新；本页只读不写这两份存储，写方是外壳（记）与两页（清 / 删）。
  const [recent] = useState<RecentObject[]>(() => listRecentObjects(window.localStorage));
  const [views] = useState<SavedView[]>(() => listSavedViews(window.localStorage));

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="max-w-[1080px] mx-auto px-6 py-6">
        <header className="mb-5">
          <h1 className="text-[20px] font-bold text-idpxyz-textBright">{pageTitleById.workbench}</h1>
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-1">
            今天要处理的事都在这里，点一格进到对应的列表；下面是你最近打开过的对象和全部功能入口。
          </p>
        </header>

        <TodoGrid onNavigate={onNavigate} />

        {/* 行点直接写 hash：与两张页面的行点同一条路，外壳经 hashchange 回流，不另起跳转方式。 */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-6">
          <MyWorkCard
            icon={History}
            title={pageTitleById['recent-objects']}
            total={recent.length}
            emptyNote="这台电脑上还没有打开过运单、客户等对象；打开过的会记在这里，点一下就能回去。"
            onViewAll={onNavigate ? () => onNavigate('recent-objects') : undefined}
          >
            {recent.slice(0, MY_WORK_PREVIEW_LIMIT).map((row) => (
              <li key={`${row.moduleId}/${row.objectId}`}>
                <button
                  type="button"
                  className={myWorkRowClass}
                  onClick={() => {
                    window.location.hash = recentObjectHash(row);
                  }}
                >
                  <span className="text-[12px] text-idpxyz-text truncate">{row.title}</span>
                  <span className="hidden sm:inline text-[11px] text-idpxyz-textMuted truncate">
                    {moduleTitleOf(row.moduleId)}
                  </span>
                  <span className="ml-auto shrink-0 text-idpxyz-textMuted">
                    <InstantCell value={row.at} />
                  </span>
                </button>
              </li>
            ))}
          </MyWorkCard>
          <MyWorkCard
            icon={Star}
            title={pageTitleById['saved-views']}
            total={views.length}
            emptyNote="这台电脑上还没有保存过视图。列表页的「保存视图」还没开放，开放后存下的筛选条件会列在这里。"
            onViewAll={onNavigate ? () => onNavigate('saved-views') : undefined}
          >
            {views.slice(0, MY_WORK_PREVIEW_LIMIT).map((row) => (
              <li key={row.id}>
                <button
                  type="button"
                  className={myWorkRowClass}
                  onClick={() => {
                    window.location.hash = savedViewHash(row);
                  }}
                >
                  <span className="text-[12px] text-idpxyz-text truncate">{row.name}</span>
                  <span className="hidden sm:inline text-[11px] text-idpxyz-textMuted truncate">
                    {moduleTitleOf(row.moduleId)}
                  </span>
                  {row.isDefault ? (
                    <span className="ml-auto shrink-0">
                      <Tag>默认</Tag>
                    </span>
                  ) : null}
                </button>
              </li>
            ))}
          </MyWorkCard>
        </div>

        <h2 className="mb-2.5 text-[13px] font-bold text-idpxyz-textBright">全部功能</h2>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          {entrySections.map((section) => (
            <section key={section.title} className="rounded border border-idpxyz-border p-4">
              <h3 className="mb-2 text-[12px] font-bold text-idpxyz-textBright">{section.title}</h3>
              <ul className="space-y-0.5">
                {section.items.map((item) => (
                  <li key={item.id}>
                    <button
                      type="button"
                      onClick={onNavigate ? () => onNavigate(item.id) : undefined}
                      className="w-full rounded px-2 py-1 text-left text-[12px] text-idpxyz-text hover:bg-idpxyz-hover"
                    >
                      {item.label}
                    </button>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>
      </div>
    </div>
  );
}
