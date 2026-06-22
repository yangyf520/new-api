/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Link, getRouteApi } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { SectionPageLayout } from '@/components/layout'
import { DataTablePage } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api } from '@/lib/api'
import { formatTimestamp } from '@/lib/format'
import {
  getTokenApplySectionTitle,
  type TokenApplySection,
} from './section-registry'
import {
  formatApplicationBalanceAmount,
  formatCurrencyAmount,
  formatValueWithUnit,
  rowCurrency,
} from './format'
import { buildPolicyTree, flattenPolicyTree } from './policy-tree'
import { TokenApplyTokenDialog } from './token-dialog'

const sectionRoute = getRouteApi('/_authenticated/token-apply/$section')
const detailRoute = getRouteApi('/_authenticated/token-apply/$section/$id')

const SCOPE_TYPE_TAG: Record<string, { variant: 'default' | 'secondary' | 'outline'; labelKey: string }> = {
  org: { variant: 'secondary', labelKey: 'Organization' },
  team: { variant: 'default', labelKey: 'Team' },
  project: { variant: 'outline', labelKey: 'Project' },
  token: { variant: 'outline', labelKey: 'Token' },
}

const ACTION_TAG: Record<string, { variant: 'default' | 'secondary' | 'outline' | 'destructive'; labelKey: string }> = {
  issue: { variant: 'secondary', labelKey: 'Issue' },
  increase: { variant: 'default', labelKey: 'Increase' },
  decrease: { variant: 'destructive', labelKey: 'Decrease' },
  adjust: { variant: 'outline', labelKey: 'Adjust' },
}

type TokenApplication = {
  id: number
  ticket_no: string
  user_name: string
  user_email: string
  work_no: string
  org_code: string
  org_name: string
  amount: number
  currency: string
  quota_mode: string
  token_id: number
  remain_quota: number
  used_quota: number
  remain_amount?: number
  used_amount?: number
  issued_time: number
  remark?: string
  logs?: TokenApplicationLog[]
}

type TokenApplicationLog = {
  id: number
  action: string
  change_ticket_no: string
  amount_before: number
  amount_after: number
  budget_delta: number
  created_at: number
}

type PolicyRow = {
  id: number
  parent_id?: number | null
  scope_type: string
  scope_code: string
  token_type: string
  currency: string
  period_type: string
  period_key: string
  enabled: boolean
  depth?: number
}

type BudgetPolicyRow = PolicyRow & {
  total_amount: number
  approved_amount: number
  remaining_amount: number
}

type ConsumptionPolicyRow = PolicyRow & {
  cap_amount: number
  used_amount: number
  remaining_amount: number
}

async function fetchApplications(page: number, pageSize: number, keyword: string) {
  const query = new URLSearchParams({
    p: String(page),
    page_size: String(pageSize),
  })
  if (keyword.trim()) query.set('keyword', keyword.trim())
  const res = await api.get(`/api/token-apply/records?${query}`)
  return {
    items: (res.data?.data?.items ?? []) as TokenApplication[],
    total: (res.data?.data?.total ?? 0) as number,
  }
}

async function fetchPolicies<T>(path: string) {
  const res = await api.get(path)
  return (res.data?.data ?? []) as T[]
}

