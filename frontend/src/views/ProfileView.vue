<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import {
  api,
  type AppPassword,
  type GridSize,
  type Identity,
  type Language,
  type SignInOptions,
  type TwoFactorStatus,
  type ViewerField,
  type WebDAVAlbum,
} from '../api/client'
import AppFormGroup from '../components/AppFormGroup.vue'
import AppPageHeader from '../components/AppPageHeader.vue'
import AppPasswordDialog from '../components/AppPasswordDialog.vue'
import ChangePasswordDialog from '../components/ChangePasswordDialog.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import CopyField from '../components/CopyField.vue'
import FolderAlbumDialog from '../components/FolderAlbumDialog.vue'
import SetPasswordDialog from '../components/SetPasswordDialog.vue'
import TwoFactorDisableDialog from '../components/TwoFactorDisableDialog.vue'
import TwoFactorSetupDialog from '../components/TwoFactorSetupDialog.vue'
import { formatDate, languages, t } from '../i18n'
import { apiError } from '../i18n/apiErrors'
import { gridSizes } from '../lib/timeline'
import { viewerFields } from '../lib/viewerFields'
import { useAuthStore } from '../stores/auth'
import { useNoticesStore } from '../stores/notices'

const auth = useAuthStore()

const passwordDialog = ref(false)
const saving = ref(false)
const error = ref('')

const shown = computed(() => viewerFields.filter((f) => !auth.user?.viewerHiddenFields.includes(f)))

// Each language is named in itself, so that a user who cannot read the current one finds theirs.
const languageChoices = computed(() => [
  { value: 'auto', title: t('profile.languageAuto') },
  ...languages.map((value) => ({ value, title: t('profile.languageName', {}, { locale: value }) })),
])

type Changes = {
  viewerShowsOriginals?: boolean
  viewerHiddenFields?: ViewerField[]
  navigationHiddenOnPhones?: boolean
  gridSquares?: boolean
  gridSize?: GridSize
  language?: Language
}

// Each change is saved at once, like the switch on the "Shared with me" page.
async function save(body: Changes) {
  saving.value = true
  error.value = ''
  const { data, error: failure } = await api.PATCH('/me', { body })
  saving.value = false
  if (!data) {
    error.value = apiError(failure, t('profile.saveFailed'))
    return
  }
  auth.user = data
}

function setShown(value: ViewerField[] | null) {
  const visible = value ?? []
  void save({ viewerHiddenFields: viewerFields.filter((f) => !visible.includes(f)) })
}

const notices = useNoticesStore()
const webdavAddress = `${window.location.origin}/webdav/`
const appPasswords = ref<AppPassword[]>([])
const appPasswordsLoaded = ref(false)
const appPasswordDialog = ref(false)
const revokeOpen = ref(false)
const revoking = ref<AppPassword>()

async function loadAppPasswords() {
  const { data } = await api.GET('/me/app-passwords')
  if (!data) {
    error.value = t('profile.appPasswordsFailed')
    return
  }
  appPasswords.value = data
  appPasswordsLoaded.value = true
}

function startRevoke(password: AppPassword) {
  revoking.value = password
  revokeOpen.value = true
}

async function revoke() {
  const password = revoking.value
  if (!password) {
    return
  }
  const { error: failure } = await api.DELETE('/me/app-passwords/{id}', { params: { path: { id: password.id } } })
  if (failure) {
    error.value = apiError(failure)
    return
  }
  appPasswords.value = appPasswords.value.filter((p) => p.id !== password.id)
  notices.notify(t('profile.revoked', { name: password.name }))
}

const folderAlbums = ref<WebDAVAlbum[]>([])
const folderAlbumsLoaded = ref(false)
const folderAlbumDialog = ref(false)

async function loadFolderAlbums() {
  const { data } = await api.GET('/me/webdav-albums')
  if (!data) {
    error.value = t('profile.folderAlbumsFailed')
    return
  }
  folderAlbums.value = data
  folderAlbumsLoaded.value = true
}

function linked(link: WebDAVAlbum) {
  folderAlbums.value = [...folderAlbums.value, link].sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0))
  notices.notify(t('profile.linked', { path: link.path, album: link.albumName }))
}

async function unlink(link: WebDAVAlbum) {
  const { error: failure } = await api.DELETE('/me/webdav-albums/{id}', { params: { path: { id: link.id } } })
  if (failure) {
    error.value = apiError(failure)
    return
  }
  folderAlbums.value = folderAlbums.value.filter((l) => l.id !== link.id)
  notices.notify(t('profile.unlinked', { path: link.path, album: link.albumName }))
}

function formatUsed(value: string | null) {
  return value ? formatDate(value) : t('profile.never')
}

