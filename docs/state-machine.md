# VPS State Machine

## Lifecycle states

| State | Description |
|---|---|
| queued | Paid order waiting for free node capacity (waitlist) |
| creating | Infrastructure provisioning (VirtFusion) |
| starting | Hypervisor/VM boot |
| running | VPS operational |
| restarting | Reboot in progress |
| stopping | User-initiated shutdown |
| stopped | Powered off, resources retained |
| suspending | Suspension for non-payment |
| suspended | Frozen, access restricted |
| deleting | Resource cleanup |
| deleted | Final state (soft-delete in DB) |
| error | Provisioning or operation failure |

## Transitions

```
queued -> creating -> starting -> running <-> restarting
              |                      |
            error               stopping -> stopped
                                     |
                              suspending -> suspended
                                     |
                                deleting -> deleted
```

## Waitlist

When a client pays for an order and the region has online nodes but **no free capacity**,
the instance is created in `queued` (no `node_id`, no provision outbox yet).

The worker periodically promotes the oldest `queued` instance for a region that has
free capacity: `queued → creating` + `instance.provision_requested`. Capacity is freed
when another instance is deleted (or otherwise leaves an active slot).

`queued` does **not** count toward `capacity_instances`.

## Billing status (separate field)

| Status | Description |
|---|---|
| active | Paid and current |
| past_due | Payment overdue |
| grace_period | N days grace, VPS may still run |
| suspended | Non-payment suspension triggered |
| cancelled | Subscription cancelled |

## Grace period flow

```
active -> past_due -> grace_period (VPS still running)
                   -> suspended (triggers vps: suspending -> suspended)
```

Payment received while suspended: `suspended -> starting -> running`.
