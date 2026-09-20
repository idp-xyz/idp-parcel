import { useState, type ReactNode } from 'react';
import { CircleCheck, CircleDashed, FlaskConical, History, Radio, Star } from 'lucide-react';
import { Button, Tag } from '@idpxyz/ui-primitives';
import { navigationSections, moduleInfoById } from '../navigation';
import { readinessOf, type ModuleReadiness } from '../page-registry';
import { listRecentObjects, recentObjectHash, type RecentObject } from './my-work/recent-objects';
import { listSavedViews, savedViewHash, type SavedView } from './my-work/saved-views';
import { InstantCell, moduleTitleOf } from './my-work/shared';

// 工作台=「我的工作」两块 + 模块就绪度总览。数据全部派生自 UI 自身的事实：前者读本机浏览器的
// localStorage（打开过的对象地址、存下的筛选态，由 pages/my-work 两份纯逻辑保管），后者派生自登记事实
// （导航登记 + 页面登记 + 接线登记）——两者同一口径，都不发请求、都不放任何伪造的业务统计。业务数字要等
// 各查询端点按 ADR-0017 闸门放行后由对应页面呈现，总览不越位代答；「最近对象」列的是地址与拼出的标题，
// 不是对象名称，「本机 N 条」数的是这台浏览器的记录，不是任何业务计数。

const readinessMeta: Record<
  ModuleReadiness,
  { word: string; explain: string; icon: typeof CircleCheck; toneClass: string }
> = {
  live: {
    word: '已接线',
    // 两种来源都算：业务页接 parcel-api 端点，「我的工作」两页接本机浏览器的存储——都已接到各自唯一的真实来源。
    explain: '页面已接真实数据来源：parcel-api 端点，或「我的工作」两页读的本机浏览器事实',
    icon: Radio,
    toneClass: 'text-idpxyz-accent border-idpxyz-accent',
  },
  skeleton: {
    word: '页面骨架',
    explain: '栏目按 CONTEXT 语义搭好，数据区如实呈现未配置态',
    icon: CircleCheck,
    toneClass: 'text-idpxyz-text border-idpxyz-border',
  },
  demo: {
    word: '合成 S 演示',
    explain: '功能完整，数据为隔离合成 S，不承载业务语义',
    icon: FlaskConical,
    toneClass: 'text-idpxyz-textMuted border-idpxyz-border',
  },
  planned: {
    word: '规划占位',
    explain: '有文档出处的模块条目，页面待建',
    icon: CircleDashed,
    toneClass: 'text-idpxyz-textMuted border-idpxyz-border border-dashed',
  },
};

interface ModuleEntry {
  id: string;
  label: string;
  readiness: ModuleReadiness;
}

interface SectionOverview {
  title: string;
  items: ModuleEntry[];
  /** 分区内已接线数，与条目档位同源派生，不是手工维护的第二份状态。 */
  liveCount: number;
}

// 总览跳过「总览」自身；其余分区按导航原序呈现，不另造第二套分组。
const sections: SectionOverview[] = navigationSections
  .filter((section) => section.title !== '总览')
  .map((section) => {
    const items = section.items.map((item) => ({
      id: item.id,
      label: item.label,
      readiness: readinessOf(item.id),
    }));
    return {
      title: section.title,
      items,
      liveCount: items.filter((item) => item.readiness === 'live').length,
    };
  });

const totals = sections
  .flatMap((section) => section.items)
  .reduce(
    (acc, item) => {
      acc[item.readiness] += 1;
      return acc;
    },
    { live: 0, skeleton: 0, demo: 0, planned: 0 } as Record<ModuleReadiness, number>,
  );

/** 工作台上每块只列前几条；全量、搜索与按模块筛在各自的页面上。 */
const MY_WORK_PREVIEW_LIMIT = 8;

// 「我的工作」一块卡的壳：标题 + 本机条数 + 「查看全部」，空时一句实话而不是样例行。
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
        <span className="text-[11px] font-normal text-idpxyz-textMuted">本机 {total} 条</span>
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

