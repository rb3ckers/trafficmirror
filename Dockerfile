FROM debian:stretch

ADD ./init.sh /init
RUN chmod +x /init && apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

ADD build/trafficmirror /trafficmirror

CMD ["/init"]

EXPOSE 8080
