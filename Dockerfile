FROM ubuntu:22.04
MAINTAINER zy
ADD ./netbalance /usr/local/bin
CMD ["netbalance"]