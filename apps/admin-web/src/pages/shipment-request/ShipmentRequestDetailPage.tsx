import { useEffect, useState } from 'react';
import {
  DetailPageTemplate,
  type DetailField,
  type DetailMeta,
  type DetailSection,
  type DetailSummaryStat,
  type TemplateViewState,
} from '../../templates';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@idpxyz/ui-primitives';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails, type TechnicalRow } from '../../components/TechnicalDetails';
import { objectNouns } from '../../vocabulary/objects';
import { problemNote } from './presentation';
import {
  findShipmentRequestView,
  findShipmentRequestViewsByDocumentNumber,
  REQUEST_NOT_VISIBLE_CODE,
  type ApiResult,
  type ShipmentRequestDetail,
  type ViewDetailResponseBody,
} from './api';
import { shipmentDetailSourceOf, type ShipmentRef } from './shipment-ref';
import { customerDisplayOf, type CustomerNameIndex } from './shipment-request-list';
import { useCustomerNames } from './use-customer-names';
import { formatInstant } from '../moment';
import { contentCategoryLabel, goodsLineRowsOf, packingSummaryOf } from './goods-view';
import { documentNumberAbsenceNoteOf, shipmentDisplayNoOf, shipmentRequestIdsOfDisplay } from './ref-no';
import { documentTypeWords } from '../../vocabulary/document-numbering';

// 运单详情（领域：委托查阅），对 GET /shipment-request-views?shipmentRequestId=… 真实端点取数。地址第二段是屏上那个号——有运单号
// 是运单号、老运单是去前缀的委托标识，壳层的标签、状态栏与最近打开照地址写字；打开时按运单号与这个号可能对应的委托标识一起
// 认出是哪一份（shipment-ref.ts 的 shipmentDetailSourceOf）。
//
// 字段与区块跟已落地的查询契约走（api.ts 的 ShipmentRequestDetail 镜像），读模型没有的东西（目的地、收件人、接受基线快照）
// 不虚构；那些查阅面扩进读模型时，先扩 ports 的记录与端点响应，再回来加区块。
//
// 改版第一步（设计原则三、四）：首屏只放人要看的——客户、客户单号、件数、状态、几个时刻；委托标识、客户账户编码、提交批次、
// 提交主体、来源请求键、提交版本、续办引用、决定标识全部折进页末的「技术详情」。状态经用语词典成中文胶囊，不再带英文原码后缀。
// 页头的号是运单号即单据号（ADR-0355 决定十一）；没有单据号的委托页头照旧显委托标识，技术详情首行注明为什么没有。
//
// 「统一不可见结果」（CONTEXT.md）：单份查阅的 404 SHIPMENT_REQUEST_NOT_VISIBLE 是终局业务答案，不存在、越权与其他租户对象同一
// 语义——本页对它只说「找不到」，不按取数失败原因分辨呈现「不存在」与「无权查看」，也不提供重试。

function buildMeta(view: ShipmentRequestDetail): DetailMeta[] {
  return [
    { label: objectNouns.submittedAt, value: <span className="font-mono">{formatInstant(view.submittedAt)}</span> },
    { label: '系统接收时间', value: <span className="font-mono">{formatInstant(view.receivedAt)}</span> },
  ];
}

// 指标带只放读口原样给的计数：件数取读模型的 declaredParcelCount 而不是数 declaredParcels 的长度——
// 两者今天相等，但前者是服务端的陈述，后者是页面自己算的。
function buildSummary(view: ShipmentRequestDetail): DetailSummaryStat[] {
  return [
    { label: objectNouns.parcelCount, value: view.declaredParcelCount },
    // 此前版本数是「同一委托边界内纠错 / 补充」的痕迹计数，0 表示首版即当前版。
    { label: '修改过几次', value: view.priorVersionCount },
  ];
}

function buildBasicFields(view: ShipmentRequestDetail, names: CustomerNameIndex): DetailField[] {
  const customer = customerDisplayOf(view, names);
  return [
    { label: objectNouns.customer, value: customer.isCode ? <span className="font-mono">{customer.text}</span> : customer.text },
    { label: objectNouns.customerReference, value: <span className="font-mono">{view.sourceRequestKey}</span> },
    { label: objectNouns.status, value: <StatusPill code={view.state} /> },
    { label: objectNouns.parcelCount, value: String(view.declaredParcelCount) },
    { label: objectNouns.submittedAt, value: <span className="font-mono">{formatInstant(view.submittedAt)}</span> },
    { label: '客户系统发出时间', value: <span className="font-mono">{formatInstant(view.occurredAt)}</span> },
    { label: '系统接收时间', value: <span className="font-mono">{formatInstant(view.receivedAt)}</span> },
  ];
}

