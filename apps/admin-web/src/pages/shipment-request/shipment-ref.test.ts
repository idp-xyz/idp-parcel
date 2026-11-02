import { test } from 'node:test';
import { deepEqual, equal, match, ok } from 'node:assert/strict';
import { lookUpShipmentRef, resolveShipmentRef, shipmentDetailSourceOf, shipmentRefProblem, shipmentRefText } from './shipment-ref';
import { shipmentRequestIdsOfDisplay } from './ref-no';
import type { ApiResult, ShipmentRequestDetail, ShipmentRequestSummary, ViewDetailResponseBody, ViewsListResponseBody } from './api';

// 本文件钉撤回与取消表单「哪一票运单」那一格的解析:按运单号查与按委托标识查两份答案合成——解析成一份、没有、不止一份、
// 读不动各是一格;老运单没有运单号,只能靠委托标识;确认弹层里运单号与委托标识都写出来。

const summary = (overrides: Partial<ShipmentRequestSummary>): ShipmentRequestSummary => ({
  shipmentRequestId: 'SHR-1',
  customerAccountId: 'SYN-ACCOUNT-01',
  source: 'src',
  sourceRequestKey: 'K-1',
  state: 'SUBMITTED',
  submissionVersionId: 'SV-1',
  declaredParcelCount: 1,
  submittedAt: '2026-11-02T03:00:00Z',
  ...overrides,
});

const detail = (overrides: Partial<ShipmentRequestSummary>): ShipmentRequestDetail => ({
  ...summary(overrides),
  batchId: 'B-1',
  occurredAt: '2026-11-02T03:00:00Z',
  receivedAt: '2026-11-02T03:00:00Z',
  declaredParcels: [],
  priorVersionCount: 0,
  acceptanceTask: { state: 'RUNNING' },
});

const listed = (...requests: ShipmentRequestSummary[]): ApiResult<ViewsListResponseBody> => ({
  kind: 'outcome',
  status: 200,
  body: { outcome: 'LISTED', requests },
});
const found = (request: ShipmentRequestDetail): ApiResult<ViewDetailResponseBody> => ({
  kind: 'outcome',
  status: 200,
  body: { outcome: 'REQUEST_VIEW', request },
});
const notVisible: ApiResult<ViewDetailResponseBody> = { kind: 'callerProblem', status: 404, code: 'SHIPMENT_REQUEST_NOT_VISIBLE' };

test('按运单号对上一份就解析成它的委托标识', () => {
  const resolution = resolveShipmentRef('P20261102000001', listed(summary({ documentNumber: 'P20261102000001' })), notVisible);
  deepEqual(resolution, { kind: 'resolved', ref: { shipmentRequestId: 'SHR-1', documentNumber: 'P20261102000001' } });
  equal(shipmentRefProblem('P20261102000001', resolution), undefined);
});

test('老运单没有运单号,填委托标识也解析得成;确认语如实说没有运单号', () => {
  const resolution = resolveShipmentRef('SHR-OLD', listed(), found(detail({ shipmentRequestId: 'SHR-OLD', documentNumberAbsence: 'ISSUED_BEFORE_NUMBERING' })));
  deepEqual(resolution, { kind: 'resolved', ref: { shipmentRequestId: 'SHR-OLD' } });
  if (resolution.kind !== 'resolved') return;
  equal(shipmentRefText(resolution.ref), '委托标识 SHR-OLD 的运单(没有运单号)');
});

test('有号的运单填了委托标识,确认语同时写出运单号与委托标识', () => {
  const resolution = resolveShipmentRef('SHR-1', listed(), found(detail({ documentNumber: 'P20261102000001' })));
  if (resolution.kind !== 'resolved') throw new Error(resolution.kind);
  equal(shipmentRefText(resolution.ref), '运单号 P20261102000001(委托标识 SHR-1)');
});

test('两边对上的是同一票只算一份', () => {
  // 租户的格式恰好与委托标识同形时,按号与按标识会指到同一份。
  const same = summary({ shipmentRequestId: 'SHR-1', documentNumber: 'SHR-1' });
  equal(resolveShipmentRef('SHR-1', listed(same), found(detail({ documentNumber: 'SHR-1' }))).kind, 'resolved');
});

test('对上不止一票就不替人选,说出是哪几票', () => {
  const resolution = resolveShipmentRef(
    'X-1',
    listed(summary({ shipmentRequestId: 'SHR-A', documentNumber: 'X-1' })),
    found(detail({ shipmentRequestId: 'X-1', documentNumber: 'P20261102000009' })),
  );
  equal(resolution.kind, 'ambiguous');
  const problem = shipmentRefProblem('X-1', resolution) ?? '';
  match(problem, /不止一票/);
  match(problem, /SHR-A/);
});

test('都没有就说找不到,并提示老运单要填委托标识;按号查只认逐字相等的行', () => {
  const resolution = resolveShipmentRef('P404', listed(summary({ documentNumber: 'P4040' })), notVisible);
  equal(resolution.kind, 'notFound');
  match(shipmentRefProblem('P404', resolution) ?? '', /找不到运单号或委托标识为「P404」/);
});

