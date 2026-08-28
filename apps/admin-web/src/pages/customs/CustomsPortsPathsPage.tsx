import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatRange } from '../catalogue-view';
import {
  listPortsPaths,
  type CandidatePortListResponseBody,
  type DeclarationPathListResponseBody,
} from './api';
import { directionLabels, labelOf, portsPathsRegistryLabels } from './presentation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['customs-ports-paths'];

// 本页三签（票 admin-remainder-mechanism-batch/03）：
//
// 已接线的是目录登记册查阅两签——「合规候选口岸」（口岸标识 + 生效区间）与「申报
// 路径」（路径标识 + 三维路径事实 + 生效区间），都接 GET /customs-ports-paths 按
// registry 分派。两册按（键，生效起点）版本化，全部版本连同区间原样上列，区间判读
// 归读者（端点不收评估时点参数，裁决在 query_ports_paths.go 文件头）。
//
// 「合规候选区域」保持如实占位：区域维未建模（等自己的票），本页不为它虚构列表。
// 旧骨架里口岸行的「所属区域」「关务适用性判断」「限制及解除结果」三列随建模落地
// 撤下——目录事实只有键与区间，适用性是判断链的产物、限制归 customs-restrictions
// 页，都不是本册的列；申报路径的「报关服务方」同理不在三维之内（改路硬句管的是
// 换维须重新请求适用性判断，那是判断链的规则，不是目录字段）。

interface CatalogueRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

function col(id: string, header: string, mono = false): ListColumn<CatalogueRow> {
  return {
    id,
    header,
    className: mono ? 'font-mono text-xs' : undefined,
    render: (row) => row.values[id] ?? '—',
  };
}

// —— 合规候选口岸签（接真面）——

// 口岸册一行两件事实：哪个口岸、哪个生效区间。同口岸多版本逐行在列（当前版终点
// 显示「持续有效」），版本序由服务端定（键升序、版本新在前），页面不重排。
const portColumns: ListColumn<CatalogueRow>[] = [
  col('port', '口岸标识', true),
  col('applies', '生效区间', true),
];

function portRows(body: CandidatePortListResponseBody): CatalogueRow[] {
  return body.ports.map((record) => ({
    // 行键循库主键 (tenant, port_ref, applies_from)：同口岸各版本起点唯一。
    key: `port:${record.port}:${record.appliesFrom}`,
    values: {
      port: record.port,
      applies: formatRange(record.appliesFrom, record.appliesUntil),
    },
  }));
}

function CandidatePortsTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<CandidatePortListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listPortsPaths('candidate-port').then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const rows = answer?.kind === 'outcome' ? portRows(answer.body) : [];
  const needle = search.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<CatalogueRow>
      title="合规候选口岸"
      description={`${info.owner}——一行即「该口岸在该生效区间内是本租户的合规候选」；所属区域与关务适用性判断不在册`}
      search={{ value: search, onChange: setSearch, placeholder: '搜索口岸标识' }}
      filterSummary={
        answer?.kind === 'outcome'
          ? `${portsPathsRegistryLabels['candidate-port']} ${rows.length} 版`
          : undefined
      }
      columns={portColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /customs-ports-paths?registry=candidate-port',
        emptyTitle: '当前租户尚无已登记的合规候选口岸',
        emptyDescription:
          '读取入口已配置,但口岸目录为空;真实口岸参数属实例半边,页面不会预置口岸。登记走受控 CLI(parcel-customs-register candidate-port)。',
      })}
    />
  );
}

// —— 申报路径签（接真面）——

// 路径册一行四件事实：哪条路径、经哪个口岸、按哪个方向、以哪种申报模式，外加生效
// 区间。口岸列是标识引用——「引用的口岸此刻是否在册」是读者拿两签对照的判断，本列
// 不代答。
const pathColumns: ListColumn<CatalogueRow>[] = [
  col('path', '申报路径标识', true),
  col('port', '经由口岸', true),
  col('direction', '进出口方向'),
  col('mode', '申报模式', true),
  col('applies', '生效区间', true),
];

