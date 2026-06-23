<template>
  <section class="public-hero">
    <div class="hero-stars">
      <span v-for="i in 20" :key="i" class="star" :style="starStyle(i)"></span>
    </div>
    <div class="hero-row">
      <div class="hero-head">
        <h1 class="hero-title">软件包</h1>
        <p class="hero-subtitle">浏览与搜索企业内所有制品</p>
      </div>
      <div class="hero-moon">
        <div class="moon-body">
          <div class="moon-shadow"></div>
        </div>
      </div>
    </div>
    <div class="hero-accent-line"></div>
  </section>
</template>

<script setup lang="ts">
function starStyle(i: number) {
  const x = ((i * 37) % 100)
  const y = ((i * 53) % 80) + 5
  const size = 1 + (i % 3)
  const delay = (i * 0.3) % 4
  const opacity = 0.2 + (i % 5) * 0.15
  return {
    left: `${x}%`,
    top: `${y}%`,
    width: `${size}px`,
    height: `${size}px`,
    animationDelay: `${delay}s`,
    opacity: `${opacity}`,
  }
}
</script>

<style scoped>
.public-hero {
  position: relative;
  padding: 24px 0 0;
  overflow: hidden;
  min-height: 100px;
}

.hero-stars {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.star {
  position: absolute;
  border-radius: 50%;
  background: var(--lunar-accent, #7c3aed);
  animation: twinkle 3s ease-in-out infinite alternate;
}

@keyframes twinkle {
  0% { opacity: 0.15; }
  100% { opacity: 0.6; }
}

.hero-row {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 12px 0 20px;
}

.hero-head {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.hero-title {
  margin: 0;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: -0.5px;
  line-height: 1.1;
  color: var(--lunar-silver, #1e1b4b);
}

.hero-subtitle {
  margin: 0;
  font-size: 14px;
  line-height: 1.4;
  color: var(--lunar-silver-muted, #4c4885);
}

.hero-moon {
  flex-shrink: 0;
}

.moon-body {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  background: radial-gradient(circle at 35% 35%, #e9e0ff 0%, #c4b5fd 50%, #a78bfa 100%);
  box-shadow:
    0 0 20px rgba(196, 181, 253, 0.3),
    0 0 60px rgba(167, 139, 250, 0.15),
    inset -3px -3px 8px rgba(0, 0, 0, 0.05);
  position: relative;
  overflow: hidden;
}

.moon-shadow {
  position: absolute;
  top: -10px;
  right: -10px;
  width: 50px;
  height: 50px;
  border-radius: 50%;
  background: var(--lunar-bg-deep, #f0f2f8);
  box-shadow: 0 0 10px 4px var(--lunar-bg-deep, #f0f2f8);
}

.hero-accent-line {
  height: 1px;
  background: linear-gradient(90deg,
    var(--lunar-border, rgba(139, 92, 246, 0.12)) 0%,
    var(--lunar-accent, #7c3aed) 50%,
    var(--lunar-border, rgba(139, 92, 246, 0.12)) 100%);
  opacity: 0.5;
}

@media (max-width: 640px) {
  .hero-moon {
    display: none;
  }
  .hero-title {
    font-size: 22px;
  }
}
</style>
