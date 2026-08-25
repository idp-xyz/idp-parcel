// 本目录 fetch 出口:价卡目录与计价参考序列查阅(ADR-0077、票 master-data-wiring/02)。
// 形状以 internal/parcelpricing/adapters/http 传输层为准,此处只做镜像不虚构。
//
// 响应判读按 ADR-0022:HTTP 状态码只回答「服务端有没有形成答案」,业务判别一律在
// 响应体的 `outcome`。五格判别与全程追踪页同款。

let apiBase = '';

export function configurePricingApi(options: { basePrefix: string }): void {
  apiBase = options.basePrefix;
}

export interface PriceCardRecord {
  planId: string;
  planVersion: string;
  direction: string;
  purpose: string;
  scope: string;
  rateTableId: string;
  rateTableVersion: string;
  effectiveFrom: string;
  effectiveTo?: string;
  canonicalization: string;
  contentDigest: string;
  sourceFileName: string;
  sourceFileSha256: string;
  authorizationId: string;
  authorizationVersion: string;
  publicationApprover: string;
  registeredAt: string;
}

export interface PriceCardListResponseBody {
  outcome: 'PRICE_CARDS_LISTED';
  cards: PriceCardRecord[];
}

export interface ReferenceSeriesRecord {
  seriesId: string;
  seriesVersion: string;
  kind: string;
  sourceIdentifier: string;
  registrant: string;
  quoteBasisId?: string;
  quoteBasisVersion?: string;
  effectiveFrom: string;
  effectiveTo?: string;
  evidenceGrade: string;
  priorVersion?: string;
  correctionBasis?: string;
  canonicalization: string;
  contentDigest: string;
  registeredAt: string;
}

export interface ReferenceSeriesListResponseBody {
  outcome: 'REFERENCE_SERIES_LISTED';
  series: ReferenceSeriesRecord[];
}

export type ApiResult<Body> =
  | { kind: 'outcome'; status: number; body: Body }
  | { kind: 'unconfigured' }
  | { kind: 'callerProblem'; status: number; code: string }
  | { kind: 'noAnswer'; status: number; code: string }
  | { kind: 'transport'; message: string };

export function listPriceCards(): Promise<ApiResult<PriceCardListResponseBody>> {
  return exchange<PriceCardListResponseBody>('/pricing-price-cards', { method: 'GET' });
}

export function listReferenceSeries(): Promise<ApiResult<ReferenceSeriesListResponseBody>> {
  return exchange<ReferenceSeriesListResponseBody>('/pricing-reference-series', {
    method: 'GET',
  });
}

async function exchange<Body>(path: string, init: RequestInit): Promise<ApiResult<Body>> {
  let response: Response;
  try {
    response = await fetch(apiBase + path, init);
  } catch (cause) {
    return {
      kind: 'transport',
      message: cause instanceof Error ? cause.message : String(cause),
    };
  }

  let parsed: unknown;
  try {
    parsed = await response.json();
  } catch {
    return {
      kind: 'transport',
      message: `响应不是 JSON(HTTP ${response.status}),请求可能未到达 parcel-api`,
    };
  }

  if (response.ok) {
    return { kind: 'outcome', status: response.status, body: parsed as Body };
  }

  const code =
    (parsed as { error?: { code?: string } } | null)?.error?.code ?? 'UNKNOWN';
  if (response.status === 403 && code === 'ACCESS_CHANNEL_NOT_CONFIGURED') {
    return { kind: 'unconfigured' };
  }
  if (response.status >= 500) {
    return { kind: 'noAnswer', status: response.status, code };
  }
  return { kind: 'callerProblem', status: response.status, code };
}
