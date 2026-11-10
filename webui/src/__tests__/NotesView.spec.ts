import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import NotesView from '@/views/NotesView.vue'
import * as api from '@/lib/api/api'
import { NOTE_DRAG_TYPE } from '@/lib/noteDrag'
import { dispatchDrag, fakeDataTransfer, mountWithApp, problem } from './helpers'

vi.mock('@/lib/api/api')

const me = { userId: 'u1', subject: 'alice', tenantId: 't1', tenant: 'acme' }
const note = (id: string, createdBy: string) => ({
    id,
    createdBy,
    title: `title ${id}`,
    body: '',
    createdAt: '2026-01-02T03:04:05Z',
    tagIds: [] as number[]
})

describe('NotesView', () => {
    beforeEach(() => {
        vi.mocked(api.getMe).mockResolvedValue(me)
        vi.mocked(api.listTags).mockResolvedValue([])
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

    it('lists the notes of the tag in the URL, with the tag named', async () => {
        vi.mocked(api.listTags).mockResolvedValue([
            {
                id: 1,
                parentId: 0,
                name: 'work',
                children: [{ id: 2, parentId: 1, name: 'reports', children: [] }]
            }
        ])
        vi.mocked(api.listNotes).mockResolvedValue([{ ...note('n1', 'u1'), tagIds: [2] }])
        const w = await mountWithApp(NotesView)
        await w.vm.$router.push('/?tag=1&q=x')
        await flushPromises()

        expect(api.listNotes).toHaveBeenLastCalledWith(1)
        expect(w.find('[data-test="tag-filter-bar"]').text()).toContain('Notes tagged work')
        expect(w.find('[data-test="tag-filter-clear"]').attributes('href')).toBe('/?q=x')
    })

    it('says when a tag has no notes, and when it is gone', async () => {
        vi.mocked(api.listNotes).mockResolvedValue([])
        const w = await mountWithApp(NotesView)
        await w.vm.$router.push('/?tag=9')
        await flushPromises()
        expect(w.find('[data-test="empty-tag"]').exists()).toBe(true)
        expect(w.find('[data-test="tag-filter-bar"]').text()).toContain('does not exist any more')
    })

    it('shows the tags on the cards, and a card drags as a note', async () => {
        vi.mocked(api.listTags).mockResolvedValue([
            { id: 3, parentId: 0, name: 'home', children: [] }
        ])
        vi.mocked(api.listNotes).mockResolvedValue([{ ...note('n1', 'u1'), tagIds: [3, 99] }])
        const w = await mountWithApp(NotesView)

        // a tag the tree does not have (deleted meanwhile) is left out
        expect(w.findAll('[data-test="note-tag"]').map((c) => c.text())).toEqual(['home'])

        const drag = fakeDataTransfer()
        dispatchDrag(w.find('.card').element, 'dragstart', drag)
        expect(JSON.parse(drag.getData(NOTE_DRAG_TYPE))).toEqual({
            id: 'n1',
            title: 'title n1',
            tagIds: [3, 99]
        })
    })
})
