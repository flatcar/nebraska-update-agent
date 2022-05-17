FROM registry.fedoraproject.org/fedora:35
ARG ARCH=amd64
COPY nua /bin/
ENTRYPOINT [ "/bin/nua" ]
