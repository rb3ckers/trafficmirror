FROM debian:stretch

ADD build/trafficmirror /trafficmirror

CMD ["trafficmirror"]

EXPOSE 8080
