/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { KeyRound } from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { CopyButton } from '@/components/copy-button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  formatApplicationBalanceAmount,
  formatCurrencyAmount,
  rowCurrency,
} from './format'

type ApplicationRow = {
  id: number
  ticket_no: string
  amount?: number
  currency?: string
  token_id?: number
}

type ApplicationToken = {
  token_id: number
  token_name: string
  token_key: string
  remain_quota: number
  used_quota: number
  remain_amount?: number
  used_amount?: number
  currency?: string
}

async function fetchApplicationToken(id: number) {
  const res = await api.get(`/api/token-apply/records/${id}/token`)
  return (res.data?.data ?? null) as ApplicationToken | null
}

function DetailRow(props: { label: string; value: ReactNode }) {
  return (
    <div className='flex flex-col gap-1'>
      <span className='text-muted-foreground text-xs'>{props.label}</span>
      <div className='text-sm font-medium'>{props.value}</div>
    </div>
  )
}

function QuotaStat(props: { label: string; value: string; className?: string }) {
  return (
    <div className='bg-muted/40 flex min-w-0 flex-col gap-1 rounded-lg border px-4 py-3'>
      <span className='text-muted-foreground text-xs'>{props.label}</span>
      <span
        className={`text-sm leading-snug font-semibold break-words ${props.className ?? ''}`}
      >
        {props.value}
      </span>
    </div>
  )
}

export function TokenApplyTokenDialog(props: {
  application: ApplicationRow | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [token, setToken] = useState<ApplicationToken | null>(null)

  const loadToken = useCallback(async () => {
    if (!props.application?.id) {
      setToken(null)
      return
    }
    setLoading(true)
    try {
      const data = await fetchApplicationToken(props.application.id)
      setToken(data)
    } catch {
      setToken(null)
      toast.error(t('Failed to load'))
    } finally {
      setLoading(false)
    }
  }, [props.application?.id, t])

  useEffect(() => {
    if (props.open) {
      void loadToken()
    } else {
      setToken(null)
    }
  }, [props.open, loadToken])

  const currency = rowCurrency(token || props.application || {})

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='gap-0 overflow-hidden p-0 sm:max-w-xl'>
        <DialogHeader className='border-b px-6 py-4'>
          <DialogTitle>{t('Application Token')}</DialogTitle>
        </DialogHeader>
        <div className='px-6 py-4'>
          {loading ? (
            <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>
          ) : !token ? (
            <p className='text-muted-foreground py-8 text-center text-sm'>
              {t('Not found')}
            </p>
          ) : (
            <div className='flex flex-col gap-4'>
              <div className='bg-muted/40 rounded-lg border p-4'>
                <div className='mb-3 flex items-center gap-2'>
                  <KeyRound className='text-primary size-4' />
                  <span className='text-sm font-semibold'>{t('Token Key')}</span>
                </div>
                <div className='flex items-center gap-2 px-1 py-1'>
                  {token.token_key ? (
                    <CopyButton
                      value={token.token_key}
                      variant='ghost'
                      size='icon'
                      tooltip={t('Copy')}
                    />
                  ) : null}
                  <span className='text-muted-foreground min-w-0 flex-1 overflow-x-auto text-sm whitespace-nowrap'>
                    {token.token_key || '-'}
                  </span>
                </div>
              </div>

              <div className='bg-muted/40 rounded-lg border p-4'>
                <h3 className='mb-3 text-sm font-semibold'>{t('Token Info')}</h3>
                <div className='grid gap-4 sm:grid-cols-2'>
                  <DetailRow
                    label={t('Process No.')}
                    value={props.application?.ticket_no || '-'}
                  />
                  <DetailRow label={t('Token Name')} value={token.token_name || '-'} />
                  <DetailRow label={t('Token ID')} value={token.token_id} />
                </div>
              </div>

              <div className='bg-muted/40 rounded-lg border p-4'>
                <h3 className='mb-3 text-sm font-semibold'>{t('Quota Info')}</h3>
                <div className='grid grid-cols-3 gap-3'>
                  <QuotaStat
                    label={t('Remaining Amount')}
                    value={formatApplicationBalanceAmount(
                      token,
                      'remain_amount',
                      'remain_quota',
                      currency
                    )}
                    className='text-success'
                  />
                  <QuotaStat
                    label={t('Used Amount')}
                    value={formatApplicationBalanceAmount(
                      token,
                      'used_amount',
                      'used_quota',
                      currency
                    )}
                  />
                  <QuotaStat
                    label={t('Approved Amount')}
                    value={formatCurrencyAmount(props.application?.amount, currency)}
                    className='text-primary'
                  />
                </div>
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
