FROM cpm-base:latest
MAINTAINER crunchy

VOLUME ["/monitordata"]

# open up the monitor server port
EXPOSE 13000
EXPOSE 8083
EXPOSE 8086

USER root

ADD bin /var/cpm/bin
ADD conf /var/cpm/conf

# get influxdb
RUN rpm -Uvh http://s3.amazonaws.com/influxdb/influxdb-latest-1.x86_64.rpm

RUN chown -R root:root /monitordata

CMD ["/var/cpm/bin/start-mon.sh"]
