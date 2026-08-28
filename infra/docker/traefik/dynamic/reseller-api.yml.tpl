http:
  routers:
    reseller-api-proxy:
      rule: "Host(`${API_DOMAIN}`) && PathPrefix(`/api/v1`)"
      entryPoints:
        - websecure
      tls:
        certResolver: le
      service: back-gateway
      priority: 30
    reseller-api-proxy-http:
      rule: "Host(`${API_DOMAIN}`) && PathPrefix(`/api/v1`)"
      entryPoints:
        - web
      middlewares:
        - redirect-https
      service: back-gateway
      priority: 30
    reseller-api-docs:
      rule: "Host(`${API_DOMAIN}`) && PathPrefix(`/api-docs`)"
      entryPoints:
        - websecure
      tls:
        certResolver: le
      service: reseller-api-static
      middlewares:
        - security-headers
      priority: 20
    reseller-api-docs-http:
      rule: "Host(`${API_DOMAIN}`) && PathPrefix(`/api-docs`)"
      entryPoints:
        - web
      middlewares:
        - redirect-https
        - security-headers
      service: reseller-api-static
      priority: 20
    reseller-api:
      rule: "Host(`${API_DOMAIN}`)"
      entryPoints:
        - websecure
      tls:
        certResolver: le
      service: reseller-api-static
      middlewares:
        - security-headers
      priority: 10
    reseller-api-http:
      rule: "Host(`${API_DOMAIN}`)"
      entryPoints:
        - web
      middlewares:
        - redirect-https
        - security-headers
      service: reseller-api-static
      priority: 10
  services:
    back-gateway:
      loadBalancer:
${TRANSPORT_BLOCK}
        servers:
          - url: "${BACK_GATEWAY_URL}"
    reseller-api-static:
      loadBalancer:
        servers:
          - url: "http://reseller-api-static:80"
  serversTransports:
    internal-back-tls:
      insecureSkipVerify: true