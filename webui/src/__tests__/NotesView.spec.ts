import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import NotesView from '@/views/NotesView.vue'
import * as api from '@/lib/api/api'
import { mountWithApp, problem } from './helpers'

vi.mock('@/lib/api/api')

const me = { userId: 'u1', subject: 'alice', tenantId: 't1', tenant: 'acme' }
const note = (id: string, createdBy: string) => ({
    id,
    createdBy,
    title: `title ${id}`,
    body: '',
    createdAt: '2026-01-02T03:04:05Z'
})

describe('NotesView', () => {
    beforeEach(() => {
        vi.mocked(api.getMe).mockResolvedValue(me)
    })

    it('lists the notes with their author', async () => {
        vi.mocked(api.listNotes).mockResolvedValue([note('n1', 'u1'), note('n2', 'u2')])
        const w = await mountWithApp(NotesView)
        const items = w.findAll('[data-test="note"]')
        expect(items).toHaveLength(2)
        expect(items[0].text()).toContain('by you')
        expect(items[1].text()).toContain('by another member')
    })

    it('filters the list by the search query in the URL', async () => {
        vi.mocked(api.listNotes).mockResolvedValue([
            { ...note('n1', 'u1'), title: 'Buy milk' },
            { ...note('n2', 'u1'), title: 'Call Bob', body: 'about the MILK order' },
            { ...note('n3', 'u1'), title: 'Read a book' }
        ])
        const w = await mountWithApp(NotesView)
        await w.vm.$router.push('/?q=milk')
        await flushPromises()
        expect(w.findAll('[data-test="note"]')).toHaveLength(2)

        await w.vm.$router.push('/?q=nothing')
        await flushPromises()
        expect(w.find('[data-test="no-match"]').exists()).toBe(true)
    })

    it('shows the empty state', async () => {
        vi.mocked(api.listNotes).mockResolvedValue([])
        const w = await mountWithApp(NotesView)
        expect(w.find('[data-test="empty"]').exists()).toBe(true)
    })

    it('shows an error when the list fails', async () => {
        vi.mocked(api.listNotes).mockRejectedValue(problem(500, { detail: 'boom' }))
        const w = await mountWithApp(NotesView)
        expect(w.find('[data-test="note"]').exists()).toBe(false)
        expect(w.find('[data-test="empty"]').exists()).toBe(false)
    })
})
