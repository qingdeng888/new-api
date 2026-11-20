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
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { DataTableToolbar } from '@/components/data-table'

import { useTableUrlState, type NavigateFn } from '../use-table-url-state'

const rows = [{ name: 'new api' }]
const columns = [{ accessorKey: 'name' }]
const searchRoute = createRootRoute({
  validateSearch: (search: Record<string, unknown>) => search,
  component: TableSearchFixture,
})

function TableSearchFixture() {
  return (
    <SearchToolbar
      search={searchRoute.useSearch()}
      navigate={searchRoute.useNavigate()}
    />
  )
}

function SearchToolbar(props: {
  search: Record<string, unknown>
  navigate: NavigateFn
  debounceMs?: number
}) {
  const state = useTableUrlState({
    search: props.search,
    navigate: props.navigate,
  })
  const table = useReactTable({
    data: rows,
    columns,
    getCoreRowModel: getCoreRowModel(),
    state: {
      globalFilter: state.globalFilter,
      columnFilters: state.columnFilters,
      pagination: state.pagination,
    },
    onGlobalFilterChange: state.onGlobalFilterChange,
  })
  return (
    <DataTableToolbar
      table={table}
      searchPlaceholder='Search channels'
      searchDebounceMs={props.debounceMs ?? 10}
      hideViewOptions
    />
  )
}

function renderTableSearch() {
  const history = createMemoryHistory({
    initialEntries: ['/?filter=old&page=3'],
  })
  const router = createRouter({ routeTree: searchRoute, history })
  render(<RouterProvider router={router} />)
  return { history, router }
}

