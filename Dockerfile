FROM registry.fedoraproject.org/fedora:34
COPY luc /bin/
ENTRYPOINT [ "/bin/luc" ]
