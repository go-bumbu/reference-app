<script setup lang="ts">
import { computed, ref } from 'vue'
import Menu from 'primevue/menu'
import type { MenuItem } from 'primevue/menuitem'

// The top bar's profile button: the signed-in user's initial, opening a menu
// with who is signed in and the way to the settings.
const props = defineProps<{
    subject: string
    tenant: string
    active?: boolean
}>()

const menu = ref<InstanceType<typeof Menu>>()
const initial = computed(() => props.subject.charAt(0).toUpperCase() || '?')

const items: MenuItem[] = [
    { label: 'Settings', icon: 'pi pi-cog', route: { name: 'settings' }, test: 'open-settings' }
]

function toggle(e: Event) {
    menu.value?.toggle(e)
}
</script>

<template>
    <button
        type="button"
        class="profile-btn"
        :class="{ active }"
        aria-haspopup="true"
        aria-controls="profile-menu"
        :aria-label="`Profile: ${subject}`"
        :title="subject"
        data-test="profile"
        @click="toggle"
    >
        {{ initial }}
    </button>
    <Menu id="profile-menu" ref="menu" :model="items" popup>
        <template #start>
            <div class="profile-identity" data-test="identity">
                <div class="profile-subject">{{ subject }}</div>
                <div class="muted">{{ tenant }}</div>
            </div>
        </template>
        <template #item="{ item, props: itemProps }">
            <RouterLink v-slot="{ href, navigate }" :to="item.route" custom>
                <a :href="href" v-bind="itemProps.action" :data-test="item.test" @click="navigate">
                    <span :class="item.icon" aria-hidden="true" />
                    <span>{{ item.label }}</span>
                </a>
            </RouterLink>
        </template>
    </Menu>
</template>

<style scoped>
/* A round avatar, the height of the Add note button beside it. */
.profile-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 2rem;
    height: 2rem;
    border: 1px solid var(--app-border);
    border-radius: 999px;
    background: var(--app-hover);
    color: var(--app-text-primary);
    font-size: 0.85rem;
    font-weight: 600;
    cursor: pointer;
    transition:
        border-color 0.15s,
        color 0.15s;
}
.profile-btn:hover,
.profile-btn.active {
    border-color: var(--app-accent);
    color: var(--app-accent);
}
.profile-btn:focus-visible {
    outline: none;
    box-shadow: 0 0 0 3px var(--app-focus-ring);
}

.profile-identity {
    padding: 0.5rem 0.75rem 0.625rem;
    margin-bottom: 0.25rem;
    border-bottom: 1px solid var(--app-border);
    font-size: 0.85rem;
}
.profile-subject {
    font-weight: 600;
    color: var(--app-text-primary);
}
</style>
