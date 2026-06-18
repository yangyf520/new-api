/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

/** Quota units → application currency amount (matches backend QuotaToAmount). */
export function quotaToApplicationAmount(
  quota: number,
  currency = 'CNY'
): number {
  const config =
    useSystemConfigStore.getState().config?.currency ?? DEFAULT_CURRENCY_CONFIG
  const quotaPerUnit =
    config.quotaPerUnit > 0
      ? config.quotaPerUnit
      : DEFAULT_CURRENCY_CONFIG.quotaPerUnit
  const q = Number(quota || 0)
  if (!Number.isFinite(q) || q <= 0) return 0
  const usd = q / quotaPerUnit
  const normalized = String(currency || 'CNY')
    .trim()
    .toUpperCase()
  if (normalized === 'USD') return roundApplicationAmount(usd)
  if (normalized === 'CNY' || normalized === 'RMB' || normalized === '') {
    const rate =
      config.usdExchangeRate > 0
        ? config.usdExchangeRate
        : DEFAULT_CURRENCY_CONFIG.usdExchangeRate
    return roundApplicationAmount(usd * rate)
  }
  return roundApplicationAmount(usd)
}

function roundApplicationAmount(amount: number): number {
  return Math.round(amount * 10000) / 10000
}