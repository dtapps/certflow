import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/',
      name: 'dashboard',
      component: () => import('../views/Dashboard.vue'),
    },
    {
      path: '/certificates',
      name: 'certificates',
      component: () => import('../views/Certificates.vue'),
    },
    {
      path: '/certificates/apply',
      name: 'apply',
      component: () => import('../views/CertApply.vue'),
    },
    {
      path: '/certificates/:id',
      name: 'cert-detail',
      component: () => import('../views/CertDetail.vue'),
    },
    {
      path: '/ca',
      name: 'ca',
      component: () => import('../views/CAConfig.vue'),
    },
    {
      path: '/dns',
      name: 'dns',
      component: () => import('../views/DNSProviders.vue'),
    },
    {
      path: '/deploy',
      name: 'deploy',
      component: () => import('../views/DeployTargets.vue'),
    },
    {
      path: '/deploy/new',
      name: 'deploy-new',
      component: () => import('../views/DeployTargetForm.vue'),
    },
    {
      path: '/deploy/:id/edit',
      name: 'deploy-edit',
      component: () => import('../views/DeployTargetForm.vue'),
    },
    {
      path: '/monitor',
      name: 'monitor',
      component: () => import('../views/Monitor.vue'),
    },
    {
      path: '/scan',
      name: 'scan',
      component: () => import('../views/Scan.vue'),
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('../views/Settings.vue'),
    },
    {
      path: '/personal-center',
      name: 'personal-center',
      component: () => import('../views/PersonalCenter.vue'),
    },
  ],
})

export default router
