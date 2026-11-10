import { readonly, ref } from 'vue'
import { startNoteDrag, type DraggedNote } from '@/lib/noteDrag'

// Whether a note is being dragged anywhere in the app. The sidebar opens its
// whole tag tree while one is, so a note can be dropped on a tag that sits
// inside a closed one. A module-level singleton: there is one drag at a time.
const dragging = ref(false)

function end(): void {
    dragging.value = false
}

// The card that started the drag says when it ends (dragend). A card removed
// from the page mid-drag (the list refetched, the route changed) never does, so
// the window's own end of the drag is the safety net.
function start(event: DragEvent, note: DraggedNote): void {
    if (!startNoteDrag(event, note)) return
    dragging.value = true
    window.addEventListener('dragend', end, { once: true })
    window.addEventListener('drop', end, { once: true })
}

export function useNoteDrag() {
    return { dragging: readonly(dragging), start, end }
}
