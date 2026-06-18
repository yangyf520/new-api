/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import type { ReactNode } from 'react'
import { quotaToApplicationAmount } from '@/lib/quota-application'

type FormatOptions = {
  formatNumber?: boolean
  prefix?: string
}

function normalizeApplicationCurrency(currency = 'CNY'): string {
  const code = String(currency || 'CNY').trim().toUpperCase()
  if (code === 'RMB' || code === '') return 'CNY'
  return code
}

function applicationCurrencyFractionDigits(currency = 'CNY'): number {
  switch (normalizeApplicationCurrency(currency)) {
    case 'CNY':
    case 'USD':
      return 2
    default:
      return 2
  }
}

export function formatMoneyNumber(
  value: number | string,
  currency = 'CNY'
): string {
  const n = Number(value)
  if (!Number.isFinite(n)) {
    return String(value)
  }
  const digits = applicationCurrencyFractionDigits(currency)
  const factor = 10 ** digits
  const rounded = Math.round(n * factor) / factor
  return rounded.toLocaleString(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: digits,
  })
}

export function formatValueWithUnit(
  value: number | string | null | undefined,
  unit?: string,
  options: FormatOptions = {}
): ReactNode {
  if (value == null || value === '') {
    return '-'
  }
  const display = options.formatNumber
    ? Number(value).toLocaleString()
    : `${options.prefix ?? ''}${formatMoneyNumber(value as number | string, unit)}`
  if (!unit) {
    return display
  }
  return (
    <span>
      {display}
      <span className='text-muted-foreground text-xs'> ({unit})</span>
    </span>
  )
}

export function formatCurrencyAmount(
  value: number | string | null | undefined,
  currency = 'CNY',
  options: FormatOptions = {}
): ReactNode {
  return formatValueWithUnit(value, currency || 'CNY', options)
}

export function formatApplicationBalanceAmount(
  row: Record<string, unknown> | null | undefined,
  amountField: string,
  quotaField: string,
  currency = 'CNY'
): ReactNode {
  const amount = row?.[amountField]
  if (amount != null && amount !== '') {
    return formatCurrencyAmount(amount as number, currency)
  }
  return formatApplicationQuota(row?.[quotaField] as number | null | undefined, currency)
}

export function formatApplicationQuota(
  value: number | null | undefined,
  currency = 'CNY'
): ReactNode {
  if (value == null) {
    return '-'
  }
  const amount = quotaToApplicationAmount(value, currency)
  return formatCurrencyAmount(amount, currency || 'CNY')
}

export function rowCurrency(
  row: { currency?: string },
  fallback = 'CNY'
): string {
  return row.currency || fallback
}
