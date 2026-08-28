# ADR 002: Custom Go gateway, not Kong

## Status

Accepted

## Decision

Thin Go gateway (chi router) for JWT, CORS, rate limit, routing. No Kong/Traefik API gateway product.

## Rationale

Kong adds ops overhead before we need it. Go gateway is sufficient for MVP.
