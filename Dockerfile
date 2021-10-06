FROM registry.fedoraproject.org/fedora:34
ARG ARCH=amd64
COPY nuc /bin/
ENTRYPOINT [ "/bin/nuc" ]
