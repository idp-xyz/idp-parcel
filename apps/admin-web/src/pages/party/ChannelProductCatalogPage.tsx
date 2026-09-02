import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { RegistrationPanel } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import {
  commercialRegistrationEndpoints,
  listProductChannelMappings,
  productChannelOutcomeLabels,
  registerCommercial,
  type ProductChannelMappingListResponseBody,
  type ProductChannelMappingRecord,
} from './api';
import { problemNote, registrationSnapshotHints, registrationTitles } from './presentation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['channel-product-catalog'];

// 行对象是产品—渠道映射的最新登记修订（票 admin-remainder-mechanism-batch/02）。
// 骨架期这里列过渠道服务方与商业适用性——渠道产品目录身份正文不在本上下文预造
// （ADR-0072：渠道本体等 PAR-INT-01 的接入证据），映射册只持标识引用，接线时按
// 「不以空列伪装已实现」撤下，渠道本体列随渠道接入另立。
const columns: ListColumn<ProductChannelMappingRecord>[] = [
  {
    id: 'mapping',
    header: '映射标识 / 修订',
    render: (row) => (
      <div className="min-w-40">
        <p className="font-mono font-medium text-idpxyz-text">{row.mappingId}</p>
        <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">r{row.revision}</p>
      </div>
    ),
  },
  {
    id: 'product',
    header: '服务产品版本',
    render: (row) => (
      <div className="min-w-44">
        <p className="font-mono text-idpxyz-text">{row.productObjectId}</p>
        <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">{row.productVersionLabel}</p>
      </div>
    ),
  },
  {
    id: 'channels',
    header: '渠道产品引用',
    // 空数组是显式登记的“未配置”声明（该产品尚无可用渠道候选），如实显示这句话
    // 而不是留白——留白读起来像数据缺件，而这格恰恰是登记者说出的内容。
    render: (row) =>
      row.channels.length > 0 ? (
        <div className="min-w-44 font-mono text-xs">
          {row.channels.map((channel) => (
            <p key={channel}>{channel}</p>
          ))}
        </div>
      ) : (
        <span className="text-idpxyz-textMuted">未配置（尚无可用渠道候选）</span>
      ),
  },
  {
    id: 'basis',
    header: '登记依据',
    className: 'font-mono text-xs',
    render: (row) => row.basis,
  },
  {
    id: 'effective',
    header: '有效区间',
    className: 'min-w-64 font-mono text-xs',
    render: (row) => formatRange(row.effectiveStartsAt, row.effectiveEndsAt),
  },
  {
    id: 'registered-at',
    header: '登记时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.registeredAt),
  },
];

/**
 * 渠道产品目录（party-commercial）。行对象是产品—渠道映射的最新登记修订：服务产品
 * 版本 × 渠道产品标识引用 × 有效区间。映射只定义新渠道决策的候选范围，不把候选
 * 伪装成实际选择——某次交易实际用了哪个渠道由拥有该交易的上下文记录；调整绑定或
 * 区间形成新修订，不覆盖历史。
 */
function ProductChannelMappingTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<ProductChannelMappingListResponseBody> | null>(
    null,
  );

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listProductChannelMappings().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const mappings = answer?.kind === 'outcome' ? answer.body.mappings : [];
  const needle = search.trim().toLowerCase();
  // 过滤只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约。
  const visibleMappings = needle
    ? mappings.filter((row) =>
        [row.mappingId, row.productObjectId, row.productVersionLabel, row.basis, ...row.channels]
          .some((value) => value.toLowerCase().includes(needle)),
      )
    : mappings;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<ProductChannelMappingRecord>
      title={info.title}
      description={`${info.owner}——行对象是产品—渠道映射的最新登记修订，渠道以标识引用，渠道本体待接入`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索映射、服务产品或渠道引用',
      }}
      filterSummary={
        // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 笔」会与状态区
        // 「这不是目录为空」直接矛盾（README 列表页上列通则第六条）。
        answer?.kind === 'outcome' ? `当前返回 ${mappings.length} 笔映射` : undefined
      }
      columns={columns}
      rows={visibleMappings}
      rowKey={(row) => row.mappingId}
      viewState={catalogueViewState(answer, mappings.length, retry, {
        module: info,
        endpoint: 'GET /commercial-product-channel-mappings',
        emptyTitle: '当前租户尚无产品—渠道映射登记',
        emptyDescription:
          '读取入口已配置，但登记册为空；页面不会预置映射或渠道候选。登记可走本页「登记」签，或受控 CLI parcel-commercial register-products。',
      })}
    />
  );
}

/**
 * 渠道产品目录：查阅映射册，外加登记签（ADR-0085，票 admin-write-faces/02 切片 02c）。
 *
 * 登记签不是「编辑映射」的表单：映射按修订推进，调整绑定或区间翻旧插新、不覆盖行，
 * 改指产品则是另一笔映射——所以这里只有登记一个动作，没有行级编辑或删除面。墙降之前
 * 它必然答 403「接入渠道未配置」，那是诚实答案；墙降当天在装配点换真 Intake 即点亮，
 * 本页一行不用改。
 */
export function ChannelProductCatalogPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="catalog" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalog">渠道产品目录</TabsTrigger>
          <TabsTrigger value="register">登记映射</TabsTrigger>
        </TabsList>
        <TabsContent
          value="catalog"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <ProductChannelMappingTable />
        </TabsContent>
        <TabsContent
          value="register"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <RegistrationPanel
            moduleId="channel-product-catalog"
            title={registrationTitles['product-channel-mapping']}
            endpoint={`POST ${commercialRegistrationEndpoints['product-channel-mapping']}`}
            snapshotHint={registrationSnapshotHints['product-channel-mapping']}
            submit={(snapshot) => registerCommercial('product-channel-mapping', snapshot)}
            outcomeLabels={productChannelOutcomeLabels}
            problemNote={problemNote}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
