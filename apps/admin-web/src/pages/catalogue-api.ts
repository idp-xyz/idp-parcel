export type ApiResult<Body> =
  | { kind: 'outcome'; status: number; body: Body }
  | { kind: 'unconfigured' }
  // `detail` 是服务端随 4xx 交回的理由散文（票 admin-web-group-legal-entities/11；首例是 party-commercial 写口的
  // 形状拒绝）。散文不是代数：原样示出、不查表、不据此分支——纪律同登记答案的 `cause`。缺席即服务端没给，
  // 不是漏字段；5xx 不带它，那是依赖故障的内部原文。
  | { kind: 'callerProblem'; status: number; code: string; detail?: string }
  | { kind: 'noAnswer'; status: number; code: string }
  | { kind: 'transport'; message: string };

let apiBase = '';

export function configureMasterDataApi(config: { basePrefix: string }) {
  apiBase = config.basePrefix.replace(/\/$/, '');
}

function requestUrl(path: string): string {
  return `${apiBase}${path}`;
}

// 主数据页共用同一套 HTTP 结果代数；非 2xx 不吸收到业务 outcome。
export async function exchangeMasterData<Body>(path: string): Promise<ApiResult<Body>> {
  let response: Response;
  try {
    response = await fetch(requestUrl(path), {
      method: 'GET',
      headers: { Accept: 'application/json' },
    });
  } catch (error) {
    return {
      kind: 'transport',
      message: error instanceof Error ? error.message : String(error),
    };
  }

  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    return {
      kind: 'transport',
      message: `响应不是 JSON（HTTP ${response.status}），请求可能未到达 parcel-api`,
    };
  }

  if (response.ok) {
    return { kind: 'outcome', status: response.status, body: payload as Body };
  }

  return classifyMasterDataResponse<Body>(response, payload);
}

// 登记写面共用同一套结果代数（ADR-0085：在线登记口与登记 CLI 消费同一登记用例，答案
// 代数一致）。它与查阅那半只差方法与请求体——分成两个函数而不是加一个可选参数，是为了
// 让「这一次是读还是写」在调用点看得见：写行的 Intake 与读行不是同一个，隔离读准入
// （ADR-0078）换得了读行换不了写行，调用点长得一样会让这条区别在阅读时消失。
export async function postMasterData<Body>(
  path: string,
  payload: unknown,
): Promise<ApiResult<Body>> {
  let response: Response;
  try {
    response = await fetch(requestUrl(path), {
      method: 'POST',
      headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
  } catch (error) {
    return {
      kind: 'transport',
      message: error instanceof Error ? error.message : String(error),
    };
  }

  let body: unknown;
  try {
    body = await response.json();
  } catch {
    return {
      kind: 'transport',
      message: `响应不是 JSON（HTTP ${response.status}），请求可能未到达 parcel-api`,
    };
  }

  return classifyMasterDataResponse<Body>(response, body);
}

function classifyMasterDataResponse<Body>(
  response: Response,
  payload: unknown,
): ApiResult<Body> {
  if (response.ok) {
    return { kind: 'outcome', status: response.status, body: payload as Body };
  }

  const problem = (payload as { error?: { code?: string; detail?: unknown } } | null)?.error;
  const code = problem?.code ?? 'UNKNOWN';
  if (response.status === 403 && code === 'ACCESS_CHANNEL_NOT_CONFIGURED') {
    return { kind: 'unconfigured' };
  }
  if (response.status >= 500) {
    return { kind: 'noAnswer', status: response.status, code };
  }
  // 只在服务端真给了一句字符串时才带上这一格：别的形状不是散文，不冒充；缺席时不多出一个值为 undefined 的键，
  // 调用侧按「有没有这一格」判，不必再判值。
  const detail = typeof problem?.detail === 'string' ? problem.detail : undefined;
  return detail === undefined
    ? { kind: 'callerProblem', status: response.status, code }
    : { kind: 'callerProblem', status: response.status, code, detail };
}
