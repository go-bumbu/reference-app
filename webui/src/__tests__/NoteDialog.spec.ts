import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import NoteDialog from '@/components/NoteDialog.vue'
import * as api from '@/lib/api/api'
import { mountWithApp, problem } from './helpers'

vi.mock('@/lib/api/api')

// PrimeVue's Dialog teleports to <body>, so the form is looked up there.
const $ = <T extends Element = HTMLElement>(sel: string) => document.body.querySelector<T>(sel)

async function type(sel: string, value: string) {
    const el = $<HTMLInputElement>(sel)!
    el.value = value
    el.dispatchEvent(new Event('input'))
    await flushPromises()
}

async function submit() {
    $<HTMLFormElement>('[data-test="note-form"]')!.dispatchEvent(new Event('submit'))
    await flushPromises()
}

const created = {
    id: 'n1',
    createdBy: 'u1',
    title: 'hello',
    body: '',
    createdAt: '2026-01-02T03:04:05Z'
}

describe('NoteDialog', () => {
    beforeEach(() => vi.clearAllMocks())

    it('creates a note and closes', async () => {
        vi.mocked(api.createNote).mockResolvedValue(created)
        const w = await mountWithApp(NoteDialog, { visible: true })

        await type('[data-test="title"]', 'hello')
        await submit()

        expect(api.createNote).toHaveBeenCalledWith({ title: 'hello', body: '' })
        expect(w.emitted('update:visible')?.at(-1)).toEqual([false])
    })

    it('shows the server validation error on the field and stays open', async () => {
        vi.mocked(api.createNote).mockRejectedValue(
            problem(422, { errors: [{ pointer: '/title', detail: 'must not be empty' }] })
        )
        const w = await mountWithApp(NoteDialog, { visible: true })

        await submit()

        expect($('[data-test="title-error"]')?.textContent).toBe('must not be empty')
        expect(w.emitted('update:visible')).toBeUndefined()
    })

    it('given a note, opens filled in and saves it as an update', async () => {
        const note = { ...created, title: 'Plans', body: 'some text' }
        vi.mocked(api.updateNote).mockResolvedValue({ ...note, title: 'Better plans' })
        const w = await mountWithApp(NoteDialog, { visible: true, note })

        expect($<HTMLInputElement>('[data-test="title"]')!.value).toBe('Plans')
        expect($<HTMLTextAreaElement>('[data-test="body"]')!.value).toBe('some text')
        expect($('[data-test="submit"]')?.textContent).toContain('Save')

        await type('[data-test="title"]', 'Better plans')
        await submit()

        expect(api.updateNote).toHaveBeenCalledWith('n1', {
            title: 'Better plans',
            body: 'some text'
        })
        expect(api.createNote).not.toHaveBeenCalled()
        expect(w.emitted('update:visible')?.at(-1)).toEqual([false])
    })
})
