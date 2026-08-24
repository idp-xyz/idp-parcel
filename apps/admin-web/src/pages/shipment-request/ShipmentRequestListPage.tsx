import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { StatusBadgeFor, type DomainStatus } from '../../domain/status';
import { moduleInfoById } from '../../navigation';
import { requestStateLabels } from './presentation';
import { ShipmentRequestDetailPage } from './ShipmentRequestDetailPage';

// 页面标题的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['shipment-request-inquiry'];

// 委托查阅列表。栏目取 parcel-shipment CONTEXT.md 与 UC-PS-001 的原词;查询端点
// 未建(adapters/http 现仅有提交与撤回两个动作端点),数据区如实呈现「未配置」态,
// 不发请求、不含合成数据。
//
// 接线时的两条查询语义红线,出处均为 CONTEXT.md,先钉在这里免得接线的人现找:
// 「授权查询作用域」——查询只消费共享身份/授权能力传入的作用域引用,本页不自建
// 跨客户查询层;「统一不可见结果」——不存在、越权与其他租户对象对外同一语义,
// 列表与详情都不得区分呈现。

/**
 * 列表行形状。查询契约未建,这是页面侧暂定,字段跟着栏目走;真契约落地时以
 * 它为准重谈,不得反过来把这里当已发布的查询 Schema(与 api.ts 草案同一态度)。
 */
export interface ShipmentRequestListRow {
  /** 委托标识(UC-PS-001 已接受结果的「委托与包裹内部标识」)。 */
  shipmentRequestId: string;
  /** 客户委托参考(UC-PS-001 输入语义契约·服务请求组)。 */
  customerShipmentReference: string;
  /** 委托状态(domain.ShipmentRequestState 的字符串,词表见 presentation)。 */
  state: string;
  /** 请求的服务产品或服务要求(UC-PS-001 服务请求组)。 */
  requestedServiceProduct: string;
  /** 目的服务范围(UC-PS-001 寄收件范围组)。 */
  destinationServiceScope: string;
  /** 声明包裹件数(CONTEXT「委托接受时至少包含一个客户声明包裹」)。 */
  declaredParcelCount: number;
  /** 系统接收时间 receivedAt(CONTEXT 来源信封元数据);展示格式化归接线时定。 */
  receivedAt: string;
}

// 状态徽章:词表词取自 domain/status 的领域状态词表(两处词同源于 CONTEXT 原词,
// 断言只桥接两个模块的类型边界);词表没收录的状态原样示码,不猜色调。
function requestStateBadge(state: string) {
  const word = requestStateLabels[state];
  return word !== undefined ? (
    <StatusBadgeFor status={word as DomainStatus} />
  ) : (
    <span className="font-mono text-[12px] text-idpxyz-textMuted">{state}</span>
  );
}

const columns: ListColumn<ShipmentRequestListRow>[] = [
  {
    id: 'shipment-request-id',
    header: '委托标识',
    className: 'w-[180px]',
    render: (row) => (
      <span className="font-mono text-[12px] text-idpxyz-accent">
        {row.shipmentRequestId}
      </span>
    ),
  },
  {
    id: 'customer-shipment-reference',
    header: '客户委托参考',
    render: (row) => <span className="font-mono text-[12px]">{row.customerShipmentReference}</span>,
  },
  {
    id: 'state',
    header: '委托状态',
    className: 'w-[112px]',
    render: (row) => requestStateBadge(row.state),
  },
  {
    id: 'requested-service-product',
    header: '请求的服务产品',
    render: (row) => row.requestedServiceProduct,
  },
  {
    id: 'destination-service-scope',
    header: '目的服务范围',
    render: (row) => row.destinationServiceScope,
  },
  {
    id: 'declared-parcel-count',
    header: '声明包裹',
    align: 'right',
    className: 'w-[88px]',
    render: (row) => row.declaredParcelCount,
  },
  {
    id: 'received-at',
    header: '系统接收时间',
    className: 'w-[168px]',
    render: (row) => <span className="font-mono text-[12px]">{row.receivedAt}</span>,
  },
];

export function ShipmentRequestListPage() {
  const [keyword, setKeyword] = useState('');
  // 钻取选中:列表与详情共用一个导航位,选中后整区切详情,返回键回列表。
  const [selectedId, setSelectedId] = useState<string | null>(null);

  if (selectedId !== null) {
    return (
      <ShipmentRequestDetailPage
        shipmentRequestId={selectedId}
        onBack={() => setSelectedId(null)}
      />
    );
  }

  return (
    <ListPageTemplate<ShipmentRequestListRow>
      title={info.title}
      description="小包托运(parcel-shipment)的委托与其生命周期结果,查询按授权查询作用域过滤。"
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: '按委托标识或客户委托参考检索',
      }}
      columns={columns}
      rows={[]}
      rowKey={(row) => row.shipmentRequestId}
      onRowClick={(row) => setSelectedId(row.shipmentRequestId)}
      viewState={{
        kind: 'unconfigured',
        title: '委托查询端点尚未建立',
        description:
          '后端现仅提供提交与决定前撤回两个动作端点,委托查阅的查询契约(含授权查询作用域' +
          '与统一不可见结果语义)待建。本页不发请求、不含合成数据;栏目骨架已按' +
          ' parcel-shipment CONTEXT 与 UC-PS-001 原词搭好。',
      }}
    />
  );
}
