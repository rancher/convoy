FROM ubuntu:15.10
MAINTAINER Sheng Yang <sheng.yang@rancher.com>

RUN apt-get install -y \
        libaio1 \
        nbd-client

COPY convoy* /usr/local/bin/
RUN chmod a+x /usr/local/bin/convoy*
