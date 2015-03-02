#!/bin/bash

# Copyright 2015 Crunchy Data Solutions, Inc.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

#
# start pg, will initdb if /pgdata is empty as a way to bootstrap
#

source /opt/cpm/bin/setenv.sh

chgrp postgres $CLUSTER_LOG
chmod g+w $CLUSTER_LOG

export LD_LIBRARY_PATH=/usr/pgsql-9.4/lib

#
# the normal startup of pg
#
su - postgres -c 'pg_ctl -D /pgdata  start'
