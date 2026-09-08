import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate } from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState } from '../catalogue-view';
import { listSupplierAgreements, type SupplierAgreementListResponseBody } from './api';
import {
  supplierAgreementColumns,
  supplierAgreementRowsOf,
  type SupplierAgreementRow,
} from './supplier-agreement-rows';
import { SupplierAgreementPublicationForm } from './SupplierAgreementPublicationForm';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['supplier-agreements'];

// 列与行转写在 supplier-agreement-rows.ts（票 admin-write-faces/19）：版本壳之外，0021 正文逐键上列，正文在不在由
// 服务端的 contentRegistered 说，不拿正文键的有无去推。壳在正文缺是合法状态——壳可先入册，正文随「发布协议版本」
// 签登记——目录里两态各有各的样子（「未登记」/「已登记」），布尔为真而键缺则如实点名为响应不合契约。
// 骨架期这里列过的结算条件仍不上：0021 正文里没有它（结算方式归「商业规则与策略」页的结算政策册），读口没有透，
// 不是转写漏了；采购价格条件在正文里是采购定价方案的引用串，按引用上列、不读方案内容。

// 目录读面。reloadToken 由页面在发布签落定后递增，让同页目录立刻刷新可见（票 admin-write-faces/11 完成判据）；
// 自己的「重试」另有一把键，两把任一变都重取。
function SupplierAgreementCatalogue({ reloadToken }: { reloadToken: number }) {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<SupplierAgreementListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listSupplierAgreements().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey, reloadToken]);

  const rows = answer?.kind === 'outcome' ? supplierAgreementRowsOf(answer.body) : [];
  const needle = search.trim().toLowerCase();
  // 过滤只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约。搜的是转写后的各格，
  // 正文列（供应商、方案引用）因此也搜得到。
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<SupplierAgreementRow>
      title={info.title}
      description={`${info.owner}——行对象是供应商协议版本壳与已登记正文（供应商、责任法人、采购方案引用、协议范围与区间）；壳可先入册、正文随发布登记，只有壳的行正文列显「未登记」`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索协议、版本、范围、供应商或方案引用',
      }}
      filterSummary={
        // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 份」会与状态区
        // 「这不是目录为空」直接矛盾（README 列表页上列通则「计数摘要必须带 outcome 守卫」）。
        answer?.kind === 'outcome' ? `当前返回 ${rows.length} 份协议版本` : undefined
      }
      columns={supplierAgreementColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /commercial-supplier-agreements',
        emptyTitle: '当前租户尚无供应商协议版本',
        emptyDescription: '读取入口已配置，但目录为空；页面不会预置供应商或采购条件。',
      })}
    />
  );
}

/**
 * 供应商协议（party-commercial）。行对象是供应商商业协议版本：它定义可复用的
 * 采购条件、价格和结算责任，不等于一次实际运输委托、订舱、履约事实或供应商
 * 账单——实际委托快照归 transport-fulfillment，成本与应付归 settlement-accounting。
 *
 * 两签：目录读面，与「发布协议版本」的逐字段表单（票 admin-write-faces/11；ADR-0101 决定八）。写签跟着读签走：
 * 发布落定的版本就在左签目录里，页面借 reloadToken 让它立刻可见。受控发布的 JSON 镜像签留在「商业规则与策略」页
 * 作高级口，本页不重复摆。
 */
export function SupplierAgreementsPage() {
  const [reloadToken, setReloadToken] = useState(0);
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="catalogue" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalogue">协议目录</TabsTrigger>
          <TabsTrigger value="publish">发布协议版本</TabsTrigger>
        </TabsList>
        <TabsContent
          value="catalogue"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <SupplierAgreementCatalogue reloadToken={reloadToken} />
        </TabsContent>
        <TabsContent
          value="publish"
          className="flex-1 flex flex-col overflow-auto data-[state=inactive]:hidden"
        >
          <SupplierAgreementPublicationForm onPublished={() => setReloadToken((value) => value + 1)} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
