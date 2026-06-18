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
import z from 'zod'
import { createFileRoute, redirect } from '@tanstack/react-router'
import {
  isTokenApplySection,
  TOKEN_APPLY_DEFAULT_SECTION,
} from '@/features/token-apply/section-registry'
import { TokenApplySectionPage } from '@/features/token-apply'

const searchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(20),
  filter: z.string().optional().catch(''),
})

export const Route = createFileRoute('/_authenticated/token-apply/$section')({
  beforeLoad: ({ params }) => {
    if (!isTokenApplySection(params.section)) {
      throw redirect({
        to: '/token-apply/$section',
        params: { section: TOKEN_APPLY_DEFAULT_SECTION },
      })
    }
  },
  validateSearch: searchSchema,
  component: TokenApplySectionPage,
})
