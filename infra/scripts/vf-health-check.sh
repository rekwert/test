#!/bin/bash
systemctl is-active vf-nginx vf-php8-fpm libvirtd
curl -sk -o /dev/null -w "local_login:%{http_code}\n" https://127.0.0.1/login
ss -tlnp | grep nginx | head -3
