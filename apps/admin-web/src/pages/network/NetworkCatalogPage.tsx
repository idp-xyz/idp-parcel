import { useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, useAddressKeyword, type ListColumn } from '../../templates';
import { MultiRegistrationPanel, chipClass, type RegistrationTarget } from '../../components/registration';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import {
  listNetworkCatalog,
  networkCatalogConditions,
  networkRegistrationEndpoints,
  registerNetworkCatalogVersion,
  registrationOutcomeLabels,
  registrationRefusalReasonLabels,
  type NetworkCatalogFamily,
  type NetworkVersionRecord,
} from './api';
import {
  adjustmentKindLabels,
  familyLabels,
  keywordPlaceholders,
  labelOf,
  networkCatalogFamilies,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
  targetKindLabels,
} from './presentation';

const info = moduleInfoById['network-catalog'];

interface CatalogRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

function col(
  id: string,
  header: string,
  options?: { mono?: boolean; align?: 'left' | 'center' | 'right'; className?: string },
): ListColumn<CatalogRow> {
  return {
    id,
    header,
    align: options?.align,
    className: options?.className,
    render: (row) =>
      options?.mono ? (
        <span className="font-mono text-[12px]">{row.values[id] ?? '—'}</span>
      ) : (
        row.values[id] ?? '—'
      ),
  };
}

// 列向与传输层各族行体同形(MCP-3 裁决总则:读面有的上列,读面没有的不上列)。
// 日历正文、策略正文等内容列今天不存在(PAR-NET-14),不发明列。
const familyColumns: Record<NetworkCatalogFamily, ListColumn<CatalogRow>[]> = {
  node: [
    col('code', '节点代码', { mono: true }),
    col('version', '版本', { align: 'right', className: 'w-[72px]' }),
    col('businessTimezone', '业务时区', { mono: true }),
    col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
  ],
  connection: [
    col('code', '连接代码', { mono: true }),
    col('version', '版本', { align: 'right', className: 'w-[72px]' }),
    col('endpoints', '端点节点', { mono: true }),
    col('businessTimezone', '业务时区', { mono: true }),
    col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
  ],
  line: [
    col('code', '线路代码', { mono: true }),
    col('version', '版本', { align: 'right', className: 'w-[72px]' }),
    col('segments', '组成段(按序)', { mono: true, className: 'min-w-64' }),
    col('applicableScope', '适用范围', { mono: true }),
    col('businessTimezone', '业务时区', { mono: true }),
    col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
  ],
  // 服务区域族由专页承担(裁决③),本页 chip 不含它;封闭集类型要求键在场,列表留空。
  'service-area': [],
  'service-calendar': [
    col('targetKind', '适用对象类别'),
    col('targetCode', '适用对象代码', { mono: true }),
    col('version', '版本', { align: 'right', className: 'w-[72px]' }),
    col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
  ],
  'availability-adjustment': [
    col('code', '调整代码', { mono: true }),
    col('version', '版本', { align: 'right', className: 'w-[72px]' }),
    col('targetKind', '适用对象类别'),
    col('targetCode', '适用对象代码', { mono: true }),
    col('adjustmentKind', '调整种类'),
    col('source', '来源', { mono: true }),
    col('window', '生效/解除', { mono: true, className: 'min-w-64' }),
  ],
  'route-strategy': [
    col('code', '策略代码', { mono: true }),
    col('version', '版本', { align: 'right', className: 'w-[72px]' }),
    col('applicableScope', '适用范围', { mono: true }),
    col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
  ],
};

// 行体是七族字段并集上的可选字段(与 api.ts 的 NetworkVersionRecord 同形),这里按
// 当前族取值;缺席字段如实显「—」,不为区间完整编造时刻。
function rowValues(family: NetworkCatalogFamily, record: NetworkVersionRecord): CatalogRow {
  const effective = record.effectiveFrom
    ? formatRange(record.effectiveFrom, record.effectiveTo)
    : '—';
  switch (family) {
    case 'connection':
      return {
        key: `connection:${record.code}@${record.version}`,
        values: {
          code: record.code ?? '—',
          version: String(record.version),
          endpoints: `${record.fromNode ?? '—'} → ${record.toNode ?? '—'}`,
          businessTimezone: record.businessTimezone ?? '—',
          effective,
        },
      };
    case 'line':
      return {
        key: `line:${record.code}@${record.version}`,
        values: {
          code: record.code ?? '—',
          version: String(record.version),
          segments: (record.segments ?? []).join(' → '),
          applicableScope: record.applicableScope ?? '—',
          businessTimezone: record.businessTimezone ?? '—',
          effective,
        },
      };
    case 'service-calendar':
      return {
        key: `service-calendar:${record.targetKind}:${record.targetCode}@${record.version}`,
        values: {
          targetKind: labelOf(targetKindLabels, record.targetKind ?? ''),
          targetCode: record.targetCode ?? '—',
          version: String(record.version),
          effective,
        },
      };
    case 'availability-adjustment':
      return {
        key: `availability:${record.code}@${record.version}`,
        values: {
          code: record.code ?? '—',
          version: String(record.version),
          targetKind: labelOf(targetKindLabels, record.targetKind ?? ''),
          targetCode: record.targetCode ?? '—',
          adjustmentKind: labelOf(adjustmentKindLabels, record.kind ?? ''),
          source: record.source ?? '—',
          window: `${record.effectiveAt ? formatInstant(record.effectiveAt) : '—'} → ${
            record.liftedAt ? formatInstant(record.liftedAt) : '未解除'
          }`,
        },
      };
    default:
      // node 与 route-strategy 同为「代码 + 版本 + 区间」骨架;route-strategy 另有
      // 适用范围列,值缺席时由列渲染兜「—」。
      return {
        key: `${family}:${record.code}@${record.version}`,
        values: {
          code: record.code ?? '—',
          version: String(record.version),
          businessTimezone: record.businessTimezone ?? '—',
          applicableScope: record.applicableScope ?? '—',
          effective,
        },
      };
  }
}

