import { CircleCheck, CircleDashed, FlaskConical, Radio } from 'lucide-react';
import { navigationSections, moduleInfoById } from '../navigation';
import { readinessOf, type ModuleReadiness } from '../page-registry';

// 工作台=模块就绪度总览。数据全部派生自 UI 自身的登记事实（导航登记 +
// 页面登记 + 接线登记），不发请求、不放任何伪造的业务统计——业务数字要等
// 各查询端点按 ADR-0017 闸门放行后由对应页面呈现，总览不越位代答。

const readinessMeta: Record<
  ModuleReadiness,
  { word: string; explain: string; icon: typeof CircleCheck; toneClass: string }
> = {
  live: {
    word: '已接线',
    explain: '页面对 parcel-api 真实端点发请求',
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
}

// 总览跳过「总览」自身；其余分区按导航原序呈现，不另造第二套分组。
const sections: SectionOverview[] = navigationSections
  .filter((section) => section.title !== '总览')
  .map((section) => ({
    title: section.title,
    items: section.items.map((item) => ({
      id: item.id,
      label: item.label,
      readiness: readinessOf(item.id),
    })),
  }));

const totals = sections
  .flatMap((section) => section.items)
  .reduce(
    (acc, item) => {
      acc[item.readiness] += 1;
      return acc;
    },
    { live: 0, skeleton: 0, demo: 0, planned: 0 } as Record<ModuleReadiness, number>,
  );

export function Workbench({ onNavigate }: { onNavigate?: (id: string) => void }) {
  return (
    <div className="flex-1 overflow-y-auto">
      <div className="max-w-[1080px] mx-auto px-6 py-6">
        <header className="mb-5">
          <h1 className="text-[20px] font-bold text-idpxyz-textBright">IDP Parcel 租户管理台</h1>
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-1">
            模块就绪度总览。业务数据按 ADR-0017 的准入闸门放行后由各模块页面呈现，
            本页只陈述交付面自身的事实，不放任何未确认参数或伪造统计。
          </p>
        </header>

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
              <h2 className="text-[13px] font-bold text-idpxyz-textBright mb-2.5">
                {section.title}
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
