<script setup>
import AdminUsers from '@/components/admin/AdminUsers.vue'
import AdminSettings from '@/components/admin/AdminSettings.vue'
import AdminDictionary from '@/components/admin/AdminDictionary.vue'
</script>

<template>
  <div>
    <h1>后台管理</h1>
    <AdminUsers />
    <AdminSettings />
    <AdminDictionary />
  </div>
</template>

<style>
h2 {
  font-size: 16px;
  font-weight: 600;
  margin: 32px 0 12px;
}
label {
  display: block;
  font-size: 13px;
  color: var(--muted);
  margin: 12px 0 4px;
}
label:first-child {
  margin-top: 0;
}
input[type='text'],
input[type='password'] {
  width: 100%;
  padding: 10px 12px;
  font-size: 14px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--bg);
  color: var(--text);
  outline: none;
}
.checkbox-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 12px 0 4px;
}
.checkbox-row input {
  width: auto;
}
button.primary {
  margin-top: 16px;
  padding: 10px 16px;
  font-size: 14px;
  border: none;
  border-radius: 8px;
  background: var(--accent);
  color: #fff;
  cursor: pointer;
}
button.primary:disabled {
  opacity: 0.6;
  cursor: default;
}
.msg {
  font-size: 13px;
  margin-top: 10px;
  min-height: 16px;
}
.msg.error {
  color: var(--danger);
}
.msg.ok {
  color: var(--accent);
}
table {
  width: 100%;
  border-collapse: collapse;
  font-size: 14px;
}
th,
td {
  text-align: left;
  padding: 8px 6px;
  border-bottom: 1px solid var(--border);
}
th {
  color: var(--muted);
  font-weight: 500;
}
.badge {
  font-size: 11px;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 2px 6px;
  border-radius: 5px;
}
.link-btn {
  border: none;
  background: transparent;
  color: var(--accent);
  font-size: 12px;
  cursor: pointer;
  padding: 4px 6px;
}
.link-btn.danger {
  color: var(--muted);
}
.link-btn.danger:hover {
  color: var(--danger);
}
.export-btn {
  display: inline-block;
  margin-bottom: 12px;
  font-size: 13px;
  color: var(--accent);
  text-decoration: none;
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 5px 10px;
}
.export-btn:hover {
  border-color: var(--accent);
}
.senses {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.sense {
  display: flex;
  align-items: baseline;
  gap: 6px;
  flex-wrap: wrap;
}
.sense .pos {
  font-size: 11px;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 1px 5px;
  border-radius: 5px;
  white-space: nowrap;
}
.sense .translation {
  font-size: 13px;
  color: var(--text);
  word-break: break-word;
}
code.generated-password {
  background: var(--accent-soft);
  color: var(--accent);
  padding: 2px 6px;
  border-radius: 5px;
  font-size: 13px;
}
</style>
