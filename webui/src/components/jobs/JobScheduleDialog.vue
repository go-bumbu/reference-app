<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import type { Job, JobSchedule } from '@/lib/api/types'
import {
    useCreateJobSchedule,
    useDeleteJobSchedule,
    useUpdateJobSchedule
} from '@/composables/useApi'
import { apiErrorMessage, apiErrorStatus, fieldErrors } from '@/lib/apiError'
import { formatDate } from '@/lib/format'
import { SCHEDULE_PRESETS, scheduleLabel } from '@/lib/jobs'

// The schedules of one job, after aether's ScheduleDialog: a manager, not a
// one-shot form. It lists the tenant's schedules of the job (edit, remove) over
// a form that adds one, or edits the one picked; it stays open after a save.
// `job` is looked up from the jobs query by the parent, so a save's refetch
// shows here at once.
const props = defineProps<{
    job: Job | null
}>()

const visible = defineModel<boolean>('visible', { required: true })

const createSchedule = useCreateJobSchedule()
const updateSchedule = useUpdateJobSchedule()
const deleteSchedule = useDeleteJobSchedule()

const schedules = computed<JobSchedule[]>(() => props.job?.schedules ?? [])

// null: the form adds a schedule; otherwise the id of the one being edited.
const editingId = ref<string | null>(null)
const cron = ref('')
const enabled = ref(true)
const cronError = ref('')
const formError = ref('')
const saving = ref(false)
const removingId = ref<string | null>(null)

function resetForm() {
    editingId.value = null
    cron.value = ''
    enabled.value = true
    cronError.value = ''
    formError.value = ''
}

watch(visible, (v) => {
    if (v) resetForm()
})

function setPreset(expr: string) {
    cron.value = expr
    cronError.value = ''
}

function editRow(s: JobSchedule) {
    editingId.value = s.id
    cron.value = s.cron
    enabled.value = s.enabled
    cronError.value = ''
    formError.value = ''
}

// A 422 names the field (the cron expression); anything else goes over the form.
function showError(err: unknown) {
    const field = fieldErrors(err).cron
    if (apiErrorStatus(err) === 422 && field) {
        cronError.value = field
        return
    }
    formError.value = apiErrorMessage(err)
}

async function save() {
    const job = props.job
    const expr = cron.value.trim()
    if (!job) return
    if (!expr) {
        cronError.value = 'A cron expression is required'
        return
    }
    cronError.value = ''
    formError.value = ''
    saving.value = true
    try {
        if (editingId.value) {
            await updateSchedule.mutateAsync({
                job: job.name,
                id: editingId.value,
                patch: { cron: expr, enabled: enabled.value }
            })
        } else {
            await createSchedule.mutateAsync({
                job: job.name,
                input: { cron: expr, enabled: enabled.value }
            })
        }
        resetForm()
    } catch (err) {
        showError(err)
    } finally {
        saving.value = false
    }
}

async function remove(s: JobSchedule) {
    const job = props.job
    if (!job) return
    formError.value = ''
    removingId.value = s.id
    try {
        await deleteSchedule.mutateAsync({ job: job.name, id: s.id })
        if (editingId.value === s.id) resetForm()
    } catch (err) {
        formError.value = apiErrorMessage(err)
    } finally {
        removingId.value = null
    }
}

const busy = computed(() => saving.value || removingId.value !== null)
</script>

