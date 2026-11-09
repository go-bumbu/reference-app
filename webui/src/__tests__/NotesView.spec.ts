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

    it('shows the empty state', async () => {
        vi.mocked(api.listNotes).mockResolvedValue([])
        const w = await mountWithApp(NotesView)
        expect(w.find('[data-test="empty"]').exists()).toBe(true)
    })

    it('creates a note, clears the form and refreshes the list', async () => {
        vi.mocked(api.listNotes)
            .mockResolvedValueOnce([])
            .mockResolvedValue([note('n1', 'u1')])
        vi.mocked(api.createNote).mockResolvedValue(note('n1', 'u1'))
        const w = await mountWithApp(NotesView)

        await w.find('[data-test="title"]').setValue('hello')
        await w.find('form').trigger('submit')
        await flushPromises()

        expect(api.createNote).toHaveBeenCalledWith({ title: 'hello', body: '' })
        expect((w.find('[data-test="title"]').element as HTMLInputElement).value).toBe('')
        expect(w.findAll('[data-test="note"]')).toHaveLength(1)
    })

    it('shows the server validation error on the field', async () => {
        vi.mocked(api.listNotes).mockResolvedValue([])
        vi.mocked(api.createNote).mockRejectedValue(
            problem(422, { errors: [{ pointer: '/title', detail: 'must not be empty' }] })
        )
        const w = await mountWithApp(NotesView)

        await w.find('form').trigger('submit')
        await flushPromises()

        expect(w.find('[data-test="title-error"]').text()).toBe('must not be empty')
    })
})
