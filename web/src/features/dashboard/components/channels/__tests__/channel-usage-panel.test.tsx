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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { ChannelUsagePanel } from '../channel-usage-panel'

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
  returnNull: false,
})

type ChannelUsageFixture = {
  start_timestamp: number
  end_timestamp: number
  channels: Array<Record<string, unknown>>
}

let client: QueryClient
let fixture: ChannelUsageFixture

const totals = (overrides: Record<string, number> = {}) => ({
  request_count: 0,
  success_count: 0,
  failure_count: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  cache_read_tokens: 0,
  quota: 0,
  ...overrides,
})

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  fixture = {
    start_timestamp: 1000,
    end_timestamp: 2000,
    channels: [
      {
        channel_id: 5,
        channel_name: 'multi-key',
        channel_type: 1,
        removed: false,
        ...totals({
          request_count: 7,
          success_count: 6,
          failure_count: 1,
          prompt_tokens: 150,
          completion_tokens: 60,
          cache_read_tokens: 25,
        }),
        keys: [
          {
            key_index: 0,
            key_label: 'sk-f**********irst',
            fingerprint: 'aaaa',
            removed: false,
            ...totals({ request_count: 6, success_count: 5, failure_count: 1 }),
            models: [{ model_name: 'gpt-4o', ...totals({ request_count: 6 }) }],
          },
          {
            key_index: -1,
            key_label: '',
            fingerprint: 'bbbb',
            removed: true,
            ...totals({ request_count: 1, success_count: 1 }),
            models: [],
          },
        ],
      },
    ],
  }
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/data/channels') {
      return { data: { success: true, data: fixture } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

function renderPanel() {
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <ChannelUsagePanel />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

async function findChannelToggle(name: string) {
  const toggle = await screen.findByRole('button', { name: new RegExp(name) })
  return toggle
}

describe('channel usage panel', () => {
  it('lists each channel with its totals and keeps keys collapsed', async () => {
    renderPanel()

    const toggle = await findChannelToggle('multi-key')
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByText('#5')).toBeInTheDocument()
    expect(screen.getByText('7')).toBeInTheDocument()
    expect(screen.queryByText(/Key #1/)).not.toBeInTheDocument()
  })

  it('reveals the channel keys with their usage when expanded', async () => {
    const user = userEvent.setup()
    renderPanel()

    const toggle = await findChannelToggle('multi-key')
    await user.click(toggle)

    await waitFor(() => expect(toggle).toHaveAttribute('aria-expanded', 'true'))
    expect(screen.getByText('Key #1 sk-f**********irst')).toBeInTheDocument()
    expect(screen.getByText('Deleted key')).toBeInTheDocument()
    expect(screen.getByText('gpt-4o')).toBeInTheDocument()
  })

  it('collapses the keys again when the channel row is clicked twice', async () => {
    const user = userEvent.setup()
    renderPanel()

    const toggle = await findChannelToggle('multi-key')
    await user.click(toggle)
    await waitFor(() => expect(screen.getByText(/Key #1/)).toBeInTheDocument())

    await user.click(toggle)
    await waitFor(() =>
      expect(screen.queryByText(/Key #1/)).not.toBeInTheDocument()
    )
  })

  it('shows an empty state when the window holds no usage', async () => {
    fixture = { ...fixture, channels: [] }
    renderPanel()

    expect(await screen.findByText('No data')).toBeInTheDocument()
  })
})
