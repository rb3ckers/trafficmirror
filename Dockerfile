FROM debian:stretch

ADD build/trafficmirror /trafficmirror
ADD ./init.sh /init
RUN chmod +x /init && apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

CMD ["/init"]

EXPOSE 8080