const identities = ref<Identity[]>([])
const identitiesLoaded = ref(false)
const providers = ref<SignInOptions['providers']>([])
const unlinkOpen = ref(false)
const unlinking = ref<Identity>()

async function loadIdentities() {
  const [list, options] = await Promise.all([api.GET('/me/identities'), api.GET('/auth/options')])
  if (!list.data || !options.data) {
    error.value = t('profile.providersFailed')
    return
  }
  identities.value = list.data
  providers.value = options.data.providers
  identitiesLoaded.value = true
}

// The issuer is an address, which says enough about who the provider is.
function issuerName(issuer: string) {
  try {
    return new URL(issuer).host
  } catch {
    return issuer
  }
}

function startUnlink(identity: Identity) {
  unlinking.value = identity
  unlinkOpen.value = true
}

async function unlinkIdentity() {
  const identity = unlinking.value
  if (!identity) {
    return
  }
  const { error: failure } = await api.DELETE('/me/identities/{id}', { params: { path: { id: identity.id } } })
  if (failure) {
    // The list is far down the page, so the alert at the top would not be seen.
    notices.notify(apiError(failure), 'error')
    return
  }
  identities.value = identities.value.filter((i) => i.id !== identity.id)
  notices.notify(t('profile.providerUnlinked'))
}

// A link made at a provider comes back here with its outcome in the address.
const route = useRoute()
const router = useRouter()
const linkErrors = ['expired', 'denied', 'unavailable', 'taken', 'wrong_account', 'failed'] as const
// Signing in again at the provider to set a first password comes back with setPassword=1.
const setPasswordOpen = ref(false)

async function passwordSet() {
  notices.notify(t('profile.passwordSet'))
  const { data } = await api.GET('/me')
  if (data) {
    auth.user = data
  }
}

function showLinkOutcome() {
  if (route.query.linked) {
    notices.notify(t('profile.providerLinked'))
  }
  if (route.query.setPassword && auth.user && !auth.user.hasPassword) {
    setPasswordOpen.value = true
  }
  const code = String(route.query.error ?? '').replace(/^oidc_/, '')
  if ((linkErrors as readonly string[]).includes(code)) {
    error.value = t(`login.oidcErrors.${code as (typeof linkErrors)[number]}`)
  }
  if (route.query.linked || route.query.error || route.query.setPassword) {
    void router.replace({ path: route.path })
  }
}

const twoFactor = ref<TwoFactorStatus | null>(null)
const twoFactorSetup = ref(false)
const twoFactorDisable = ref(false)

async function loadTwoFactor() {
  const { data } = await api.GET('/me/2fa')
  twoFactor.value = data ?? null
}

// The user's own flag follows too, as the administrators' list and the sign-in read it.
async function twoFactorChanged() {
  await loadTwoFactor()
  const { data } = await api.GET('/me')
  if (data) {
    auth.user = data
  }
}

onMounted(() => {
  void loadTwoFactor()
  showLinkOutcome()
  void loadAppPasswords()
  void loadFolderAlbums()
  void loadIdentities()
})
</script>

