import os
env={}
for line in open('/opt/testVPStrade/infra/docker/.env'):
    if '=' in line and not line.strip().startswith('#'):
        k,v=line.strip().split('=',1)
        env[k]=v.strip().strip('"').strip("'")
k=env.get('VIRTFUSION_API_KEY','')
print('BACK_LEN', len(k))
print('BACK_PREFIX', k[:12])
print('BACK_DOT', '.' in k)
