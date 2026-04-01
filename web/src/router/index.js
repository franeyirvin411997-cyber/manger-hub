import { createRouter, createWebHistory } from 'vue-router'
import NodeList from '../views/NodeList.vue'
import ProxyList from '../views/ProxyList.vue'
import GroupList from '../views/GroupList.vue'
import RuleList from '../views/RuleList.vue'
import Login from '../views/Login.vue'
import OperationList from '../views/OperationList.vue'
import AppTemplateList from '../views/AppTemplateList.vue'

const routes = [
  {
    path: '/',
    redirect: '/nodes'
  },
  {
    path: '/login',
    name: 'Login',
    component: Login
  },
  {
    path: '/nodes',
    name: 'Nodes',
    component: NodeList,
    meta: { requiresAuth: true }
  },
  {
    path: '/proxies',
    name: 'Proxies',
    component: ProxyList,
    meta: { requiresAuth: true }
  },
  {
    path: '/groups',
    name: 'Groups',
    component: GroupList,
    meta: { requiresAuth: true }
  },
  {
    path: '/rules',
    name: 'Rules',
    component: RuleList,
    meta: { requiresAuth: true }
  },
  {
    path: '/apps',
    name: 'Apps',
    component: AppTemplateList,
    meta: { requiresAuth: true }
  },
  {
    path: '/operations',
    name: 'Operations',
    component: OperationList,
    meta: { requiresAuth: true }
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach((to, from, next) => {
  if (to.meta.requiresAuth) {
    const token = localStorage.getItem('auth_token')
    if (!token) {
      return next({ name: 'Login' })
    }
  }
  next()
})

export default router
