<script setup lang="ts">
// The header of a page as PatternFly lays it out: breadcrumbs to the pages
// above, the page title, a short description, and the page actions on the
// right. The title is the page's only h1.

export interface Crumb {
  title: string
  to: string
}

defineProps<{
  title: string
  description?: string
  breadcrumbs?: Crumb[]
}>()
</script>

<template>
  <header class="page-header">
    <nav v-if="breadcrumbs?.length" aria-label="Breadcrumb">
      <ol class="page-header__breadcrumbs">
        <li v-for="crumb in breadcrumbs" :key="crumb.to">
          <router-link :to="crumb.to">{{ crumb.title }}</router-link>
          <v-icon icon="mdi-chevron-right" size="x-small" class="page-header__divider" aria-hidden="true" />
        </li>
        <li aria-current="page">{{ title }}</li>
      </ol>
    </nav>
    <div class="page-header__main">
      <div class="page-header__text">
        <h1>{{ title }}</h1>
        <p v-if="description || $slots.description" class="page-header__description">
          <slot name="description">{{ description }}</slot>
        </p>
      </div>
      <div v-if="$slots.actions" class="page-header__actions">
        <slot name="actions" />
      </div>
    </div>
  </header>
</template>

<style scoped>
@layer vuetify-overrides {
  /* PatternFly page sections: 16px above and below, 24px at the sides. */
  .page-header {
    padding: 16px 24px;
  }

  @media (max-width: 599.98px) {
    .page-header {
      padding: 16px;
    }
  }

  .page-header__breadcrumbs {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    margin: 0 0 16px;
    padding: 0;
    list-style: none;
    font-size: 0.875rem;
    line-height: 1.5;
  }

  .page-header__breadcrumbs li {
    display: inline-flex;
    align-items: center;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .page-header__breadcrumbs a {
    color: rgb(var(--v-theme-primary));
    text-decoration: none;
  }

  .page-header__breadcrumbs a:hover {
    text-decoration: underline;
  }

  .page-header__divider {
    margin: 0 8px;
    color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
  }

  /* The row is as tall as a control, so that actions shown only some of the
     time do not change the height of the header. */
  .page-header__main {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 16px;
    min-height: 40px;
  }

  .page-header__text {
    flex: 1 1 240px;
    min-width: 0;
  }

  .page-header__text h1 {
    overflow-wrap: anywhere;
  }

  .page-header__description {
    margin: 8px 0 0;
  }

  .page-header__actions {
    display: flex;
    flex: none;
    align-items: center;
    gap: 8px;
  }
}
</style>
