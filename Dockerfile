FROM ubuntu:22.04
MAINTAINER zy
WORKDIR /
COPY ./netbalance /usr/local/bin
CMD ["netbalance"]