// Dragging a tag in the tag editor moves it. Its drag carries the tag's id
// under a type of its own, like a note's (noteDrag.ts), so a row takes a tag and
// nothing else: not a note dragged in from the list behind the dialog, not a link.

export const TAG_DRAG_TYPE = 'application/x-refapp-tag'

/** Puts the tag into the drag. False when the event carries no data to put it in. */
export function startTagDrag(event: DragEvent, tagId: number): boolean {
    if (!event.dataTransfer) return false
    event.dataTransfer.setData(TAG_DRAG_TYPE, String(tagId))
    event.dataTransfer.effectAllowed = 'move'
    return true
}

/** Whether the drag carries a tag; during dragover only the types can be read. */
export function carriesTag(event: DragEvent): boolean {
    return event.dataTransfer?.types.includes(TAG_DRAG_TYPE) ?? false
}
