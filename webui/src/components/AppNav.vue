<script setup lang="ts">
import Tooltip from 'primevue/tooltip'

// The desktop left rail: the tags. A collapsed rail keeps them as icons alone,
// with the name in a tooltip and an aria-label.
//
// PLACEHOLDER: notes have no tags yet (no API, no module support), so the rail
// shows a fixed, inert list to give the shell its final shape. Replace it with
// a TanStack query once the API serves tags, and make each one filter the list.
const PLACEHOLDER_TAGS = ['work', 'personal', 'ideas', 'reading', 'archive']

const props = defineProps<{ collapsed?: boolean }>()

const vTooltip = Tooltip

function tagName(name: string) {
    return props.collapsed ? name : undefined
}

function tagTip(name: string) {
    return { value: name, disabled: !props.collapsed, showDelay: 80 }
}
</script>

<template>
    <nav class="sidebar-nav" :class="{ collapsed }" aria-label="Tags">
        <h2 v-if="!collapsed" class="sidebar-heading">Tags</h2>
        <ul class="tag-list">
            <li
                v-for="tag in PLACEHOLDER_TAGS"
                :key="tag"
                v-tooltip.right="tagTip(tag)"
                :aria-label="tagName(tag)"
                class="sidebar-tag"
                data-test="sidebar-tag"
            >
                <i class="pi pi-tag" aria-hidden="true" /><span>{{ tag }}</span>
            </li>
        </ul>
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

.sidebar-heading {
    margin: 0 0 0.5rem;
    padding: 0 0.75rem;
    font-size: 0.7rem;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--app-text-muted);
}

.tag-list {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    margin: 0;
    padding: 0;
    list-style: none;
}

.sidebar-tag {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.5rem 0.75rem;
    border-radius: var(--app-radius);
    color: var(--app-text-muted);
    font-size: 0.85rem;
    font-weight: 600;
}
.sidebar-tag i {
    flex-shrink: 0;
    width: 1rem;
    font-size: 0.85rem;
    text-align: center;
}
.sidebar-tag span {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

/* Collapsed, the tags are their icons alone, centred. The label stays at no
   width, so a row keeps its height. */
.sidebar-nav.collapsed .sidebar-tag {
    justify-content: center;
    gap: 0;
    padding: 0.5rem 0;
}
.sidebar-nav.collapsed .sidebar-tag span {
    width: 0;
}
</style>
