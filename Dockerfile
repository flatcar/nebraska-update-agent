FROM registry.fedoraproject.org/fedora:34
ARG ARCH=amd64
COPY luc /bin/
ENTRYPOINT [ "/bin/luc" ]
