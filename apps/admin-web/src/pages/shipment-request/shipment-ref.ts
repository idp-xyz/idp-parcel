// 撤回与取消表单里「哪一票运单」那一格的解析(纯逻辑,无 React,node:test 钉着)。
//
// 运营手里的是运单号(委托的单据号,ADR-0355 决定十一),而撤回、取消两口照首方契约按委托标识找委托(后续命令不收运单号)。
// 所以提交前先经委托查阅读口把它解析成委托标识:按运单号查(`?documentNumber=`),同时按委托标识查(`?shipmentRequestId=`)——
// 上线前的老运单没有运单号,只能填委托标识。两边对上的委托去重后恰好一份才算解析成;没有、不止一份、读不动各说各的。
//
// 解析只是让运营不必去技术详情里抄委托标识,不是授权:找到了也只说明在本人可见的范围里有这一票,撤得撤不了由撤回口答。

import { accessRefusalHeadline } from '../../auth/access-refusal';
import { documentTypeWords } from '../../vocabulary/document-numbering';
import {
  REQUEST_NOT_VISIBLE_CODE,
  type ApiResult,
  type ShipmentRequestSummary,
  type ViewDetailResponseBody,
  type ViewsListResponseBody,
} from './api';

const numberName = documentTypeWords.SHIPMENT.numberName;

/** 解析到的一票运单:委托标识必在,运单号在有号时在场。 */
export interface ShipmentRef {
  shipmentRequestId: string;
  documentNumber?: string;
}

export type ShipmentRefResolution =
  | { kind: 'resolved'; ref: ShipmentRef }
  | { kind: 'notFound' }
  | { kind: 'ambiguous'; candidates: ShipmentRef[] }
  | { kind: 'unreadable'; reason: string };

function refOf(summary: ShipmentRequestSummary): ShipmentRef {
  return summary.documentNumber === undefined
    ? { shipmentRequestId: summary.shipmentRequestId }
    : { shipmentRequestId: summary.shipmentRequestId, documentNumber: summary.documentNumber };
}

/** 一次读的答案里说不出「有没有」的那几格:读口没形成答案,不能当成「没有」。 */
function unreadableReason<Body>(answer: ApiResult<Body>): string | null {
  switch (answer.kind) {
    case 'outcome':
      return null;
    case 'unconfigured':
    case 'loginRequired':
    case 'notGranted':
      return accessRefusalHeadline(answer.kind);
    case 'callerProblem':
      return answer.code === REQUEST_NOT_VISIBLE_CODE ? null : `查询没有被接受(${answer.status} ${answer.code})`;
    case 'noAnswer':
      return `服务这次没有答出来(${answer.status} ${answer.code})`;
    case 'transport':
      return `没有连上服务(${answer.message})`;
  }
}

/**
 * 把按运单号查与按委托标识查的两份答案合成一个解析结果。按运单号只认逐字相等的行(读口本就逐字比,这里再守一道);
 * 按委托标识查到的那一份不论有没有运单号都算(老运单要靠它)。两边是同一份就是一份。
 */
export function resolveShipmentRef(
  input: string,
  byNumber: ApiResult<ViewsListResponseBody>,
  byId: ApiResult<ViewDetailResponseBody>,
): ShipmentRefResolution {
  const reason = unreadableReason(byNumber) ?? unreadableReason(byId);
  if (reason !== null) return { kind: 'unreadable', reason };

  const candidates = new Map<string, ShipmentRef>();
  if (byNumber.kind === 'outcome') {
    for (const row of byNumber.body.requests) {
      if (row.documentNumber === input) candidates.set(row.shipmentRequestId, refOf(row));
    }
  }
  if (byId.kind === 'outcome' && byId.body.request.shipmentRequestId === input) {
    candidates.set(byId.body.request.shipmentRequestId, refOf(byId.body.request));
  }
  const found = [...candidates.values()];
  if (found.length === 0) return { kind: 'notFound' };
  if (found.length > 1) return { kind: 'ambiguous', candidates: found };
  return { kind: 'resolved', ref: found[0] };
}

