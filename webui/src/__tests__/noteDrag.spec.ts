import { describe, expect, it } from 'vitest'
import { carriesNote, droppedNote, NOTE_DRAG_TYPE, startNoteDrag } from '@/lib/noteDrag'
import { fakeDataTransfer } from './helpers'

const withData = (dataTransfer: ReturnType<typeof fakeDataTransfer> | null) =>
    ({ dataTransfer }) as unknown as DragEvent

describe('noteDrag', () => {
    it('carries the note under its own type', () => {
        const drag = fakeDataTransfer()
        expect(startNoteDrag(withData(drag), { id: 'n1', title: 'Plan', tagIds: [2] })).toBe(true)
        expect(drag.types).toEqual([NOTE_DRAG_TYPE])
        expect(carriesNote(withData(drag))).toBe(true)
        expect(droppedNote(withData(drag))).toEqual({ id: 'n1', title: 'Plan', tagIds: [2] })
    })

    it('starts no drag without data to put it in', () => {
        expect(startNoteDrag(withData(null), { id: 'n1', title: 'Plan', tagIds: [] })).toBe(false)
        expect(carriesNote(withData(null))).toBe(false)
        expect(droppedNote(withData(null))).toBeNull()
    })

    it('reads nothing but a note', () => {
        expect(carriesNote(withData(fakeDataTransfer({ 'text/plain': 'x' })))).toBe(false)
        for (const raw of ['{', '{"id":1,"title":"x"}', '{"title":"x"}', '"n1"']) {
            expect(droppedNote(withData(fakeDataTransfer({ [NOTE_DRAG_TYPE]: raw })))).toBeNull()
        }
        // tag ids that are not numbers are dropped, a missing list is empty
        expect(
            droppedNote(
                withData(
                    fakeDataTransfer({
                        [NOTE_DRAG_TYPE]: '{"id":"n1","title":"x","tagIds":[1,"2"]}'
                    })
                )
            )
        ).toEqual({ id: 'n1', title: 'x', tagIds: [1] })
        expect(
            droppedNote(withData(fakeDataTransfer({ [NOTE_DRAG_TYPE]: '{"id":"n1","title":"x"}' })))
        ).toEqual({ id: 'n1', title: 'x', tagIds: [] })
    })
})
