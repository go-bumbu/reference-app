import { readonly, ref } from 'vue'

const STORAGE_KEY = 'refapp-tags-expanded'

function load(): Set<number> {
    try {
        const ids: unknown = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '[]')
        return new Set(Array.isArray(ids) ? ids.filter((id) => typeof id === 'number') : [])
    } catch {
        return new Set()
    }
}

function save(ids: Set<number>): void {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify([...ids]))
    } catch {
        // storage unavailable (private mode): keep the in-memory value
    }
}

// Which tags of the sidebar's tree show their children, kept on the device: a
// view preference, not server state. Only the sidebar reads it, so each call
// loads its own copy instead of sharing a module-level singleton. Ids of tags
// deleted since stay in the set, harmlessly.
export function useTagExpansion() {
    const expanded = ref(load())

    function update(next: Set<number>) {
        expanded.value = next
        save(next)
    }

    function toggle(id: number): void {
        const next = new Set(expanded.value)
        if (!next.delete(id)) next.add(id)
        update(next)
    }

    // Opens the tags (those above the active one), leaving the others as they are.
    function open(ids: number[]): void {
        if (ids.every((id) => expanded.value.has(id))) return
        update(new Set([...expanded.value, ...ids]))
    }

    return { expanded: readonly(expanded), toggle, open }
}
