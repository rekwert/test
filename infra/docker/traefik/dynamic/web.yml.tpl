http:
  routers:
    web:
      rule: "Host(`${DOMAIN}`) && !PathPrefix(`/api`)"
      entryPoints:
        - websecure
      tls:
        certResolver: le
      service: web-app
      middlewares:
        - security-headers
      priority: 1
    web-http:
      rule: "Host(`${DOMAIN}`) && !PathPrefix(`/api`)"
      entryPoints:
        - web
      middlewares:
        - redirect-https
        - security-headers
      service: web-app
      priority: 1
  middlewares:
    redirect-https:
      redirectScheme:
        scheme: https
        permanent: true
    security-headers:
      headers:
        stsSeconds: 31536000
        stsIncludeSubdomains: true
        stsPreload: true
        forceSTSHeader: true
        contentTypeNosniff: true
        frameDeny: true
        referrerPolicy: "strict-origin-when-cross-origin"
  services:
    web-app:
      loadBalancer:
        servers:
          - url: "http://web:3000"
