<script setup lang="ts">
import SettingsRow from '@/components/settings/SettingsRow.vue'
import { useInfo, useMe } from '@/composables/useApi'

// Who is signed in and what server this is: /me and /info, both cached by
// the shell already.
const { data: me } = useMe()
const { data: info } = useInfo()
</script>

<template>
    <section aria-label="About">
        <h2 class="set-section-title">About</h2>
        <div class="set-card">
            <SettingsRow name="User" desc="The login you are signed in as.">
                <span class="set-val" data-test="about-user">{{ me?.subject ?? '…' }}</span>
            </SettingsRow>
            <SettingsRow name="Tenant" desc="Notes are shared with everyone in it.">
                <span class="set-val" data-test="about-tenant">{{ me?.tenant ?? '…' }}</span>
            </SettingsRow>
            <SettingsRow name="Server version" desc="The build of the running server.">
                <span class="set-val mono" data-test="about-version">{{
                    info?.version ?? '…'
                }}</span>
            </SettingsRow>
            <SettingsRow v-if="info?.commit" name="Commit" desc="The source it was built from.">
                <span class="set-val mono">{{ info.commit }}</span>
            </SettingsRow>
        </div>
    </section>
</template>

<style scoped>
.set-val {
    font-size: 0.9rem;
    color: var(--app-text-muted);
    overflow-wrap: anywhere;
}

.mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.82rem;
}
</style>