<template>
  <div v-if="auth.user" class="page">
    <AppPageHeader :title="auth.user.login" :description="auth.isAdmin ? t('roles.admin') : t('roles.user')" />
    <div class="page-section profile">

      <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />

      <h2 class="mb-2">{{ t('profile.language') }}</h2>
      <AppFormGroup v-slot="field" :label="t('profile.interfaceLanguage')" class="language mb-6">
        <v-select
          v-bind="field"
          :model-value="auth.user.language"
          :items="languageChoices"
          :disabled="saving"
          :hint="t('profile.languageHint')"
          @update:model-value="save({ language: $event })"
        />
      </AppFormGroup>

      <h2 class="mb-2">{{ t('profile.grid') }}</h2>
      <v-switch
        :model-value="auth.user.gridSquares"
        :disabled="saving"
        :label="t('profile.squares')"
        :hint="t('profile.squaresHint')"
        persistent-hint
        color="primary"
        density="compact"
        class="mb-4"
        @update:model-value="save({ gridSquares: Boolean($event) })"
      />
      <AppFormGroup v-slot="field" :label="t('profile.gridSize')" class="mb-6">
        <!-- The toggle is inline; the block keeps it under the label. -->
        <div>
          <v-btn-toggle
            v-bind="field"
            :model-value="auth.user.gridSize"
            mandatory
            divided
            variant="outlined"
            density="compact"
            role="group"
            :aria-label="t('profile.gridSize')"
            :disabled="saving"
            data-testid="grid-size"
            @update:model-value="save({ gridSize: $event })"
          >
            <v-btn v-for="size in gridSizes" :key="size" :value="size" :text="t(`profile.gridSizes.${size}`)" />
          </v-btn-toggle>
        </div>
      </AppFormGroup>

      <h2 class="mb-2">{{ t('profile.viewer') }}</h2>
      <v-switch
        :model-value="auth.user.viewerShowsOriginals"
        :disabled="saving"
        :label="t('profile.originals')"
        :hint="t('profile.originalsHint')"
        persistent-hint
        color="primary"
        density="compact"
        class="mb-4"
        @update:model-value="save({ viewerShowsOriginals: Boolean($event) })"
      />

      <h3 class="mb-1">{{ t('profile.details') }}</h3>
      <div class="text-medium-emphasis mb-2">
        {{ t('profile.detailsHint') }}
      </div>
      <div class="fields mb-6">
        <v-checkbox
          v-for="field in viewerFields"
          :key="field"
          :model-value="shown"
          :value="field"
          :label="t(`viewerFields.${field}`)"
          :disabled="saving"
          density="compact"
          hide-details
          @update:model-value="setShown"
        />
      </div>

      <h2 class="mb-2">{{ t('profile.phone') }}</h2>
      <v-switch
        :model-value="auth.user.navigationHiddenOnPhones"
        :disabled="saving"
        :label="t('profile.hideMenu')"
        :hint="t('profile.hideMenuHint')"
        persistent-hint
        color="primary"
        density="compact"
        class="mb-6"
        @update:model-value="save({ navigationHiddenOnPhones: Boolean($event) })"
      />

      <h2 class="mb-2">{{ t('profile.password') }}</h2>
      <template v-if="!auth.user.hasPassword">
        <div class="text-medium-emphasis mb-4">{{ t('profile.noPassword') }}</div>
        <template v-if="providers.length > 0">
          <div class="text-medium-emphasis mb-4">{{ t('profile.setPasswordHint') }}</div>
          <div class="mb-6">
            <!-- A plain link: the whole page goes to the provider, which asks for the sign-in again. -->
            <v-btn
              v-for="provider in providers"
              :key="provider.id"
              :href="`/api/v1/auth/oidc/${provider.id}/login?password=1`"
              prepend-icon="fa-key"
              variant="tonal"
              class="mr-2 mb-2"
              data-testid="set-password"
              :text="t('profile.setPasswordVia', { name: provider.name })"
            />
          </div>
        </template>
        <SetPasswordDialog v-model="setPasswordOpen" @saved="passwordSet" />
      </template>
      <template v-else>
        <v-btn prepend-icon="fa-key" variant="tonal" :text="t('changePassword.title')" class="mb-6" @click="passwordDialog = true" />
        <ChangePasswordDialog v-model="passwordDialog" />
      </template>

      <template v-if="auth.user.hasPassword && twoFactor">
        <h2 class="mb-2">{{ t('twoFactor.title') }}</h2>
        <div class="text-medium-emphasis mb-4">{{ t('twoFactor.hint') }}</div>
        <div v-if="!twoFactor.available && !twoFactor.enabled" class="text-medium-emphasis mb-6">{{ t('twoFactor.unavailable') }}</div>
        <div v-else class="mb-6">
          <div class="mb-2" data-testid="two-factor-state">
            {{ twoFactor.enabled ? t('twoFactor.on') : t('twoFactor.off') }}<template v-if="twoFactor.enabled"> · {{ t('twoFactor.left', { count: twoFactor.recoveryCodesLeft }) }}</template>
          </div>
          <v-btn v-if="!twoFactor.enabled" prepend-icon="fa-user-shield" variant="tonal" :text="t('twoFactor.enable')" @click="twoFactorSetup = true" />
          <v-btn v-else prepend-icon="fa-user-shield" variant="tonal" :text="t('twoFactor.disable')" @click="twoFactorDisable = true" />
        </div>
        <TwoFactorSetupDialog v-model="twoFactorSetup" @enabled="twoFactorChanged" />
        <TwoFactorDisableDialog v-model="twoFactorDisable" @disabled="twoFactorChanged" />
      </template>

      <template v-if="providers.length > 0 || identities.length > 0">
        <h2 class="mb-2">{{ t('profile.providers') }}</h2>
        <div class="text-medium-emphasis mb-4">{{ t('profile.providersHint') }}</div>
        <v-table v-if="identities.length > 0" class="mb-4">
          <thead>
            <tr>
              <th>{{ t('profile.provider') }}</th>
              <th>{{ t('profile.email') }}</th>
              <th>{{ t('profile.created') }}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            <tr v-for="identity in identities" :key="identity.id" data-testid="identity">
              <td>{{ issuerName(identity.issuer) }}</td>
              <td>{{ identity.email }}</td>
              <td>{{ formatDate(identity.createdAt) }}</td>
              <td class="text-right">
                <v-btn icon="fa-link-slash" size="small" variant="text" :aria-label="t('profile.unlinkProvider', { name: issuerName(identity.issuer) })" @click="startUnlink(identity)" />
              </td>
            </tr>
          </tbody>
        </v-table>
        <div v-else-if="identitiesLoaded" class="text-medium-emphasis mb-4">{{ t('profile.noProviders') }}</div>
        <div class="mb-6">
          <v-btn
            v-for="provider in providers"
            :key="provider.id"
            :href="`/api/v1/auth/oidc/${provider.id}/login?link=1`"
            prepend-icon="fa-link"
            variant="tonal"
            class="mr-2 mb-2"
            data-testid="link-provider"
            :text="t('profile.linkProvider', { name: provider.name })"
          />
        </div>
        <ConfirmDialog
          v-model="unlinkOpen"
          :title="t('profile.unlinkTitle')"
          :text="t('profile.unlinkText', { name: unlinking ? issuerName(unlinking.issuer) : '' })"
          :action="t('profile.unlinkAction')"
          @confirm="unlinkIdentity"
        />
      </template>

      <h2 class="mb-2">{{ t('profile.webdav') }}</h2>
      <div class="text-medium-emphasis mb-4">
        {{ t('profile.webdavHint') }}
      </div>
      <div class="webdav-access mb-6">
        <AppFormGroup v-slot="field" :label="t('profile.address')" class="mb-0">
          <CopyField v-bind="field" :value="webdavAddress" :label="t('profile.addressLower')" />
        </AppFormGroup>
        <AppFormGroup v-slot="field" :label="t('profile.login')" class="mb-0">
          <CopyField v-bind="field" :value="auth.user.login" :label="t('profile.loginLower')" />
        </AppFormGroup>
      </div>

      <h3 class="mb-2">{{ t('profile.appPasswords') }}</h3>
      <v-table v-if="appPasswords.length > 0" class="mb-4">
        <thead>
          <tr>
            <th>{{ t('profile.device') }}</th>
            <th>{{ t('profile.created') }}</th>
            <th>{{ t('profile.lastUsed') }}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          <tr v-for="password in appPasswords" :key="password.id">
            <td>{{ password.name }}</td>
            <td>{{ formatDate(password.createdAt) }}</td>
            <td>{{ formatUsed(password.lastUsedAt) }}</td>
            <td class="text-right">
              <v-btn icon="fa-trash-can" size="small" variant="text" :aria-label="t('profile.revoke', { name: password.name })" @click="startRevoke(password)" />
            </td>
          </tr>
        </tbody>
      </v-table>
      <div v-else-if="appPasswordsLoaded" class="text-medium-emphasis mb-4">{{ t('profile.noAppPasswords') }}</div>
      <v-btn prepend-icon="fa-plus" variant="tonal" :text="t('appPassword.new')" @click="appPasswordDialog = true" />
      <AppPasswordDialog v-model="appPasswordDialog" @created="appPasswords.push($event)" />
      <ConfirmDialog
        v-model="revokeOpen"
        :title="t('profile.revokeTitle')"
        :text="t('profile.revokeText', { name: revoking?.name })"
        :action="t('profile.revokeAction')"
        @confirm="revoke"
      />

      <h3 class="mt-6 mb-1">{{ t('profile.folderAlbums') }}</h3>
      <div class="text-medium-emphasis mb-2">
        {{ t('profile.folderAlbumsHint') }}
      </div>
      <v-table v-if="folderAlbums.length > 0" class="mb-4">
        <thead>
          <tr>
            <th>{{ t('folderAlbum.folder') }}</th>
            <th>{{ t('folderAlbum.album') }}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          <tr v-for="link in folderAlbums" :key="link.id">
            <td>{{ link.path }}</td>
            <td>
              <router-link :to="`/albums/${link.albumId}`">{{ link.albumName }}</router-link>
            </td>
            <td class="text-right">
              <v-btn icon="fa-link-slash" size="small" variant="text" :aria-label="t('profile.unlink', { path: link.path })" @click="unlink(link)" />
            </td>
          </tr>
        </tbody>
      </v-table>
      <div v-else-if="folderAlbumsLoaded" class="text-medium-emphasis mb-4">{{ t('profile.noFolders') }}</div>
      <v-btn prepend-icon="fa-link" variant="tonal" :text="t('profile.linkFolder')" @click="folderAlbumDialog = true" />
      <FolderAlbumDialog v-model="folderAlbumDialog" @created="linked" />
    </div>
  </div>
</template>

<style scoped>
.profile {
  max-width: 768px;
}

.webdav-access {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 24px 16px;
}

.language {
  max-width: 320px;
}

.fields {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
}
</style>
