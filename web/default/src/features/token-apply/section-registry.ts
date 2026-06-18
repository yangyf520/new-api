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
import { type TFunction } from 'i18next'
import { FileText, Gauge, ReceiptText } from 'lucide-react'
import type { NavGroup, SidebarView } from '@/components/layout/types'

export const TOKEN_APPLY_SECTIONS = [
  'records',
  'budget',
  'consumption',
] as const

export type TokenApplySection = (typeof TOKEN_APPLY_SECTIONS)[number]

export const TOKEN_APPLY_DEFAULT_SECTION: TokenApplySection = 'records'

export function isTokenApplySection(value: string): value is TokenApplySection {
  return (TOKEN_APPLY_SECTIONS as readonly string[]).includes(value)
}

const SECTION_TITLE: Record<TokenApplySection, string> = {
  records: 'Token Applications',
  budget: 'Budget Policies',
  consumption: 'Consumption Policies',
}

export function getTokenApplySectionTitle(section: TokenApplySection): string {
  return SECTION_TITLE[section]
}

export function getTokenApplyNavGroups(t: TFunction): NavGroup[] {
  return [
    {
      id: 'token-apply',
      title: t('Department Quota'),
      items: [
        {
          title: t('Token Applications'),
          url: '/token-apply',
          icon: FileText,
        },
        {
          title: t('Budget Policies'),
          url: '/token-apply/budget',
          icon: Gauge,
        },
        {
          title: t('Consumption Policies'),
          url: '/token-apply/consumption',
          icon: ReceiptText,
        },
      ],
    },
  ]
}

export const TOKEN_APPLY_VIEW: SidebarView = {
  id: 'token-apply',
  pathPattern: /^\/token-apply(\/|$)/,
  parent: {
    to: '/profile',
    label: 'Back to Personal',
  },
  getNavGroups: getTokenApplyNavGroups,
}
