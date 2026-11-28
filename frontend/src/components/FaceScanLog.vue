<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";

import { api, type FaceScans, type PhotoFaceScan } from "../api/client";
import { formatDateTime, t } from "../i18n";
import AppFormGroup from "./AppFormGroup.vue";
import EmptyState from "./EmptyState.vue";

const log = ref<FaceScans | null>(null);
const photoId = ref("");
const lookup = ref<PhotoFaceScan | null>(null);
const lookupError = ref<"notFound" | "internal" | null>(null);
const lookingUp = ref(false);
const validPhotoId = computed(() => {
  const input = photoId.value.trim();
  return /^[0-9]+$/.test(input) && Number.isSafeInteger(Number(input)) && Number(input) > 0;
});
let timer: ReturnType<typeof setInterval> | undefined;

async function findPhoto() {
  if (!validPhotoId.value || lookingUp.value) return;
  const id = Number(photoId.value.trim());
  lookup.value = null;
  lookupError.value = null;
  lookingUp.value = true;
  try {
    const { data, response } = await api.GET("/ml/faces/{id}", { params: { path: { id } } });
    lookup.value = data ?? null;
    if (!data) lookupError.value = response.status === 404 ? "notFound" : "internal";
  } catch {
    lookupError.value = "internal";
  } finally {
    lookingUp.value = false;
  }
}

async function load() {
  const { data } = await api.GET("/ml/faces");
  if (data) {
    log.value = data;
  }
}

onMounted(() => {
  void load();
  timer = setInterval(() => void load(), 5000);
});
onUnmounted(() => clearInterval(timer));
</script>

<template>
  <div data-testid="face-scan-log">
    <p class="text-medium-emphasis">{{ t("faceLog.description") }}</p>
    <template v-if="log">
      <p class="mb-2">
        {{ t("faceLog.service") }} · {{ t(`faceLog.serviceStates.${log.serviceState}`) }}
      </p>
      <p class="mb-4">
        {{
          t("faceLog.totals", {
            scanned: log.scanned,
            withFaces: log.withFaces,
            failing: log.failing,
          })
        }}
      </p>
      <form class="photo-lookup mb-4" @submit.prevent="findPhoto">
        <AppFormGroup v-slot="field" :label="t('faceLog.photoId')" class="mb-0">
          <v-text-field v-bind="field" v-model="photoId" inputmode="numeric" density="compact" variant="outlined" />
        </AppFormGroup>
        <v-btn type="submit" :disabled="!validPhotoId" :loading="lookingUp">{{ t("faceLog.lookup") }}</v-btn>
      </form>
      <p v-if="lookupError" role="alert" class="mb-4 text-error">{{ t(`apiErrors.${lookupError}`) }}</p>
      <p v-if="lookup" class="mb-4" role="status">
        <strong>{{ lookup.name || lookup.photoId }} · #{{ lookup.photoId }}</strong>
        <template v-if="lookup.state === 'ineligible'"> · {{ t("faceLog.ineligible") }}</template>
        <template v-else-if="lookup.state === 'waiting'"> · {{ t("faceLog.waiting") }}</template>
        <template v-else>
          ·
          <template v-if="lookup.state === 'error'">{{ t("faceLog.failed", { error: lookup.error }) }}</template>
          <template v-else-if="lookup.faces">{{ t("faceLog.found", { n: lookup.faces }, lookup.faces) }}</template>
          <template v-else>{{ t("faceLog.noFaces") }}</template>
          <template v-if="lookup.scannedAt"> · {{ formatDateTime(lookup.scannedAt) }}</template>
        </template>
      </p>
      <EmptyState
        v-if="log.scans.length === 0"
        icon="fa-users"
        :title="t('faceLog.none')"
        :text="t('faceLog.noneText')"
      />
      <v-table v-else>
        <thead>
          <tr>
            <th>{{ t("faceLog.time") }}</th>
            <th>{{ t("faceLog.photo") }}</th>
            <th>{{ t("faceLog.result") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="scan in log.scans" :key="scan.photoId">
            <td>{{ formatDateTime(scan.scannedAt) }}</td>
            <td>{{ scan.name }} <span class="text-medium-emphasis">#{{ scan.photoId }}</span></td>
            <td v-if="scan.error" class="text-error">
              {{ t("faceLog.failed", { error: scan.error }) }}
            </td>
            <td v-else>
              {{
                scan.faces > 0
                  ? t("faceLog.found", { n: scan.faces }, scan.faces)
                  : t("faceLog.noFaces")
              }}
            </td>
          </tr>
        </tbody>
      </v-table>
    </template>
  </div>
</template>

<style scoped>
.photo-lookup {
  display: flex;
  align-items: flex-end;
  flex-wrap: wrap;
  gap: 0.75rem;
}
</style>
