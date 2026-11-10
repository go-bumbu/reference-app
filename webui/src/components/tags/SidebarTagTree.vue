<script setup lang="ts">
import type { Tag } from '@/lib/api/types'
import { useSidebarTags } from './sidebarTags'

// One level of the sidebar's tag tree: each tag a link that narrows the notes
// list to it, with a caret that opens the tags below it, set in under it. A
// note dragged from the list can be dropped on any tag to give it that tag;
// AppNav, which provides the context, does the rest.
//
// A nested list of links, not an ARIA tree: a tree promises arrow-key
// navigation, and these are links like any other, reached with Tab. The caret
// says whether its tag is open (aria-expanded), and only the active tag is the
// current page (RouterLink would mark every link of the notes list, as it
// ignores the query).
defineOptions({ name: 'SidebarTagTree' })
defineProps<{ tags: Tag[]; depth: number }>()

const ctx = useSidebarTags()
</script>

<template>
    <ul class="tag-level">
        <li v-for="tag in tags" :key="tag.id">
            <div
                class="sidebar-tag"
                :class="{
                    active: ctx.activeId.value === tag.id,
                    'drop-target': ctx.dropId.value === tag.id
                }"
                :style="{ paddingLeft: `${0.25 + depth * 0.9}rem` }"
                data-test="sidebar-tag"
                :data-tag-id="tag.id"
                @dragover="ctx.onDragOver($event, tag)"
                @dragleave="ctx.onDragLeave($event, tag)"
                @drop="ctx.onDrop($event, tag)"
            >
                <button
                    v-if="tag.children.length"
                    type="button"
                    class="caret"
                    :aria-label="`Tags under ${tag.name}`"
                    :aria-expanded="ctx.isOpen(tag)"
                    data-test="tag-caret"
                    @click="ctx.toggle(tag)"
                >
                    <i
                        :class="ctx.isOpen(tag) ? 'pi pi-chevron-down' : 'pi pi-chevron-right'"
                        aria-hidden="true"
                    />
                </button>
                <span v-else class="caret-spacer" aria-hidden="true" />
                <RouterLink v-slot="{ href, navigate }" :to="ctx.linkTo(tag)" custom>
                    <a
                        :href="href"
                        class="tag-link"
                        :title="tag.name"
                        :aria-current="ctx.activeId.value === tag.id ? 'page' : undefined"
                        @click="navigate"
                    >
                        <i class="pi pi-tag" aria-hidden="true" /><span>{{ tag.name }}</span>
                    </a>
                </RouterLink>
            </div>
            <SidebarTagTree
                v-if="tag.children.length && ctx.isOpen(tag)"
                :tags="tag.children"
                :depth="depth + 1"
            />
        </li>
    </ul>
</template>

<style scoped>
.tag-level {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    margin: 0;
    padding: 0;
    list-style: none;
}

.sidebar-tag {
    display: flex;
    align-items: center;
    gap: 0.15rem;
    min-width: 0;
    border-radius: var(--app-radius);
    color: var(--app-text-muted);
    font-size: 0.85rem;
    font-weight: 600;
    transition:
        background 0.15s,
        color 0.15s,
        box-shadow 0.15s;
}
.sidebar-tag:hover {
    background: var(--app-hover);
}
.sidebar-tag.active {
    background: var(--app-accent-soft);
    color: var(--app-text-primary);
}
/* A dragged note is over the tag: dropping it here tags the note. */
.sidebar-tag.drop-target {
    background: var(--app-accent-soft);
    box-shadow: inset 0 0 0 2px var(--app-accent);
    color: var(--app-text-primary);
}

.caret,
.caret-spacer {
    flex-shrink: 0;
    width: 1.25rem;
    height: 1.25rem;
}
.caret {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    border: none;
    border-radius: var(--app-radius);
    background: transparent;
    color: inherit;
    font-size: 0.65rem;
    cursor: pointer;
}
.caret:hover {
    color: var(--app-accent);
}
.caret:focus-visible {
    outline: none;
    box-shadow: 0 0 0 2px var(--app-focus-ring);
}

.tag-link {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-width: 0;
    padding: 0.4rem 0.5rem 0.4rem 0.2rem;
    border-radius: var(--app-radius);
    color: inherit;
    text-decoration: none;
}
.tag-link i {
    flex-shrink: 0;
    font-size: 0.8rem;
}
.tag-link span {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.tag-link:focus-visible {
    outline: none;
    box-shadow: 0 0 0 2px var(--app-focus-ring);
}
</style>
