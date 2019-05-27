FROM debian:stretch

ADD build/trafficmirror /trafficmirror
ADD ./init.sh /init
RUN chmod +x /init

CMD ["/init"]

EXPOSE 8080
