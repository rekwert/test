# ADR 001: Go microservices in Docker containers

## Status

Accepted

## Context

Need scalable backend for VPS control plane.

## Decision

Go microservices: auth, billing, vps, notification. All run as separate Docker containers on one back machine. Custom Go API gateway.

## Consequences

- Service isolation without 5 separate VMs
- Independent deploy per container
- Requires event catalog and clear service boundaries