describe('table URL search state', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  })

  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('restores the search term and page when browser history changes the URL', () => {
    const navigate = vi.fn<NavigateFn>()
    const initialSearch: Record<string, unknown> = { filter: 'openai', page: 3 }
    const { result, rerender } = renderHook(
      (search: Record<string, unknown>) =>
        useTableUrlState({ search, navigate }),
      { initialProps: initialSearch }
    )
    expect(result.current.globalFilter).toBe('openai')
    expect(result.current.pagination.pageIndex).toBe(2)

    rerender({ filter: 'claude', page: 2 })
    expect(result.current.globalFilter).toBe('claude')
    expect(result.current.pagination.pageIndex).toBe(1)

    rerender({})
    expect(result.current.globalFilter).toBe('')
    expect(result.current.pagination.pageIndex).toBe(0)
    expect(navigate).not.toHaveBeenCalled()
  })

  it('reads the configured search key when it changes on the same page', () => {
    const navigate = vi.fn<NavigateFn>()
    const search = { filter: 'model', vFilter: 'vendor' }
    const { result, rerender } = renderHook(
      (key: string) =>
        useTableUrlState({
          search,
          navigate,
          globalFilter: { key },
        }),
      { initialProps: 'filter' }
    )

    rerender('vFilter')
    expect(result.current.globalFilter).toBe('vendor')
  })

  it('commits an edited search term and resets pagination while keeping other filters', () => {
    let search: Record<string, unknown> = {
      filter: 'openai',
      page: 3,
      pageSize: 50,
      status: ['enabled'],
    }
    const navigate: NavigateFn = (options) => {
      if (typeof options.search === 'function') {
        search = options.search(search)
      }
    }
    const { result, rerender } = renderHook(() =>
      useTableUrlState({ search, navigate })
    )

    act(() => result.current.onGlobalFilterChange?.('  claude  '))
    rerender()

    expect(result.current.globalFilter).toBe('claude')
    expect(result.current.pagination).toEqual({ pageIndex: 0, pageSize: 50 })
    expect(search.status).toEqual(['enabled'])
  })

  it('keeps newer input when an earlier search submission is written back to the URL', async () => {
    const user = userEvent.setup()
    const navigate = vi.fn<NavigateFn>()
    const view = render(
      <SearchToolbar
        search={{ filter: 'old' }}
        navigate={navigate}
        debounceMs={0}
      />
    )
    const input = screen.getByPlaceholderText('Search channels')
    await user.clear(input)
    expect(navigate).toHaveBeenCalled()
    fireEvent.change(input, { target: { value: 'n' } })
    expect(input).toHaveValue('n')

    view.rerender(
      <SearchToolbar search={{}} navigate={navigate} debounceMs={0} />
    )
    expect(input).toHaveValue('n')
    view.rerender(
      <SearchToolbar
        search={{ filter: 'n' }}
        navigate={navigate}
        debounceMs={0}
      />
    )
    expect(input).toHaveValue('n')
  })

  it('discards repeated normalized submissions when navigating away and resets pending input', async () => {
    const user = userEvent.setup()
    const navigate = vi.fn<NavigateFn>()
    const view = render(
      <SearchToolbar
        search={{ filter: 'new' }}
        navigate={navigate}
        debounceMs={0}
      />
    )
    const input = screen.getByPlaceholderText('Search channels')
    fireEvent.change(input, { target: { value: 'new ' } })
    view.rerender(
      <SearchToolbar
        search={{ filter: 'new' }}
        navigate={navigate}
        debounceMs={0}
      />
    )
    fireEvent.change(input, { target: { value: 'new  ' } })
    view.rerender(
      <SearchToolbar
        search={{ filter: 'other' }}
        navigate={navigate}
        debounceMs={0}
      />
    )
    expect(input).toHaveValue('other')
    view.rerender(
      <SearchToolbar
        search={{ filter: 'new' }}
        navigate={navigate}
        debounceMs={0}
      />
    )
    expect(input).toHaveValue('new')

    fireEvent.change(input, { target: { value: 'pending' } })
    await user.click(screen.getByRole('button', { name: 'Reset' }))
    view.rerender(
      <SearchToolbar search={{}} navigate={navigate} debounceMs={0} />
    )
    expect(input).toHaveValue('')
  })

  it('keeps a multiword search draft and restores the input when browser history resets the URL', async () => {
    const user = userEvent.setup()
    const { router, history } = renderTableSearch()
    const input = await screen.findByPlaceholderText('Search channels')

    await user.clear(input)
    await user.type(input, 'new ')
    await waitFor(() => expect(router.state.location.search.filter).toBe('new'))
    expect(input).toHaveValue('new ')

    await user.type(input, 'api')
    await waitFor(() =>
      expect(router.state.location.search.filter).toBe('new api')
    )
    expect(router.state.location.search.page).toBeUndefined()

    await act(async () => {
      await router.navigate({ to: '/', search: {} })
    })
    expect(input).toHaveValue('')
    await act(async () => history.back())
    await waitFor(() => expect(input).toHaveValue('new api'))
  })

  it('does not revive a discarded space draft when history returns to its trimmed value', async () => {
    const user = userEvent.setup()
    const { router, history } = renderTableSearch()
    const input = await screen.findByPlaceholderText('Search channels')
    await user.clear(input)
    await user.type(input, 'new ')
    await waitFor(() => expect(router.state.location.search.filter).toBe('new'))
    expect(input).toHaveValue('new ')

    await act(async () => {
      await router.navigate({ to: '/', search: { filter: 'other' } })
    })
    expect(input).toHaveValue('other')
    await act(async () => history.back())

    await waitFor(() => expect(input).toHaveValue('new'))
  })

  it('commits Chinese IME text only after composition ends', async () => {
    const { router } = renderTableSearch()
    const input = await screen.findByPlaceholderText('Search channels')

    fireEvent.compositionStart(input)
    fireEvent.change(input, { target: { value: '中文' } })
    expect(input).toHaveValue('中文')
    expect(router.state.location.search.filter).toBe('old')

    fireEvent.compositionEnd(input)
    await waitFor(() =>
      expect(router.state.location.search.filter).toBe('中文')
    )
    expect(input).toHaveValue('中文')
    expect(router.state.location.search.page).toBeUndefined()
  })

  it('keeps global search disabled when the URL contains a search term', () => {
    const { result } = renderHook(() =>
      useTableUrlState({
        search: { filter: 'ignored' },
        navigate: vi.fn<NavigateFn>(),
        globalFilter: { enabled: false },
      })
    )

    expect(result.current.globalFilter).toBeUndefined()
    expect(result.current.onGlobalFilterChange).toBeUndefined()
  })
})
