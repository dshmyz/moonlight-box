<template>
  <div class="footer-content">
    <div class="footer-left">
      <router-link to="/" class="footer-brand">
        <svg class="footer-logo" viewBox="0 0 40 40" xmlns="http://www.w3.org/2000/svg">
          <defs>
            <linearGradient id="moonlight-footer" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" style="stop-color:#c4b5fd" />
              <stop offset="100%" style="stop-color:#a78bfa" />
            </linearGradient>
          </defs>
          <circle cx="20" cy="20" r="12" fill="url(#moonlight-footer)" />
          <circle cx="24" cy="16" r="10" fill="var(--lunar-bg-surface)" />
          <circle cx="10" cy="10" r="1.5" fill="#c4b5fd" />
          <circle cx="32" cy="14" r="1" fill="#c4b5fd" opacity="0.6" />
          <circle cx="28" cy="30" r="1.2" fill="#c4b5fd" opacity="0.4" />
        </svg>
        Moonlight Box
      </router-link>
      <span class="footer-desc">企业级组件仓库管理平台</span>
    </div>
    <div class="footer-right">
      <router-link to="/about" class="footer-link">关于</router-link>
      <span class="footer-divider" />
      <span class="footer-copyright">&copy; 2026 Moonlight Box</span>
    </div>
  </div>
</template>

<script setup lang="ts">
</script>

<style scoped>
.footer-content {
  max-width: 1200px;
  margin: 0 auto;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 20px;
}

.footer-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.footer-brand {
  display: flex;
  align-items: center;
  gap: 8px;
  text-decoration: none;
  color: var(--lunar-silver);
  font-weight: 700;
  font-size: 14px;
  letter-spacing: -0.3px;
}

.footer-brand:hover {
  color: var(--lunar-accent);
}

.footer-logo {
  width: 20px;
  height: 20px;
}

.footer-desc {
  color: var(--lunar-silver-dim);
  font-size: 12px;
}

.footer-right {
  display: flex;
  align-items: center;
  gap: 12px;
  color: var(--lunar-silver-dim);
  font-size: 12px;
}

.footer-link {
  color: var(--lunar-silver-muted);
  text-decoration: none;
  transition: color 0.2s ease;
}

.footer-link:hover {
  color: var(--lunar-accent);
}

.footer-divider {
  width: 1px;
  height: 12px;
  background: var(--lunar-border);
}

.footer-copyright {
  color: var(--lunar-silver-dim);
}
</style>