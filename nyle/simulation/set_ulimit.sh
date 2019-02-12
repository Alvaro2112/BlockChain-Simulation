#!/usr/bin/env bash

echo "PreScript runs in simulation '$1' on '$(hostname)' and says: $(date)"
ulimit -Sn
ulimit -Hn
ulimit -n
sudo su -c "echo 'fs.file-max = 1000000' >> /etc/sysctl.conf"
sudo su -c "whoami"

sudo sysctl -p
sudo su -c "echo '* soft nofile 900000' >> /etc/security/limits.conf"
sudo su -c "echo '* hard nofile 900000' >> /etc/security/limits.conf"

sudo su -c "echo '/etc/pam.d/common-session*' >> session required pam_limits.so"
ulimit -n 1000000

exit

#sudo sh -c "ulimit -Hn 1000000 && exec su $LOGNAME"
#exec $LOGNAME
#sudo sh -c "ulimit -Sn 800000"
#ulimit -Hn 800000

ulimit -Sn
ulimit -Hn
ulimit -n