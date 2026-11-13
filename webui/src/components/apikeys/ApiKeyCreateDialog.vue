<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import SelectButton from 'primevue/selectbutton'
import DatePicker from 'primevue/datepicker'
import Message from 'primevue/message'
import type { ApiKeyCreated, ApiKeyScope } from '@/lib/api/types'
import { useCreateApiKey } from '@/composables/useApi'
import { apiErrorMessage, apiErrorStatus, fieldErrors } from '@/lib/apiError'
import { areaLabel, defaultExpiry, scopesFrom, type AreaChoice } from '@/lib/apiKeys'

// Creates an API key in two steps: the form (a name, a level per area, an
// expiry), then the token, shown once with a copy button. Closing the dialog
// forgets the token.
const props = defineProps<{
    scopes: ApiKeyScope[]
}>()

const visible = defineModel<boolean>('visible', { required: true })

const create = useCreateApiKey()

const name = ref('')
const choices = ref<Record<string, AreaChoice>>({})
const expiresAt = ref<Date | null>(null)
const errors = ref<Record<string, string>>({})
const formError = ref('')
const created = ref<ApiKeyCreated | null>(null)
const copied = ref(false)

const minDate = computed(() => {
    const d = new Date()
    d.setDate(d.getDate() + 1)
    return d
})

function reset() {
    name.value = ''
    choices.value = Object.fromEntries(props.scopes.map((s) => [s.area, 'none' as AreaChoice]))
    expiresAt.value = defaultExpiry()
    errors.value = {}
    formError.value = ''
    created.value = null
    copied.value = false
}

watch(visible, (v) => {
    if (v) reset()
})

function levelOptions(s: ApiKeyScope) {
    return [
        { label: 'None', value: 'none' },
        ...s.levels.map((l) => ({ label: l === 'write' ? 'Read & write' : 'Read', value: l }))
    ]
}

async function submit() {
    errors.value = {}
    formError.value = ''
    const scopes = scopesFrom(choices.value)
    if (!name.value.trim()) errors.value.name = 'A name is required'
    if (!scopes.length) errors.value.scopes = 'Grant at least one area'
    if (!expiresAt.value) errors.value.expiresAt = 'Pick when the key expires'
    if (Object.keys(errors.value).length) return
    try {
        created.value = await create.mutateAsync({
            name: name.value.trim(),
            scopes,
            expiresAt: expiresAt.value?.toISOString()
        })
    } catch (err) {
        const fields = fieldErrors(err)
        if (apiErrorStatus(err) === 422 && Object.keys(fields).length) {
            errors.value = fields
            return
        }
        formError.value = apiErrorMessage(err)
    }
}

async function copy() {
    if (!created.value) return
    try {
        await navigator.clipboard.writeText(created.value.token)
        copied.value = true
    } catch {
        formError.value = 'Copying failed: select the key and copy it yourself.'
    }
}
</script>

<template>
    <Dialog
        v-model:visible="visible"
        :header="created ? 'API key created' : 'New API key'"
        modal
        :style="{ width: '34rem' }"
        :breakpoints="{ '640px': '100vw' }"
    >
        <div v-if="created" class="created" data-test="apikey-created">
            <Message severity="warn" :closable="false">
                Copy the key now: it is shown only this once.
            </Message>
            <div class="token-row">
                <InputText
                    :model-value="created.token"
                    readonly
                    class="token"
                    aria-label="API key"
                    data-test="apikey-token"
                    @focus="($event.target as HTMLInputElement).select()"
                />
                <Button
                    :icon="copied ? 'pi pi-check' : 'pi pi-copy'"
                    :label="copied ? 'Copied' : 'Copy'"
                    data-test="apikey-copy"
                    @click="copy"
                />
            </div>
            <p class="hint">
                Send it as <code>Authorization: Bearer &lt;key&gt;</code> to
                <code>/api/key/v0</code>.
            </p>
            <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
            <div class="actions">
                <Button label="Done" data-test="apikey-done" @click="visible = false" />
            </div>
        </div>

        <form v-else class="form" @submit.prevent="submit">
            <div class="field">
                <label for="apikey-name">Name</label>
                <InputText
                    id="apikey-name"
                    v-model="name"
                    maxlength="100"
                    placeholder="e.g. backup script"
                    :invalid="!!errors.name"
                    data-test="apikey-name"
                />
                <small v-if="errors.name" class="error">{{ errors.name }}</small>
            </div>

            <fieldset class="field">
                <legend>Access</legend>
                <div v-for="s in scopes" :key="s.area" class="area-row">
                    <span class="area-name">{{ areaLabel(s.area) }}</span>
                    <SelectButton
                        v-model="choices[s.area]"
                        :options="levelOptions(s)"
                        option-label="label"
                        option-value="value"
                        :allow-empty="false"
                        :aria-label="`Access to ${areaLabel(s.area)}`"
                        :data-test="`apikey-area-${s.area}`"
                    />
                </div>
                <small v-if="errors.scopes" class="error">{{ errors.scopes }}</small>
            </fieldset>

            <div class="field">
                <label for="apikey-expires">Expires</label>
                <DatePicker
                    v-model="expiresAt"
                    input-id="apikey-expires"
                    :min-date="minDate"
                    show-icon
                    :invalid="!!errors.expiresAt"
                    data-test="apikey-expires"
                />
                <small v-if="errors.expiresAt" class="error">{{ errors.expiresAt }}</small>
            </div>

            <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
            <div class="actions">
                <Button label="Cancel" severity="secondary" text @click="visible = false" />
                <Button
                    type="submit"
                    label="Create key"
                    :loading="create.isPending.value"
                    data-test="apikey-submit"
                />
            </div>
        </form>
    </Dialog>
</template>

<style scoped>
.form,
.created {
    display: flex;
    flex-direction: column;
    gap: 1rem;
}
.field {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    border: none;
    margin: 0;
    padding: 0;
}
.field label,
.field legend {
    font-size: 0.85rem;
    font-weight: 600;
    padding: 0;
    margin-bottom: 0.4rem;
}
.area-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    flex-wrap: wrap;
}
.area-name {
    font-size: 0.9rem;
}
.error {
    color: var(--app-danger);
}
.token-row {
    display: flex;
    gap: 0.5rem;
}
.token {
    flex: 1;
    min-width: 0;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.82rem;
}
.hint {
    margin: 0;
    font-size: 0.85rem;
    color: var(--app-text-muted);
}
.actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
}
</style>
