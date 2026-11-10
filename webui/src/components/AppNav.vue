<script setup lang="ts">
import { computed, provide, ref, watch } from 'vue'
import { useRoute, type RouteLocationRaw } from 'vue-router'
import Tooltip from 'primevue/tooltip'
import { useToast } from 'primevue/usetoast'
import SidebarTagTree from '@/components/tags/SidebarTagTree.vue'
import TagsDialog from '@/components/tags/TagsDialog.vue'
import { SIDEBAR_TAGS } from '@/components/tags/sidebarTags'
import { useSetNoteTag, useTags } from '@/composables/useApi'
import { useNoteDrag } from '@/composables/useNoteDrag'
import { useTagExpansion } from '@/composables/useTagExpansion'
import type { Tag } from '@/lib/api/types'
import { apiErrorMessage } from '@/lib/apiError'
import { carriesNote, droppedNote } from '@/lib/noteDrag'
import { ancestorIds, tagIdFromQuery, tagPath } from '@/lib/tagTree'

// The desktop left rail: the tenant's tag tree. A tag narrows the notes list to
// the notes under it (`?tag=`, the search kept); clicking the active tag again
// releases the filter, and so does the clear button the heading shows while a
// tag is active (after codex's media rail). A note dragged from the list
// and dropped on a tag gets that tag. The tree is edited in a dialog
// (TagsDialog), opened from the pencil by the heading: nothing about tags lives
// in the settings.
//
// Collapsed, the rail keeps the tags at the root as icons alone, with the name
// in a tooltip and an aria-label; they take a dropped note as well.
//
// Every link here goes to the notes list, which RouterLink would mark active,
// and aria-current, all at once: it ignores the query. They are rendered
// through its `custom` slot, so only the selection is the current page.
const props = defineProps<{ collapsed?: boolean }>()

const vTooltip = Tooltip
const route = useRoute()
const toast = useToast()
const { data: tags, error } = useTags()
const setTag = useSetNoteTag()
const { expanded, toggle, open } = useTagExpansion()
const { dragging } = useNoteDrag()

const editorOpen = ref(false)

const tree = computed(() => tags.value ?? [])
const query = computed(() => (typeof route.query.q === 'string' ? { q: route.query.q } : {}))
const activeId = computed(() => (route.name === 'notes' ? tagIdFromQuery(route.query.tag) : null))

// The tags above the active one open when it becomes active (a reload, a
// shared link), so it shows wherever it is; their carets still close them.
watch(
    [activeId, tree],
    ([id, all]) => {
        if (id !== null) open(ancestorIds(all, id))
    },
    { immediate: true }
)

// A tag links to its notes; the active one links back to the unfiltered list,
// so a second click releases the filter.
function linkTo(tag: Tag): RouteLocationRaw {
    if (activeId.value === tag.id) return clearLink.value
    return { name: 'notes', query: { ...query.value, tag: String(tag.id) } }
}
const clearLink = computed<RouteLocationRaw>(() => ({ name: 'notes', query: query.value }))

// While a note is dragged every tag is open, so it can reach any of them.
function isOpen(tag: Tag): boolean {
    return dragging.value || expanded.value.has(tag.id)
}

// --- dropping a note on a tag -------------------------------------------------
const dropId = ref<number | null>(null)
watch(dragging, (on) => {
    if (!on) dropId.value = null
})

function onDragOver(event: DragEvent, tag: Tag) {
    if (!carriesNote(event)) return // a file or a link from elsewhere: not ours
    event.preventDefault() // take the drop
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
    dropId.value = tag.id
}

function onDragLeave(event: DragEvent, tag: Tag) {
    const row = event.currentTarget as HTMLElement | null
    if (row?.contains(event.relatedTarget as Node | null)) return // into a child of the row
    if (dropId.value === tag.id) dropId.value = null
}

// Each drop awaits its own call, so two notes dropped quickly both get their
// toast (a mutation's per-call callbacks would fire for the last one only).
async function onDrop(event: DragEvent, tag: Tag) {
    dropId.value = null
    const note = droppedNote(event)
    if (!note) return
    event.preventDefault()
    const path = tagPath(tree.value, tag.id) || tag.name
    if (note.tagIds.includes(tag.id)) {
        toast.add({
            severity: 'info',
            summary: `“${note.title}” is tagged ${path} already`,
            life: 3000
        })
        return
    }
    try {
        await setTag.mutateAsync({ noteId: note.id, tagId: tag.id, add: true })
        toast.add({
            severity: 'success',
            summary: `Tagged “${note.title}” with ${path}`,
            life: 3000
        })
    } catch (err) {
        toast.add({ severity: 'error', summary: apiErrorMessage(err), life: 5000 })
    }
}

provide(SIDEBAR_TAGS, {
    activeId,
    dropId,
    isOpen,
    toggle: (tag) => toggle(tag.id),
    linkTo,
    onDragOver,
    onDragLeave,
    onDrop
})

function railTip(name: string) {
    return { value: name, disabled: !props.collapsed, showDelay: 80 }
}
</script>