// 逐族查阅版本原文;chip 六族不含服务区域(MCP-3 裁决③,服务区域由专页承担)。
// 读口已迁到 ADR-0144(票 catalogue-read-pagination/03、05):检索词沿地址上的 ?q= 停稳后下推,翻页走游标,换族或换检索词
// 回第一页;次序是读口各族的缺省序,本页不另排。不在答回来的那一页上再筛——截断之后再筛,答出的「没有」可能是假的。
function NetworkCatalogTable() {
  const [familyId, setFamilyId] = useState<NetworkCatalogFamily>('node');
  const [keyword, setKeyword] = useAddressKeyword();
  const conditions = networkCatalogConditions(familyId, { q: useSettledKeyword(keyword) });
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listNetworkCatalog);
  // 手上的答复认的是此刻这一问的条件签名(族在其中),答的一定是当前族,不会把上一族的行套进这一族的列。
  const rows =
    answer?.kind === 'outcome'
      ? answer.body.versions.map((record) => rowValues(familyId, record))
      : [];

  return (
    <ListPageTemplate<CatalogRow>
      title={info.title}
      description={`${info.owner}——逐族查阅版本原文,不选版、不折叠为路由判断`}
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: keywordPlaceholders[familyId],
      }}
      filters={
        <>
          {networkCatalogFamilies.map((candidate) => (
            <button
              key={candidate}
              type="button"
              className={chipClass(candidate === familyId)}
              onClick={() => setFamilyId(candidate)}
            >
              {familyLabels[candidate]}
            </button>
          ))}
        </>
      }
      filterSummary={
        // 计数只在拿到业务答案后显示:未配置/错误态下「0 个版本」会与状态区
        // 「这不是目录为空」的说明自相矛盾(与 pricing 两页同一守卫)。
        answer?.kind === 'outcome'
          ? `${familyLabels[familyId]} 本页 ${rows.length} 个版本`
          : undefined
      }
      columns={familyColumns[familyId]}
      rows={rows}
      rowKey={(row) => row.key}
      emptyRowsNote={`当前检索条件下没有匹配的${familyLabels[familyId]}版本`}
      pagination={pagination}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /network-catalog?family=${familyId}`,
        emptyTitle: `当前租户尚无${familyLabels[familyId]}版本`,
        emptyDescription: '读取入口已配置,但该目录族为空;页面不会借其他族的数据补位。',
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

// 登记签装本页读签的同六族——服务区域的登记面随它的读面归专页,写签不比读签多铺一族:
// 那会让同一本册在两处都能登,而其中一处的页面上根本看不到登进去的结果。
const registrationTargets: RegistrationTarget[] = networkCatalogFamilies.map((family) => ({
  id: family,
  label: familyLabels[family],
  title: registrationTitles[family],
  endpoint: `POST ${networkRegistrationEndpoints[family]}`,
  snapshotHint: registrationSnapshotHints[family],
  submit: (snapshot) => registerNetworkCatalogVersion(family, snapshot),
  outcomeLabels: registrationOutcomeLabels,
  refusalReasonLabels: registrationRefusalReasonLabels,
}));

/**
 * 版本化网络目录:逐族查阅版本原文,外加登记签(ADR-0085,票 admin-write-faces/02
 * 切片 02a)。
 *
 * 登记签不是「新建版本」的表单:目录修订按笔推进,新版翻旧插新、不覆盖行,停用由
 * 可用性调整另族陈述——所以这里只有登记一个动作,没有行级编辑或删除面。墙降之前它
 * 必然答 403「接入渠道未配置」,那是诚实答案;墙降当天在装配点换真 Intake 即点亮,
 * 本页一行不用改。
 */
export function NetworkCatalogPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="catalog" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalog">网络目录</TabsTrigger>
          <TabsTrigger value="register">登记网络目录</TabsTrigger>
        </TabsList>
        <TabsContent
          value="catalog"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <NetworkCatalogTable />
        </TabsContent>
        <TabsContent
          value="register"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <MultiRegistrationPanel
            moduleId="network-catalog"
            targets={registrationTargets}
            problemNote={problemNote}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