function technicalRowsOf(view: ShipmentRequestDetail): TechnicalRow[] {
  const absenceNote = documentNumberAbsenceNoteOf(view);
  return [
    ...(absenceNote ? [{ label: documentTypeWords.SHIPMENT.numberName, value: absenceNote }] : []),
    { label: '委托标识', value: view.shipmentRequestId },
    { label: '客户账户编码', value: view.customerAccountId },
    { label: '提交批次', value: view.batchId },
    { label: '提交主体', value: view.source },
    { label: '来源请求键', value: view.sourceRequestKey },
    { label: '当前提交版本', value: view.submissionVersionId },
    { label: '续办引用', value: view.acceptanceTask.lastAttemptContinuation ?? '' },
    { label: '决定标识', value: view.decision?.decisionId ?? '' },
  ];
}

// 货物明细与平台货件两个区块（ADR-0373、ADR-0374，票 declared-goods-lines/04）。没报就不出区块，不显一个空表。
function goodsSectionsOf(view: ShipmentRequestDetail): DetailSection[] {
  const sections: DetailSection[] = [];
  if (view.platformShipment) {
    sections.push({
      id: 'platform-shipment',
      title: '平台货件',
      description: '客户报的平台货件：一个平台货件一张运单，每箱的平台箱标号在上面的包裹表里。号由客户交来，本系统不向平台核对。',
      content: (
        <dl className="space-y-1">
          <FieldRow label="平台" value={view.platformShipment.platform} />
          <FieldRow label={objectNouns.platformShipmentNumber} value={view.platformShipment.shipmentId} mono />
          <FieldRow label={objectNouns.platformReferenceNumber} value={view.platformShipment.referenceId ?? '—'} mono />
        </dl>
      ),
    });
  }
  if (view.goods) {
    const goods = view.goods;
    const rows = goodsLineRowsOf(goods);
    const category = contentCategoryLabel(goods.contentCategory);
    sections.push({
      id: 'goods-details',
      title: objectNouns.goodsDetails,
      description: '客户申报的货物，每行一种货，值按客户给的原样显示、不换算；取自箱单或商品申报资料的格在行下注明。',
      content: (
        <div className="space-y-2">
          {category && (
            <dl className="space-y-1">
              <FieldRow
                label={objectNouns.contentCategory}
                value={goods.contentCategoryNote ? `${category}（${goods.contentCategoryNote.value}）` : category}
              />
            </dl>
          )}
          {rows.length > 0 && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>行</TableHead>
                  <TableHead>品名</TableHead>
                  <TableHead>数量</TableHead>
                  <TableHead>单价</TableHead>
                  <TableHead>行总价</TableHead>
                  <TableHead>币种</TableHead>
                  <TableHead>净重</TableHead>
                  <TableHead>编码</TableHead>
                  <TableHead>原产国</TableHead>
                  <TableHead>SKU</TableHead>
                  <TableHead>销售订单</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((row) => (
                  <TableRow key={row.line}>
                    <TableCell>{row.line}</TableCell>
                    <TableCell>
                      <div>{row.description}</div>
                      {row.sourceNotes.length > 0 && (
                        <div className="type-caption text-idpxyz-textMuted">{row.sourceNotes.join('；')}</div>
                      )}
                    </TableCell>
                    <TableCell>{row.quantity}</TableCell>
                    <TableCell className="font-mono">{row.unitValue}</TableCell>
                    <TableCell className="font-mono">{row.lineValue}</TableCell>
                    <TableCell>{row.currency}</TableCell>
                    <TableCell>{row.netWeight}</TableCell>
                    <TableCell className="font-mono">{row.commodityCodes}</TableCell>
                    <TableCell>{row.originCountry}</TableCell>
                    <TableCell className="font-mono">{row.sku}</TableCell>
                    <TableCell className="font-mono">{row.salesOrder}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </div>
      ),
    });
  }
  return sections;
}

function buildSections(view: ShipmentRequestDetail): DetailSection[] {
  const sections: DetailSection[] = [
    {
      id: 'declared-parcels',
      title: objectNouns.parcel,
      description:
        '客户申报的每一件：重量与尺寸按客户填的原样显示；受理后件数固定，增减要走取消或新建关联运单。「装箱」一列是这一箱装了货物明细的哪几行，没报即整票清单。',
      content: (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{objectNouns.parcelNumber}</TableHead>
              <TableHead>{objectNouns.customerParcelNumber}</TableHead>
              <TableHead>{objectNouns.platformBoxLabel}</TableHead>
              <TableHead>申报重量</TableHead>
              <TableHead>申报尺寸</TableHead>
              <TableHead>{objectNouns.packing}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {view.declaredParcels.map((parcel) => (
              <TableRow key={parcel.parcelId}>
                <TableCell>
                  <span className="font-mono type-body">{parcel.parcelId}</span>
                </TableCell>
                <TableCell>
                  <span className="font-mono type-body">{parcel.customerParcelNumber ?? '—'}</span>
                </TableCell>
                <TableCell>
                  <span className="font-mono type-body">{parcel.platformBoxLabel ?? '—'}</span>
                </TableCell>
                <TableCell>
                  {parcel.declaredWeightValue ? `${parcel.declaredWeightValue} ${parcel.declaredWeightUnit ?? ''}` : '未填'}
                </TableCell>
                <TableCell>
                  {parcel.dimensions
                    ? `${parcel.dimensions.length} × ${parcel.dimensions.width} × ${parcel.dimensions.height} ${parcel.dimensions.unit}`
                    : '未填'}
                </TableCell>
                <TableCell>{packingSummaryOf(parcel.packing) || '—'}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ),
    },
    ...goodsSectionsOf(view),
    {
      id: 'acceptance-task',
      title: '受理进度',
      description: '受理到了哪一步，以及最近一次没能推进的原因——只留成功那半，卡了十轮的运单看起来会和刚建的一样。',
      content: (
        <dl className="space-y-1">
          <FieldRow label="进度" value={<StatusPill code={view.acceptanceTask.state} />} />
          {view.acceptanceTask.lastAttemptReason !== undefined && (
            <FieldRow label="最近处理结果" value={view.acceptanceTask.lastAttemptReason} />
          )}
          {view.acceptanceTask.lastAttemptedAt !== undefined && (
            <FieldRow label="最近处理时间" value={formatInstant(view.acceptanceTask.lastAttemptedAt)} mono />
          )}
        </dl>
      ),
    },
  ];

  if (view.decision) {
    sections.push({
      id: 'decision',
      title: '受理决定',
      description: '决定一经形成不会改写；之后要停掉这张运单，走取消而不是撤回。',
      content: (
        <dl className="space-y-1">
          <FieldRow label="决定" value={<StatusPill code={view.decision.kind} />} />
          <FieldRow label="决定时间" value={formatInstant(view.decision.decidedAt)} mono />
        </dl>
      ),
    });
  }

  sections.push({
    id: 'technical',
    title: objectNouns.technicalDetails,
    description: '对接、排查或对日志时用的编号；日常不用看。',
    content: <TechnicalDetails rows={technicalRowsOf(view)} />,
  });

  return sections;
}

/** 区块内的键值行，排版对齐 DetailPageTemplate 基本信息区。 */
function FieldRow({ label, value, mono }: { label: string; value: React.ReactNode; mono?: boolean }) {
  return (
    <div className="flex gap-3 type-body leading-5">
      <dt className="w-[128px] shrink-0 text-idpxyz-textMuted">{label}</dt>
      <dd className={`min-w-0 flex-1 break-words text-idpxyz-text ${mono ? 'font-mono' : ''}`}>{value}</dd>
    </div>
  );
}

/** 打开详情读到的东西：详情答案，或地址里那个号对上了不止一张运单。 */
type DetailLoad = { kind: 'detail'; answer: ApiResult<ViewDetailResponseBody> } | { kind: 'ambiguous'; candidates: ShipmentRef[] };

async function loadShipmentDetail(shipmentRef: string): Promise<DetailLoad> {
  const [byNumber, byIds] = await Promise.all([
    findShipmentRequestViewsByDocumentNumber(shipmentRef),
    Promise.all(shipmentRequestIdsOfDisplay(shipmentRef).map((id) => findShipmentRequestView(id))),
  ]);
  const source = shipmentDetailSourceOf(shipmentRef, byNumber, byIds);
  if (source.kind === 'byId') return { kind: 'detail', answer: await findShipmentRequestView(source.shipmentRequestId) };
  return source;
}

// 取数答案 → 模板页面态。不可见走空态（终局答案、无重试）；其余错误格与列表页同款。
function viewStateOf(loaded: DetailLoad | null, shipmentRef: string, retry: () => void): TemplateViewState {
  if (loaded === null) return { kind: 'loading' };
  if (loaded.kind === 'ambiguous') {
    // 从列表再点「打开详情」写的还是这个号，会回到这一格；完整委托标识（SHR- 开头）只对得上一张，所以指那条路。
    return {
      kind: 'error',
      title: '对上了不止一张运单',
      description:
        `「${shipmentRef}」对上了不止一张运单（例如一张的${documentTypeWords.SHIPMENT.numberName}与另一张的委托标识写出来相同），没法替你选。` +
        '下面技术详情列出了这几张的完整委托标识（SHR- 开头）：把地址栏最后一段换成其中一个即可打开那一张。',
      technical: loaded.candidates.map((candidate) => ({
        label: candidate.documentNumber ? `委托标识（${documentTypeWords.SHIPMENT.numberName} ${candidate.documentNumber}）` : '委托标识',
        value: candidate.shipmentRequestId,
      })),
    };
  }
  const answer = loaded.answer;
  switch (answer.kind) {
    case 'outcome':
      return { kind: 'ready' };
    case 'unconfigured':
      return {
        kind: 'unconfigured',
        title: '接入渠道未配置',
        description: '运单查询的接入渠道还没配置好，这不是故障：重试不会改变结果，需要先完成渠道配置。',
      };
    case 'loginRequired':
    case 'notGranted':
      return { kind: answer.kind };
    case 'callerProblem':
      if (answer.code === REQUEST_NOT_VISIBLE_CODE) {
        return {
          kind: 'empty',
          title: '找不到这张运单',
          description: '在你能看到的范围内找不到这张运单。请从运单列表重新打开；不存在与无权查看在这里是同一个答案。',
        };
      }
      return {
        kind: 'error',
        title: '这次查询没有被接受',
        description: problemNote(answer.code),
        technical: [
          { label: 'HTTP 状态', value: String(answer.status) },
          { label: '问题码', value: answer.code },
        ],
      };
    case 'noAnswer':
      return {
        kind: 'error',
        title: '这次没有取到运单',
        description: problemNote(answer.code),
        onRetry: retry,
        technical: [
          { label: 'HTTP 状态', value: String(answer.status) },
          { label: '问题码', value: answer.code },
        ],
      };
    case 'transport':
      return {
        kind: 'error',
        title: '连不上服务',
        description: '请求没有到达后端；请确认网络与服务在运行后重试。',
        onRetry: retry,
        technical: [{ label: '传输层', value: answer.message }],
      };
  }
}

export function ShipmentRequestDetailPage({
  shipmentRef,
  onBack,
}: {
  /** 地址里带的号：有运单号是运单号，老运单是委托标识。本页自取数，号只定位对象，不单独证明查询权限。 */
  shipmentRef: string;
  onBack?: () => void;
}) {
  const [loaded, setLoaded] = useState<DetailLoad | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  const names = useCustomerNames();

  useEffect(() => {
    let cancelled = false;
    setLoaded(null);
    void loadShipmentDetail(shipmentRef).then((result) => {
      if (!cancelled) setLoaded(result);
    });
    return () => {
      cancelled = true;
    };
  }, [shipmentRef, reloadToken]);

  const view = loaded?.kind === 'detail' && loaded.answer.kind === 'outcome' ? loaded.answer.body.request : undefined;

  return (
    <DetailPageTemplate
      title={objectNouns.shipment}
      identifier={view ? shipmentDisplayNoOf(view) : shipmentRef}
      status={view ? <StatusPill code={view.state} /> : undefined}
      description="这张运单的客户、件数、受理进度与决定。物流动态在「物流动态」页按包裹号查。"
      headerActions={
        onBack ? (
          <button
            type="button"
            onClick={onBack}
            className="rounded border border-idpxyz-border px-3 py-1.5 type-body text-idpxyz-textMuted hover:bg-idpxyz-hover"
          >
            返回列表
          </button>
        ) : undefined
      }
      meta={view ? buildMeta(view) : []}
      summary={view ? buildSummary(view) : undefined}
      basicFields={view ? buildBasicFields(view, names) : []}
      sections={view ? buildSections(view) : undefined}
      // 签只有「概要」（基本信息 + 现有区块）：委托的事件流与审计留痕都没有面向 UI 的读口，「时间线」「审计」两签无内容不出，
      // 不为凑签造读口。
      tabs={[]}
      viewState={viewStateOf(loaded, shipmentRef, () => setReloadToken((token) => token + 1))}
    />
  );
}
