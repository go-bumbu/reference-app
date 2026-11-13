<script setup lang="ts">
import { computed, ref } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import ApiKeyCreateDialog from '@/components/apikeys/ApiKeyCreateDialog.vue'
import {
    useApiKeyScopes,
    useApiKeys,
    useApiKeysEnabled,
    useRevokeApiKey
} from '@/composables/useApi'
import { apiErrorMessage } from '@/lib/apiError'
import type { ApiKey } from '@/lib/api/types'
import { keyState, scopeLabel } from '@/lib/apiKeys'
import { formatDate } from '@/lib/format'

// The signed-in user's API keys, after codex's: list them, create one (its
// token shown once), revoke one. A key lets a script use the API without the
// proxy's login, limited to the areas it grants. When the server offers no
// keys (its auth method is not "proxy") the page says so instead.
const toast = useToast()
const confirm = useConfirm()

const enabled = useApiKeysEnabled()
const scopesQuery = useApiKeyScopes()
const keysQuery = useApiKeys(() => enabled.value === true)
const revoke = useRevokeApiKey()

const keys = computed(() => keysQuery.data.value ?? [])
const scopes = computed(() => scopesQuery.data.value ?? [])

const loadError = computed(() => {
    if (enabled.value === false) return ''
    if (scopesQuery.isError.value) return apiErrorMessage(scopesQuery.error.value)
    if (keysQuery.isError.value) return apiErrorMessage(keysQuery.error.value)
    return ''
})

const createVisible = ref(false)

const STATE_TAGS = {
    active: null,
    expired: { value: 'Expired', severity: 'warn' },
    unusable: { value: 'Unusable', severity: 'danger' }
} as const

function revokeKey(key: ApiKey) {
    confirm.require({
        header: 'Revoke API key',
        message: `Revoke “${key.name}”? Anything using it stops working at once; this cannot be undone.`,
        icon: 'pi pi-trash',
        rejectProps: { label: 'Cancel', severity: 'secondary', text: true },
        acceptProps: { label: 'Revoke', severity: 'danger' },
        accept: () =>
            revoke.mutate(key.id, {
                onSuccess: () =>
                    toast.add({ severity: 'success', summary: 'API key revoked', life: 3000 }),
                onError: (err) =>
                    toast.add({ severity: 'error', summary: apiErrorMessage(err), life: 5000 })
            })
    })
}
</script>

<template>
    <section aria-label="API keys">
        <h2 class="set-section-title">API keys</h2>
        <p class="set-note">
            A key lets a script or another program use the API as you, without signing in, but only
            for what it grants. Keys expire, and you can revoke one at any time.
        </p>

        <Message v-if="enabled === false" severity="info" :closable="false" data-test="apikeys-off">
            This server offers no API keys: they need the server to run behind an authenticating
            proxy.
        </Message>

        <template v-else>
            <Message v-if="loadError" severity="error" :closable="false" class="load-error">
                {{ loadError }}
            </Message>
            <div class="toolbar">
                <Button
                    icon="pi pi-plus"
                    label="New key"
                    size="small"
                    :disabled="!scopes.length"
                    data-test="apikey-new"
                    @click="createVisible = true"
                />
            </div>
            <div class="table-fit">
                <DataTable
                    :value="keys"
                    data-key="id"
                    :loading="keysQuery.isLoading.value"
                    size="small"
                    data-test="apikeys-table"
                >
                    <template #empty>
                        <div class="empty-table">No API keys yet</div>
                    </template>
                    <Column header="Name">
                        <template #body="{ data }">
                            <span class="key-name" :data-test="`apikey-${data.id}`">{{
                                data.name
                            }}</span>
                            <Tag
                                v-if="STATE_TAGS[keyState(data)]"
                                v-bind="STATE_TAGS[keyState(data)]!"
                                class="state"
                            />
                        </template>
                    </Column>
                    <Column header="Access">
                        <template #body="{ data }">
                            <span v-if="!data.scopes.length" class="muted">—</span>
                            <span v-for="s in data.scopes" :key="s" class="scope">{{
                                scopeLabel(s)
                            }}</span>
                        </template>
                    </Column>
                    <Column header="Last used" class="col-wide">
                        <template #body="{ data }">
                            <span class="muted">{{
                                data.lastUsedAt ? formatDate(data.lastUsedAt) : 'Never'
                            }}</span>
                        </template>
                    </Column>
                    <Column header="Expires" class="col-wide">
                        <template #body="{ data }">
                            <span class="muted">{{
                                data.expiresAt ? formatDate(data.expiresAt) : 'Never'
                            }}</span>
                        </template>
                    </Column>
                    <Column header="" class="col-actions">
                        <template #body="{ data }">
                            <Button
                                icon="pi pi-trash"
                                text
                                rounded
                                size="small"
                                severity="danger"
                                :aria-label="`Revoke ${data.name}`"
                                :data-test="`apikey-revoke-${data.id}`"
                                @click="revokeKey(data)"
                            />
                        </template>
                    </Column>
                </DataTable>
            </div>
        </template>

        <ApiKeyCreateDialog v-model:visible="createVisible" :scopes="scopes" />
    </section>
</template>

<style scoped>
.toolbar {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 0.75rem;
}
.load-error {
    margin-bottom: 1rem;
}
.key-name {
    font-size: 0.9rem;
    font-weight: 600;
}
.state {
    margin-left: 0.5rem;
}
.scope {
    display: block;
    font-size: 0.85rem;
}
.muted {
    font-size: 0.85rem;
    color: var(--app-text-muted);
}
.empty-table {
    padding: 2rem;
    text-align: center;
    color: var(--app-text-muted);
}
.table-fit {
    overflow-x: auto;
}
.table-fit :deep(.col-actions) {
    width: 3rem;
    text-align: right;
}
@media (max-width: 640px) {
    .table-fit :deep(.col-wide) {
        display: none;
    }
}
</style>
