cpm-node deployment in Kuber
===========================================

1. Start an OpenShift all-in-one server


        openshift start

2. set selinux off on the /www volume

        chcon -Rt svirt_sandbox_file_t /var/lib/pgsql/dude

        This is due to a bug in Docker with respect to handling selinux enabled systems!  The bug is logged on
        the Docker site, as:

        https://github.com/docker/docker/pull/5910


2. Use the command line to transform the template, and then send each object to the server:


        openshift kube process -c node-template.json | openshift kube apply -c -

   Note: `-c -` tells the CLI to read a file from STDIN - you can use this in other places as well.

Alternatively, using the Openshift 'deployments' concept, but as of
now, this doesn't work it appears?:

X. You can deploy cpm-node-config.json with:

	$ openshift kube apply -c cpm-node-config.json

