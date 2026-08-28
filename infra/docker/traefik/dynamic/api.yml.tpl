http:
  routers:
    api:
      rule: "Host(`${DOMAIN}`) && PathPrefix(`/api`)"
      entryPoints:
        - websecure
      tls:
        certResolver: le
      service: back-gateway
      priority: 20
    api-http:
      rule: "Host(`${DOMAIN}`) && PathPrefix(`/api`)"
      entryPoints:
        - web
      middlewares:
        - redirect-https
      service: back-gateway
      priority: 20
  services:
    back-gateway:
      loadBalancer:
${TRANSPORT_BLOCK}
        servers:
          - url: "${BACK_GATEWAY_URL}"
  serversTransports:
    internal-back-tls:
      insecureSkipVerify: true
