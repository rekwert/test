#!/bin/bash
PHP=/opt/virtfusion/php8/bin/php
cd /opt/virtfusion/app/control

$PHP <<'PHP'
<?php
require __DIR__ . '/vendor/autoload.php';
$app = require __DIR__ . '/bootstrap/app.php';
$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();

$h = App\Models\Hypervisor::find(1);
if (!$h) { echo "no hypervisor\n"; exit(1); }

echo "Methods on Hypervisor model:\n";
foreach (get_class_methods($h) as $m) {
    if (stripos($m, 'resource') !== false || stripos($m, 'connect') !== false || stripos($m, 'commission') !== false || stripos($m, 'accept') !== false || stripos($m, 'client') !== false || stripos($m, 'api') !== false) {
        echo "  $m\n";
    }
}

// Try to refresh resources
foreach (['refreshResources','updateResources','syncResources','fetchResources','pollResources','testConnection'] as $method) {
    if (method_exists($h, $method)) {
        echo "Calling $method...\n";
        try {
            $r = $h->$method();
            echo "Result: ".json_encode($r)."\n";
        } catch (Throwable $e) {
            echo "ERR $method: ".$e->getMessage()."\n";
        }
    }
}

// Try HypervisorClient if exists
foreach (['App\\Core\\Hypervisor\\Client','App\\Services\\Hypervisor\\Client','App\\Library\\Hypervisor\\HypervisorAPI'] as $cls) {
    if (class_exists($cls)) {
        echo "Trying class $cls\n";
        try {
            $c = app($cls);
            echo get_class($c)."\n";
        } catch (Throwable $e) {
            echo "ERR: ".$e->getMessage()."\n";
        }
    }
}
PHP
