import { createRouter, createWebHistory } from 'vue-router'

import { useAuthStore } from './stores/auth'
import AlbumsView from './views/AlbumsView.vue'
import AlbumView from './views/AlbumView.vue'
import LoginView from './views/LoginView.vue'
import TagsView from './views/TagsView.vue'
import TimelineView from './views/TimelineView.vue'
import TrashView from './views/TrashView.vue'
import UsersView from './views/UsersView.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: LoginView, meta: { public: true } },
    { path: '/', component: TimelineView },
    { path: '/albums', component: AlbumsView },
    { path: '/albums/:id(\\d+)', component: AlbumView },
    { path: '/tags', component: TagsView },
    { path: '/trash', component: TrashView },
    { path: '/users', component: UsersView, meta: { admin: true } },
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
