# ADR 003: Redis Streams instead of RabbitMQ

## Status

Accepted

## Decision

Use Redis Streams on DB machine for async events. No RabbitMQ on MVP.

## Rationale

Redis already required for cache. Fewer moving parts. Sufficient for startup load.
