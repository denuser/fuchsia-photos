<script setup lang="ts">
// A form field laid out as a PatternFly form group: the label above the control, a mark on required
// fields, and the helper text or the error below. The control goes into the default slot without a
// label of its own and binds the slot props, which give it the id the label points to:
//
//   <AppFormGroup label="Login" required v-slot="field">
//     <v-text-field v-bind="field" v-model="login" />
//   </AppFormGroup>
//
// Helper text is the control's own hint, which the group always shows.
import { computed, useId } from 'vue'

const props = defineProps<{ label: string; required?: boolean }>()

const id = useId()
const field = computed(() => ({ id, 'aria-required': props.required ? 'true' : undefined }))

const inputDefaults = { hideDetails: 'auto', persistentHint: true }
const defaults = {
  VTextField: inputDefaults,
  VTextarea: inputDefaults,
  VSelect: inputDefaults,
  VCombobox: inputDefaults,
  VAutocomplete: inputDefaults,
}
</script>

<template>
  <div class="form-group">
    <label :for="id" class="form-group__label">
      {{ label }}<span v-if="required" class="form-group__required" aria-hidden="true">&nbsp;*</span>
    </label>
    <v-defaults-provider :defaults="defaults">
      <slot v-bind="field" />
    </v-defaults-provider>
  </div>
</template>

<style scoped>
/* Groups stacked in a form keep 24px apart. The rule sits in Vuetify's override layer, so a
   group in a row drops the margin with a utility class such as mb-0. */
@layer vuetify-overrides {
  .form-group:not(:last-child) {
    margin-bottom: 24px;
  }
}

.form-group__label {
  display: inline-block;
  padding-bottom: 8px;
  font-size: 0.875rem;
  font-weight: 500;
  line-height: 1.5;
}

.form-group__required {
  color: rgb(var(--v-theme-error));
}

/* Helper text and errors sit right under the control, at the body size. */
.form-group :deep(.v-input__details:not(.v-input__details--hidden)) {
  padding-inline: 0;
  padding-top: 4px;
  min-height: 0;
}

.form-group :deep(.v-input__details--hidden) {
  display: none;
}

.form-group :deep(.v-messages) {
  font-size: 0.875rem;
  line-height: 1.5;
  opacity: 1;
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
}

.form-group :deep(.v-input--error .v-messages) {
  color: rgb(var(--v-theme-error));
}
</style>
