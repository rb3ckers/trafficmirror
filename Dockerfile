FROM debian:stretch

RUN apt-get update && apt-get install -y ca-certificates

ADD ./init.sh /init
RUN chmod +x /init

ADD build/trafficmirror /trafficmirror

CMD ["/init"]

EXPOSE 8080
