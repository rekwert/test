# RBAC Matrix

## Roles

| Role | Description |
|---|---|
| owner | Platform creators, full access |
| admin | Trusted staff, almost full access |
| support | Customer support, limited actions |
| client | Portal user, own resources only |

## Permissions

| Permission | owner | admin | support | client |
|---|---|---|---|---|
| auth.users.read | yes | yes | yes | own |
| auth.users.write | yes | yes | no | own |
| plans.read | yes | yes | yes | yes |
| plans.write | yes | yes | no | no |
| instances.read.all | yes | yes | yes | no |
| instances.read.own | yes | yes | yes | yes |
| instances.start/stop/reboot | yes | yes | yes | own |
| instances.delete | yes | yes | no | own |
| instances.reinstall | yes | yes | no | own |
| billing.read.all | yes | yes | no | no |
| billing.read.own | yes | yes | no | yes |
| billing.topup.own | yes | yes | no | yes |
| billing.config | yes | no | no | no |
| audit.read | yes | yes | yes | no |
| instances.force_suspend | yes | yes | no | no |

## JWT claims

```json
{
  "sub": "user-uuid",
  "roles": ["client"],
  "permissions": ["instances.read.own", "instances.start/stop/reboot"]
}
```

## Audit

All admin/support actions logged: actor, action, entity, old/new values, IP, timestamp.

Support cannot view unmasked root passwords (masked only).
