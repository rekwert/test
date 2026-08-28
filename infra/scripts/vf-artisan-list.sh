#!/bin/bash
/opt/virtfusion/php8/bin/php /opt/virtfusion/app/control/artisan list 2>/dev/null | grep -iE 'os|template|media|license|hypervisor' | head -50