function pathRows(body: DeclarationPathListResponseBody): CatalogueRow[] {
  return body.paths.map((record) => ({
    key: `path:${record.path}:${record.appliesFrom}`,
    values: {
      path: record.path,
      port: record.port,
      direction: labelOf(directionLabels, record.direction),
      mode: record.declarationMode,
      applies: formatRange(record.appliesFrom, record.appliesUntil),
    },
  }));
}

function DeclarationPathsTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<DeclarationPathListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listPortsPaths('declaration-path').then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const rows = answer?.kind === 'outcome' ? pathRows(answer.body) : [];
  const needle = search.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<CatalogueRow>
      title="申报路径"
      description={`${info.owner}——目录只登路径三维事实，不做路由选择；路由在合格候选中选择是 network-routing 的事`}
      search={{ value: search, onChange: setSearch, placeholder: '搜索申报路径 / 口岸 / 申报模式' }}
      filterSummary={
        answer?.kind === 'outcome'
          ? `${portsPathsRegistryLabels['declaration-path']} ${rows.length} 版`
          : undefined
      }
      columns={pathColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /customs-ports-paths?registry=declaration-path',
        emptyTitle: '当前租户尚无已登记的申报路径',
        emptyDescription:
          '读取入口已配置,但申报路径目录为空;真实路径参数属实例半边,页面不会预置路径。登记走受控 CLI(parcel-customs-register declaration-path)。',
      })}
    />
  );
}

// —— 合规候选区域签（如实占位）——

/**
 * 区域行字段取 CONTEXT-MAP「customs-compliance ↔ network-routing」协作句原词。
 * 区域维未建模，栏目只取那句出现过的词，不虚构字段；建模落地时以真实定义为准重谈。
 */
const regionColumns: ListColumn<CatalogueRow>[] = [
  col('code', '区域标识', true),
  col('applicability', '关务适用性判断'),
  col('restrictions', '限制及解除结果（引用）', true),
];

function CandidateRegionsTable() {
  const [search, setSearch] = useState('');

  return (
    <ListPageTemplate<CatalogueRow>
      title="合规候选区域"
      description={`${info.owner}——关务提供合规候选区域，路由只在合格候选中选择`}
      search={{ value: search, onChange: setSearch, placeholder: '按区域标识检索' }}
      columns={regionColumns}
      rows={[]}
      rowKey={(row) => row.key}
      viewState={{
        kind: 'unconfigured',
        title: '合规候选区域维尚未建模',
        description:
          '口岸与申报路径两册已建模接线（左侧两签）；区域维的域模型与持久化未建，本签不发请求、不含未确认参数的默认值。',
        facts: {
          owner: info.owner,
          source: info.source,
          unlock: '区域维建模落地、查询端点建成并经 ADR-0017 准入闸门放行后接线',
        },
      }}
    />
  );
}

/**
 * 口岸与申报路径（customs-compliance）。词取两处原句：CONTEXT-MAP「customs-compliance
 * ↔ network-routing」——关务提供合规候选区域、口岸、申报路径、限制及解除结果，路由
 * 只在合格候选中选择；customs-compliance CONTEXT.md 所有权句——本上下文拥有合规候选
 * 区域、口岸、申报路径和关务适用性判断。
 *
 * 页面只查阅、无任何选择动作；拟改路改变关务区域、口岸或报关服务方时，必须先请求
 * 新的关务适用性判断（CONTEXT-MAP 原句）——那次判断的查阅口随判断链自己的票，不在
 * 本页两册之内。已接线两签在前，未建模的区域签如实占位在后。
 */
export function CustomsPortsPathsPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="ports" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="ports">合规候选口岸</TabsTrigger>
          <TabsTrigger value="paths">申报路径</TabsTrigger>
          <TabsTrigger value="regions">合规候选区域</TabsTrigger>
        </TabsList>
        <TabsContent value="ports" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CandidatePortsTable />
        </TabsContent>
        <TabsContent value="paths" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <DeclarationPathsTable />
        </TabsContent>
        <TabsContent value="regions" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CandidateRegionsTable />
        </TabsContent>
      </Tabs>
    </div>
  );
}
