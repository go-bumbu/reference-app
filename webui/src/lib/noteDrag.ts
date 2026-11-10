// Dragging a note onto a tag gives the note that tag. The note travels in the
// drag under a type of its own, so a drop target can tell a note from a link, a
// file or text dragged in from elsewhere, and ignore those.

export const NOTE_DRAG_TYPE = 'application/x-refapp-note'

/** What a drop target learns of the dragged note: enough to name it and to tell whether it has the tag. */
export interface DraggedNote {
    id: string
    title: string
    tagIds: number[]
}

/**
 * Puts the note into the drag, for a drop that adds a tag to it (no move, no
 * copy of the note). False when the event carries no data to put it in.
 */
export function startNoteDrag(event: DragEvent, note: DraggedNote): boolean {
    if (!event.dataTransfer) return false
    const payload: DraggedNote = { id: note.id, title: note.title, tagIds: note.tagIds }
    event.dataTransfer.setData(NOTE_DRAG_TYPE, JSON.stringify(payload))
    // the card is a link: a drop outside the app still gets the note's URL
    event.dataTransfer.effectAllowed = 'copyLink'
    return true
}

/**
 * Whether the drag carries a note. During dragover only the types of the data
 * can be read, not the data itself.
 */
export function carriesNote(event: DragEvent): boolean {
    return event.dataTransfer?.types.includes(NOTE_DRAG_TYPE) ?? false
}

/** The note a drop carries, or null for anything else. */
export function droppedNote(event: DragEvent): DraggedNote | null {
    const raw = event.dataTransfer?.getData(NOTE_DRAG_TYPE)
    if (!raw) return null
    try {
        const note = JSON.parse(raw) as Partial<DraggedNote>
        if (typeof note.id !== 'string' || typeof note.title !== 'string') return null
        const tagIds = Array.isArray(note.tagIds)
            ? note.tagIds.filter((id): id is number => typeof id === 'number')
            : []
        return { id: note.id, title: note.title, tagIds }
    } catch {
        return null
    }
}
