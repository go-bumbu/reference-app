import { inject, type InjectionKey, type Ref } from 'vue'
import type { RouteLocationRaw } from 'vue-router'
import type { Tag } from '@/lib/api/types'

// What the sidebar (AppNav) hands down to every level of its tag tree
// (SidebarTagTree), which is recursive: one context instead of the same props
// and events threaded through each level.
export interface SidebarTagsContext {
    // the tag the notes list is narrowed to, if any
    activeId: Ref<number | null>
    // the tag a dragged note is over, which shows it takes the drop
    dropId: Ref<number | null>
    isOpen: (tag: Tag) => boolean
    toggle: (tag: Tag) => void
    linkTo: (tag: Tag) => RouteLocationRaw
    onDragOver: (event: DragEvent, tag: Tag) => void
    onDragLeave: (event: DragEvent, tag: Tag) => void
    onDrop: (event: DragEvent, tag: Tag) => void
}

export const SIDEBAR_TAGS: InjectionKey<SidebarTagsContext> = Symbol('sidebar-tags')

/** The context, for the tree's levels; outside AppNav there is none, which is a wiring bug. */
export function useSidebarTags(): SidebarTagsContext {
    const ctx = inject(SIDEBAR_TAGS, null)
    if (!ctx)
        throw new Error('SidebarTagTree must be rendered inside AppNav, which provides its context')
    return ctx
}
