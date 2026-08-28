#!/bin/bash
for i in $(seq 1 30); do
  if grep -q "REINSTALL COMPLETE" /root/vf-reinstall.nohup 2>/dev/null; then
    echo "=== DONE ==="
    grep -A6 "Installation Complete" /root/vf-reinstall.nohup 2>/dev/null | tail -10
    tail -5 /root/vf-reinstall.nohup
    exit 0
  fi
  if grep -qi "error\|failed" /root/vf-reinstall.nohup 2>/dev/null | tail -1 | grep -qi fatal; then
    echo "=== MAYBE FAILED ==="
    tail -20 /root/vf-reinstall.nohup
  fi
  echo "--- poll $i $(date -Is) ---"
  tail -2 /root/vf-reinstall.nohup
  sleep 30
done
echo "=== TIMEOUT still running ==="
ps aux | grep vf-full | grep -v grep
tail -10 /root/vf-reinstall.nohup
