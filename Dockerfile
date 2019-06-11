FROM debian:stretch

ADD ./init.sh /init.sh
RUN chmod +x /init.sh && apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

ADD build/trafficmirror /trafficmirror

CMD ["/init.sh"]

EXPOSE 8080
