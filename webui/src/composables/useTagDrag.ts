import { ref, type Ref } from 'vue'
import type { Tag, TagPatch } from '@/lib/api/types'
import { carriesTag, startTagDrag } from '@/lib/tagDrag'
import { dropPosition, findTag, resolveTagDrop, type DropPos } from '@/lib/tagTree'

/**
 * Moving the tags of the tag editor by drag: a row dragged onto another nests
 * under it (the row's middle) or goes before or after it (its edges), and the
 * strip at the foot of the list takes it back to the top level. Which drops are
 * allowed, and what they send, is lib/tagTree's; this keeps the drag's state
 * and calls `move` with the PATCH a drop means.
 */
export function useTagDrag(tree: Ref<Tag[]>, move: (tagId: number, patch: TagPatch) => void) {
    // the tag being dragged, and where it would land
    const draggedId = ref<number | null>(null)
    const drop = ref<{ id: number; pos: DropPos } | null>(null)
    const rootDrop = ref(false)

    function reset() {
        draggedId.value = null
        drop.value = null
        rootDrop.value = false
    }

    function onDragStart(event: DragEvent, tag: Tag) {
        if (startTagDrag(event, tag.id)) draggedId.value = tag.id
    }

    function ours(event: DragEvent): boolean {
        return draggedId.value !== null && carriesTag(event)
    }

    function patchFor(event: DragEvent, tag: Tag): { pos: DropPos; patch: TagPatch } | null {
        if (!ours(event) || draggedId.value === null) return null
        const row = event.currentTarget as HTMLElement
        const pos = dropPosition(row.getBoundingClientRect(), event.clientY)
        const patch = resolveTagDrop(tree.value, draggedId.value, tag.id, pos)
        return patch ? { pos, patch } : null
    }

    function onDragOver(event: DragEvent, tag: Tag) {
        const target = patchFor(event, tag)
        if (!target) {
            if (drop.value?.id === tag.id) drop.value = null
            return
        }
        event.preventDefault() // take the drop
        if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
        drop.value = { id: tag.id, pos: target.pos }
        rootDrop.value = false
    }

    // Leaving the row for anything but a child of it takes its mark away, so no
    // row promises a drop the pointer is no longer over.
    function onDragLeave(event: DragEvent, tag: Tag) {
        const row = event.currentTarget as HTMLElement | null
        if (row?.contains(event.relatedTarget as Node | null)) return
        if (drop.value?.id === tag.id) drop.value = null
    }

    function onDrop(event: DragEvent, tag: Tag) {
        const target = patchFor(event, tag)
        const dragged = draggedId.value
        reset()
        if (!target || dragged === null) return
        event.preventDefault()
        move(dragged, target.patch)
    }

    function onRootDragOver(event: DragEvent) {
        const dragged = draggedId.value === null ? undefined : findTag(tree.value, draggedId.value)
        if (!ours(event) || !dragged || dragged.parentId === 0) return
        event.preventDefault()
        rootDrop.value = true
        drop.value = null
    }

    function onRootDragLeave() {
        rootDrop.value = false
    }

    function onRootDrop(event: DragEvent) {
        const dragged = draggedId.value
        const ok = rootDrop.value
        reset()
        if (!ok || dragged === null) return
        event.preventDefault()
        move(dragged, { parentId: 0 })
    }

    return {
        draggedId,
        drop,
        rootDrop,
        reset,
        onDragStart,
        onDragEnd: reset,
        onDragOver,
        onDragLeave,
        onDrop,
        onRootDragOver,
        onRootDragLeave,
        onRootDrop
    }
}