/** 两次读由调用方给:页面给 api.ts 的两个查阅函数,测试给替身。 */
export interface ShipmentRefLookups {
  byNumber: (documentNumber: string) => Promise<ApiResult<ViewsListResponseBody>>;
  byId: (shipmentRequestId: string) => Promise<ApiResult<ViewDetailResponseBody>>;
}

/** 按填的那一格同时问两口再合成;填的值去两端空白后原样去问,不折大小写(号逐字比)。 */
export async function lookUpShipmentRef(raw: string, lookups: ShipmentRefLookups): Promise<ShipmentRefResolution> {
  const input = raw.trim();
  const [byNumber, byId] = await Promise.all([lookups.byNumber(input), lookups.byId(input)]);
  return resolveShipmentRef(input, byNumber, byId);
}

/**
 * 运单详情从哪来。详情地址第二段是屏上那个号——有运单号是运单号,老运单是委托标识(ShipmentRequestListPage 的 detailHash),
 * 壳层的标签、状态栏与最近打开都按地址第二段写字,写出来就是运单号。打开时照表单那样两口一起问,再定详情:
 *   - 按委托标识就读到了那一份:用这份详情,不再读;
 *   - 按运单号对上一份:按它的委托标识再读一次详情;
 *   - 两边都没有:交回按委托标识查的那一格「看不见」,与原先按委托标识打开查无同一说法;
 *   - 读不动:交回读不动的那一份,页面按它的格说;
 *   - 对上不止一份:另说,不替人选。
 */
export type ShipmentDetailSource =
  | { kind: 'detail'; answer: ApiResult<ViewDetailResponseBody> }
  | { kind: 'byId'; shipmentRequestId: string }
  | { kind: 'ambiguous'; candidates: ShipmentRef[] };

export function shipmentDetailSourceOf(
  input: string,
  byNumber: ApiResult<ViewsListResponseBody>,
  byId: ApiResult<ViewDetailResponseBody>,
): ShipmentDetailSource {
  const resolution = resolveShipmentRef(input, byNumber, byId);
  switch (resolution.kind) {
    case 'resolved':
      return byId.kind === 'outcome' && byId.body.request.shipmentRequestId === resolution.ref.shipmentRequestId
        ? { kind: 'detail', answer: byId }
        : { kind: 'byId', shipmentRequestId: resolution.ref.shipmentRequestId };
    case 'notFound':
      return { kind: 'detail', answer: byId };
    case 'ambiguous':
      return { kind: 'ambiguous', candidates: resolution.candidates };
    case 'unreadable':
      // 读不动的是哪一口就交回哪一口;两口的非业务答案都不带体,换个体类型照样成立。
      if (unreadableReason(byId) !== null) return { kind: 'detail', answer: byId };
      return byNumber.kind === 'outcome' ? { kind: 'detail', answer: byId } : { kind: 'detail', answer: byNumber };
  }
}

/** 解析没成时表单格旁那一句;解析成了交回 undefined。 */
export function shipmentRefProblem(input: string, resolution: ShipmentRefResolution): string | undefined {
  switch (resolution.kind) {
    case 'resolved':
      return undefined;
    case 'notFound':
      return `在你能看到的运单里找不到${numberName}或委托标识为「${input}」的这一票。请核对;上线前的老运单没有${numberName},填运单详情技术详情里的委托标识。`;
    case 'ambiguous':
      return (
        `「${input}」对上了不止一票运单(${resolution.candidates.map((ref) => ref.shipmentRequestId).join('、')}),` +
        '没法替你选。请改填其中那一票的委托标识。'
      );
    case 'unreadable':
      return `这次没能按「${input}」找运单:${resolution.reason}。稍后再试。`;
  }
}

/** 确认弹层里指名这一票的说法:运单号与委托标识都写出来;老运单没有运单号就如实说没有。 */
export function shipmentRefText(ref: ShipmentRef): string {
  return ref.documentNumber === undefined
    ? `委托标识 ${ref.shipmentRequestId} 的运单(没有${numberName})`
    : `${numberName} ${ref.documentNumber}(委托标识 ${ref.shipmentRequestId})`;
}
