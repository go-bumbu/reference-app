import { readonly, ref } from 'vue'

const STORAGE_KEY = 'refapp-sidebar-collapsed'

function load(): boolean {
    try {
        return localStorage.getItem(STORAGE_KEY) === '1'
    } catch {
        return false
    }
}

// Module-level singleton, like useTheme: one sidebar for the whole app. A view
// preference kept on the device, not server state, so it is no TanStack query.
const collapsed = ref(load())

function toggle(): void {
    collapsed.value = !collapsed.value
    try {
        localStorage.setItem(STORAGE_KEY, collapsed.value ? '1' : '0')
    } catch {
        // storage unavailable (private mode): keep the in-memory value
    }
}

export function useSidebar() {
    return { collapsed: readonly(collapsed), toggle }
}
