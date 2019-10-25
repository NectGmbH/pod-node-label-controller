FROM alpine:3.8

COPY ./pod-node-label-controller /bin/pod-node-label-controller

ENTRYPOINT [ "/bin/pod-node-label-controller" ]