function TokenApplyPageShell(props: {
  title: string
  actions?: ReactNode
  children: ReactNode
}) {
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{props.title}</SectionPageLayout.Title>
      {props.actions ? (
        <SectionPageLayout.Actions>{props.actions}</SectionPageLayout.Actions>
      ) : null}
      <SectionPageLayout.Content>{props.children}</SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function DetailField(props: { label: string; value: ReactNode }) {
  return (
    <div className='flex flex-col gap-1'>
      <span className='text-muted-foreground text-xs'>{props.label}</span>
      <span className='text-sm'>{props.value}</span>
    </div>
  )
}

function displayValue(value: unknown) {
  if (value === null || value === undefined || value === '') return '-'
  return String(value)
}

function policyTreeColumns<T extends PolicyRow>(
  t: (key: string) => string
): ColumnDef<T>[] {
  return [
    {
      accessorKey: 'scope_code',
      header: t('Scope'),
      cell: ({ row }) => {
        const config = SCOPE_TYPE_TAG[row.original.scope_type] ?? {
          variant: 'outline' as const,
          labelKey: row.original.scope_type,
        }
        return (
          <span
            className='inline-flex items-center gap-2'
            style={{ paddingLeft: `${(row.original.depth ?? 0) * 16}px` }}
          >
            <Badge variant={config.variant}>{t(config.labelKey)}</Badge>
            <span>{row.original.scope_code}</span>
          </span>
        )
      },
    },
    { accessorKey: 'token_type', header: t('Token Type') },
    { accessorKey: 'period_type', header: t('Period') },
    { accessorKey: 'period_key', header: t('Period Key') },
    {
      accessorKey: 'enabled',
      header: t('Enabled'),
      cell: ({ row }) => (row.original.enabled ? t('Yes') : t('No')),
    },
  ]
}

function currencyColumn<T extends { currency?: string }>(
  t: (key: string) => string,
  headerKey: string,
  accessor: keyof T,
  fractionDigits?: number
): ColumnDef<T> {
  return {
    accessorKey: accessor as string,
    header: t(headerKey),
    cell: ({ row }) =>
      formatCurrencyAmount(
        row.original[accessor] as number,
        rowCurrency(row.original),
        { fractionDigits }
      ),
  }
}

function ApplicationsSection() {
  const { t } = useTranslation()
  const [tokenDialogApp, setTokenDialogApp] = useState<TokenApplication | null>(null)
  const isMobile = useMediaQuery('(max-width: 640px)')
  const navigate = sectionRoute.useNavigate()
  const search = sectionRoute.useSearch()
  const {
    globalFilter,
    onGlobalFilterChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search,
    navigate,
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
  })

  const keyword = globalFilter ?? ''
  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'token-apply-records',
      pagination.pageIndex + 1,
      pagination.pageSize,
      keyword,
    ],
    queryFn: () =>
      fetchApplications(pagination.pageIndex + 1, pagination.pageSize, keyword),
    placeholderData: (previousData) => previousData,
  })

  const columns = useMemo<ColumnDef<TokenApplication>[]>(
    () => [
      {
        accessorKey: 'ticket_no',
        header: t('Process No.'),
        cell: ({ row }) =>
          row.original.token_id > 0 ? (
            <button
              type='button'
              className='text-primary hover:underline'
              onClick={() => setTokenDialogApp(row.original)}
            >
              {row.original.ticket_no}
            </button>
          ) : (
            row.original.ticket_no
          ),
      },
      {
        accessorKey: 'user_name',
        header: t('Applicant'),
        cell: ({ row }) => row.original.user_name || row.original.user_email || '-',
      },
      {
        accessorKey: 'work_no',
        header: t('Employee ID'),
        cell: ({ row }) => row.original.work_no || '-',
      },
      {
        accessorKey: 'org_code',
        header: t('Organization'),
        cell: ({ row }) =>
          `${row.original.org_code || ''} ${row.original.org_name || ''}`.trim() || '-',
      },
      {
        accessorKey: 'amount',
        header: t('Approved Amount'),
        cell: ({ row }) =>
          formatCurrencyAmount(row.original.amount, rowCurrency(row.original)),
      },
      {
        accessorKey: 'remain_quota',
        header: t('Remaining Amount'),
        cell: ({ row }) =>
          formatApplicationBalanceAmount(
            row.original,
            'remain_amount',
            'remain_quota',
            rowCurrency(row.original)
          ),
      },
      {
        accessorKey: 'issued_time',
        header: t('Issued At'),
        cell: ({ row }) =>
          row.original.issued_time
            ? formatTimestamp(row.original.issued_time)
            : '-',
      },
      {
        id: 'actions',
        header: t('Action'),
        cell: ({ row }) => (
          <Link
            to='/token-apply/$section/$id'
            params={{ section: 'records', id: String(row.original.id) }}
            className='text-primary hover:underline'
          >
            {t('View Changes')}
          </Link>
        ),
      },
    ],
    [t]
  )

  const table = useReactTable({
    data: data?.items ?? [],
    columns,
    state: { globalFilter, pagination },
    onGlobalFilterChange,
    onPaginationChange,
    manualPagination: true,
    pageCount: Math.max(1, Math.ceil((data?.total ?? 0) / pagination.pageSize)),
    getCoreRowModel: getCoreRowModel(),
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [pageCount, ensurePageInRange])

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle={t('No token applications found')}
        toolbarProps={{
          searchPlaceholder: t('Search process no., org, applicant...'),
        }}
      />
      <TokenApplyTokenDialog
        application={tokenDialogApp}
        open={Boolean(tokenDialogApp)}
        onOpenChange={(open) => {
          if (!open) setTokenDialogApp(null)
        }}
      />
    </>
  )
}

