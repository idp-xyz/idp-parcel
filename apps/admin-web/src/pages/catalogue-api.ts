export type ApiResult<Body> =
  | { kind: 'outcome'; status: number; body: Body }
  | { kind: 'unconfigured' }
  | { kind: 'callerProblem'; status: number; code: string }
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

  const code =
    (payload as { error?: { code?: string } } | null)?.error?.code ?? 'UNKNOWN';
  if (response.status === 403 && code === 'ACCESS_CHANNEL_NOT_CONFIGURED') {
    return { kind: 'unconfigured' };
  }
  if (response.status >= 500) {
    return { kind: 'noAnswer', status: response.status, code };
  }
  return { kind: 'callerProblem', status: response.status, code };
}