export function Workbench({ onNavigate }: { onNavigate?: (id: string) => void }) {
  // 一次读进 state：工作台每次从别页回来都重挂载，重读即最新；本页只读不写这两份存储，写方是外壳（记）与两页（清 / 删）。
  const [recent] = useState<RecentObject[]>(() => listRecentObjects(window.localStorage));
  const [views] = useState<SavedView[]>(() => listSavedViews(window.localStorage));

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="max-w-[1080px] mx-auto px-6 py-6">
        <header className="mb-5">
          <h1 className="text-[20px] font-bold text-idpxyz-textBright">IDP Parcel 租户管理台</h1>
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-1">
            「我的工作」与模块就绪度总览。业务数据按 ADR-0017 的准入闸门放行后由各模块页面呈现，
            本页只陈述交付面自身的事实——这台浏览器的记录、各模块的就绪档位——不放任何未确认参数或伪造统计。
          </p>
        </header>

        {/* 「我的工作」两块（票 admin-web-ux-alignment/02 第 4 条）：黄金标准把 My Work 放在业务导航之上，这里同样取上。
            数据是本机浏览器的事实，与下面的就绪度总览同一口径——都是 UI 自身知道的事，不是业务统计。
            行点直接写 hash：与两张页面的行点同一条路，外壳经 hashchange 回流，不另起跳转方式。 */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-6">
          <MyWorkCard
            icon={History}
            title="最近对象"
            total={recent.length}
            emptyNote="这台浏览器还没有打开过对象；从列表页进到某个对象的详情后，它的地址会记在这里。"
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
            title="保存视图"
            total={views.length}
            emptyNote="这台浏览器还没有保存过视图；在列表页的过滤条上按「保存当前视图」存下筛选态后，它会列在这里。"
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

        {/* 四档统计与语义说明：档位由页面登记机制派生，不是手工维护的第二份状态。 */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
          {(Object.keys(readinessMeta) as ModuleReadiness[]).map((kind) => {
            const meta = readinessMeta[kind];
            const Icon = meta.icon;
            return (
              <div key={kind} className="rounded border border-idpxyz-border p-3">
                <div className="flex items-center gap-2">
                  <Icon className="h-4 w-4 text-idpxyz-textMuted" aria-hidden />
                  <span className="text-[13px] font-bold text-idpxyz-textBright">{meta.word}</span>
                  <span className="ml-auto text-[18px] font-bold text-idpxyz-accent">
                    {totals[kind]}
                  </span>
                </div>
                <p className="text-[11px] leading-4 text-idpxyz-textMuted mt-1.5">{meta.explain}</p>
              </div>
            );
          })}
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {sections.map((section) => (
            <section key={section.title} className="rounded border border-idpxyz-border p-4">
              <h2 className="flex items-baseline gap-2 text-[13px] font-bold text-idpxyz-textBright mb-2.5">
                <span>{section.title}</span>
                <span className="ml-auto text-[11px] font-normal text-idpxyz-textMuted">
                  已接线 {section.liveCount}/{section.items.length}
                </span>
              </h2>
              <ul className="space-y-1">
                {section.items.map((item) => {
                  const meta = readinessMeta[item.readiness];
                  const owner = moduleInfoById[item.id]?.owner;
                  return (
                    <li key={item.id}>
                      <button
                        type="button"
                        onClick={onNavigate ? () => onNavigate(item.id) : undefined}
                        className="w-full flex items-center gap-2 rounded px-2 py-1.5 text-left hover:bg-idpxyz-hover"
                      >
                        <span className="text-[12px] text-idpxyz-text">{item.label}</span>
                        {owner ? (
                          <span className="hidden sm:inline text-[11px] text-idpxyz-textMuted truncate">
                            {owner}
                          </span>
                        ) : null}
                        <span
                          className={`ml-auto shrink-0 rounded border px-1.5 py-0.5 text-[10px] ${meta.toneClass}`}
                        >
                          {meta.word}
                        </span>
                      </button>
                    </li>
                  );
                })}
              </ul>
            </section>
          ))}
        </div>
      </div>
    </div>
  );
}