function BudgetPoliciesSection() {
  const { t } = useTranslation()
  const { data, isLoading, isFetching } = useQuery({
    queryKey: ['token-apply-budget'],
    queryFn: () => fetchPolicies<BudgetPolicyRow>('/api/token-apply/budget'),
  })

  const rows = useMemo(
    () => flattenPolicyTree(buildPolicyTree(data ?? [])),
    [data]
  )

  const columns = useMemo<ColumnDef<BudgetPolicyRow>[]>(
    () => [
      ...(policyTreeColumns<BudgetPolicyRow>(t) as ColumnDef<BudgetPolicyRow>[]),
      currencyColumn(t, 'Cap Amount', 'total_amount'),
      currencyColumn(t, 'Approved Amount', 'approved_amount'),
      currencyColumn(t, 'Remaining Amount', 'remaining_amount'),
    ],
    [t]
  )

  const table = useReactTable({
    data: rows,
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No budget policies found')}
    />
  )
}

function ConsumptionPoliciesSection() {
  const { t } = useTranslation()
  const { data, isLoading, isFetching } = useQuery({
    queryKey: ['token-apply-consumption-policies'],
    queryFn: () =>
      fetchPolicies<ConsumptionPolicyRow>('/api/token-apply/consumption'),
  })

  const rows = useMemo(
    () => flattenPolicyTree(buildPolicyTree(data ?? [])),
    [data]
  )

  const columns = useMemo<ColumnDef<ConsumptionPolicyRow>[]>(
    () => [
      ...(policyTreeColumns<ConsumptionPolicyRow>(t) as ColumnDef<ConsumptionPolicyRow>[]),
      currencyColumn(t, 'Cap Amount', 'cap_amount', 4),
      currencyColumn(t, 'Used Amount', 'used_amount', 4),
      currencyColumn(t, 'Remaining Amount', 'remaining_amount', 4),
    ],
    [t]
  )

  const table = useReactTable({
    data: rows,
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No consumption policies found')}
    />
  )
}

const SECTION_CONTENT: Record<TokenApplySection, () => React.JSX.Element> = {
  records: ApplicationsSection,
  budget: BudgetPoliciesSection,
  consumption: ConsumptionPoliciesSection,
}

export function TokenApplySectionPage() {
  const { t } = useTranslation()
  const { section } = sectionRoute.useParams()
  const Content = SECTION_CONTENT[section as TokenApplySection]

  return (
    <TokenApplyPageShell title={t(getTokenApplySectionTitle(section as TokenApplySection))}>
      <Content />
    </TokenApplyPageShell>
  )
}

export function TokenApplyDetailPage() {
  const { t } = useTranslation()
  const { id } = detailRoute.useParams()
  const applicationId = Number(id)

  const { data, isLoading } = useQuery({
    queryKey: ['token-apply-application', applicationId],
    queryFn: async () => {
      const res = await api.get(`/api/token-apply/records/${applicationId}`)
      return res.data?.data as TokenApplication | undefined
    },
    enabled: Number.isFinite(applicationId) && applicationId > 0,
  })

  if (isLoading) {
    return (
      <TokenApplyPageShell title={t('Application Detail')}>
        <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>
      </TokenApplyPageShell>
    )
  }

  if (!data) {
    return (
      <TokenApplyPageShell title={t('Application Detail')}>
        <p className='text-muted-foreground text-sm'>{t('Not found')}</p>
      </TokenApplyPageShell>
    )
  }

  const currency = rowCurrency(data)

  return (
    <TokenApplyPageShell
      title={`${t('Application Detail')} #${data.id}`}
      actions={
        <Button
          variant='outline'
          render={<Link to='/token-apply/$section' params={{ section: 'records' }} />}
        >
          {t('Back to list')}
        </Button>
      }
    >
      <div className='grid gap-4 lg:grid-cols-2'>
        <div className='bg-muted/40 h-full rounded-lg border p-4'>
          <h3 className='mb-3 text-sm font-semibold'>{t('Basic Info')}</h3>
          <div className='bg-background grid gap-4 rounded-lg border p-4 sm:grid-cols-2'>
            <DetailField
              label={t('Organization')}
              value={displayValue(`${data.org_code || ''} ${data.org_name || ''}`.trim())}
            />
            <DetailField label={t('Employee ID')} value={displayValue(data.work_no)} />
            <DetailField
              label={t('Applicant')}
              value={displayValue(data.user_name || data.user_email || data.work_no)}
            />
            <DetailField
              label={t('Issued At')}
              value={data.issued_time ? formatTimestamp(data.issued_time) : '-'}
            />
            <DetailField label={t('Remarks')} value={displayValue(data.remark)} />
          </div>
        </div>
        <div className='bg-muted/40 h-full rounded-lg border p-4'>
          <h3 className='mb-3 text-sm font-semibold'>{t('Quota & Token')}</h3>
          <div className='bg-background grid gap-4 rounded-lg border p-4 sm:grid-cols-2'>
            <DetailField label={t('Token ID')} value={displayValue(data.token_id)} />
            <DetailField label={t('Quota Mode')} value={displayValue(data.quota_mode)} />
            <DetailField
              label={t('Approved Amount')}
              value={
                <span className='text-primary font-semibold'>
                  {formatCurrencyAmount(data.amount, currency)}
                </span>
              }
            />
            <DetailField
              label={t('Used Amount')}
              value={
                <span className='font-semibold'>
                  {formatApplicationBalanceAmount(data, 'used_amount', 'used_quota', currency)}
                </span>
              }
            />
            <DetailField
              label={t('Remaining Amount')}
              value={
                <span className='text-success font-semibold'>
                  {formatApplicationBalanceAmount(
                    data,
                    'remain_amount',
                    'remain_quota',
                    currency
                  )}
                </span>
              }
            />
          </div>
        </div>
      </div>

      <div className='bg-muted/40 mt-4 rounded-lg border p-4'>
        <h3 className='mb-3 text-base font-semibold'>{t('Application Changes')}</h3>
        <div className='rounded-lg border bg-background'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Action Type')}</TableHead>
                <TableHead>{t('Change Ticket')}</TableHead>
                <TableHead>{t('Amount Change')}</TableHead>
                <TableHead>{t('Budget Delta')}</TableHead>
                <TableHead>{t('Changed At')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(data.logs ?? []).length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className='text-muted-foreground text-center'>
                    {t('No audit records')}
                  </TableCell>
                </TableRow>
              ) : (
                data.logs!.map((log) => {
                  const action = ACTION_TAG[log.action] ?? {
                    variant: 'outline' as const,
                    labelKey: log.action,
                  }
                  return (
                    <TableRow key={log.id}>
                      <TableCell>
                        <Badge variant={action.variant}>{t(action.labelKey)}</Badge>
                      </TableCell>
                      <TableCell>
                        {displayValue(
                          log.change_ticket_no ||
                            (log.action === 'issue' ? data.ticket_no : '')
                        )}
                      </TableCell>
                      <TableCell>
                        {formatValueWithUnit(log.amount_before, currency)}
                        {' → '}
                        {formatValueWithUnit(log.amount_after, currency)}
                      </TableCell>
                      <TableCell>
                        {formatCurrencyAmount(log.budget_delta, currency, {
                          prefix: Number(log.budget_delta) > 0 ? '+' : '',
                        })}
                      </TableCell>
                      <TableCell>
                        {log.created_at ? formatTimestamp(log.created_at) : '-'}
                      </TableCell>
                    </TableRow>
                  )
                })
              )}
            </TableBody>
          </Table>
        </div>
      </div>
    </TokenApplyPageShell>
  )
}