<template>
    <nav class="sidebar-nav" :class="{ collapsed }" aria-label="Tags">
        <!-- The full tree, while the rail is open. -->
        <div class="nav-full">
            <div class="sidebar-heading-row">
                <div class="heading-left">
                    <h2 class="sidebar-heading">Tags</h2>
                    <button
                        type="button"
                        class="heading-btn"
                        aria-label="Edit tags"
                        title="Edit tags"
                        data-test="edit-tags"
                        @click="editorOpen = true"
                    >
                        <i class="pi pi-pencil" aria-hidden="true" />
                    </button>
                </div>
                <RouterLink
                    v-if="activeId !== null"
                    v-slot="{ href, navigate }"
                    :to="clearLink"
                    custom
                >
                    <a
                        :href="href"
                        class="heading-btn"
                        aria-label="Clear tag filter"
                        title="Clear tag filter"
                        data-test="clear-tag-filter"
                        @click="navigate"
                    >
                        <i class="pi pi-filter-slash" aria-hidden="true" />
                    </a>
                </RouterLink>
            </div>
            <p v-if="error" class="sidebar-hint" data-test="tags-error">
                {{ apiErrorMessage(error) }}
            </p>
            <p v-else-if="tags && !tags.length" class="sidebar-hint" data-test="tags-empty">
                No tags yet.
                <button type="button" class="link-btn" @click="editorOpen = true">Add one</button>
            </p>
            <SidebarTagTree v-else-if="tags" :tags="tree" :depth="0" />
        </div>

        <!-- Collapsed: the tags at the root, as icons. -->
        <ul class="nav-rail">
            <li>
                <button
                    v-tooltip.right="railTip('Edit tags')"
                    type="button"
                    class="rail-item"
                    aria-label="Edit tags"
                    data-test="rail-edit-tags"
                    @click="editorOpen = true"
                >
                    <i class="pi pi-pencil" aria-hidden="true" />
                </button>
            </li>
            <li v-if="activeId !== null">
                <RouterLink v-slot="{ href, navigate }" :to="clearLink" custom>
                    <a
                        v-tooltip.right="railTip('Clear tag filter')"
                        :href="href"
                        class="rail-item"
                        aria-label="Clear tag filter"
                        data-test="rail-clear-tag-filter"
                        @click="navigate"
                    >
                        <i class="pi pi-filter-slash" aria-hidden="true" />
                    </a>
                </RouterLink>
            </li>
            <li v-for="tag in tree" :key="tag.id">
                <RouterLink v-slot="{ href, navigate }" :to="linkTo(tag)" custom>
                    <a
                        v-tooltip.right="railTip(tag.name)"
                        :href="href"
                        class="rail-item"
                        :class="{ active: activeId === tag.id, 'drop-target': dropId === tag.id }"
                        :aria-current="activeId === tag.id ? 'page' : undefined"
                        :aria-label="tag.name"
                        data-test="rail-tag"
                        @click="navigate"
                        @dragover="onDragOver($event, tag)"
                        @dragleave="onDragLeave($event, tag)"
                        @drop="onDrop($event, tag)"
                    >
                        <i class="pi pi-tag" aria-hidden="true" />
                    </a>
                </RouterLink>
            </li>
        </ul>
        <TagsDialog v-model:visible="editorOpen" />
    </nav>
</template>

<style scoped>
.sidebar-nav {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
    padding: 1rem 0.5rem;
    overflow-x: hidden;
    overflow-y: auto;
}

/* One of the two shows: the tree while the rail is open, the icons while it is
   collapsed (App.vue forces the icons on a phone). */
.nav-full {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    min-width: 0;
}
.nav-rail {
    display: none;
    flex-direction: column;
    align-items: center;
    gap: 0.15rem;
    margin: 0;
    padding: 0;
    list-style: none;
}
.sidebar-nav.collapsed .nav-full {
    display: none;
}
.sidebar-nav.collapsed .nav-rail {
    display: flex;
}

.sidebar-heading-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin: 0 0 0.35rem;
    padding: 0 0.25rem 0 0.75rem;
}
.heading-left {
    display: flex;
    align-items: center;
    gap: 0.25rem;
}

.sidebar-heading {
    margin: 0;
    font-size: 0.7rem;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--app-text-muted);
}

.heading-btn,
.rail-item {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: none;
    border-radius: var(--app-radius);
    background: transparent;
    color: var(--app-text-muted);
    cursor: pointer;
    text-decoration: none;
    transition:
        background 0.15s,
        color 0.15s,
        box-shadow 0.15s,
        opacity 0.15s;
}
.heading-btn {
    width: 1.75rem;
    height: 1.75rem;
    font-size: 0.8rem;
}
.rail-item {
    width: 2.25rem;
    height: 2.25rem;
    font-size: 0.85rem;
}
.heading-btn:hover,
.rail-item:hover {
    background: var(--app-hover);
    color: var(--app-accent);
}
.rail-item.active {
    background: var(--app-accent-soft);
    color: var(--app-text-primary);
}
.rail-item.drop-target {
    background: var(--app-accent-soft);
    box-shadow: inset 0 0 0 2px var(--app-accent);
}
.heading-btn:focus-visible,
.rail-item:focus-visible {
    outline: none;
    box-shadow: 0 0 0 2px var(--app-focus-ring);
}

/* As in codex's media rail, the heading's buttons (the pencil, and the clear
   while a tag is active) stay hidden until the tree is hovered or the button is
   keyboard-focused, so the heading reads clean at rest. Touch devices have no
   hover and keep them shown; opacity keeps their slots, so nothing shifts. */
@media (hover: hover) {
    .heading-btn {
        opacity: 0;
    }
    .nav-full:hover .heading-btn,
    .heading-btn:focus-visible {
        opacity: 1;
    }
}

.sidebar-hint {
    margin: 0.5rem 0 0;
    padding: 0 0.75rem;
    font-size: 0.8rem;
    color: var(--app-text-muted);
}

.link-btn {
    padding: 0;
    border: none;
    background: none;
    color: var(--app-accent);
    font: inherit;
    cursor: pointer;
    text-decoration: underline;
}
</style>
