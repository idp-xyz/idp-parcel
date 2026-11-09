import { useState } from 'react';
import { useAddressKeyword, type ListCursorPaginationProps, type WorkbenchChip, type WorkbenchSort } from '../../templates';
import type { ApiResult } from '../catalogue-api';
import { catalogueConditionsNarrow, type CataloguePage } from '../catalogue-query';
import { pastFirstPage } from '../shipment-request/cursor-pager';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import type { RouteRegisterConditions } from './api';
import {
  allStatuses,
  newestFirst,
  routeListConditions,
  routeSortParam,
  routeStatusChips,
  routeStatusDimensions,
  type RouteRegister,
  type RouteSortColumn,
} from './route-plan-conditions';

export interface RouteRegisterList<Body> {
  /** 检索框（地址上的 `?q=`）：停稳后下推成 `q`。 */
  keyword: string;
  setKeyword: (keyword: string) => void;
  /** 只筛本页的那一框：读口检索不认的几格在到手的这一页上找。 */
  refine: string;
  setRefine: (refine: string) => void;
  /** 下推的状态胶囊。 */
  statusChips: WorkbenchChip[];
  /** 「清除筛选」时一并清掉胶囊与只筛本页的框；检索框由工作台自己清。 */
  clearFilters: () => void;
  /** 下推的表头先后；这一册的列不对应可排维时缺席。 */
  pushedSort: { value: WorkbenchSort; onChange: (sort: WorkbenchSort) => void } | undefined;
  answer: ApiResult<Body> | null;
  pagination: ListCursorPaginationProps | undefined;
  /** 重取当前这一页，连同页上的运单号。 */
  refresh: () => void;
  /** 重取了几次：交给 useShipmentNumbers 作刷新令牌。 */
  refreshes: number;
  /** 这一问带没带收窄条件，或已翻过第一页（catalogueViewState 的 narrowed）。 */
  narrowed: boolean;
}

/**
 * 路线规划一册的列表状态（票 catalogue-read-pagination-rollout/14）：检索词、状态胶囊与表头先后拼成读口的条件，按游标取页（ADR-0144）；
 * 只筛本页的框不进条件。换册时页面给新 key 重挂，胶囊、框与先后回到初始，检索词在地址上、跟着留下。`load` 要是模块级的读口函数。
 */
export function useRouteRegisterList<Body extends { page: CataloguePage }>(
  register: RouteRegister,
  load: (conditions: RouteRegisterConditions, after: string | null) => Promise<ApiResult<Body>>,
  sortColumn?: RouteSortColumn,
): RouteRegisterList<Body> {
  const status = routeStatusDimensions[register];
  const [keyword, setKeyword] = useAddressKeyword();
  const [picked, setPicked] = useState(allStatuses);
  const [refine, setRefine] = useState('');
  const [sort, setSort] = useState<WorkbenchSort | null>(sortColumn ? newestFirst(sortColumn) : null);
  const [refreshes, setRefreshes] = useState(0);
  const conditions = routeListConditions(
    useSettledKeyword(keyword),
    status,
    picked,
    sortColumn && sort ? routeSortParam(sort, sortColumn) : undefined,
  );
  const { answer, pagination, retry } = useCatalogueCursor(conditions, load);
  return {
    keyword,
    setKeyword,
    refine,
    setRefine,
    statusChips: routeStatusChips(status, picked, setPicked),
    clearFilters: () => {
      setPicked(allStatuses);
      setRefine('');
    },
    pushedSort: sort ? { value: sort, onChange: setSort } : undefined,
    answer,
    pagination,
    refresh: () => {
      retry();
      setRefreshes((count) => count + 1);
    },
    refreshes,
    narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
  };
}
