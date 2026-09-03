import { test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import type {
  InitialRouteListResponseBody,
  InitialRouteRecord,
  RoutePlanApplicability,
  RouteReassessmentListResponseBody,
  RouteReassessmentRecord,
} from './api';

// 前后端之间唯一的共享物是后端包 testdata/ 里的契约夹具（票 admin-web-audit-followups/04）：
// Go 测试把 GET /route-plans 端点真答出的响应体写在那里并逐字节守着，这里读**同一个文件**校
// 本目录手写的 TS 响应类型。不复制第二份进前端树——两份就要再立一道比对门禁，且改一处时
// 另一处不会跟。路径相对 apps/admin-web：scripts/run-tests.mjs 固定以它为 cwd 起 node --test。
const fixtureRoot = '../../internal/networkrouting/adapters/http/testdata';

function readFixture(name: string): unknown {
  return JSON.parse(readFileSync(`${fixtureRoot}/${name}`, 'utf8'));
}

// TS 类型在运行时不存在，所以拿夹具校类型要先把类型的键清单写成值。下面两个工具类型让
// 清单与类型在编译期对齐：多列一个键（或把可缺席的列进必备）`satisfies` 红；漏列一个键
// `Exhaustive` 红并把漏的那个键名摆在报错里。于是 TS 这侧改名、加键、删键都在编译期红，
// Go 那侧改名、加键在运行时红——这就是「改任一侧两侧都红」的机制。
type RequiredKeys<T> = { [K in keyof T]-?: {} extends Pick<T, K> ? never : K }[keyof T];
type OptionalKeys<T> = Exclude<keyof T, RequiredKeys<T>>;
type Exhaustive<T, Listed extends PropertyKey> = [Exclude<keyof T, Listed>] extends [never]
  ? true
  : Exclude<keyof T, Listed>;

interface KeyShape {
  readonly required: readonly string[];
  readonly optional: readonly string[];
  /** 值是对象的键，各带自己的形状。 */
  readonly nested?: Readonly<Record<string, KeyShape>>;
  /** 值是对象数组（册的行）的键，各带行形状。 */
  readonly rows?: Readonly<Record<string, KeyShape>>;
}

type Row = Record<string, unknown>;

/**
 * 一个对象的键集合必须落在类型认识的键里，必备键一个不缺；值按形状逐键核：对象与行数组
 * 递归，其余一律字符串——两册的列面今天没有别的值类型，有了再开格。
 */
function assertShape(value: unknown, shape: KeyShape, where: string): Row {
  ok(typeof value === 'object' && value !== null && !Array.isArray(value), `${where} 不是对象`);
  const record = value as Row;
  for (const key of shape.required) {
    ok(key in record, `${where} 缺必备键 ${key}——后端把它改名或去掉了，TS 类型还当它必在`);
  }
  for (const key of Object.keys(record)) {
    ok(
      shape.required.includes(key) || shape.optional.includes(key),
      `${where} 有 TS 类型不认识的键 ${key}——后端形状变了，前端类型没跟`,
    );
    const nested = shape.nested?.[key];
    const rows = shape.rows?.[key];
    if (nested) {
      assertShape(record[key], nested, `${where}.${key}`);
    } else if (rows) {
      assertRows(record[key], rows, `${where}.${key}`);
    } else {
      equal(typeof record[key], 'string', `${where}.${key} 不是字符串`);
    }
  }
  return record;
}

/**
 * 一册的行集合：每行过 assertShape，再核可缺席键的覆盖——每个可缺席键（含嵌套对象里的）
 * 要在夹具里至少出场一次，否则那一格被后端改名时这里不会红，夹具没覆盖到的格等于没校。
 */
function assertRows(value: unknown, shape: KeyShape, where: string): void {
  ok(Array.isArray(value), `${where} 不是数组`);
  ok(value.length > 0, `${where} 的夹具没有行，行形状无从校`);
  const records = (value as unknown[]).map((row, index) => assertShape(row, shape, `${where}[${index}]`));
  assertOptionalKeysCovered(records, shape, where);
}

function assertOptionalKeysCovered(records: readonly Row[], shape: KeyShape, where: string): void {
  for (const key of shape.optional) {
    ok(
      records.some((row) => key in row),
      `${where} 的夹具里没有任何一行带 ${key}，可缺席键没有实例可核`,
    );
  }
  for (const [key, nested] of Object.entries(shape.nested ?? {})) {
    const present = records.flatMap((row) => (row[key] === undefined ? [] : [row[key] as Row]));
    assertOptionalKeysCovered(present, nested, `${where}[].${key}`);
  }
}

// —— 初始路由册 ——

const initialRouteBodyRequired = ['outcome', 'judgments'] as const satisfies readonly RequiredKeys<InitialRouteListResponseBody>[];
const initialRouteBodyComplete: Exhaustive<InitialRouteListResponseBody, (typeof initialRouteBodyRequired)[number]> = true;

const applicabilityRequired = ['state', 'transitionedAt'] as const satisfies readonly RequiredKeys<RoutePlanApplicability>[];
const applicabilityOptional = ['basis', 'successor'] as const satisfies readonly OptionalKeys<RoutePlanApplicability>[];
const applicabilityComplete: Exhaustive<
  RoutePlanApplicability,
  (typeof applicabilityRequired)[number] | (typeof applicabilityOptional)[number]
> = true;

const judgmentRequired = [
  'customerAccountId',
  'shipmentRequestId',
  'acceptanceBaseline',
  'declaredParcelId',
  'servicePurpose',
  'conclusion',
  'recordedAt',
] as const satisfies readonly RequiredKeys<InitialRouteRecord>[];
const judgmentOptional = ['planVersion', 'applicability'] as const satisfies readonly OptionalKeys<InitialRouteRecord>[];
const judgmentComplete: Exhaustive<
  InitialRouteRecord,
  (typeof judgmentRequired)[number] | (typeof judgmentOptional)[number]
> = true;

const judgmentShape: KeyShape = {
  required: judgmentRequired,
  optional: judgmentOptional,
  nested: { applicability: { required: applicabilityRequired, optional: applicabilityOptional } },
};

// —— 路由复核册 ——

const reassessmentBodyRequired = ['outcome', 'reassessments'] as const satisfies readonly RequiredKeys<RouteReassessmentListResponseBody>[];
const reassessmentBodyComplete: Exhaustive<RouteReassessmentListResponseBody, (typeof reassessmentBodyRequired)[number]> = true;

const reassessmentRequired = [
  'correlationId',
  'customerAccountId',
  'shipmentRequestId',
  'acceptanceBaseline',
  'declaredParcelId',
  'servicePurpose',
  'conclusion',
  'reassessedAt',
  'recordedAt',
] as const satisfies readonly RequiredKeys<RouteReassessmentRecord>[];
const reassessmentOptional = [
  'reviewedPlan',
  'lapseBasis',
  'candidateState',
  'rerouteState',
] as const satisfies readonly OptionalKeys<RouteReassessmentRecord>[];
const reassessmentComplete: Exhaustive<
  RouteReassessmentRecord,
  (typeof reassessmentRequired)[number] | (typeof reassessmentOptional)[number]
> = true;

const reassessmentShape: KeyShape = { required: reassessmentRequired, optional: reassessmentOptional };

// 这几个常量只为让编译器替清单作证，运行时没有读者。
void initialRouteBodyComplete;
void applicabilityComplete;
void judgmentComplete;
void reassessmentBodyComplete;
void reassessmentComplete;

test('GET /route-plans?register=initial-route 的契约夹具与 InitialRouteListResponseBody 逐键对得上', () => {
  const body = assertShape(
    readFixture('route_plans_initial_route.json'),
    { required: initialRouteBodyRequired, optional: [], rows: { judgments: judgmentShape } },
    '响应体',
  );
  equal(body.outcome, 'INITIAL_ROUTES_LISTED' satisfies InitialRouteListResponseBody['outcome']);
});

test('GET /route-plans?register=reassessment 的契约夹具与 RouteReassessmentListResponseBody 逐键对得上', () => {
  const body = assertShape(
    readFixture('route_plans_reassessment.json'),
    { required: reassessmentBodyRequired, optional: [], rows: { reassessments: reassessmentShape } },
    '响应体',
  );
  equal(body.outcome, 'ROUTE_REASSESSMENTS_LISTED' satisfies RouteReassessmentListResponseBody['outcome']);
});
