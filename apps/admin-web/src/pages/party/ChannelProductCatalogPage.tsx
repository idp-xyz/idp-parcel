import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['channel-product-catalog'];

/**
 * 渠道产品目录列表行。字段取 party-commercial CONTEXT.md「渠道产品」
 * 「渠道约束」定义与渠道商业可用性规则；接线前没有任何实例数据。
 */
export interface ChannelProductRow {
  /** 渠道产品标识：目录身份稳定，不等同于运营企业服务产品、实际承运商或一次具体面单交易。 */
  id: string;
  /** 渠道产品。 */
  name: string;
  /** 渠道服务方：承运商直营渠道、承运商代理商、转售商或聚合平台。 */
  channelProvider: string;
  /**
   * 商业适用性：停止商业可用或相关映射到期只将其排除在新的渠道选择之外，
   * 不删除历史依据，也不自动关闭包裹；恢复可用只恢复候选资格。
   */
  commercialApplicability: string;
  /**
   * 可复用渠道约束：客户合同或客户明确授权对可用渠道范围的限制。约束可以
   * 保留运营企业在允许范围内的选择权，也可以指定一个渠道产品；指定渠道产品
   * 时该次适用选择被锁定，锁定不据此锁定底层承运商或实际承运商。
   */
  reusableConstraints: string;
  /** 适用有效期。 */
  validity: string;
}

const columns: ListColumn<ChannelProductRow>[] = [
  { id: 'id', header: '渠道产品标识', className: 'font-mono', render: (row) => row.id },
  { id: 'name', header: '渠道产品', render: (row) => row.name },
  { id: 'channel-provider', header: '渠道服务方', render: (row) => row.channelProvider },
  { id: 'commercial-applicability', header: '商业适用性', render: (row) => row.commercialApplicability },
  { id: 'reusable-constraints', header: '可复用渠道约束', render: (row) => row.reusableConstraints },
  { id: 'validity', header: '适用有效期', render: (row) => row.validity },
];

/**
 * 渠道产品目录（party-commercial）。行对象是外部渠道产品的目录身份与商业
 * 适用性：渠道产品由渠道服务方提供，party-commercial 决定可复用的候选范围、
 * 合同约束和授权是否有效，但不把候选关系伪装成实际选择——委托级约束快照、
 * 具体渠道选择和锁定结果由拥有该交易的上下文记录。查阅面，不设登记动作。
 */
export function ChannelProductCatalogPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<ChannelProductRow>
      title={info.title}
      description={info.owner}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索渠道产品 / 渠道服务方',
      }}
      columns={columns}
      // 接线前无实例：行数据与总数届时由 party-commercial 应用端口供给。
      rows={[]}
      rowKey={(row) => row.id}
      pagination={{
        page,
        pageSize,
        total: 0,
        onPageChange: setPage,
        onPageSizeChange: setPageSize,
      }}
      viewState={{
        kind: 'unconfigured',
        title: '参与方与商业模块尚未接线',
        description: `业务端点按 ADR-0017 的准入闸门尚未放行，本页不发请求、不含未确认参数的默认值。场景出处：${info.source}`,
      }}
    />
  );
}