<template>
    <Dialog
        v-model:visible="visible"
        :header="job ? `Schedules: ${job.title}` : 'Schedules'"
        modal
        :style="{ width: '36rem' }"
        :breakpoints="{ '640px': '100vw' }"
    >
        <div class="schedule-manager">
            <div v-if="schedules.length" class="schedule-list">
                <div
                    v-for="s in schedules"
                    :key="s.id"
                    class="schedule-row"
                    :data-test="`schedule-${s.id}`"
                >
                    <div class="schedule-row-info">
                        <span class="schedule-row-label">{{ scheduleLabel(s) }}</span>
                        <code v-if="scheduleLabel(s) !== s.cron" class="muted">{{ s.cron }}</code>
                        <span v-if="!s.enabled" class="muted">(paused)</span>
                        <span v-else-if="s.nextRunAt" class="schedule-next muted">
                            next {{ formatDate(s.nextRunAt) }}
                        </span>
                    </div>
                    <div class="schedule-row-actions">
                        <Button
                            icon="pi pi-pencil"
                            text
                            rounded
                            size="small"
                            aria-label="Edit schedule"
                            :disabled="busy"
                            :data-test="`edit-schedule-${s.id}`"
                            @click="editRow(s)"
                        />
                        <Button
                            icon="pi pi-trash"
                            text
                            rounded
                            size="small"
                            severity="danger"
                            aria-label="Remove schedule"
                            :loading="removingId === s.id"
                            :disabled="busy"
                            :data-test="`remove-schedule-${s.id}`"
                            @click="remove(s)"
                        />
                    </div>
                </div>
            </div>
            <p v-else class="muted no-schedules">No schedules yet.</p>

            <div class="schedule-form">
                <h3 class="form-title">{{ editingId ? 'Edit schedule' : 'Add schedule' }}</h3>
                <div class="presets">
                    <span class="preset-label">Preset:</span>
                    <Button
                        v-for="p in SCHEDULE_PRESETS"
                        :key="p.cron"
                        :label="p.label"
                        size="small"
                        :severity="cron === p.cron ? undefined : 'secondary'"
                        :data-test="`preset-${p.label}`"
                        @click="setPreset(p.cron)"
                    />
                </div>
                <div class="field">
                    <label for="schedule-cron">Cron expression</label>
                    <InputText
                        id="schedule-cron"
                        v-model="cron"
                        placeholder="e.g. 0 3 * * * (daily at 03:00)"
                        :invalid="!!cronError"
                        aria-describedby="schedule-cron-help"
                        data-test="schedule-cron"
                        @input="cronError = ''"
                    />
                    <small v-if="cronError" class="error" data-test="schedule-cron-error">{{
                        cronError
                    }}</small>
                    <small v-else id="schedule-cron-help" class="muted">
                        5 fields (min hour day month weekday) or 6 with seconds first; at most once
                        a minute, in the server's time zone.
                    </small>
                </div>
                <div class="field-inline">
                    <Checkbox v-model="enabled" :binary="true" inputId="schedule-enabled" />
                    <label for="schedule-enabled">Enabled</label>
                </div>
                <Message v-if="formError" severity="error" :closable="false">
                    {{ formError }}
                </Message>
                <div class="form-actions">
                    <Button
                        v-if="editingId"
                        label="Cancel edit"
                        text
                        severity="secondary"
                        :disabled="busy"
                        @click="resetForm"
                    />
                    <Button
                        :label="editingId ? 'Save changes' : 'Add schedule'"
                        icon="pi pi-check"
                        :loading="saving"
                        :disabled="removingId !== null"
                        data-test="schedule-save"
                        @click="save"
                    />
                </div>
            </div>
        </div>
        <template #footer>
            <Button label="Close" text severity="secondary" @click="visible = false" />
        </template>
    </Dialog>
</template>

<style scoped>
.schedule-manager {
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
    padding: 0.5rem 0;
}
.schedule-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
}
.schedule-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--app-border);
    border-radius: var(--app-radius-lg);
}
.schedule-row-info {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.85rem;
}
.schedule-row-label {
    font-size: 0.9rem;
    font-weight: 600;
}
.schedule-row-actions {
    display: flex;
    align-items: center;
    gap: 0.25rem;
}
.no-schedules {
    margin: 0;
}
.schedule-form {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    padding-top: 1rem;
    border-top: 1px solid var(--app-border);
}
.form-title {
    margin: 0;
    font-size: 1rem;
}
.presets {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
}
.preset-label {
    font-weight: 600;
}
.field {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
}
.field :deep(input) {
    width: 100%;
}
.field small {
    font-size: 0.8rem;
}
.field-inline {
    display: flex;
    align-items: center;
    gap: 0.5rem;
}
.error {
    color: var(--app-danger);
}
.form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
}
</style>
