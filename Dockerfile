FROM registry.fedoraproject.org/fedora:35
ARG ARCH=amd64
COPY nuc /bin/
ENTRYPOINT [ "/bin/nuc" ]
