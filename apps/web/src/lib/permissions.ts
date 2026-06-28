export type Permission = 'knowledge:read' | 'qa:use' | 'reports:write' | 'system:admin'

export function hasPermission(permissions: Permission[], permission: Permission) {
  return permissions.includes(permission)
}
