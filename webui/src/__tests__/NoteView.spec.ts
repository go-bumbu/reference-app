import { describe, expect, it, vi } from 'vitest'
import NoteView from '@/views/NoteView.vue'
import * as api from '@/lib/api/api'
import { mountWithApp, problem } from './helpers'

vi.mock('@/lib/api/api')

describe('NoteView', () => {
    it('shows the note', async () => {
        vi.mocked(api.getNote).mockResolvedValue({
            id: 'n1',
            createdBy: 'u1',
            title: 'Plans',
            body: 'text',
            createdAt: '2026-01-02T03:04:05Z'
        })
        const w = await mountWithApp(NoteView, { id: 'n1' })
        expect(api.getNote).toHaveBeenCalledWith('n1')
        expect(w.find('h1').text()).toBe('Plans')
    })

    it('says a missing note does not exist', async () => {
        vi.mocked(api.getNote).mockRejectedValue(problem(404))
        const w = await mountWithApp(NoteView, { id: 'gone' })
        expect(w.find('[data-test="not-found"]').exists()).toBe(true)
    })
})
