import { createRouter, createWebHistory } from 'vue-router'

import { useAuthStore } from './stores/auth'
import AdminView from './views/AdminView.vue'
import AllPhotosView from './views/AllPhotosView.vue'
import AlbumsView from './views/AlbumsView.vue'
import AlbumView from './views/AlbumView.vue'
import ArchiveView from './views/ArchiveView.vue'
import FavoritesView from './views/FavoritesView.vue'
import MemoriesView from './views/MemoriesView.vue'
import ImportView from './views/ImportView.vue'
import ActivityView from './views/ActivityView.vue'
import LoginView from './views/LoginView.vue'
import ProfileView from './views/ProfileView.vue'
import SearchView from './views/SearchView.vue'
import SharedView from './views/SharedView.vue'
import TagsView from './views/TagsView.vue'
import TimelineView from './views/TimelineView.vue'
import TrashView from './views/TrashView.vue'
import UsersView from './views/UsersView.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: LoginView, meta: { public: true } },
    { path: '/', component: TimelineView },
    { path: '/search', component: SearchView },
    { path: '/albums', component: AlbumsView },
    { path: '/albums/:id(\\d+)', component: AlbumView },
    { path: '/favorites', component: FavoritesView },
    { path: '/memories', component: MemoriesView },
    // Loaded when first opened, so that the map library stays out of the main bundle.
    { path: '/map', component: () => import('./views/MapView.vue') },
    { path: '/archive', component: ArchiveView },
    { path: '/shared', component: SharedView },
    { path: '/tags', component: TagsView },
    { path: '/trash', component: TrashView },
    { path: '/profile', component: ProfileView },
    { path: '/all', component: AllPhotosView, meta: { admin: true } },
    { path: '/import', component: ImportView, meta: { admin: true } },
    { path: '/users', component: UsersView, meta: { admin: true } },
    { path: '/activity', component: ActivityView, meta: { admin: true } },
    { path: '/admin', component: AdminView, meta: { admin: true } },
    { path: '/:rest(.*)*', redirect: '/' },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  await auth.load()

  if (to.meta.public) {
    return auth.user ? '/' : true
  }
  if (!auth.user) {
    return '/login'
  }
  if (to.meta.admin && !auth.isAdmin) {
    return '/'
  }
  return true
})
