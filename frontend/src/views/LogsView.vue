<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";

import { t } from "../i18n";
import ActivityLog from "../components/ActivityLog.vue";
import AppPageHeader from "../components/AppPageHeader.vue";
import FaceScanLog from "../components/FaceScanLog.vue";

const route = useRoute();
const router = useRouter();

// The tab lives in the address, so a link opens the right log.
const tab = computed({
  get: () => (route.query.tab === "ml" ? "ml" : "activity"),
  set: (value: string) => void router.replace({ query: { tab: value } }),
});
</script>

<template>
  <div class="page">
    <AppPageHeader
      :title="t('logs.title')"
      :description="t('logs.description')"
    />

    <v-tabs v-model="tab" class="px-4 px-sm-6">
      <v-tab value="activity">{{ t("logs.activity") }}</v-tab>
      <v-tab value="ml">{{ t("logs.ml") }}</v-tab>
    </v-tabs>

    <div class="page-section logs">
      <ActivityLog v-if="tab === 'activity'" />
      <FaceScanLog v-else />
    </div>
  </div>
</template>

<style scoped>
.logs {
  max-width: 1200px;
}
</style>
