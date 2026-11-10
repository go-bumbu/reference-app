<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import ConfirmDialog from 'primevue/confirmdialog'
import Message from 'primevue/message'
import Toast from 'primevue/toast'
import AppNav from '@/components/AppNav.vue'
import NoteDialog from '@/components/NoteDialog.vue'
import ProfileMenu from '@/components/ProfileMenu.vue'
import SearchField from '@/components/SearchField.vue'
import { useInfo, useMe } from '@/composables/useApi'
import { useSidebar } from '@/composables/useSidebar'
import { apiErrorMessage, apiErrorStatus } from '@/lib/apiError'

const route = useRoute()
const router = useRouter()
const { collapsed: sidebarCollapsed, toggle: toggleSidebar } = useSidebar()
const { data: me, error: meError } = useMe()
const { data: info } = useInfo()

// A 401 on /me means nothing authenticated this browser: in "proxy" mode the
// page was opened without the proxy in front of the server.
const unauthenticated = computed(() => apiErrorStatus(meError.value) === 401)
const version = computed(() => info.value?.version ?? '')

const isSettings = computed(() => route.path === '/settings' || route.path.startsWith('/settings/'))

// Back, at the top bar's left, on every page but the notes list. It goes back
// through the app's own history; a page opened directly (a deep link, a reload
// with no earlier entry) has none, so it goes to the list instead of leaving.
const showBack = computed(() => route.name !== 'notes')
function goBack() {
    if (router.options.history.state.back) router.back()
    else router.push({ name: 'notes' })
}

// The top bar's Add note opens the create dialog over whatever page is shown.
const addOpen = ref(false)

// The search query lives in the URL (`?q=`), so it survives a reload and a
// shared link; the notes list reads it from there. Typing on another page
// takes you to the list. replace, not push: every keystroke is no history entry.
// On the list, the search narrows the tag's notes (`?tag=`), which stays.
const search = computed({
    get: () => (typeof route.query.q === 'string' ? route.query.q : ''),
    set: (q: string) => {
        const tag =
            route.name === 'notes' && typeof route.query.tag === 'string'
                ? { tag: route.query.tag }
                : {}
        const target = { name: 'notes', query: { ...tag, ...(q ? { q } : {}) } }
        if (route.name === 'notes') router.replace(target)
        else if (q) router.push(target)
    }
})
</script>

<template>
    <div class="app-container">
        <header class="topbar">
            <div class="topbar-brand" :class="{ collapsed: sidebarCollapsed && !isSettings }">
                <RouterLink
                    to="/"
                    class="topbar-brand-link"
                    aria-label="Bumbu reference app — home"
                >
                    <span class="topbar-brand-icon">◆</span>
                    <span class="topbar-title">Bumbu reference app</span>
                </RouterLink>
                <button
                    v-if="!isSettings"
                    class="sidebar-collapse-btn"
                    type="button"
                    data-test="sidebar-toggle"
                    :aria-label="sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'"
                    :aria-expanded="!sidebarCollapsed"
                    @click="toggleSidebar"
                >
                    <i :class="sidebarCollapsed ? 'pi pi-angle-right' : 'pi pi-angle-left'" />
                </button>
            </div>
            <div class="topbar-main">
                <button
                    v-if="showBack"
                    class="topbar-back-btn"
                    type="button"
                    aria-label="Back"
                    title="Back"
                    data-test="back"
                    @click="goBack"
                >
                    <i class="pi pi-arrow-left" aria-hidden="true" />
                </button>
                <!-- The search filters the notes; the settings have nothing to search. -->
                <div v-if="!isSettings" class="topbar-search">
                    <SearchField
                        v-model="search"
                        placeholder="Search notes..."
                        aria-label="Search notes"
                    />
                </div>
                <div class="topbar-actions">
                    <Button
                        v-if="me"
                        class="topbar-add-btn"
                        label="Add note"
                        icon="pi pi-plus"
                        size="small"
                        data-test="add-note"
                        @click="addOpen = true"
                    />
                    <ProfileMenu
                        v-if="me"
                        :subject="me.subject"
                        :tenant="me.tenant"
                        :active="isSettings"
                    />
                </div>
            </div>
        </header>

        <div class="body-row">
            <!-- The settings have a sidebar of their own (SettingsShell), as codex's do. -->
            <aside v-if="!isSettings" class="sidebar" :class="{ collapsed: sidebarCollapsed }">
                <AppNav :collapsed="sidebarCollapsed" />
                <footer v-if="version && !sidebarCollapsed" class="sidebar-footer muted">
                    server {{ version }}
                </footer>
            </aside>

            <main class="main-content">
                <div class="content-inner">
                    <Message v-if="unauthenticated" severity="warn" data-test="unauthenticated">
                        You are not authenticated. The server expects an authenticating proxy in
                        front of it.
                    </Message>
                    <Message v-else-if="meError" severity="error">{{
                        apiErrorMessage(meError)
                    }}</Message>
                    <RouterView v-else />
                </div>
            </main>
        </div>
        <NoteDialog v-model:visible="addOpen" />
        <ConfirmDialog />
        <Toast />
    </div>
