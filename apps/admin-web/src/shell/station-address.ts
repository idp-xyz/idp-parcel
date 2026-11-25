// 作业台的地址（纯 TypeScript，node:test 直接测）。
//
// 作业台不带外壳：落在这些地址上，App 不渲染 Layout（没有侧栏、标签栏、检查器），渲染全屏的作业台。地址是 `#/station`
// （选工位）与 `#/station/<工位>`，可以带查询串（`#/station/pick?wave=…`）。这里只认「这是作业台的地址」；工位词是 pick 还是
// 别的、认不认得，归作业台自己答，所以新增一个工位不用改这里。
//
// 判据是第一段恰为 station：`#/stationery` 这类只是前缀相同的别的地址不算，空地址也不算。第二段之后的多余段不认，只取前两段——
// 作业台的地址不分层，多出来的是手敲的或旧链接带的，不值得为它另起一页。第一段按外壳认标签的同一种写法先解码再比，
// 免得外壳认作 station 而这里不认。

export interface StationAddress {
  /** 工位词；选工位那一页没有。 */
  station: string | null;
  /** 查询串里的各项，不带问号。 */
  query: URLSearchParams;
}

const stationSegment = 'station';

function decodeSegment(segment: string): string | null {
  try {
    return decodeURIComponent(segment);
  } catch {
    return null;
  }
}

export function parseStationAddress(hash: string): StationAddress | null {
  const body = hash.replace(/^#\/?/, '');
  const mark = body.indexOf('?');
  const path = mark < 0 ? body : body.slice(0, mark);
  const queryText = mark < 0 ? '' : body.slice(mark + 1);
  const [first, second] = path.split('/');
  if (first === undefined || decodeSegment(first) !== stationSegment) return null;
  // 第二段解不开码时按原样留着：它认不认得是工位那一侧的事，这里不因它把整个地址判成别的页。
  const station = second === undefined || second === '' ? null : (decodeSegment(second) ?? second);
  return { station, query: new URLSearchParams(queryText) };
}

/**
 * 造一个作业台地址。空值的查询项不写：`wave` 没有就是「全部波次」，不留一个空的 `?wave=`。
 */
export function stationHash(station: string | null, params: Readonly<Record<string, string | undefined>> = {}): string {
  const path = station === null ? `#/${stationSegment}` : `#/${stationSegment}/${encodeURIComponent(station)}`;
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') query.set(key, value);
  }
  const text = query.toString();
  return text === '' ? path : `${path}?${text}`;
}

/** App 此刻该摆什么。 */
export type AppScreen =
  | { kind: 'shell' }
  | { kind: 'station'; address: StationAddress }
  /** 部署开关明确关着：作业台地址上摆「本部署没有启用」的说明，不渲染工位。 */
  | { kind: 'station-not-enabled' };

/**
 * 地址不是作业台的一律是外壳，不看开关：外壳自己管仓配入口摆不摆。作业台地址上，开关明确关着才摆说明；
 * 答出之前与答不出（`warehouseOffered` 为真，口径同 useWarehouseOffered）照常渲染工位，让服务端去答。
 */
export function appScreen(hash: string, warehouseOffered: boolean): AppScreen {
  const address = parseStationAddress(hash);
  if (address === null) return { kind: 'shell' };
  return warehouseOffered ? { kind: 'station', address } : { kind: 'station-not-enabled' };
}
