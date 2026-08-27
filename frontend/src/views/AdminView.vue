<script setup>
import AdminUsers from '@/components/admin/AdminUsers.vue'
import AdminSettings from '@/components/admin/AdminSettings.vue'
import AdminDictionary from '@/components/admin/AdminDictionary.vue'
</script>

<template>
  <div class="admin-page">
    <h1>后台管理</h1>
    <AdminUsers />
    <AdminSettings />
    <AdminDictionary />
  </div>
</template>

<!-- 后台管理页的样式是全局的（非 scoped）：AdminUsers / AdminDictionary 等子组件
     里的 .link-btn / .badge / .sense 等 class 样式定义在这里，靠子组件模板复用。
     但全局选择器会泄漏到其它页面（比如 .sense 和背单词卡片重名、button.primary 会命中
     卡片里的归档按钮），所以一律加 .admin-page 前缀收窄作用域，只作用于后台页。 -->
<style>
.admin-page h2 {
  font-size: 16px;
  font-weight: 600;
  margin: 32px 0 12px;
}
.admin-page label {
  display: block;
  font-size: 13px;
  color: var(--muted);
  margin: 12px 0 4px;
}
.admin-page label:first-child {
  margin-top: 0;
}
.admin-page input[type='text'],
.admin-page input[type='password'] {
  width: 100%;
  padding: 10px 12px;
  font-size: 14px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--bg);
  color: var(--text);
  outline: none;
}
.admin-page .checkbox-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 12px 0 4px;
}
.admin-page .checkbox-row input {
  width: auto;
}
.admin-page button.primary {
  margin-top: 16px;
  padding: 10px 16px;
  font-size: 14px;
  border: none;
  border-radius: 8px;
  background: var(--accent);
  color: #fff;
  cursor: pointer;
}
.admin-page button.primary:disabled {
  opacity: 0.6;
  cursor: default;
}
.admin-page .msg {
  font-size: 13px;
  margin-top: 10px;
  min-height: 16px;
}
.admin-page .msg.error {
  color: var(--danger);
}
.admin-page .msg.ok {
  color: var(--accent);
}
.admin-page table {
  width: 100%;
  border-collapse: collapse;
  font-size: 14px;
}
.admin-page th,
.admin-page td {
  text-align: left;
  padding: 8px 6px;
  border-bottom: 1px solid var(--border);
}
.admin-page th {
  color: var(--muted);
  font-weight: 500;
}
/* 列多的表格（用户管理、词库管理）给横向滚动容器 + 右侧操作列 sticky：
   窄屏放不下时其它列横向滚动，操作列固定在右侧不跟着滚。sticky 列必须有不透明背景，
   否则滚动时下方内容会透出来；z-index 保证它盖在滚动穿过来的单元格之上。 */
.admin-page .table-scroll {
  overflow-x: auto;
}
.admin-page .table-scroll table {
  min-width: 760px;
}
.admin-page th.op-col,
.admin-page td.op-col {
  position: sticky;
  right: 0;
  background: var(--card-bg);
  z-index: 1;
  white-space: nowrap;
}
.admin-page .nowrap {
  white-space: nowrap;
}
.admin-page .badge {
  font-size: 11px;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 2px 6px;
  border-radius: 5px;
}
.admin-page .badge.disabled {
  color: var(--danger);
  background: var(--danger-soft);
}
.admin-page .link-btn {
  border: none;
  background: transparent;
  color: var(--accent);
  font-size: 12px;
  cursor: pointer;
  padding: 4px 6px;
}
.admin-page .link-btn.danger {
  color: var(--muted);
}
.admin-page .link-btn.danger:hover {
  color: var(--danger);
}
.admin-page .link-btn:disabled {
  color: var(--muted);
  cursor: not-allowed;
  opacity: 0.6;
}
.admin-page .export-btn {
  display: inline-block;
  font-size: 13px;
  color: var(--accent);
  text-decoration: none;
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 8px 14px;
}
.admin-page .export-btn:hover {
  border-color: var(--accent);
}
.admin-page .senses {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.admin-page .sense {
  display: flex;
  align-items: baseline;
  gap: 6px;
  flex-wrap: wrap;
}
.admin-page .sense .pos {
  font-size: 11px;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 1px 5px;
  border-radius: 5px;
  white-space: nowrap;
}
.admin-page .sense .translation {
  font-size: 13px;
  color: var(--text);
  word-break: break-word;
}
.admin-page code.generated-password {
  background: var(--accent-soft);
  color: var(--accent);
  padding: 2px 6px;
  border-radius: 5px;
  font-size: 13px;
}
</style>