</template>

<style scoped>
/* App scaffolding, after codex: a full-viewport column (top bar, then the body
   row). The body row holds the sidebar and the content; main-content is the
   one scroll container for views. */
.app-container {
    display: flex;
    flex-direction: column;
    height: 100vh;
    height: 100dvh;
    width: 100%;
    overflow: hidden;
    background-color: var(--app-background);
}

.body-row {
    display: flex;
    flex: 1;
    min-height: 0;
    overflow: hidden;
}

/* --- Top bar --- */
.topbar {
    display: flex;
    align-items: center;
    flex-shrink: 0;
    height: var(--app-topbar-height);
    background: var(--app-panel);
    border-bottom: 1px solid var(--app-border);
}

/* As wide as the sidebar below it, collapsed or not. */
.topbar-brand {
    display: flex;
    align-items: center;
    flex-shrink: 0;
    gap: 0.625rem;
    width: var(--app-sidebar-width);
    height: 100%;
    padding: 0 1rem 0 1.25rem;
    transition: width 0.3s ease;
}
.topbar-brand.collapsed {
    width: var(--app-sidebar-collapsed-width);
    justify-content: center;
    padding: 0;
}

.topbar-brand-link {
    display: inline-flex;
    align-items: center;
    gap: 0.625rem;
    min-width: 0;
    text-decoration: none;
    border-radius: var(--app-radius);
}
.topbar-brand.collapsed .topbar-brand-link {
    display: none;
}
.topbar-brand-link:hover {
    opacity: 0.8;
}
.topbar-brand-link:focus-visible {
    outline: 2px solid var(--app-focus-ring);
    outline-offset: 2px;
}

.topbar-brand-icon {
    color: var(--app-accent);
    font-size: 1.1rem;
    line-height: 1;
}

.topbar-title {
    font-size: 1rem;
    font-weight: 600;
    color: var(--app-text-primary);
    letter-spacing: -0.01em;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
}

.sidebar-collapse-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 2rem;
    height: 2rem;
    margin-left: auto;
    border: none;
    border-radius: var(--app-radius);
    background: transparent;
    color: var(--app-text-muted);
    cursor: pointer;
    transition:
        background 0.15s,
        color 0.15s;
}
.sidebar-collapse-btn:hover {
    background: var(--app-hover);
    color: var(--app-accent);
}
.topbar-brand.collapsed .sidebar-collapse-btn {
    margin-left: 0;
}

.topbar-main {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 1rem;
    min-width: 0;
    padding: 0 0.75rem 0 1.5rem;
}

.topbar-back-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 2rem;
    height: 2rem;
    border: 1px solid var(--app-border);
    border-radius: var(--app-radius);
    background: transparent;
    color: var(--app-text-muted);
    cursor: pointer;
    transition:
        border-color 0.15s,
        color 0.15s;
}
.topbar-back-btn:hover {
    border-color: var(--app-accent);
    color: var(--app-accent);
}
.topbar-back-btn:focus-visible {
    outline: none;
    box-shadow: 0 0 0 3px var(--app-focus-ring);
}

.topbar-search {
    flex: 1;
    max-width: 480px;
    margin: 0 auto;
}

.topbar-actions {
    display: flex;
    margin-left: auto;
    align-items: center;
    gap: 0.5rem;
    flex-shrink: 0;
}

/* --- Sidebar --- */
.sidebar {
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
    width: var(--app-sidebar-width);
    background: var(--app-panel);
    border-right: 1px solid var(--app-border);
    overflow: hidden;
    transition: width 0.3s ease;
}
.sidebar.collapsed {
    width: var(--app-sidebar-collapsed-width);
}

.sidebar-footer {
    padding: 0.75rem 1.25rem;
    font-size: 0.75rem;
    white-space: nowrap;
}

/* --- Content --- */
.main-content {
    flex: 1;
    min-width: 0;
    padding: 1.5rem 2rem;
    overflow-x: hidden;
    overflow-y: auto;
}

.content-inner {
    width: 100%;
    max-width: var(--app-content-max-width);
    margin: 0 auto;
}

/* A phone has no room for the expanded rail: it stays an icon rail, the brand
   is the icon alone. */
@media (max-width: 640px) {
    .sidebar,
    .sidebar.collapsed,
    .topbar-brand,
    .topbar-brand.collapsed {
        width: var(--app-sidebar-collapsed-width);
    }
    .topbar-brand {
        justify-content: center;
        padding: 0;
    }
    .topbar-brand.collapsed .topbar-brand-link {
        display: inline-flex;
    }
    /* the tags as icons, as in the collapsed rail */
    .sidebar :deep(.nav-full) {
        display: none;
    }
    .sidebar :deep(.nav-rail) {
        display: flex;
    }
    .topbar-title,
    .sidebar-collapse-btn,
    .sidebar-footer {
        display: none;
    }
    .main-content {
        padding: 1rem;
    }
    /* Add note is its icon alone. */
    .topbar-add-btn :deep(.p-button-label) {
        display: none;
    }
    .topbar-main {
        padding: 0 0.5rem;
    }
}
</style>