test('读不动不当成「没有」:未登录、渠道未配置、5xx、没连上都说这次没能找', () => {
  const cases: ApiResult<ViewsListResponseBody>[] = [
    { kind: 'loginRequired' },
    { kind: 'unconfigured' },
    { kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' },
    { kind: 'transport', message: 'fetch failed' },
    { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' },
  ];
  for (const byNumber of cases) {
    const resolution = resolveShipmentRef('P1', byNumber, notVisible);
    equal(resolution.kind, 'unreadable', byNumber.kind);
    match(shipmentRefProblem('P1', resolution) ?? '', /这次没能按「P1」找运单/);
  }
  // 按标识那一口读不动同样不算「没有」。
  equal(resolveShipmentRef('P1', listed(), { kind: 'noAnswer', status: 503, code: 'X' }).kind, 'unreadable');
});

test('同时问两口,去两端空白后原样去问', async () => {
  const asked: string[] = [];
  const resolution = await lookUpShipmentRef('  P20261102000001 ', {
    byNumber: async (number) => {
      asked.push(`number:${number}`);
      return listed(summary({ documentNumber: 'P20261102000001' }));
    },
    byId: async (id) => {
      asked.push(`id:${id}`);
      return notVisible;
    },
  });
  ok(asked.includes('number:P20261102000001') && asked.includes('id:P20261102000001'));
  equal(resolution.kind, 'resolved');
});

// 运单详情的地址第二段是屏上那个号(有号是运单号,老运单是去前缀的委托标识),打开时按运单号与这个号可能对应的委托标识
// 一起认出是哪一份,再定详情从哪来。byIds 与 shipmentRequestIdsOfDisplay 同序:短写法问原样与补回前缀两处。

test('这个号可能对应的委托标识:整串只问它自己,短写法问原样与补回前缀两处', () => {
  deepEqual(shipmentRequestIdsOfDisplay('SHR-OLD'), ['SHR-OLD']);
  deepEqual(shipmentRequestIdsOfDisplay('OLD'), ['OLD', 'SHR-OLD']);
});

test('详情地址是运单号:按号对上一份,再按它的委托标识读详情', () => {
  const source = shipmentDetailSourceOf(
    'P20261102000001',
    listed(summary({ shipmentRequestId: 'SHR-9', documentNumber: 'P20261102000001' })),
    [notVisible, notVisible],
  );
  deepEqual(source, { kind: 'byId', shipmentRequestId: 'SHR-9' });
});

test('详情地址是老运单的短写法:补回前缀那一处读到的就是详情,不再读', () => {
  const byId = found(detail({ shipmentRequestId: 'SHR-OLD', documentNumberAbsence: 'ISSUED_BEFORE_NUMBERING' }));
  deepEqual(shipmentDetailSourceOf('OLD', listed(), [notVisible, byId]), { kind: 'detail', answer: byId });
});

test('详情地址是委托标识整串(从前收藏的地址):读到的那一份就是详情', () => {
  const numbered = found(detail({ shipmentRequestId: 'SHR-1', documentNumber: 'P20261102000001' }));
  deepEqual(shipmentDetailSourceOf('SHR-1', listed(), [numbered]), { kind: 'detail', answer: numbered });
});

test('一口唯一命中就用它,另一口读不动不让整页报错', () => {
  const down: ApiResult<ViewDetailResponseBody> = { kind: 'noAnswer', status: 503, code: 'NO_ANSWER_FORMED' };
  deepEqual(
    shipmentDetailSourceOf('P20261102000001', listed(summary({ shipmentRequestId: 'SHR-9', documentNumber: 'P20261102000001' })), [down, down]),
    { kind: 'byId', shipmentRequestId: 'SHR-9' },
  );
  const numbered = found(detail({ shipmentRequestId: 'SHR-1', documentNumber: 'P20261102000001' }));
  deepEqual(shipmentDetailSourceOf('SHR-1', { kind: 'transport', message: 'fetch failed' }, [numbered]), { kind: 'detail', answer: numbered });
});

test('都没命中:有一口读不动就交回读不动的那一份,不说找不到;各口都答了才说看不见', () => {
  const down: ApiResult<ViewDetailResponseBody> = { kind: 'noAnswer', status: 503, code: 'NO_ANSWER_FORMED' };
  deepEqual(shipmentDetailSourceOf('P1', listed(), [notVisible, down]), { kind: 'detail', answer: down });
  deepEqual(shipmentDetailSourceOf('P1', { kind: 'transport', message: 'fetch failed' }, [notVisible, notVisible]), {
    kind: 'detail',
    answer: { kind: 'transport', message: 'fetch failed' },
  });
  deepEqual(shipmentDetailSourceOf('P404', listed(), [notVisible, notVisible]), { kind: 'detail', answer: notVisible });
});

test('详情地址对上不止一份:不替人选,交回各自的委托标识与运单号', () => {
  const source = shipmentDetailSourceOf(
    'X-1',
    listed(summary({ shipmentRequestId: 'SHR-A', documentNumber: 'X-1' })),
    [notVisible, found(detail({ shipmentRequestId: 'SHR-X-1' }))],
  );
  deepEqual(source, {
    kind: 'ambiguous',
    candidates: [{ shipmentRequestId: 'SHR-A', documentNumber: 'X-1' }, { shipmentRequestId: 'SHR-X-1' }],
  });
});
